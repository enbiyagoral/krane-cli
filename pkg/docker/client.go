package docker

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func (c *Client) PullImage(ctx context.Context, imageName string) error {
	fmt.Printf("📥 Pulling: %s\n", imageName)

	reader, err := c.cli.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image %s: %w", imageName, err)
	}
	defer reader.Close()

	// Parse JSON stream and show only important messages
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse JSON line
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue // Skip non-JSON lines
		}

		// Show only important status messages
		if status, exists := result["status"]; exists {
			if statusStr, ok := status.(string); ok {
				switch statusStr {
				case "Downloading":
					if progress, hasProgress := result["progress"]; hasProgress {
						if progressStr, ok := progress.(string); ok {
							fmt.Printf("\r📥 %s: %s", statusStr, progressStr)
						}
					}
				case "Pull complete", "Download complete":
					fmt.Printf("\r✅ %s\n", statusStr)
				case "Status":
					if message, hasMessage := result["message"]; hasMessage {
						if msgStr, ok := message.(string); ok {
							fmt.Printf("ℹ️  %s\n", msgStr)
						}
					}
				}
			}
		}
	}

	// Clear progress line
	fmt.Printf("\r%-80s\r", "")
	return scanner.Err()
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

	// Parse JSON stream and catch errors
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()

		// Parse JSON line
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			// If not JSON, print directly
			fmt.Println(line)
			continue
		}

		// Return if error exists
		if errorDetail, exists := result["errorDetail"]; exists {
			if errorMap, ok := errorDetail.(map[string]interface{}); ok {
				if message, ok := errorMap["message"].(string); ok {
					return fmt.Errorf("push failed: %s", message)
				}
			}
		}

		// Also check error field
		if errorMsg, exists := result["error"]; exists {
			if errorStr, ok := errorMsg.(string); ok {
				return fmt.Errorf("push failed: %s", errorStr)
			}
		}

		// Print normal progress message
		fmt.Println(line)
	}

	return scanner.Err()
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

	// Parse JSON stream and show only important messages
	scanner := bufio.NewScanner(reader)
	lastProgress := ""

	for scanner.Scan() {
		line := scanner.Text()

		// Parse JSON line
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			continue // Skip non-JSON lines
		}

		// Return if error exists
		if errorDetail, exists := result["errorDetail"]; exists {
			if errorMap, ok := errorDetail.(map[string]interface{}); ok {
				if message, ok := errorMap["message"].(string); ok {
					return fmt.Errorf("push failed: %s", message)
				}
			}
		}

		// Also check error field
		if errorMsg, exists := result["error"]; exists {
			if errorStr, ok := errorMsg.(string); ok {
				return fmt.Errorf("push failed: %s", errorStr)
			}
		}

		// Show only important status messages
		if status, exists := result["status"]; exists {
			if statusStr, ok := status.(string); ok {
				switch statusStr {
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
					if strings.Contains(statusStr, "digest:") {
						fmt.Printf("✅ %s\n", statusStr)
					}
				}
			}
		}
	}

	// Clear progress line
	fmt.Printf("\r%-80s\r", "")

	return scanner.Err()
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
