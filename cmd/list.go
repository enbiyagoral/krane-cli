/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"krane/pkg/k8s"
	"krane/pkg/utils"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// ListOptions holds flag values for the list command.
type ListOptions struct {
	AllNamespaces     bool
	Namespace         string
	Format            string
	IncludeNamespaces []string
	ExcludeNamespaces []string
	IncludePatterns   []string
	ExcludePatterns   []string
	ShowSources       bool
}

// Validate validates list command options and returns error if invalid.
func (opts *ListOptions) Validate() error {
	validFormats := map[string]bool{"table": true, "json": true, "yaml": true}
	if !validFormats[opts.Format] {
		return fmt.Errorf("invalid format: %s (valid: table, json, yaml)", opts.Format)
	}
	return nil
}

// newListCmd constructs the list command with its own options.
func newListCmd() *cobra.Command {
	opts := &ListOptions{Format: "table"}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all container images from Kubernetes pods",
		Long: `List all container images running in Kubernetes pods.
    
This command scans all pods (or specified namespace) and extracts
the container images including init containers.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.AllNamespaces, "all-namespaces", "A", false, "List images from all namespaces")
	cmd.Flags().StringVarP(&opts.Namespace, "namespace", "n", "", "Kubernetes namespace to filter (default: all)")
	cmd.Flags().StringVarP(&opts.Format, "format", "o", "table", "Output format (table, json, yaml)")
	cmd.Flags().StringSliceVar(&opts.IncludeNamespaces, "include-namespaces", nil, "Only include these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.ExcludeNamespaces, "exclude-namespaces", nil, "Exclude these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVarP(&opts.IncludePatterns, "include", "i", nil, "Only include images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVarP(&opts.ExcludePatterns, "exclude", "e", nil, "Exclude images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().BoolVarP(&opts.ShowSources, "show-sources", "s", false, "Show source kind/name and namespace for each image")

	return cmd
}

// runList executes the list command with the given options.
func runList(ctx context.Context, opts *ListOptions) error {
	// Validate options first
	if err := opts.Validate(); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}

	// Kubernetes Client
	client, err := k8s.NewClient("")
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	// If namespace is empty, behave like all-namespaces
	effectiveAllNamespaces := opts.AllNamespaces
	if strings.TrimSpace(opts.Namespace) == "" {
		effectiveAllNamespaces = true
	}

	// Warn if namespace filters are provided but not listing across all namespaces
	if !effectiveAllNamespaces && (len(opts.IncludeNamespaces) > 0 || len(opts.ExcludeNamespaces) > 0) {
		fmt.Fprintf(os.Stderr, "⚠️ include/exclude namespaces flags only apply when --all-namespaces is used; with --namespace they are ignored.\n")
	}

	if opts.ShowSources {
		infos, err := k8s.ListPodImagesWithSource(client, effectiveAllNamespaces, opts.Namespace, opts.IncludeNamespaces, opts.ExcludeNamespaces)
		if err != nil {
			return fmt.Errorf("listing pod images: %w", err)
		}
		// Apply image filters
		var images []string
		for _, info := range infos {
			images = append(images, info.Image)
		}
		images = utils.RemoveDuplicates(images)
		filtered, err := utils.FilterImages(images, opts.IncludePatterns, opts.ExcludePatterns)
		if err != nil {
			return fmt.Errorf("invalid include/exclude patterns: %w", err)
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
		switch opts.Format {
		case "table":
			printTableGrouped(grouped)
		case "json":
			printJSONGrouped(grouped)
		case "yaml":
			printYAMLGrouped(grouped)
		default:
			printTableGrouped(grouped)
		}
		return nil
	}

	// List pod images with namespace filters
	images, err := k8s.ListPodImagesFiltered(client, effectiveAllNamespaces, opts.Namespace, opts.IncludeNamespaces, opts.ExcludeNamespaces)
	if err != nil {
		return fmt.Errorf("listing pod images: %w", err)
	}

	uniqueImages := utils.RemoveDuplicates(images)
	// Apply image include/exclude filters
	filtered, err := utils.FilterImages(uniqueImages, opts.IncludePatterns, opts.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("invalid include/exclude patterns: %w", err)
	}
	uniqueImages = filtered
	sort.Strings(uniqueImages)

	switch opts.Format {
	case "table":
		printTable(uniqueImages)
	case "json":
		printJSON(uniqueImages)
	case "yaml":
		printYAML(uniqueImages)
	default:
		printTable(uniqueImages)
	}
	return nil
}

// printTable prints images in a simple table format.
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

// groupSourcesByImage groups image source information by image name.
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

// printTableGrouped prints grouped images with their source information.
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

// ImageList represents the structure for JSON output.
type ImageList struct {
	Images []string `json:"images"`
	Total  int      `json:"total"`
}

// printJSON prints images in JSON format.
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

// printJSONGrouped prints grouped images in JSON format.
func printJSONGrouped(grouped []GroupedImage) {
	payload := GroupedImageList{Images: grouped, Total: len(grouped)}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// printYAML prints images in YAML format.
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

// printYAMLGrouped prints grouped images in YAML format.
func printYAMLGrouped(grouped []GroupedImage) {
	payload := GroupedImageList{Images: grouped, Total: len(grouped)}
	data, err := yaml.Marshal(payload)
	if err != nil {
		fmt.Printf("Error marshaling YAML: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
