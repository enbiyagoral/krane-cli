package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

// Mirror copies an image from srcRef to dstRef directly between registries,
// preserving manifest indexes (multi-arch) when present. No local Docker is used.
// If platform is non-empty (e.g., "linux/amd64"), mirrors only that platform variant.
func Mirror(ctx context.Context, srcRef, dstRef, ecrUsername, ecrPassword, platform string) error {
	// Source auth: default keychain (Docker config, env, etc.). Works for public as anon.
	srcOpt := crane.WithAuthFromKeychain(authn.DefaultKeychain)

	// Destination auth: ECR basic credentials from token
	dstAuth := &authn.Basic{Username: ecrUsername, Password: ecrPassword}
	dstOpt := crane.WithAuth(dstAuth)

	// Options build-up
	opts := []crane.Option{srcOpt, dstOpt}
	if platform != "" {
		if strings.Contains(platform, ",") {
			return fmt.Errorf("multiple platforms not yet supported: %s", platform)
		}
		parts := strings.SplitN(platform, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid platform format, expected os/arch: %s", platform)
		}
		p := &v1.Platform{OS: parts[0], Architecture: parts[1]}
		opts = append(opts, crane.WithPlatform(p))
	}

	// Perform registry-to-registry copy. crane.Copy preserves manifest lists when present.
	return crane.Copy(srcRef, dstRef, opts...)
}
