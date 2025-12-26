package install

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

var logger = logging.Component("pkg/install")

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

func InstallArtifact(src io.Reader, dest string, expectedSha256 string, stateDir, profile string) (string, error) {
	destDir := filepath.Dir(dest)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %v", err)
	}

	tempFile, err := os.CreateTemp(destDir, "itrust-update-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	hasher := sign.NewHasher() // need to add this to pkg/sign
	multiWriter := io.MultiWriter(tempFile, hasher)

	if _, err := io.Copy(multiWriter, src); err != nil {
		return "", err
	}

	actualSha256 := hasher.Sum()
	if actualSha256 != expectedSha256 {
		return "", fmt.Errorf("SHA256 mismatch: expected %s, got %s", expectedSha256, actualSha256)
	}

	if err := tempFile.Close(); err != nil {
		return "", err
	}

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
	if err := os.Rename(tempFile.Name(), dest); err != nil {
		// On windows rename might fail if file is busy
		return "", err
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(dest, 0755); err != nil {
			return "", err
		}
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
	_, err = io.Copy(out, in)
	return err
}
