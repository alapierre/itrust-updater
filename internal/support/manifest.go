package support

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

func FetchAndVerifyManifest(ctx context.Context, b backend.Backend, appId, channel, version, pubkeyPath, expectedPubkeySha string) (*manifest.Manifest, []byte, error) {
	// 1. Get and verify pubkey
	pubKeyReader, err := b.Get(ctx, pubkeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get repository public key: %v", err)
	}
	pubKey, err := io.ReadAll(pubKeyReader)
	pubKeyReader.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key: %v", err)
	}

	if err := sign.VerifyFingerprint(pubKey, expectedPubkeySha); err != nil {
		return nil, nil, fmt.Errorf("public key verification failed: %v", err)
	}

	// 2. Get manifest
	manifestPath := fmt.Sprintf("apps/%s/channels/%s.json", appId, channel)
	if version != "" && version != "latest" {
		manifestPath = fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	}

	manifestReader, err := b.Get(ctx, manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get manifest: %v", err)
	}
	var m manifest.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&m); err != nil {
		manifestReader.Close()
		return nil, nil, fmt.Errorf("failed to decode manifest: %v", err)
	}
	manifestReader.Close()

	if err := m.Verify(pubKey); err != nil {
		return nil, nil, fmt.Errorf("manifest signature verification failed: %v", err)
	}

	return &m, pubKey, nil
}
