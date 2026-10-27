package repo

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/logging"
)

var logger = logging.Component("pkg/repo")

type RepoConfig struct {
	RepoID       string
	BaseURL      string
	PubkeyPath   string
	PubkeySha256 string
}

func LoadRepoConfig(configDir, repoID string) (*RepoConfig, error) {
	if err := config.ValidateIdentifier("repository ID", repoID); err != nil {
		return nil, err
	}
	cfg := make(config.Config)
	root, err := openRepoRoot(configDir, false)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		defer root.Close()
		f, err := root.Open(repoID + ".env")
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err == nil {
			defer f.Close()
			cfg, err = config.Parse(f)
			if err != nil {
				return nil, err
			}
		}
	}
	rc := &RepoConfig{
		RepoID:       cfg.Get("ITRUST_REPO_ID", repoID),
		BaseURL:      cfg.Get("ITRUST_BASE_URL", ""),
		PubkeyPath:   cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub"),
		PubkeySha256: cfg.Get("ITRUST_REPO_PUBKEY_SHA256", ""),
	}
	if err := rc.Validate(); err != nil {
		return nil, err
	}
	if rc.RepoID != repoID {
		return nil, fmt.Errorf("repository config ID %q does not match requested ID %q", rc.RepoID, repoID)
	}
	return rc, nil
}

func SaveRepoConfig(configDir string, rc *RepoConfig) error {
	if err := rc.Validate(); err != nil {
		return err
	}
	root, err := openRepoRoot(configDir, true)
	if err != nil {
		return err
	}
	defer root.Close()
	content := fmt.Sprintf("ITRUST_REPO_ID=%s\nITRUST_BASE_URL=%s\nITRUST_REPO_PUBKEY_PATH=%s\nITRUST_REPO_PUBKEY_SHA256=%s\n",
		rc.RepoID, rc.BaseURL, rc.PubkeyPath, rc.PubkeySha256)
	name := ".repo-" + rand.Text() + ".tmp"
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(name)
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return root.Rename(name, rc.RepoID+".env")
}

func GetRepoConfigPath(configDir, repoID string) (string, error) {
	if err := config.ValidateIdentifier("repository ID", repoID); err != nil {
		return "", err
	}
	return filepath.Join(configDir, "repos", repoID+".env"), nil
}

func (rc *RepoConfig) Validate() error {
	if err := config.ValidateIdentifier("repository ID", rc.RepoID); err != nil {
		return err
	}
	for name, value := range map[string]string{"base URL": rc.BaseURL, "public key path": rc.PubkeyPath, "public key fingerprint": rc.PubkeySha256} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("invalid %s: control characters are not allowed in ENV configuration", name)
		}
	}
	return nil
}

// Anchor reads and writes inside the repositories directory. In particular,
// neither a directory symlink nor a file symlink can redirect writes to profiles.
func openRepoRoot(configDir string, create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return nil, err
		}
	}
	root, err := os.OpenRoot(configDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir("repos", 0700); err != nil && !os.IsExist(err) {
			return nil, err
		}
	}
	info, err := root.Lstat("repos")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repository configuration directory must be a real directory, not a symlink")
	}
	return root.OpenRoot("repos")
}

// LockInitialization prevents two local initializers from replacing each
// other's seed. A lock surviving a killed process requires manual recovery.
func LockInitialization(configDir, repoID string) (func() error, error) {
	if err := config.ValidateIdentifier("repository ID", repoID); err != nil {
		return nil, err
	}
	root, err := openRepoRoot(configDir, true)
	if err != nil {
		return nil, err
	}
	name := repoID + ".init-lock"
	if err := root.Mkdir(name, 0700); err != nil {
		root.Close()
		return nil, fmt.Errorf("cannot lock repository initialization (after a crash, remove %s only when no initialization is running): %w", filepath.Join(configDir, "repos", name), err)
	}
	return func() error {
		defer root.Close()
		return root.Remove(name)
	}, nil
}

func ToEnvSnippet(rc *RepoConfig) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_ID=%s\n", rc.RepoID))
	sb.WriteString(fmt.Sprintf("ITRUST_BASE_URL=%s\n", rc.BaseURL))
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_PUBKEY_PATH=%s\n", rc.PubkeyPath))
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_PUBKEY_SHA256=%s\n", rc.PubkeySha256))
	return sb.String()
}
