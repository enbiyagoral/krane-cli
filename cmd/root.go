/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
  krane list                           # List images in default namespace
  krane list --all-namespaces          # List images from all namespaces
  krane push --region eu-west-1        # Push images to ECR in eu-west-1
  krane push --dry-run                 # Preview what would be pushed`,
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
	rootCmd.AddCommand(newListCmd())
	rootCmd.AddCommand(newPushCmd())
}
