package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

type Client struct {
	cli *client.Client
}

func NewClient() (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Client{cli: cli}, nil
}

// parseDockerResponse parses Docker API JSON responses
func parseDockerResponse(reader io.Reader, onStatus func(string, map[string]interface{}), onError func(string)) error {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue // Skip non-JSON lines
		}

		// Check for errors first
		if errorDetail, exists := result["errorDetail"]; exists {
			if errorMap, ok := errorDetail.(map[string]interface{}); ok {
				if message, ok := errorMap["message"].(string); ok {
					onError(message)
					return fmt.Errorf("docker operation failed: %s", message)
				}
			}
		}

		if errorMsg, exists := result["error"]; exists {
			if errorStr, ok := errorMsg.(string); ok {
				onError(errorStr)
				return fmt.Errorf("docker operation failed: %s", errorStr)
			}
		}

		// Handle status messages
		if status, exists := result["status"]; exists {
			if statusStr, ok := status.(string); ok {
				onStatus(statusStr, result)
			}
		}
	}
	return scanner.Err()
}

func (c *Client) PullImage(ctx context.Context, imageName string) error {
	fmt.Printf("📥 Pulling: %s\n", imageName)

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Use helper function for JSON parsing
	return parseDockerResponse(reader,
		func(status string, result map[string]interface{}) {
			switch status {
			case "Downloading":
				if progress, hasProgress := result["progress"]; hasProgress {
					if progressStr, ok := progress.(string); ok {
						fmt.Printf("\r📥 %s: %s", status, progressStr)
					}
				}
			case "Pull complete", "Download complete":
				fmt.Printf("\r✅ %s\n", status)
			case "Status":
				if message, hasMessage := result["message"]; hasMessage {
					if msgStr, ok := message.(string); ok {
						fmt.Printf("ℹ️  %s\n", msgStr)
					}
				}
			}
		},
		func(errorMsg string) {
			fmt.Printf("❌ Pull error: %s\n", errorMsg)
		})
}

func (c *Client) TagImage(ctx context.Context, sourceImage, targetImage string) error {
	fmt.Printf("🏷️  Tagging: %s -> %s\n", sourceImage, targetImage)

	return c.cli.ImageTag(ctx, sourceImage, targetImage)
}

func (c *Client) PushImage(ctx context.Context, imageName string) error {
	fmt.Printf("Pushing image: %s\n", imageName)

	reader, err := c.cli.ImagePush(ctx, imageName, image.PushOptions{
		All: false,
	})
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Use helper function for JSON parsing
	return parseDockerResponse(reader,
		func(status string, result map[string]interface{}) {
			fmt.Println(status) // Print all status messages
		},
		func(errorMsg string) {
			fmt.Printf("❌ Push error: %s\n", errorMsg)
		})
}

func (c *Client) PushImageWithAuth(ctx context.Context, imageName, username, password string) error {
	fmt.Printf("📤 Pushing: %s\n", imageName)

	// Extract ECR registry domain
	parts := strings.Split(imageName, "/")
	registryDomain := parts[0]

	// Create auth config
	authConfig := registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: registryDomain,
	}

	// Encode auth config as JSON
	authConfigBytes, err := json.Marshal(authConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal auth config: %w", err)
	}

	// Base64 encode et
	authStr := base64.URLEncoding.EncodeToString(authConfigBytes)

	reader, err := c.cli.ImagePush(ctx, imageName, image.PushOptions{
		All:          false,
		RegistryAuth: authStr,
	})
	if err != nil {
		return fmt.Errorf("failed to push image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Use helper function for JSON parsing
	lastProgress := ""
	err = parseDockerResponse(reader,
		func(status string, result map[string]interface{}) {
			switch status {
			case "Pushing":
				if progress, hasProgress := result["progress"]; hasProgress {
					if progressStr, ok := progress.(string); ok {
						currentProgress := fmt.Sprintf("📤 Pushing: %s", progressStr)
						if currentProgress != lastProgress {
							fmt.Printf("\r%s", currentProgress)
							lastProgress = currentProgress
						}
					}
				}
			case "Pushed":
				fmt.Printf("\r✅ Pushed successfully\n")
			default:
				if strings.Contains(status, "digest:") {
					fmt.Printf("✅ %s\n", status)
				}
			}
		},
		func(errorMsg string) {
			fmt.Printf("❌ Push error: %s\n", errorMsg)
		})

	// Clear progress line
	fmt.Printf("\r%-80s\r", "")
	return err
}

func (c *Client) LoginToRegistry(ctx context.Context, username, password, registryURL string) error {
	// Extract registry domain for ECR login
	parts := strings.Split(registryURL, "/")
	registryDomain := parts[0]

	authConfig := registry.AuthConfig{
		Username:      username,
		Password:      password,
		ServerAddress: registryDomain,
	}

	_, err := c.cli.RegistryLogin(ctx, authConfig)
	if err != nil {
		return fmt.Errorf("failed to login to registry %s: %w", registryDomain, err)
	}

	return nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}
