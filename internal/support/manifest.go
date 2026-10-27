package support

import (
	"context"
	"fmt"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

func FetchAndVerifyManifest(ctx context.Context, b backend.Backend, expected manifest.Identity, pubkeyPath, expectedPubkeySha string) (*manifest.Manifest, []byte, error) {
	// 1. Get and verify pubkey
	pubKeyReader, err := b.Get(ctx, pubkeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get repository public key: %v", err)
	}
	defer pubKeyReader.Close()

	pubKey, err := sign.ReadPublicKey(pubKeyReader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key: %w", err)
	}

	if err := sign.VerifyFingerprint(pubKey, expectedPubkeySha); err != nil {
		return nil, nil, fmt.Errorf("public key verification failed: %v", err)
	}

	// 2. Get manifest
	if expected.Version == "latest" {
		expected.Version = ""
	}
	manifestPath := fmt.Sprintf("apps/%s/channels/%s.json", expected.AppID, expected.Channel)
	if expected.Version != "" {
		manifestPath = fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", expected.AppID, expected.Version)
	}

	manifestReader, err := b.Get(ctx, manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get manifest: %v", err)
	}
	defer manifestReader.Close()

	m, err := manifest.Read(manifestReader)
	if err != nil {
		return nil, nil, err
	}

	if err := m.VerifyFor(pubKey, expected); err != nil {
		return nil, nil, err
	}

	return m, pubKey, nil
}
