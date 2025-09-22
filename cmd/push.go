/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"context"
	"fmt"

	"krane/pkg/docker"
	"krane/pkg/ecr"
	"krane/pkg/k8s"
	"krane/pkg/utils"

	"github.com/spf13/cobra"
)

// pushCmd represents the push command
var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push container images to AWS ECR",
	Long: `Push all container images from Kubernetes cluster to AWS ECR.
    
This command pulls images from cluster, creates ECR repositories if needed,
re-tags them for ECR, and pushes them.`,
	Run: func(cmd *cobra.Command, args []string) {
		runPush()
	},
}

var (
	region           string
	repositoryPrefix string
	namespace        string
	dryRun           bool
)

func init() {
	rootCmd.AddCommand(pushCmd)

	pushCmd.Flags().StringVar(&region, "region", "eu-west-1", "AWS region for ECR")
	pushCmd.Flags().StringVar(&repositoryPrefix, "prefix", "k8s-backup", "ECR repository prefix/namespace")
	pushCmd.Flags().StringVar(&namespace, "namespace", "", "Kubernetes namespace to filter (default: all)")
	pushCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be pushed without actually pushing")
}

func runPush() {
	fmt.Println("🚀 Starting image push to AWS ECR...")

	// 1. Create ECR client
	ecrClient, err := ecr.NewClient(region)
	if err != nil {
		handleError("Error creating ECR client", err)
	}

	fmt.Printf("🏷️  ECR Registry: %s\n", ecrClient.GetRegistryURL())

	// 2. Get images from Kubernetes
	k8sClient, err := k8s.NewClient("")
	if err != nil {
		handleError("Error creating Kubernetes client", err)
	}

	images, err := k8s.ListPodImages(k8sClient, namespace)
	if err != nil {
		handleError("Error listing pod images", err)
	}

	uniqueImages := utils.RemoveDuplicates(images)
	fmt.Printf("📦 Found %d unique images\n", len(uniqueImages))

	// 3. Create Docker client
	dockerClient, err := docker.NewClient()
	if err != nil {
		handleError("Error creating Docker client", err)
	}
	defer dockerClient.Close()

	// 4. Get ECR auth token
	ctx := context.Background()
	username, password, err := ecrClient.GetAuthToken(ctx)
	if err != nil {
		handleError("Error getting ECR auth token", err)
	}

	fmt.Println("🔑 ECR authentication successful")

	// 5. Process each image for push
	for i, image := range uniqueImages {
		fmt.Printf("\n[%d/%d] 📦 Processing: %s\n", i+1, len(uniqueImages), image)

		targetImage, repoName, err := ecrClient.ConvertImageName(image, repositoryPrefix)
		if err != nil {
			fmt.Printf("❌ Failed to convert image name %s: %v\n", image, err)
			continue
		}

		if dryRun {
			fmt.Printf("🔍 DRY RUN: Would push %s -> %s\n", image, targetImage)
			continue
		}

		// Create ECR repository
		if err := ecrClient.CreateRepository(ctx, repoName); err != nil {
			fmt.Printf("❌ Failed to create repository %s: %v\n", repoName, err)
			continue
		}

		// Pull -> Tag -> Push
		if err := processImage(ctx, dockerClient, image, targetImage, username, password); err != nil {
			fmt.Printf("❌ Failed to process %s: %v\n", image, err)
			continue
		}

		fmt.Printf("✅ Successfully pushed: %s\n", targetImage)
	}

	fmt.Println("\n🎉 Push operation completed!")
}

func processImage(ctx context.Context, client *docker.Client, sourceImage, targetImage, username, password string) error {
	// 1. Pull original image
	if err := client.PullImage(ctx, sourceImage); err != nil {
		return fmt.Errorf("pull failed: %w", err)
	}

	// 2. Tag for ECR
	if err := client.TagImage(ctx, sourceImage, targetImage); err != nil {
		return fmt.Errorf("tag failed: %w", err)
	}

	// 3. Push to ECR with authentication
	if err := client.PushImageWithAuth(ctx, targetImage, username, password); err != nil {
		return fmt.Errorf("push failed: %w", err)
	}

	return nil
}
