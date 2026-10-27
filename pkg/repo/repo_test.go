package repo

import (
	"os"
	"path/filepath"
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

	path, err := GetRepoConfigPath(tmpDir, "test-repo")
	if err != nil {
		t.Fatal(err)
	}
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

func TestRepoConfigRejectsUnsafeIDs(t *testing.T) {
	for _, id := range []string{"../apps/victim", "../../victim", `..\apps\victim`, "/tmp/victim", `C:\temp\victim`, ".", "..", "name\nITRUST_BASE_URL=bad", ""} {
		t.Run(id, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "config")
			if path, err := GetRepoConfigPath(dir, id); err == nil || path != "" {
				t.Fatal("unsafe config path returned")
			}
			if err := SaveRepoConfig(dir, &RepoConfig{RepoID: id}); err == nil {
				t.Fatal("unsafe write allowed")
			}
			if _, err := LoadRepoConfig(dir, id); err == nil {
				t.Fatal("unsafe read allowed")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("invalid ID created directories")
			}
		})
	}
}

func TestRepoConfigRejectsInjectedValuesAndIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := SaveRepoConfig(dir, &RepoConfig{RepoID: "safe", BaseURL: "https://example.invalid\nITRUST_REPO_ID=../victim"}); err == nil {
		t.Fatal("ENV injection accepted")
	}
	if err := os.MkdirAll(filepath.Join(dir, "repos"), 0700); err != nil {
		t.Fatal(err)
	}
	path, _ := GetRepoConfigPath(dir, "safe")
	for _, content := range []string{"ITRUST_REPO_ID=../victim\n", "ITRUST_REPO_ID=other\n"} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadRepoConfig(dir, "safe"); err == nil {
			t.Fatal("mismatching stored ID accepted")
		}
	}
}

func TestRepoConfigDoesNotFollowEscapingSymlinks(t *testing.T) {
	for _, kind := range []string{"directory", "file"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "config")
			outside := filepath.Join(root, "outside")
			for _, path := range []string{dir, outside} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			victim := filepath.Join(outside, "safe.env")
			if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
				t.Fatal(err)
			}
			link, target := filepath.Join(dir, "repos"), outside
			if kind == "file" {
				if err := os.Mkdir(link, 0700); err != nil {
					t.Fatal(err)
				}
				link, target = filepath.Join(link, "safe.env"), victim
			}
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := LoadRepoConfig(dir, "safe"); err == nil {
				t.Fatal("read escaped repository directory")
			}
			err := SaveRepoConfig(dir, &RepoConfig{RepoID: "safe"})
			if kind == "directory" && err == nil {
				t.Fatal("write followed directory symlink")
			}
			if kind == "file" && err != nil {
				t.Fatalf("atomic replacement of file symlink failed: %v", err)
			}
			data, _ := os.ReadFile(victim)
			if string(data) != "unchanged" {
				t.Fatal("external file overwritten")
			}
		})
	}
}

func TestInitializationLock(t *testing.T) {
	dir := t.TempDir()
	unlock, err := LockInitialization(dir, "safe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockInitialization(dir, "safe"); err == nil {
		t.Fatal("concurrent initialization allowed")
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	unlock, err = LockInitialization(dir, "safe")
	if err != nil {
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
}
