package install

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/manifest"
)

func historyManifest(at time.Time, hash string) *manifest.Manifest {
	return &manifest.Manifest{
		Payload:   manifest.Payload{App: manifest.AppInfo{ID: "app"}, Channel: "stable", GeneratedAt: at},
		Signature: manifest.Signature{PayloadSha256: hash},
	}
}

func TestManifestHistoryOrdering(t *testing.T) {
	at := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name     string
		at       time.Time
		hash     string
		explicit bool
		changed  bool
		wantErr  bool
	}{
		{"identical", at, "new", false, false, false},
		{"older", at.Add(-time.Hour), "old", false, false, true},
		{"same timestamp different payload", at, "different", false, false, true},
		{"newer", at.Add(time.Hour), "newer", false, true, false},
		{"explicit older", at.Add(-time.Hour), "old", true, false, false},
		{"explicit same timestamp different payload", at, "different", true, false, false},
		{"explicit newer", at.Add(time.Hour), "newer", true, true, false},
		{"missing timestamp", time.Time{}, "missing", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &ManifestHistory{}
			if _, err := h.Accept(historyManifest(at, "new"), "key", false); err != nil {
				t.Fatal(err)
			}
			changed, err := h.Accept(historyManifest(tc.at, tc.hash), "key", tc.explicit)
			if (err != nil) != tc.wantErr || changed != tc.changed {
				t.Fatalf("changed=%v, err=%v", changed, err)
			}
			if !changed && h.Records[manifestScope("key", "app", "stable")].PayloadSHA256 != "new" {
				t.Fatal("historical or rejected manifest replaced checkpoint")
			}
		})
	}
}

func TestManifestHistoryScopes(t *testing.T) {
	at := time.Now().UTC()
	h := &ManifestHistory{}
	h.Accept(historyManifest(at, "new"), "key", false)
	for _, tc := range []struct{ key, app, channel string }{
		{"other-key", "app", "stable"}, {"key", "other-app", "stable"}, {"key", "app", "beta"},
	} {
		m := historyManifest(at.Add(-time.Hour), "old")
		m.Payload.App.ID, m.Payload.Channel = tc.app, tc.channel
		if changed, err := h.Accept(m, tc.key, false); err != nil || !changed {
			t.Fatalf("independent scope rejected: changed=%v, err=%v", changed, err)
		}
	}
	if _, err := h.Accept(historyManifest(at.Add(-time.Hour), "old"), "key", false); err == nil {
		t.Fatal("returning to original scope lost replay protection")
	}
}

func TestManifestHistoryPersistence(t *testing.T) {
	dir := t.TempDir()
	h := &ManifestHistory{}
	at := time.Now().UTC()
	h.Accept(historyManifest(at, strings.Repeat("a", 64)), "key", false)
	if err := SaveManifestHistory(dir, "profile", h); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManifestHistory(dir, "profile")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := loaded.Accept(historyManifest(at.Add(-time.Hour), strings.Repeat("b", 64)), "key", false); err == nil {
		t.Fatal("replay accepted after reloading")
	}
	if other, err := LoadManifestHistory(dir, "other-profile"); err != nil || other != nil {
		t.Fatalf("profiles share history: %v, %v", other, err)
	}
	files, err := os.ReadDir(filepath.Join(dir, "manifest-history"))
	if err != nil || len(files) != 1 {
		t.Fatalf("unexpected files after atomic save: %v, %v", files, err)
	}
}

func TestManifestHistoryRejectsCorruption(t *testing.T) {
	for _, data := range []string{`{`, `null`, `{}`, `{"records":{}}`, `{"records":{"bad":{}}}`} {
		t.Run(data, func(t *testing.T) {
			dir := t.TempDir()
			path := manifestHistoryPath(dir, "p")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadManifestHistory(dir, "p"); err == nil {
				t.Fatal("corrupt history treated as a new installation")
			}
		})
	}
}

func TestManifestHistoryFailedSavePreservesCheckpoint(t *testing.T) {
	dir := t.TempDir()
	h := &ManifestHistory{}
	h.Accept(historyManifest(time.Now().UTC(), strings.Repeat("a", 64)), "key", false)
	if err := SaveManifestHistory(dir, "p", h); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifestHistoryPath(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	// A timestamp outside JSON's supported range fails during encoding.
	h.Accept(historyManifest(time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC), "bad"), "key", false)
	if err := SaveManifestHistory(dir, "p", h); err == nil {
		t.Fatal("expected save failure")
	}
	after, _ := os.ReadFile(manifestHistoryPath(dir, "p"))
	if string(before) != string(after) || !json.Valid(after) {
		t.Fatal("failed save damaged persisted checkpoint")
	}
}

func TestProfileLock(t *testing.T) {
	dir := t.TempDir()
	unlock, err := LockProfile(dir, "p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockProfile(dir, "p"); err == nil {
		t.Fatal("concurrent update was allowed")
	}
	other, err := LockProfile(dir, "other")
	if err != nil {
		t.Fatal(err)
	}
	if err := other(); err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock, err = LockProfile(dir, "p")
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
