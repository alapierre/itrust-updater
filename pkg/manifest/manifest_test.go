package manifest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/sign"
)

func TestManifestSignVerify(t *testing.T) {
	seedB64 := "tG8Y/V8NOnR5i/YkO9uH0WlG6G6fR5e7uI9oP9kI9mI=" // 32 bytes b64
	keyID := "test-key"

	payload := Payload{
		SchemaVersion: 1,
		Repo:          RepoInfo{ID: "repo1", Name: "Repo 1"},
		App:           AppInfo{ID: "app1", Name: "App 1"},
		Channel:       "stable",
		GeneratedAt:   time.Now().UTC(),
		Latest: Release{
			Version:     "1.2.3",
			ReleaseDate: time.Now().UTC(),
			Artifacts: []Artifact{
				{
					OS:     "linux",
					Arch:   "amd64",
					Type:   "binary",
					URL:    "apps/app1/v1.2.3/linux/amd64/app1",
					Size:   1024,
					Sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
				},
			},
		},
	}

	m, err := SignManifest(payload, seedB64, keyID)
	if err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}

	pubKey, err := sign.SeedToPubKey(seedB64)
	if err != nil {
		t.Fatalf("SeedToPubKey failed: %v", err)
	}

	if err := m.Verify(pubKey); err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	// Test find artifact
	a, err := m.FindArtifact("linux", "amd64")
	if err != nil {
		t.Errorf("FindArtifact failed: %v", err)
	}
	if a.OS != "linux" {
		t.Errorf("Expected linux, got %s", a.OS)
	}

	// Corrupt payload
	m.Payload.Channel = "corrupted"
	if err := m.Verify(pubKey); err == nil {
		t.Error("Verify should have failed for corrupted payload")
	}
}

func TestArtifactsList(t *testing.T) {
	// ArtifactsList is simple struct, just test JSON marshaling
	al := ArtifactsList{
		Version: "1.0.0",
		Artifacts: []Artifact{
			{OS: "windows", Arch: "amd64", Sha256: "sha"},
		},
	}
	data, err := json.Marshal(al)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var al2 ArtifactsList
	if err := json.Unmarshal(data, &al2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if al2.Version != "1.0.0" || len(al2.Artifacts) != 1 {
		t.Errorf("Data mismatch")
	}
}
