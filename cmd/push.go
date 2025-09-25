/*
Copyright © 2025 Krane CLI menbiyagoral@gmail.com
*/
package cmd

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"krane/pkg/ecr"
	"krane/pkg/k8s"
	"krane/pkg/transfer"
	"krane/pkg/utils"

	"github.com/spf13/cobra"
)

// PushOptions holds flag values for the push command.
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
	MaxConcurrent     int
}

// ImageJob represents a single image processing job.
type ImageJob struct {
	Index       int
	Total       int
	Image       string
	TargetImage string
	RepoName    string
}

// JobResult represents the result of processing an image job.
type JobResult struct {
	Job     ImageJob
	Error   error
	Skipped bool
}

// newPushCmd constructs the push command with its own options.
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
	cmd.Flags().StringVar(&opts.RepositoryPrefix, "prefix", "krane", "ECR repository prefix/namespace")
	cmd.Flags().StringVar(&opts.Namespace, "namespace", "", "Kubernetes namespace to filter (default: all)")
	cmd.Flags().BoolVar(&opts.DryRun, "dry-run", false, "Show what would be pushed without actually pushing")
	cmd.Flags().StringVarP(&opts.Platform, "platform", "p", "", "Limit mirror to a single platform (e.g. linux/amd64). If empty, mirror multi-arch when available.")
	cmd.Flags().StringSliceVar(&opts.IncludeNamespaces, "include-namespaces", nil, "Only include these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.ExcludeNamespaces, "exclude-namespaces", nil, "Exclude these namespaces (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.IncludePatterns, "include", nil, "Only include images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().StringSliceVar(&opts.ExcludePatterns, "exclude", nil, "Exclude images matching these patterns (prefix or regex; if regex compiles, it's used)")
	cmd.Flags().BoolVar(&opts.SkipExisting, "skip-existing", false, "Skip mirroring if the target ECR tag already exists")
	cmd.Flags().IntVar(&opts.MaxConcurrent, "max-concurrent", 3, "Maximum number of concurrent image transfers")

	return cmd
}

// runPush executes the push command with the given options.
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

	// 5. Process images concurrently
	if opts.DryRun {
		// For dry run, process sequentially to maintain clean output
		for i, image := range uniqueImages {
			fmt.Printf("\n[%d/%d] 📦 Processing: %s\n", i+1, len(uniqueImages), image)

			targetImage, _, err := ecrClient.ConvertImageName(image, opts.RepositoryPrefix)
			if err != nil {
				fmt.Printf("❌ Failed to convert image name %s: %v\n", image, err)
				continue
			}

			fmt.Printf("🔍 DRY RUN: Would push %s -> %s\n", image, targetImage)
		}
	} else {
		// Process images concurrently
		if err := processImagesConcurrently(ctx, ecrClient, uniqueImages, opts); err != nil {
			return fmt.Errorf("concurrent processing failed: %w", err)
		}
	}

	fmt.Println("\n🎉 Push operation completed!")
	return nil
}

// processImagesConcurrently processes images using worker pool pattern.
func processImagesConcurrently(ctx context.Context, ecrClient *ecr.Client, images []string, opts *PushOptions) error {
	// Prepare jobs
	jobs := make([]ImageJob, 0, len(images))
	for i, image := range images {
		targetImage, repoName, err := ecrClient.ConvertImageName(image, opts.RepositoryPrefix)
		if err != nil {
			fmt.Printf("❌ Failed to convert image name %s: %v\n", image, err)
			continue
		}

		jobs = append(jobs, ImageJob{
			Index:       i + 1,
			Total:       len(images),
			Image:       image,
			TargetImage: targetImage,
			RepoName:    repoName,
		})
	}

	// Create channels
	jobChan := make(chan ImageJob, len(jobs))
	resultChan := make(chan JobResult, len(jobs))

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < opts.MaxConcurrent; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, workerID, ecrClient, opts, jobChan, resultChan)
		}(i)
	}

	// Send jobs
	go func() {
		defer close(jobChan)
		for _, job := range jobs {
			select {
			case jobChan <- job:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Process results
	successCount := 0
	errorCount := 0
	skippedCount := 0
	for result := range resultChan {
		if result.Skipped {
			fmt.Printf("⏭️  [%d/%d] Skipped (already exists): %s\n",
				result.Job.Index, result.Job.Total, result.Job.TargetImage)
			skippedCount++
		} else if result.Error != nil {
			fmt.Printf("❌ [%d/%d] Failed %s: %v\n",
				result.Job.Index, result.Job.Total, result.Job.Image, result.Error)
			errorCount++
		} else {
			fmt.Printf("✅ [%d/%d] Successfully pushed: %s\n",
				result.Job.Index, result.Job.Total, result.Job.TargetImage)
			successCount++
		}
	}

	fmt.Printf("\n📊 Summary: %d successful, %d skipped, %d failed\n", successCount, skippedCount, errorCount)
	return nil
}

// worker processes jobs from the job channel.
func worker(ctx context.Context, workerID int, ecrClient *ecr.Client, opts *PushOptions, jobs <-chan ImageJob, results chan<- JobResult) {
	for job := range jobs {
		fmt.Printf("🔄 [%d/%d] Worker %d processing: %s\n",
			job.Index, job.Total, workerID, job.Image)

		err, skipped := processImageJob(ctx, ecrClient, opts, job)

		select {
		case results <- JobResult{Job: job, Error: err, Skipped: skipped}:
		case <-ctx.Done():
			return
		}
	}
}

// processImageJob processes a single image job.
func processImageJob(ctx context.Context, ecrClient *ecr.Client, opts *PushOptions, job ImageJob) (error, bool) {
	// Create ECR repository
	if err := ecrClient.CreateRepository(ctx, job.RepoName); err != nil {
		return fmt.Errorf("failed to create repository %s: %w", job.RepoName, err), false
	}

	// If skipping existing, check whether tag exists already in ECR
	if opts.SkipExisting {
		// Extract tag from targetImage (after last ':')
		tag := ""
		if idx := strings.LastIndex(job.TargetImage, ":"); idx != -1 {
			tag = job.TargetImage[idx+1:]
		}
		if tag != "" {
			exists, err := ecrClient.ImageTagExists(ctx, job.RepoName, tag)
			if err != nil {
				return fmt.Errorf("could not check existing tag for %s:%s: %w", job.RepoName, tag, err), false
			}
			if exists {
				return nil, true // Skipped, not an error
			}
		}
	}

	// Mirror source image to ECR preserving manifest lists (or single platform if provided)
	if err := transfer.Mirror(ctx, job.Image, job.TargetImage, opts.Platform); err != nil {
		return fmt.Errorf("mirror failed %s -> %s: %w", job.Image, job.TargetImage, err), false
	}

	return nil, false
}
