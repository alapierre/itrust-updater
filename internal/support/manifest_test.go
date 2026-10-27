package support

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"testing"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

type manifestBackend struct {
	backend.Backend
	files    map[string][]byte
	requests []string
}

func (b *manifestBackend) Get(_ context.Context, path string) (io.ReadCloser, error) {
	b.requests = append(b.requests, path)
	data, ok := b.files[path]
	if !ok {
		return nil, fmt.Errorf("unexpected request: %s", path)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func TestFetchAndVerifyManifestContext(t *testing.T) {
	seed := base64.StdEncoding.EncodeToString(make([]byte, 32))
	pub, err := sign.SeedToPubKey(seed)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, requestedVersion, servedApp, servedRepo, servedChannel, servedVersion string
		wantErr                                                                     bool
	}{
		{"channel", "", "app", "repo", "stable", "2.0.0", false},
		{"latest alias", "latest", "app", "repo", "stable", "2.0.0", false},
		{"wrong app", "", "other", "repo", "stable", "2.0.0", true},
		{"wrong repo", "", "app", "other", "stable", "2.0.0", true},
		{"wrong channel", "", "app", "repo", "beta", "2.0.0", true},
		{"latest wrong channel", "latest", "app", "repo", "beta", "2.0.0", true},
		{"explicit version", "1.0.0", "app", "repo", "stable", "1.0.0", false},
		{"explicit version original channel", "1.0.0", "app", "repo", "beta", "1.0.0", false},
		{"explicit version wrong app", "1.0.0", "other", "repo", "stable", "1.0.0", true},
		{"explicit version wrong repo", "1.0.0", "app", "other", "stable", "1.0.0", true},
		{"explicit wrong version", "1.0.0", "app", "repo", "stable", "2.0.0", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, err := manifest.SignManifest(manifest.Payload{
				SchemaVersion: 1, App: manifest.AppInfo{ID: tc.servedApp}, Repo: manifest.RepoInfo{ID: tc.servedRepo},
				Channel: tc.servedChannel, Latest: manifest.Release{Version: tc.servedVersion},
			}, seed, "key")
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			path := "apps/app/channels/stable.json"
			if tc.requestedVersion != "" && tc.requestedVersion != "latest" {
				path = "apps/app/releases/v" + tc.requestedVersion + "/artifacts.json"
			}
			b := &manifestBackend{files: map[string][]byte{"key.pub": pub, path: data}}
			expected := manifest.Identity{RepoID: "repo", AppID: "app", Channel: "stable", Version: tc.requestedVersion}
			verified, _, err := FetchAndVerifyManifest(context.Background(), b, expected, "key.pub", sign.SHA256(pub))
			if (err != nil) != tc.wantErr {
				t.Fatalf("wantErr=%v, got %v", tc.wantErr, err)
			}
			if tc.wantErr && verified != nil {
				t.Fatal("rejected manifest exposed to caller")
			}
			if len(b.requests) != 2 || b.requests[1] != path {
				t.Fatalf("unexpected requests: %v", b.requests)
			}
		})
	}
}

func TestFetchRejectsUnpinnedKeyBeforeReadingManifest(t *testing.T) {
	b := &manifestBackend{files: map[string][]byte{"key.pub": []byte("attacker key")}}
	m, _, err := FetchAndVerifyManifest(context.Background(), b, manifest.Identity{AppID: "app", Channel: "stable"}, "key.pub", "trusted-fingerprint")
	if err == nil || m != nil {
		t.Fatal("unpinned key accepted")
	}
	if len(b.requests) != 1 {
		t.Fatalf("manifest fetched with an untrusted key: %v", b.requests)
	}
}
