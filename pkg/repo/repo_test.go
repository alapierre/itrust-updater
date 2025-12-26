package repo

import (
	"os"
	"testing"
)

func TestRepoConfig(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "itrust-repo-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	rc := &RepoConfig{
		RepoID:       "test-repo",
		BaseURL:      "https://nexus.example.com",
		PubkeyPath:   "keys/ed25519.pub",
		PubkeySha256: "abcdef1234567890",
	}

	err = SaveRepoConfig(tmpDir, rc)
	if err != nil {
		t.Fatalf("SaveRepoConfig failed: %v", err)
	}

	path := GetRepoConfigPath(tmpDir, "test-repo")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Repo config file not created at %s", path)
	}

	loaded, err := LoadRepoConfig(tmpDir, "test-repo")
	if err != nil {
		t.Fatalf("LoadRepoConfig failed: %v", err)
	}

	if loaded.RepoID != rc.RepoID || loaded.BaseURL != rc.BaseURL || loaded.PubkeyPath != rc.PubkeyPath || loaded.PubkeySha256 != rc.PubkeySha256 {
		t.Errorf("Loaded config mismatch: %+v vs %+v", loaded, rc)
	}
}
