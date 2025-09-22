/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"fmt"
	"krane/pkg/k8s"
	"krane/pkg/utils"
	"strings"

	"github.com/spf13/cobra"
)

var (
	allNamespaces bool
	outputFormat  string
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

	// List pods images
	images, err := k8s.ListPodImages(client, namespace)
	if err != nil {
		handleError("Error listing pod images", err)
	}

	uniqueImages := utils.RemoveDuplicates(images)

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

func printJSON(images []string) {
	fmt.Println("{")
	fmt.Println(`  "images": [`)
	for i, image := range images {
		if i == len(images)-1 {
			fmt.Printf(`    "%s"`, image)
		} else {
			fmt.Printf(`    "%s",`, image)
		}
		fmt.Println()
	}
	fmt.Println("  ]")
	fmt.Println("}")
}

func printYAML(images []string) {
	fmt.Println("images:")
	for _, image := range images {
		fmt.Printf("  - %s\n", image)
	}
}
