package support

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileAndRepositoryTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadConfigWithRepoOverlay(dir, "../victim"); err == nil {
		t.Fatal("profile traversal allowed")
	}
	if err := os.Mkdir(filepath.Join(dir, "apps"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apps", "safe.env"), []byte("ITRUST_REPO_ID=../victim\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// ENV has precedence; use an invalid ID there as well to check propagation.
	t.Setenv("ITRUST_REPO_ID", "../victim")
	if _, err := LoadConfigWithRepoOverlay(dir, "safe"); err == nil {
		t.Fatal("repository validation error was ignored")
	}
}
