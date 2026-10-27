package manifest

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/sign"
)

func TestVerifyForIdentity(t *testing.T) {
	seed := base64.StdEncoding.EncodeToString(make([]byte, 32))
	pub, err := sign.SeedToPubKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	payload := Payload{
		SchemaVersion: 1, Repo: RepoInfo{ID: "repo"}, App: AppInfo{ID: "app"},
		Channel: "beta", Latest: Release{Version: "1.0.0"},
	}
	m, err := SignManifest(payload, seed, "key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		expected Identity
		wantErr  string
	}{
		{"matching channel", Identity{RepoID: "repo", AppID: "app", Channel: "beta"}, ""},
		{"matching release", Identity{RepoID: "repo", AppID: "app", Version: "1.0.0"}, ""},
		{"explicit release from another channel", Identity{RepoID: "repo", AppID: "app", Channel: "stable", Version: "1.0.0"}, ""},
		{"repo ID not configured", Identity{AppID: "app", Channel: "beta"}, ""},
		{"wrong repo", Identity{RepoID: "other", AppID: "app", Channel: "beta"}, "repo ID mismatch"},
		{"wrong app", Identity{AppID: "other", Channel: "beta"}, "app ID mismatch"},
		{"missing expected app", Identity{Channel: "beta"}, "app ID mismatch"},
		{"wrong channel", Identity{AppID: "app", Channel: "stable"}, "channel mismatch"},
		{"missing expected channel", Identity{AppID: "app"}, "channel mismatch"},
		{"wrong version", Identity{AppID: "app", Version: "2.0.0"}, "version mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := m.VerifyFor(pub, tc.expected)
			if tc.wantErr == "" && err != nil {
				t.Fatal(err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected %q, got %v", tc.wantErr, err)
			}
		})
	}
}

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

	// Test find JAR artifact
	m.Payload.Latest.Artifacts = append(m.Payload.Latest.Artifacts, Artifact{
		OS:     "any",
		Arch:   "any",
		Type:   "jar",
		URL:    "app.jar",
		Size:   500,
		Sha256: "jar-sha",
	})

	jar, err := m.FindArtifact("windows", "arm64")
	if err != nil {
		t.Errorf("FindArtifact JAR failed: %v", err)
	}
	if jar.Type != "jar" {
		t.Errorf("Expected jar, got %s", jar.Type)
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
