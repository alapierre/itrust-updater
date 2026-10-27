package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

const testReleasePath = "/apps/app/releases/v1.0.0/artifacts.json"

type pushFixture struct {
	t            *testing.T
	seed         string
	pub          []byte
	configPath   string
	artifactPath string
	mu           sync.Mutex
	existing     []byte
	uploads      map[string][]byte
	requests     int
	keyRequests  int
}

func newPushFixture(t *testing.T) *pushFixture {
	t.Helper()
	f := &pushFixture{t: t, seed: base64.StdEncoding.EncodeToString(make([]byte, 32)), uploads: make(map[string][]byte)}
	var err error
	f.pub, err = sign.SeedToPubKey(f.seed)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	f.configPath = filepath.Join(dir, "missing-project.env")
	f.artifactPath = filepath.Join(dir, "app.bin")
	if err := os.WriteFile(f.artifactPath, []byte("new linux binary"), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.requests++
		if strings.HasSuffix(r.URL.Path, ".pub") {
			f.keyRequests++
			// The repository controls this key too. It must never become trusted.
			attackerSeed := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
			pub, _ := sign.SeedToPubKey(attackerSeed)
			w.Write(pub)
			return
		}
		switch r.Method {
		case http.MethodHead, http.MethodGet:
			if r.URL.Path != testReleasePath || f.existing == nil {
				http.NotFound(w, r)
				return
			}
			if r.Method == http.MethodGet {
				w.Write(f.existing)
			}
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			f.uploads[r.URL.Path] = data
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)
	t.Setenv("HOME", t.TempDir())
	for key, value := range map[string]string{
		"ITRUST_BASE_URL": server.URL, "ITRUST_REPO_SIGNING_ED25519_SEED_B64": f.seed,
		"ITRUST_BACKEND": "nexus", "ITRUST_CHANNEL": "stable", "ITRUST_OS": "linux", "ITRUST_ARCH": "amd64",
		"ITRUST_NEXUS_USERNAME": "", "ITRUST_NEXUS_PASSWORD": "",
	} {
		t.Setenv(key, value)
	}
	return f
}

func (f *pushFixture) signedRelease() *manifest.Manifest {
	f.t.Helper()
	m, err := manifest.SignManifest(manifest.Payload{
		SchemaVersion: 1, Repo: manifest.RepoInfo{ID: "repo"}, App: manifest.AppInfo{ID: "app"},
		Channel: "stable", GeneratedAt: time.Now().UTC(),
		Latest: manifest.Release{Version: "1.0.0", Artifacts: []manifest.Artifact{{
			OS: "windows", Arch: "amd64", Type: "exe", URL: "windows.exe", Size: 7, Sha256: sign.SHA256([]byte("windows")),
		}}},
	}, f.seed, "publisher")
	if err != nil {
		f.t.Fatal(err)
	}
	return m
}

func (f *pushFixture) serve(m *manifest.Manifest) {
	f.t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.existing = data
	f.mu.Unlock()
}

func (f *pushFixture) push(force bool) error {
	return handlePush(context.Background(), f.configPath, f.artifactPath, "repo", "app", "1.0.0", false, force, true, false)
}

func TestPushRejectsUntrustedManifestBeforeAnyUpload(t *testing.T) {
	for _, scenario := range []string{"unsigned", "tampered artifact", "attacker key", "wrong app", "wrong version", "wrong repo", "missing repo", "unsupported schema", "unsupported algorithm", "malformed JSON", "oversized manifest"} {
		for _, force := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/force=%v", scenario, force), func(t *testing.T) {
				f := newPushFixture(t)
				m := f.signedRelease()
				seed := f.seed
				resign := true
				switch scenario {
				case "unsigned":
					m.Signature = manifest.Signature{}
					resign = false
				case "tampered artifact":
					m.Payload.Latest.Artifacts[0].URL = "malware.exe"
					m.Payload.Latest.Artifacts[0].Sha256 = sign.SHA256([]byte("malware"))
					resign = false
				case "attacker key":
					seed = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
				case "wrong app":
					m.Payload.App.ID = "other-app"
				case "wrong version":
					m.Payload.Latest.Version = "0.1.0"
				case "wrong repo":
					m.Payload.Repo.ID = "other-repo"
				case "missing repo":
					m.Payload.Repo.ID = ""
				case "unsupported schema":
					m.Payload.SchemaVersion = 2
				case "unsupported algorithm":
					m.Signature.Alg = "unknown"
					resign = false
				}
				if resign {
					var err error
					m, err = manifest.SignManifest(m.Payload, seed, "key")
					if err != nil {
						t.Fatal(err)
					}
				}
				f.serve(m)
				if scenario == "malformed JSON" {
					f.mu.Lock()
					f.existing = []byte("{")
					f.mu.Unlock()
				}
				if scenario == "oversized manifest" {
					f.mu.Lock()
					f.existing = append(f.existing, []byte(strings.Repeat(" ", int(manifest.MaxManifestSize)))...)
					f.mu.Unlock()
				}
				if err := f.push(force); err == nil {
					t.Fatal("untrusted manifest was accepted")
				}
				f.mu.Lock()
				defer f.mu.Unlock()
				if len(f.uploads) != 0 {
					t.Fatalf("rejected manifest still caused uploads: %v", f.uploads)
				}
				if f.keyRequests != 0 {
					t.Fatal("publisher fetched a verification key from the untrusted repository")
				}
			})
		}
	}
}

func TestPushMergesOnlyAuthenticatedRelease(t *testing.T) {
	for _, scenario := range []string{"new release", "add platform", "promote channel", "replace platform"} {
		t.Run(scenario, func(t *testing.T) {
			f := newPushFixture(t)
			m := f.signedRelease()
			windows := m.Payload.Latest.Artifacts[0]
			if scenario == "promote channel" {
				m.Payload.Channel = "beta"
			}
			if scenario == "replace platform" {
				m.Payload.Latest.Artifacts = append(m.Payload.Latest.Artifacts, manifest.Artifact{
					OS: "linux", Arch: "amd64", Type: "binary", URL: "old-linux", Sha256: sign.SHA256([]byte("old linux")),
				})
			}
			if scenario != "new release" {
				var err error
				m, err = manifest.SignManifest(m.Payload, f.seed, "key")
				if err != nil {
					t.Fatal(err)
				}
				f.serve(m)
			}
			if err := f.push(scenario == "replace platform"); err != nil {
				t.Fatal(err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if len(f.uploads) != 4 {
				t.Fatalf("expected artifact, hash and two manifests; got %d uploads", len(f.uploads))
			}
			for _, path := range []string{testReleasePath, "/apps/app/channels/stable.json"} {
				var published manifest.Manifest
				if err := json.Unmarshal(f.uploads[path], &published); err != nil {
					t.Fatal(err)
				}
				if err := published.VerifyFor(f.pub, manifest.Identity{RepoID: "repo", AppID: "app", Channel: "stable"}); err != nil {
					t.Fatal(err)
				}
				wantCount := 2
				if scenario == "new release" {
					wantCount = 1
				} else {
					a, err := published.FindArtifact("windows", "amd64")
					if err != nil || *a != windows {
						t.Fatalf("authenticated Windows artifact was not preserved: %v, %v", a, err)
					}
				}
				if len(published.Payload.Latest.Artifacts) != wantCount {
					t.Fatalf("unexpected artifact count: %d", len(published.Payload.Latest.Artifacts))
				}
				a, err := published.FindArtifact("linux", "amd64")
				if err != nil || a.Sha256 != sign.SHA256([]byte("new linux binary")) {
					t.Fatalf("new Linux artifact missing: %v, %v", a, err)
				}
			}
		})
	}
}

func TestPushRejectsInvalidSeedBeforeNetworkAccess(t *testing.T) {
	f := newPushFixture(t)
	t.Setenv("ITRUST_REPO_SIGNING_ED25519_SEED_B64", "invalid-seed")
	if err := f.push(false); err == nil || !strings.Contains(err.Error(), "invalid repository signing seed") {
		t.Fatalf("expected seed validation error, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.requests != 0 {
		t.Fatal("invalid signing key caused network access")
	}
}

func TestPushStillRequiresForceForExistingPlatform(t *testing.T) {
	f := newPushFixture(t)
	m := f.signedRelease()
	m.Payload.Latest.Artifacts[0].OS = "linux"
	m, err := manifest.SignManifest(m.Payload, f.seed, "key")
	if err != nil {
		t.Fatal(err)
	}
	f.serve(m)
	if err := f.push(false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected platform conflict, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.uploads) != 0 {
		t.Fatal("platform conflict caused uploads")
	}
}

func TestPushRejectsOversizedGeneratedManifestBeforeAnyUpload(t *testing.T) {
	f := newPushFixture(t)
	t.Setenv("ITRUST_APP_NAME", strings.Repeat("a", int(manifest.MaxManifestSize)))
	if err := f.push(false); err == nil || !strings.Contains(err.Error(), "generated manifest exceeds size limit") {
		t.Fatalf("expected generated manifest size rejection, got %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.uploads) != 0 {
		t.Fatal("manifest size rejection occurred after upload")
	}
}
