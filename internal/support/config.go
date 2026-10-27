package support

import (
	"fmt"
	"path/filepath"

	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/repo"
)

var logger = logging.Component("support")

func LoadMergedConfig(configDir, profile string) (config.Config, error) {
	if err := config.ValidateIdentifier("profile", profile); err != nil {
		return nil, err
	}
	envCfg := config.GetEnvConfig()
	repoPath := filepath.Join(configDir, "repo.env")
	repoCfg, err := config.LoadFile(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository defaults: %w", err)
	}

	profilePath := filepath.Join(configDir, "apps", profile+".env")
	profileCfg, err := config.LoadFile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load profile %s: %w", profile, err)
	}

	return config.MergeConfigs(envCfg, profileCfg, repoCfg), nil
}

// LoadConfigWithRepoOverlay loads merged config and, if ITRUST_REPO_ID is set,
// overlays repo config values only when corresponding keys are missing.
func LoadConfigWithRepoOverlay(configDir, profile string) (config.Config, error) {
	cfg, err := LoadMergedConfig(configDir, profile)
	if err != nil {
		return nil, err
	}

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID == "" {
		return cfg, nil
	}

	logger.Debugf("Loading repo config for %s", repoID)
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		return nil, fmt.Errorf("failed to load repository config: %w", err)
	}

	// Merge repo config if not already set
	if cfg.Get("ITRUST_BASE_URL", "") == "" {
		cfg["ITRUST_BASE_URL"] = rc.BaseURL
	}
	if cfg.Get("ITRUST_REPO_PUBKEY_SHA256", "") == "" {
		cfg["ITRUST_REPO_PUBKEY_SHA256"] = rc.PubkeySha256
	}
	if cfg.Get("ITRUST_REPO_PUBKEY_PATH", "") == "" {
		cfg["ITRUST_REPO_PUBKEY_PATH"] = rc.PubkeyPath
	}

	return cfg, nil
}
