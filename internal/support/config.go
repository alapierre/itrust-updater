package support

import (
	"path/filepath"

	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/sirupsen/logrus"
)

var logger = logging.Component("support")

func LoadMergedConfig(configDir, profile string, logger *logrus.Entry) config.Config {
	envCfg := config.GetEnvConfig()
	repoPath := filepath.Join(configDir, "repo.env")
	repoCfg, _ := config.LoadFile(repoPath)

	profilePath := filepath.Join(configDir, "apps", profile+".env")
	profileCfg, err := config.LoadFile(profilePath)
	if err != nil {
		logger.Warnf("Failed to load profile %s: %v", profile, err)
	}

	return config.MergeConfigs(envCfg, profileCfg, repoCfg)
}

// LoadConfigWithRepoOverlay loads merged config and, if ITRUST_REPO_ID is set,
// overlays repo config values only when corresponding keys are missing.
func LoadConfigWithRepoOverlay(configDir, profile string) config.Config {
	cfg := LoadMergedConfig(configDir, profile, logger)

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID == "" {
		return cfg
	}

	logger.Debugf("Loading repo config for %s", repoID)
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		// Silent fallback keeps current behavior (just skip overlay if repo config fails).
		return cfg
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

	return cfg
}
