package install

import (
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alapierre/itrust-updater/pkg/sign"
)

type failedReader struct{}

func (failedReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type endlessReader struct{ read int64 }

func (r *endlessReader) Read(p []byte) (int, error) {
	clear(p)
	r.read += int64(len(p))
	return len(p), nil
}

func assertNoDownloadTemps(t *testing.T, dest string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(dest), "itrust-update-*"))
	if err != nil || len(paths) != 0 {
		t.Fatalf("temporary files left: %v (error: %v)", paths, err)
	}
}

func TestRejectedDownloadPreservesDestination(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  io.Reader
		size int64
		sha  string
	}{
		{"too short", strings.NewReader("data"), 5, sign.SHA256([]byte("data"))},
		{"too long", strings.NewReader("data"), 3, sign.SHA256([]byte("data"))},
		{"missing size", strings.NewReader("data"), 0, sign.SHA256([]byte("data"))},
		{"negative size", strings.NewReader("data"), -1, sign.SHA256([]byte("data"))},
		{"no overflow", strings.NewReader("data"), math.MaxInt64, sign.SHA256([]byte("data"))},
		{"wrong hash", strings.NewReader("data"), 4, sign.SHA256([]byte("wrong"))},
		{"read failure", io.MultiReader(strings.NewReader("da"), failedReader{}), 4, sign.SHA256([]byte("data"))},
		{"error after expected bytes", io.MultiReader(strings.NewReader("data"), failedReader{}), 4, sign.SHA256([]byte("data"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "app")
			if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := InstallArtifactWithSize(tc.src, dest, tc.sha, dir, "test", "binary", tc.size); err == nil {
				t.Fatal("invalid download was installed")
			}
			data, err := os.ReadFile(dest)
			if err != nil || string(data) != "old" {
				t.Fatalf("destination changed: %q, %v", data, err)
			}
			assertNoDownloadTemps(t, dest)
		})
	}
}

func TestDownloadStopsReadingOversizedStream(t *testing.T) {
	for _, exact := range []bool{false, true} {
		r := &endlessReader{}
		dir := t.TempDir()
		dest := filepath.Join(dir, "app")
		_, err := installArtifact(r, dest, "ignored", dir, "test", "binary", 1024, exact)
		if err == nil || r.read != 1025 {
			t.Fatalf("err=%v, bytes read=%d", err, r.read)
		}
		assertNoDownloadTemps(t, dest)
	}
}

func TestExactDownloadAndEmptyArtifact(t *testing.T) {
	for _, content := range []string{"data", ""} {
		dir := t.TempDir()
		dest := filepath.Join(dir, "app")
		_, err := InstallArtifactWithSize(strings.NewReader(content), dest, sign.SHA256([]byte(content)), dir, "test", "binary", int64(len(content)))
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(dest)
		if err != nil || string(data) != content {
			t.Fatalf("content=%q, err=%v", data, err)
		}
		assertNoDownloadTemps(t, dest)
	}
}

func TestClosedTemporaryFileRemovedOnBackupOrRenameFailure(t *testing.T) {
	for _, scenario := range []string{"backup directory", "backup copy", "rename"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			dest := filepath.Join(dir, "app")
			stateDir := filepath.Join(dir, "state")
			switch scenario {
			case "backup directory":
				if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(stateDir, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}
			case "backup copy":
				if err := os.Mkdir(dest, 0700); err != nil {
					t.Fatal(err)
				}
			case "rename":
				// The temporary filename is valid, but the destination component
				// exceeds filesystem limits. This fails only at final rename.
				dest = filepath.Join(dir, strings.Repeat("a", 300))
			}
			_, err := InstallArtifactWithSize(strings.NewReader("data"), dest, sign.SHA256([]byte("data")), stateDir, "test", "binary", 4)
			if err == nil {
				t.Fatal("expected filesystem failure")
			}
			if scenario == "rename" {
				var linkErr *os.LinkError
				if !errors.As(err, &linkErr) || linkErr.Op != "rename" {
					t.Fatalf("did not reach rename: %v", err)
				}
			}
			if scenario == "backup directory" {
				data, readErr := os.ReadFile(dest)
				if readErr != nil || string(data) != "old" {
					t.Fatalf("old artifact lost: %q %v", data, readErr)
				}
			}
			paths, walkErr := filepath.Glob(filepath.Join(dir, "itrust-update-*"))
			if walkErr != nil || len(paths) != 0 {
				t.Fatalf("temporary files left: %v %v", paths, walkErr)
			}
			// Include nested directories when looking for leaked temporary files.
			if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if strings.HasPrefix(entry.Name(), "itrust-update-") {
					return errors.New("temporary file remains: " + path)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
