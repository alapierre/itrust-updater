package repo

import (
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
	path := GetRepoConfigPath(configDir, repoID)
	cfg, err := config.LoadFile(path)
	if err != nil {
		return nil, err
	}

	return &RepoConfig{
		RepoID:       cfg.Get("ITRUST_REPO_ID", repoID),
		BaseURL:      cfg.Get("ITRUST_BASE_URL", ""),
		PubkeyPath:   cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub"),
		PubkeySha256: cfg.Get("ITRUST_REPO_PUBKEY_SHA256", ""),
	}, nil
}

func SaveRepoConfig(configDir string, rc *RepoConfig) error {
	path := GetRepoConfigPath(configDir, rc.RepoID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	content := fmt.Sprintf("ITRUST_REPO_ID=%s\nITRUST_BASE_URL=%s\nITRUST_REPO_PUBKEY_PATH=%s\nITRUST_REPO_PUBKEY_SHA256=%s\n",
		rc.RepoID, rc.BaseURL, rc.PubkeyPath, rc.PubkeySha256)

	return os.WriteFile(path, []byte(content), 0600)
}

func GetRepoConfigPath(configDir, repoID string) string {
	return filepath.Join(configDir, "repos", repoID+".env")
}

func ToEnvSnippet(rc *RepoConfig) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_ID=%s\n", rc.RepoID))
	sb.WriteString(fmt.Sprintf("ITRUST_BASE_URL=%s\n", rc.BaseURL))
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_PUBKEY_PATH=%s\n", rc.PubkeyPath))
	sb.WriteString(fmt.Sprintf("ITRUST_REPO_PUBKEY_SHA256=%s\n", rc.PubkeySha256))
	return sb.String()
}
