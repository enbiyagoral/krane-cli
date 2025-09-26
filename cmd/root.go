/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global persistent flags (available to all subcommands)
var (
	globalNamespace     string
	globalAllNamespaces bool
	globalRegion        string
	globalOutput        string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "krane",
	Short: "Krane CLI",
	Long: `Krane CLI is a powerful tool for managing container images in Kubernetes clusters.

Krane helps you:
- List all container images running in your Kubernetes pods
- Push container images to AWS ECR for backup and migration
- Manage container images across different namespaces
- Convert Docker Hub images to ECR format automatically

Examples:
  krane list -n default                # List images in default namespace
  krane list -A                        # List images from all namespaces
  krane push -r eu-west-1              # Push images to ECR in eu-west-1
  krane push -d                        # Preview what would be pushed`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}

func init() {
	// Persistent global flags
	rootCmd.PersistentFlags().StringVarP(&globalNamespace, "namespace", "n", "", "Kubernetes namespace to use (default: all)")
	rootCmd.PersistentFlags().BoolVarP(&globalAllNamespaces, "all-namespaces", "A", false, "If true, use all namespaces")
	rootCmd.PersistentFlags().StringVarP(&globalRegion, "region", "r", "eu-west-1", "AWS region for ECR")
	rootCmd.PersistentFlags().StringVarP(&globalOutput, "output", "o", "table", "Global output format (table, json, yaml)")

	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newPushCmd())
}
