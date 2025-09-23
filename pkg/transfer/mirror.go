package transfer

import (
	"context"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
)

// Mirror copies an image from srcRef to dstRef directly between registries,
// preserving manifest indexes (multi-arch) when present. No local Docker is used.
func Mirror(ctx context.Context, srcRef, dstRef, ecrUsername, ecrPassword string) error {
	// Source auth: default keychain (Docker config, env, etc.). Works for public as anon.
	srcOpt := crane.WithAuthFromKeychain(authn.DefaultKeychain)

	// Destination auth: ECR basic credentials from token
	dstAuth := &authn.Basic{Username: ecrUsername, Password: ecrPassword}
	dstOpt := crane.WithAuth(dstAuth)

	// Perform registry-to-registry copy. crane.Copy preserves manifest lists when present.
	return crane.Copy(srcRef, dstRef, srcOpt, dstOpt)
}
