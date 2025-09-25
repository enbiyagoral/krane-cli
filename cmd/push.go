/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"context"
	"fmt"
	"strings"

	"krane/pkg/ecr"
	"krane/pkg/k8s"
	"krane/pkg/transfer"
	"krane/pkg/utils"

	"github.com/spf13/cobra"
)

// PushOptions holds flag values for the push command
type PushOptions struct {
	AllNamespaces     bool
	Region            string
	RepositoryPrefix  string
	Namespace         string
	DryRun            bool
	Platform          string
	SkipExisting      bool
	IncludeNamespaces []string
	ExcludeNamespaces []string
	IncludePatterns   []string
	ExcludePatterns   []string
}

// newPushCmd constructs the push command with its own options
func newPushCmd() *cobra.Command {
	opts := &PushOptions{}
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Push container images to AWS ECR",
		Long: `Mirror all container images discovered in the Kubernetes cluster to AWS ECR.
    
This command discovers images from pods (optionally filtered by namespaces and patterns),
creates ECR repositories if needed, and performs a registry-to-registry mirror preserving
multi-arch manifests. Optionally restrict to a single platform with --platform.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPush(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.AllNamespaces, "all-namespaces", false, "List images from all namespaces")
	cmd.Flags().StringVar(&opts.Region, "region", "eu-west-1", "AWS region for ECR")
	cmd.Flags().StringVar(&opts.RepositoryPrefix, "prefix", "k8s-backup", "ECR repository prefix/namespace")
	cmd.Flags().StringVar(&opts.Namespace, "namespace", "", "Kubernetes namespace to filter (default: all)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be pushed without actually pushing")
	cmd.Flags().StringVarP(&opts.Platform, "platform", "p", "", "Limit mirror to a single platform (e.g. linux/amd64). If empty, mirror multi-arch when available.")
	cmd.Flags().StringSliceVar(&opts.IncludeNamespaces, "include-namespaces", nil, "Only include these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.ExcludeNamespaces, "exclude-namespaces", nil, "Exclude these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.IncludePatterns, "include", nil, "Only include images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.ExcludePatterns, "exclude", nil, "Exclude images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip mirroring if the target ECR tag already exists")

	return cmd
}

func runPush(ctx context.Context, opts *PushOptions) error {
	fmt.Println("🚀 Starting image push to AWS ECR...")

	// 1. Create ECR client
	ecrClient, err := ecr.NewClient(opts.Region)
	if err != nil {
		return fmt.Errorf("creating ECR client: %w", err)
	}

	fmt.Printf("🏷️  ECR Registry: %s\n", ecrClient.GetRegistryURL())

	// 2. Get images from Kubernetes
	k8sClient, err := k8s.NewClient("")
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %w", err)
	}

	effectiveAllNamespaces := opts.AllNamespaces
	if strings.TrimSpace(opts.Namespace) == "" {
		effectiveAllNamespaces = true
	}

	if !effectiveAllNamespaces && (len(opts.IncludeNamespaces) > 0 || len(opts.ExcludeNamespaces) > 0) {
		fmt.Printf("⚠️ include/exclude namespaces flags only apply when --all-namespaces is used; with --namespace they are ignored.\n")
	}
	images, err := k8s.ListPodImagesFiltered(k8sClient, effectiveAllNamespaces, opts.Namespace, opts.IncludeNamespaces, opts.ExcludeNamespaces)
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
	fmt.Printf("📦 Found %d unique images\n", len(uniqueImages))

	// 3. Verify ECR authentication
	_, _, err = ecrClient.GetAuthToken(ctx)
	if err != nil {
		return fmt.Errorf("getting ECR auth token: %w", err)
	}

	fmt.Println("🔑 ECR authentication successful")

	// 5. Process each image for push
	for i, image := range uniqueImages {
		fmt.Printf("\n[%d/%d] 📦 Processing: %s\n", i+1, len(uniqueImages), image)

		targetImage, repoName, err := ecrClient.ConvertImageName(image, opts.RepositoryPrefix)
		if err != nil {
			fmt.Printf("❌ Failed to convert image name %s: %v\n", image, err)
			continue
		}

		if opts.DryRun {
			fmt.Printf("🔍 DRY RUN: Would push %s -> %s\n", image, targetImage)
			continue
		}

		// Create ECR repository
		if err := ecrClient.CreateRepository(ctx, repoName); err != nil {
			fmt.Printf("❌ Failed to create repository %s: %v\n", repoName, err)
			continue
		}

		// If skipping existing, check whether tag exists already in ECR
		if opts.SkipExisting {
			// Extract tag from targetImage (after last ':')
			tag := ""
			if idx := strings.LastIndex(targetImage, ":"); idx != -1 {
				tag = targetImage[idx+1:]
			}
			if tag != "" {
				exists, err := ecrClient.ImageTagExists(ctx, repoName, tag)
				if err != nil {
					fmt.Printf("⚠️  Could not check existing tag for %s:%s: %v\n", repoName, tag, err)
				} else if exists {
					fmt.Printf("⏭️  Skipping (tag exists): %s\n", targetImage)
					continue
				}
			}
		}

		// Mirror source image to ECR preserving manifest lists (or single platform if provided)
		if err := transfer.Mirror(ctx, image, targetImage, opts.Platform); err != nil {
			fmt.Printf("❌ Mirror failed %s -> %s: %v\n", image, targetImage, err)
			continue
		}

		fmt.Printf("✅ Successfully pushed (mirrored): %s\n", targetImage)
	}

	fmt.Println("\n🎉 Push operation completed!")
	return nil
}
