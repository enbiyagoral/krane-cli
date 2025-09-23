/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"krane/pkg/k8s"
	"krane/pkg/utils"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	allNamespaces     bool
	outputFormat      string
	includeNamespaces []string
	excludeNamespaces []string
	includePatterns   []string
	excludePatterns   []string
	showSources       bool
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all container images from Kubernetes pods",
	Long: `List all container images running in Kubernetes pods.
    
This command scans all pods (or specified namespace) and extracts
the container images including init containers.`,
	Run: func(cmd *cobra.Command, args []string) {
		runList()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().BoolVar(&allNamespaces, "all-namespaces", false, "List images from all namespaces")
	listCmd.Flags().StringVarP(&outputFormat, "format", "o", "table", "Output format (table, json, yaml)")
	listCmd.Flags().StringSliceVar(&includeNamespaces, "include-namespaces", nil, "Only include these namespaces (prefix/regex)")
	listCmd.Flags().StringSliceVar(&excludeNamespaces, "exclude-namespaces", nil, "Exclude these namespaces (prefix/regex)")
	listCmd.Flags().StringSliceVar(&includePatterns, "include", nil, "Only include images matching these patterns (prefix/regex)")
	listCmd.Flags().StringSliceVar(&excludePatterns, "exclude", nil, "Exclude images matching these patterns (prefix/regex)")
	listCmd.Flags().BoolVar(&showSources, "show-sources", false, "Show source kind/name and namespace for each image")
}

func runList() {
	// Kubernetes Client
	client, err := k8s.NewClient("")
	if err != nil {
		handleError("Error creating Kubernetes client", err)
	}

	namespace := ""
	if !allNamespaces {
		namespace = "default"
	}

	if showSources {
		infos, err := k8s.ListPodImagesWithSource(client, allNamespaces, namespace, includeNamespaces, excludeNamespaces)
		if err != nil {
			handleError("Error listing pod images", err)
		}
		// Apply image filters
		var images []string
		for _, info := range infos {
			images = append(images, info.Image)
		}
		images = utils.RemoveDuplicates(images)
		filtered, err := utils.FilterImages(images, includePatterns, excludePatterns)
		if err != nil {
			handleError("Invalid include/exclude patterns", err)
		}
		// Group by image
		grouped := groupSourcesByImage(infos, filtered)
		sort.Slice(grouped, func(i, j int) bool { return grouped[i].Image < grouped[j].Image })
		for idx := range grouped {
			sort.Slice(grouped[idx].Sources, func(a, b int) bool {
				sa, sb := grouped[idx].Sources[a], grouped[idx].Sources[b]
				if sa.Namespace != sb.Namespace {
					return sa.Namespace < sb.Namespace
				}
				if sa.SourceKind != sb.SourceKind {
					return sa.SourceKind < sb.SourceKind
				}
				return sa.SourceName < sb.SourceName
			})
		}
		// Print with sources depending on format
		switch outputFormat {
		case "table":
			printTableGrouped(grouped)
		case "json":
			printJSONGrouped(grouped)
		case "yaml":
			printYAMLGrouped(grouped)
		default:
			printTableGrouped(grouped)
		}
		return
	}

	// List pod images with namespace filters
	images, err := k8s.ListPodImagesFiltered(client, allNamespaces, namespace, includeNamespaces, excludeNamespaces)
	if err != nil {
		handleError("Error listing pod images", err)
	}

	uniqueImages := utils.RemoveDuplicates(images)
	// Apply image include/exclude filters
	filtered, err := utils.FilterImages(uniqueImages, includePatterns, excludePatterns)
	if err != nil {
		handleError("Invalid include/exclude patterns", err)
	}
	uniqueImages = filtered
	sort.Strings(uniqueImages)

	switch outputFormat {
	case "table":
		printTable(uniqueImages)
	case "json":
		printJSON(uniqueImages)
	case "yaml":
		printYAML(uniqueImages)
	default:
		printTable(uniqueImages)
	}
}

func printTable(images []string) {
	fmt.Println("CONTAINER IMAGES:")
	fmt.Println(strings.Repeat("-", 50))
	for i, image := range images {
		fmt.Printf("%d. %s\n", i+1, image)
	}
	fmt.Printf("\nTotal: %d unique images\n", len(images))
}

type GroupedImage struct {
	Image   string          `json:"image" yaml:"image"`
	Sources []k8s.ImageInfo `json:"sources" yaml:"sources"`
}

func groupSourcesByImage(infos []k8s.ImageInfo, allowedImages []string) []GroupedImage {
	allow := map[string]bool{}
	for _, img := range allowedImages {
		allow[img] = true
	}
	m := map[string][]k8s.ImageInfo{}
	for _, info := range infos {
		if !allow[info.Image] {
			continue
		}
		m[info.Image] = append(m[info.Image], info)
	}
	var out []GroupedImage
	for image, srcs := range m {
		out = append(out, GroupedImage{Image: image, Sources: srcs})
	}
	return out
}

func printTableGrouped(grouped []GroupedImage) {
	fmt.Println("CONTAINER IMAGES (GROUPED):")
	fmt.Println(strings.Repeat("-", 80))
	for i, g := range grouped {
		fmt.Printf("%d. %s\n", i+1, g.Image)
		// Show up to 3 sources, then a summary
		max := 3
		if len(g.Sources) < max {
			max = len(g.Sources)
		}
		for j := 0; j < max; j++ {
			s := g.Sources[j]
			fmt.Printf("   - ns=%s source=%s/%s\n", s.Namespace, s.SourceKind, s.SourceName)
		}
		if len(g.Sources) > max {
			fmt.Printf("   ... and %d more sources\n", len(g.Sources)-max)
		}
	}
	fmt.Printf("\nTotal: %d unique images\n", len(grouped))
}

// ImageList represents the structure for JSON output
type ImageList struct {
	Images []string `json:"images"`
	Total  int      `json:"total"`
}

func printJSON(images []string) {
	imageList := ImageList{
		Images: images,
		Total:  len(images),
	}

	jsonData, err := json.MarshalIndent(imageList, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}

	fmt.Println(string(jsonData))
}

type GroupedImageList struct {
	Images []GroupedImage `json:"images"`
	Total  int            `json:"total"`
}

func printJSONGrouped(grouped []GroupedImage) {
	payload := GroupedImageList{Images: grouped, Total: len(grouped)}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printYAML(images []string) {
	imageList := ImageList{
		Images: images,
		Total:  len(images),
	}

	yamlData, err := yaml.Marshal(imageList)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v\n", err)
		return
	}

	fmt.Println(string(yamlData))
}

func printYAMLGrouped(grouped []GroupedImage) {
	payload := GroupedImageList{Images: grouped, Total: len(grouped)}
	data, err := yaml.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
