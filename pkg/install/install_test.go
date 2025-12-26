package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSaveLoadState(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "itrust-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	state := &State{
		Profile:          "test-profile",
		AppID:            "test-app",
		Channel:          "stable",
		InstalledVersion: "1.0.0",
		InstalledSha256:  "abc",
		InstalledAt:      time.Now().Truncate(time.Second),
		Dest:             "/tmp/test",
		OS:               "linux",
		Arch:             "amd64",
	}

	err = SaveState(tmpDir, "test-profile", state)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	// Verify atomicity - check if no tmp files left
	files, _ := os.ReadDir(filepath.Join(tmpDir, "state"))
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".tmp") {
			t.Errorf("Temporary file left: %s", f.Name())
		}
	}

	loaded, err := LoadState(tmpDir, "test-profile")
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded state is nil")
	}

	if loaded.AppID != state.AppID || loaded.InstalledVersion != state.InstalledVersion {
		t.Errorf("Loaded state mismatch: %+v", loaded)
	}
}

func TestInstallArtifact_CreateDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "itrust-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	stateDir := filepath.Join(tmpDir, "state")
	dest := filepath.Join(tmpDir, "non-existent-dir", "app.bin")
	src := strings.NewReader("content")
	expectedSha := "ed7002b439e9ac845f22357d822bac1444730fbdb6016d3ec9432297b9ec9f73" // sha256 of "content"

	sha, err := InstallArtifact(src, dest, expectedSha, stateDir, "test")
	if err != nil {
		t.Fatalf("InstallArtifact failed: %v", err)
	}

	if sha != expectedSha {
		t.Errorf("Expected sha %s, got %s", expectedSha, sha)
	}

	if _, err := os.Stat(dest); err != nil {
		t.Errorf("Destination file not created: %v", err)
	}
}
