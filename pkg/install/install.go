package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

type State struct {
	Profile          string    `json:"profile"`
	AppID            string    `json:"appId"`
	Channel          string    `json:"channel"`
	InstalledVersion string    `json:"installedVersion"`
	InstalledSha256  string    `json:"installedSha256"`
	InstalledAt      time.Time `json:"installedAt"`
	Dest             string    `json:"dest"`
	OS               string    `json:"os"`
	Arch             string    `json:"arch"`
	SourceURL        string    `json:"sourceURL"`
	BackendInfo      string    `json:"backendInfo"`
}

func LoadState(stateDir, profile string) (*State, error) {
	if err := config.ValidateIdentifier("profile", profile); err != nil {
		return nil, err
	}
	path := filepath.Join(stateDir, "state", profile+".json")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var state State
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

func SaveState(stateDir, profile string, state *State) error {
	if err := config.ValidateIdentifier("profile", profile); err != nil {
		return err
	}
	path := filepath.Join(stateDir, "state", profile+".json")
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, profile+".json.*.tmp")
	if err != nil {
		return err
	}
	tempName := tempFile.Name()
	defer func() {
		if tempFile != nil {
			tempFile.Close()
			os.Remove(tempName)
		}
	}()

	encoder := json.NewEncoder(tempFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	// Ensure we don't try to close or remove it in defer if successful
	tempFile = nil

	if err := os.Rename(tempName, path); err != nil {
		os.Remove(tempName) // cleanup on rename failure
		return err
	}

	return nil
}

// DefaultMaxArtifactSize bounds the legacy API, which has no signed size argument.
const DefaultMaxArtifactSize int64 = 8 << 30

// InstallArtifact retains the original API with an 8 GiB safety limit.
// Prefer InstallArtifactWithSize for downloads described by a signed manifest.
func InstallArtifact(src io.Reader, dest string, expectedSha256 string, stateDir, profile string, artifactType string) (string, error) {
	return installArtifact(src, dest, expectedSha256, stateDir, profile, artifactType, DefaultMaxArtifactSize, false)
}

func InstallArtifactWithSize(src io.Reader, dest string, expectedSha256 string, stateDir, profile string, artifactType string, expectedSize int64) (string, error) {
	return installArtifact(src, dest, expectedSha256, stateDir, profile, artifactType, expectedSize, true)
}

func installArtifact(src io.Reader, dest string, expectedSha256 string, stateDir, profile string, artifactType string, size int64, exactSize bool) (string, error) {
	if size < 0 {
		return "", fmt.Errorf("invalid artifact size: %d", size)
	}
	if err := config.ValidateIdentifier("profile", profile); err != nil {
		return "", err
	}
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %v", err)
	}

	tempFile, err := os.CreateTemp(destDir, "itrust-update-*")
	if err != nil {
		return "", err
	}
	tempName := tempFile.Name()
	defer func() {
		if tempFile != nil {
			_ = tempFile.Close()
		}
		// Closing the handle does not finish the transaction: backup or rename
		// can still fail. Always remove the temporary path if it remains.
		_ = os.Remove(tempName)
	}()

	hasher := sign.NewHasher()
	multiWriter := io.MultiWriter(tempFile, hasher)

	written, err := io.Copy(multiWriter, io.LimitReader(src, size))
	if err != nil {
		return "", err
	}
	if exactSize && written != size {
		return "", fmt.Errorf("artifact size mismatch: expected %d bytes, got %d", size, written)
	}
	// Read at most one extra byte to detect oversized streams, without writing
	// data beyond the signed size (or the legacy API limit) to disk.
	var extra [1]byte
	if n, err := io.ReadFull(src, extra[:]); n != 0 {
		return "", fmt.Errorf("artifact exceeds size limit of %d bytes", size)
	} else if err != io.EOF {
		return "", fmt.Errorf("failed to finish artifact download: %w", err)
	}

	actualSha256 := hasher.Sum()
	if actualSha256 != expectedSha256 {
		return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSha256, actualSha256)
	}

	// Finalize permissions before replacement, so a chmod failure leaves the
	// installed artifact intact and the temporary file can still be removed.
	if runtime.GOOS != "windows" && (artifactType == "binary" || artifactType == "exe") {
		if err := tempFile.Chmod(0755); err != nil {
			return "", err
		}
	}
	if err := tempFile.Close(); err != nil {
		return "", err
	}
	tempFile = nil

	// Backup
	if _, err := os.Stat(dest); err == nil {
		backupDir := filepath.Join(stateDir, "backups", profile, time.Now().Format("20060102-150405"))
		if err := os.MkdirAll(backupDir, 0755); err != nil {
			return "", err
		}
		backupPath := filepath.Join(backupDir, filepath.Base(dest))
		if err := CopyFile(dest, backupPath); err != nil {
			return "", fmt.Errorf("failed to backup: %v", err)
		}
	}

	// Atomic replace
	if err := os.Rename(tempName, dest); err != nil {
		return "", err
	}

	return actualSha256, nil
}

func CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
