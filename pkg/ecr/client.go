package ecr

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type Client struct {
	ecrClient *ecr.Client
	stsClient *sts.Client
	region    string
	accountID string
}

func NewClient(region string) (*Client, error) {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	ecrClient := ecr.NewFromConfig(cfg)
	stsClient := sts.NewFromConfig(cfg)

	// Automatically get Account ID
	identity, err := stsClient.GetCallerIdentity(context.TODO(), &sts.GetCallerIdentityInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account ID: %w", err)
	}

	return &Client{
		ecrClient: ecrClient,
		stsClient: stsClient,
		region:    region,
		accountID: *identity.Account,
	}, nil
}

// validateECRRepositoryName validates ECR repository name according to AWS rules
func validateECRRepositoryName(name string) error {
	// ECR repository name must match: (?:[a-z0-9]+(?:[._-][a-z0-9]+)*/)*[a-z0-9]+(?:[._-][a-z0-9]+)*
	pattern := `^(?:[a-z0-9]+(?:[._-][a-z0-9]+)*/)*[a-z0-9]+(?:[._-][a-z0-9]+)*$`
	matched, err := regexp.MatchString(pattern, name)
	if err != nil {
		return fmt.Errorf("invalid repository name pattern: %w", err)
	}
	if !matched {
		return fmt.Errorf("repository name '%s' does not match ECR naming rules (lowercase, numbers, dots, dashes, underscores only)", name)
	}
	return nil
}

func (c *Client) GetRegistryURL() string {
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", c.accountID, c.region)
}

func (c *Client) GetAuthToken(ctx context.Context) (string, string, error) {
	input := &ecr.GetAuthorizationTokenInput{}
	result, err := c.ecrClient.GetAuthorizationToken(ctx, input)
	if err != nil {
		return "", "", fmt.Errorf("failed to get ECR auth token: %w", err)
	}

	if len(result.AuthorizationData) == 0 {
		return "", "", fmt.Errorf("no authorization data returned")
	}

	authData := result.AuthorizationData[0]
	token, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode auth token: %w", err)
	}

	parts := strings.SplitN(string(token), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth token format")
	}

	return parts[0], parts[1], nil // username, password
}

func (c *Client) CreateRepository(ctx context.Context, repositoryName string) error {
	input := &ecr.CreateRepositoryInput{
		RepositoryName: aws.String(repositoryName),
	}

	_, err := c.ecrClient.CreateRepository(ctx, input)
	if err != nil {
		// Repository already exists, don't return error
		if strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			fmt.Printf("📦 Repository %s already exists\n", repositoryName)
			return nil
		}
		return fmt.Errorf("failed to create repository %s: %w", repositoryName, err)
	}

	fmt.Printf("✅ Created ECR repository: %s\n", repositoryName)
	return nil
}

func (c *Client) ConvertImageName(originalImage, prefix string) (string, string, error) {
	// Examples:
	// registry.k8s.io/ingress-nginx/controller:v1.12.3@sha256:abcdef -> k8s-backup/ingress-nginx/controller:v1.12.3
	// docker.io/library/busybox@sha256:abcdef -> k8s-backup/library/busybox:sha-abcdef
	// busybox:1.37 -> k8s-backup/busybox:1.37

	image := originalImage
	var digest string

	// Extract digest if present
	if at := strings.Index(image, "@sha256:"); at != -1 {
		digest = image[at+len("@sha256:"):]
		image = image[:at]
	}

	// Split into parts
	parts := strings.Split(image, "/")

	// First part might be registry: contains dots, colons, or 'localhost'
	startIdx := 0
	if len(parts) > 1 {
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			startIdx = 1
		}
	}

	// If only 2 parts and first is registry, use the second one
	if len(parts) == 2 && startIdx == 1 {
		startIdx = 1
	}

	// Repository path (excluding registry)
	repoParts := parts[startIdx:]
	if len(repoParts) == 0 {
		// Single name only (e.g., "busybox")
		repoParts = []string{image}
	}

	// Extract tag from last part
	last := repoParts[len(repoParts)-1]
	name := last
	tag := ""
	if idx := strings.LastIndex(last, ":"); idx != -1 {
		name = last[:idx]
		tag = last[idx+1:]
	}

	// Update last part
	repoParts[len(repoParts)-1] = name

	// Determine tag: use latest if none, or deterministic short digest if digest present
	if tag == "" {
		if digest != "" {
			short := digest
			if len(short) > 12 {
				short = short[:12]
			}
			tag = "sha-" + short
		} else {
			tag = "latest"
		}
	}

	// ECR repo name: prefix + full path (excluding registry)
	repoPath := strings.Join(repoParts, "/")

	// Normalize repository name for ECR (lowercase, replace invalid chars)
	normalizedPath := strings.ToLower(repoPath)
	normalizedPath = strings.ReplaceAll(normalizedPath, ":", "-")
	normalizedPath = strings.ReplaceAll(normalizedPath, "@", "-")

	fullRepoName := fmt.Sprintf("%s/%s", prefix, normalizedPath)

	// Validate repository name
	if err := validateECRRepositoryName(fullRepoName); err != nil {
		return "", "", err
	}

	ecrImage := fmt.Sprintf("%s/%s:%s", c.GetRegistryURL(), fullRepoName, tag)

	return ecrImage, fullRepoName, nil
}
