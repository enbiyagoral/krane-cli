package ecr

import (
	"context"
	"encoding/base64"
	"fmt"
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
		// Repository zaten varsa hata verme
		if strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			fmt.Printf("📦 Repository %s already exists\n", repositoryName)
			return nil
		}
		return fmt.Errorf("failed to create repository %s: %w", repositoryName, err)
	}

	fmt.Printf("✅ Created ECR repository: %s\n", repositoryName)
	return nil
}

func (c *Client) ConvertImageName(originalImage, prefix string) (string, string) {
	// docker.io/mongodb/mongodb-community-server:6.0.5-ubi8
	// -> <account_id>.dkr.ecr.<region>.amazonaws.com/k8s-backup/mongodb-community-server:6.0.5-ubi8

	parts := strings.Split(originalImage, "/")
	var imageName, tag string

	if len(parts) == 1 {
		// busybox -> busybox:latest
		imageName = parts[0]
		tag = "latest"
	} else {
		// Get last part and remove tag
		imageWithTag := parts[len(parts)-1]
		if strings.Contains(imageWithTag, ":") {
			nameTag := strings.Split(imageWithTag, ":")
			imageName = nameTag[0]
			tag = nameTag[1]
		} else {
			imageName = imageWithTag
			tag = "latest"
		}
	}

	// Remove SHA digest
	if strings.Contains(tag, "@sha256") {
		tag = strings.Split(tag, "@")[0]
		if tag == "" {
			tag = "latest"
		}
	}

	// Prefix ekle
	fullRepoName := fmt.Sprintf("%s/%s", prefix, imageName)
	ecrImage := fmt.Sprintf("%s/%s:%s", c.GetRegistryURL(), fullRepoName, tag)

	return ecrImage, fullRepoName
}
