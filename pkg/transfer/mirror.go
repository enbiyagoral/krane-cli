package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Mirror copies an image from source to destination registry using crane.
// Preserves multi-arch manifests and handles platform-specific copying.
func Mirror(ctx context.Context, srcRef, dstRef, platform string) error {
	srcRef = normalizeImageReference(srcRef)

	opts := []crane.Option{
		crane.WithAuthFromKeychain(authn.DefaultKeychain),
		crane.WithContext(ctx),
	}

	if platform != "" {
		if err := validatePlatform(platform); err != nil {
			return err
		}
		parts := strings.SplitN(platform, "/", 2)
		opts = append(opts, crane.WithPlatform(&v1.Platform{
			OS:           parts[0],
			Architecture: parts[1],
		}))
	}

	return crane.Copy(srcRef, dstRef, opts...)
}

// validatePlatform validates the platform format (os/arch).
func validatePlatform(platform string) error {
	if strings.Contains(platform, ",") {
		return fmt.Errorf("multiple platforms not supported: %s", platform)
	}
	parts := strings.SplitN(platform, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid platform format, expected os/arch: %s", platform)
	}
	return nil
}

// normalizeImageReference adds docker.io prefix if no registry is specified.
func normalizeImageReference(ref string) string {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) >= 2 {
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			return ref
		}
	}
	return "docker.io/" + ref
}
