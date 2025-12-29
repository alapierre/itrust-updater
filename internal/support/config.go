package support

import (
	"path/filepath"

	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/sirupsen/logrus"
)

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
