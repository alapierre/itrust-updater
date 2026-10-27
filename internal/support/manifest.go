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
	defer pubKeyReader.Close()

	pubKey, err := io.ReadAll(pubKeyReader)
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
	defer manifestReader.Close()

	var m manifest.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("failed to decode manifest: %v", err)
	}

	if err := m.Verify(pubKey); err != nil {
		return nil, nil, fmt.Errorf("manifest signature verification failed: %v", err)
	}
	if m.Payload.App.ID != appId {
		return nil, nil, fmt.Errorf("manifest app ID mismatch: expected %q, got %q", appId, m.Payload.App.ID)
	}
	if version != "" && version != "latest" {
		if m.Payload.Latest.Version != version {
			return nil, nil, fmt.Errorf("manifest version mismatch: expected %q, got %q", version, m.Payload.Latest.Version)
		}
	} else if m.Payload.Channel != channel {
		return nil, nil, fmt.Errorf("manifest channel mismatch: expected %q, got %q", channel, m.Payload.Channel)
	}

	return &m, pubKey, nil
}
