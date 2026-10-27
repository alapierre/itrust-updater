package cli

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/install"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

type updateFixture struct {
	t                   *testing.T
	configDir, stateDir string
	dest, seed, pin     string
	mu                  sync.Mutex
	responses           map[string][]byte
	artifactRequests    int
	beforeChannel       func() error
}

func newUpdateFixture(t *testing.T) *updateFixture {
	t.Helper()
	f := &updateFixture{t: t, configDir: t.TempDir(), stateDir: t.TempDir(), dest: filepath.Join(t.TempDir(), "app"), responses: make(map[string][]byte)}
	f.seed = base64.StdEncoding.EncodeToString(make([]byte, 32))
	pub, err := sign.SeedToPubKey(f.seed)
	if err != nil {
		t.Fatal(err)
	}
	f.pin = sign.SHA256(pub)
	f.responses["/repo/public-keys/ed25519.pub"] = pub
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.URL.Path == "/apps/app/channels/stable.json" && f.beforeChannel != nil {
			if err := f.beforeChannel(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/artifacts/") {
			f.artifactRequests++
		}
		data, ok := f.responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(server.Close)
	for key, value := range map[string]string{
		"ITRUST_BASE_URL": server.URL, "ITRUST_APP_ID": "app", "ITRUST_CHANNEL": "stable",
		"ITRUST_DEST": f.dest, "ITRUST_REPO_PUBKEY_SHA256": f.pin, "ITRUST_BACKEND": "nexus",
		"ITRUST_REPO_PUBKEY_PATH": "repo/public-keys/ed25519.pub", "ITRUST_REPO_ID": "",
		"ITRUST_NEXUS_USERNAME": "", "ITRUST_NEXUS_PASSWORD": "",
	} {
		t.Setenv(key, value)
	}
	return f
}

func (f *updateFixture) release(version string, revision int) *manifest.Manifest {
	f.t.Helper()
	body := []byte("version " + version)
	artifact := manifest.Artifact{OS: "linux", Arch: "amd64", Type: "binary", URL: "artifacts/" + version, Size: int64(len(body)), Sha256: sign.SHA256(body)}
	m, err := manifest.SignManifest(manifest.Payload{
		SchemaVersion: 1, App: manifest.AppInfo{ID: "app"}, Channel: "stable",
		GeneratedAt: time.Date(2026, 1, revision, 0, 0, 0, 0, time.UTC),
		Latest:      manifest.Release{Version: version, Artifacts: []manifest.Artifact{artifact}},
	}, f.seed, "test")
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.responses["/"+artifact.URL] = body
	f.mu.Unlock()
	f.serveManifest(fmt.Sprintf("/apps/app/releases/v%s/artifacts.json", version), m)
	return m
}

func (f *updateFixture) serveManifest(path string, m *manifest.Manifest) {
	f.t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.responses[path] = data
	f.mu.Unlock()
}

func (f *updateFixture) channel(m *manifest.Manifest) {
	f.serveManifest("/apps/app/channels/stable.json", m)
}

func (f *updateFixture) get(version string, force bool) error {
	return handleGet(context.Background(), "profile", version, "", "linux", "amd64", f.configDir, f.stateDir, force, true, false)
}

func (f *updateFixture) mustGet(version string) {
	f.t.Helper()
	if err := f.get(version, false); err != nil {
		f.t.Fatal(err)
	}
}

func (f *updateFixture) assertVersion(version string) {
	f.t.Helper()
	data, err := os.ReadFile(f.dest)
	if err != nil || string(data) != "version "+version {
		f.t.Fatalf("destination: %q, err=%v; want version %s", data, err, version)
	}
}

func TestGetRejectsReplayAndPreservesExplicitDowngrade(t *testing.T) {
	f := newUpdateFixture(t)
	old := f.release("1.0.0", 1)
	current := f.release("2.0.0", 2)
	f.channel(current)
	f.mustGet("")
	// This timestamp is outside the signed payload and must not affect ordering.
	old.Signature.CreatedAt = time.Now().Add(24 * time.Hour)
	f.channel(old)
	for _, version := range []string{"", "latest"} {
		for _, force := range []bool{false, true} {
			if err := f.get(version, force); err == nil || !strings.Contains(err.Error(), "replay rejected") {
				t.Fatalf("version=%q force=%v: expected replay rejection, got %v", version, force, err)
			}
			f.assertVersion("2.0.0")
		}
	}
	f.mustGet("1.0.0")
	f.assertVersion("1.0.0")
	if err := f.get("", false); err == nil {
		t.Fatal("explicit downgrade erased replay protection")
	}
	f.channel(current)
	f.mustGet("")
	f.assertVersion("2.0.0")
	// Publisher intentionally rolls back through NEW authenticated metadata.
	f.channel(f.release("1.0.0", 3))
	f.mustGet("")
	f.assertVersion("1.0.0")
}

func TestGetCheckpointsMetadataEvenWhenAlreadyInstalled(t *testing.T) {
	f := newUpdateFixture(t)
	old := f.release("1.0.0", 1)
	f.channel(old)
	f.mustGet("")
	f.channel(f.release("1.0.0", 2))
	f.mustGet("")
	f.channel(old)
	if err := f.get("", false); err == nil {
		t.Fatal("up-to-date shortcut skipped checkpoint update")
	}
}

func TestGetRejectsConflictingTimestamp(t *testing.T) {
	f := newUpdateFixture(t)
	f.channel(f.release("1.0.0", 1))
	f.mustGet("")
	f.channel(f.release("2.0.0", 1))
	if err := f.get("", false); err == nil {
		t.Fatal("accepted different payload at same timestamp")
	}
	f.assertVersion("1.0.0")
}

func TestGetMigratesLegacyStateBeforeCheckingChannel(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			f := newUpdateFixture(t)
			old := f.release("1.0.0", 1)
			current := f.release("2.0.0", 2)
			st := &install.State{AppID: "app", Channel: "stable", InstalledVersion: "2.0.0", InstalledSha256: current.Payload.Latest.Artifacts[0].Sha256, OS: "linux", Arch: "amd64", Dest: f.dest}
			if err := install.SaveState(f.stateDir, "profile", st); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.dest, []byte("version 2.0.0"), 0755); err != nil {
				t.Fatal(err)
			}
			f.channel(old)
			if explicit {
				f.mustGet("1.0.0")
				f.assertVersion("1.0.0")
			}
			if err := f.get("", false); err == nil || !strings.Contains(err.Error(), "replay rejected") {
				t.Fatalf("legacy state did not establish baseline: %v", err)
			}
		})
	}
}

func TestGetDoesNotInstallWithoutPersistingHistory(t *testing.T) {
	f := newUpdateFixture(t)
	f.channel(f.release("1.0.0", 1))
	// Make the final rename fail after the initial history read succeeds.
	f.mu.Lock()
	f.beforeChannel = func() error {
		return os.MkdirAll(filepath.Join(f.stateDir, "manifest-history", sign.SHA256([]byte("profile"))+".json"), 0700)
	}
	f.mu.Unlock()
	if err := f.get("", false); err == nil || !strings.Contains(err.Error(), "failed to persist manifest history") {
		t.Fatalf("expected checkpoint persistence failure, got %v", err)
	}
	if _, err := os.Stat(f.dest); !os.IsNotExist(err) {
		t.Fatalf("destination was modified: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.artifactRequests != 0 {
		t.Fatal("download started without persistent replay protection")
	}
}

func TestGetRetainsCheckpointAfterFailedDownload(t *testing.T) {
	f := newUpdateFixture(t)
	old := f.release("1.0.0", 1)
	f.channel(f.release("2.0.0", 2))
	f.mu.Lock()
	delete(f.responses, "/artifacts/2.0.0")
	f.mu.Unlock()
	if err := f.get("", false); err == nil || !strings.Contains(err.Error(), "failed to download artifact") {
		t.Fatalf("expected download failure, got %v", err)
	}
	f.channel(old)
	if err := f.get("", false); err == nil || !strings.Contains(err.Error(), "replay rejected") {
		t.Fatalf("failed download erased protection: %v", err)
	}
}

func TestGetStopsOnCorruptHistory(t *testing.T) {
	f := newUpdateFixture(t)
	f.channel(f.release("1.0.0", 1))
	f.mustGet("")
	path := filepath.Join(f.stateDir, "manifest-history", sign.SHA256([]byte("profile"))+".json")
	if err := os.WriteFile(path, []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	f.channel(f.release("2.0.0", 2))
	if err := f.get("", true); err == nil || !strings.Contains(err.Error(), "failed to load manifest history") {
		t.Fatalf("corrupt history was ignored: %v", err)
	}
	f.assertVersion("1.0.0")
}

func TestGetRejectsConcurrentUpdate(t *testing.T) {
	f := newUpdateFixture(t)
	f.channel(f.release("1.0.0", 1))
	entered := make(chan struct{})
	finish := make(chan struct{})
	f.mu.Lock()
	f.beforeChannel = func() error {
		close(entered)
		<-finish
		return nil
	}
	f.mu.Unlock()
	done := make(chan error, 1)
	go func() { done <- f.get("", false) }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("first update ended before requesting manifest: %v", err)
	}
	err := f.get("", false)
	close(finish)
	firstErr := <-done
	if err == nil || !strings.Contains(err.Error(), "is locked") {
		t.Fatalf("concurrent update was not blocked: %v", err)
	}
	if firstErr != nil {
		t.Fatal(firstErr)
	}
}

func TestGetFailsClosedWhenLegacyBaselineCannotBeVerified(t *testing.T) {
	for _, scenario := range []string{"missing", "wrong artifact", "wrong channel"} {
		t.Run(scenario, func(t *testing.T) {
			f := newUpdateFixture(t)
			current := f.release("2.0.0", 2)
			st := &install.State{AppID: "app", Channel: "stable", InstalledVersion: "2.0.0", InstalledSha256: current.Payload.Latest.Artifacts[0].Sha256, OS: "linux", Arch: "amd64"}
			switch scenario {
			case "missing":
				f.mu.Lock()
				delete(f.responses, "/apps/app/releases/v2.0.0/artifacts.json")
				f.mu.Unlock()
			case "wrong artifact":
				st.InstalledSha256 = sign.SHA256([]byte("different artifact"))
			case "wrong channel":
				st.Channel = "beta"
			}
			if err := install.SaveState(f.stateDir, "profile", st); err != nil {
				t.Fatal(err)
			}
			f.channel(f.release("1.0.0", 1))
			if err := f.get("", false); err == nil || !strings.Contains(err.Error(), "cannot initialize replay protection") {
				t.Fatalf("unverifiable baseline was ignored: %v", err)
			}
		})
	}
}

func TestGetAuthenticatesMetadataBeforeCheckpointing(t *testing.T) {
	for _, scenario := range []string{"unsigned timestamp", "wrong app", "wrong channel", "wrong explicit version", "missing timestamp"} {
		t.Run(scenario, func(t *testing.T) {
			f := newUpdateFixture(t)
			m := f.release("1.0.0", 1)
			version := ""
			switch scenario {
			case "unsigned timestamp":
				m.Payload.GeneratedAt = m.Payload.GeneratedAt.Add(time.Hour)
			case "wrong app":
				m.Payload.App.ID = "other-app"
			case "wrong channel":
				m.Payload.Channel = "beta"
			case "wrong explicit version":
				version = "2.0.0"
			case "missing timestamp":
				m.Payload.GeneratedAt = time.Time{}
			}
			if scenario != "unsigned timestamp" {
				var err error
				m, err = manifest.SignManifest(m.Payload, f.seed, "test")
				if err != nil {
					t.Fatal(err)
				}
			}
			f.channel(m)
			f.serveManifest("/apps/app/releases/v2.0.0/artifacts.json", m)
			if err := f.get(version, false); err == nil {
				t.Fatal("invalid metadata accepted")
			}
			if h, err := install.LoadManifestHistory(f.stateDir, "profile"); err != nil || h != nil {
				t.Fatalf("invalid metadata changed history: %v, %v", h, err)
			}
		})
	}
}
