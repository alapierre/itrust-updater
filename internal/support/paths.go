package support

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	DefaultConfigDirLinux = ".config/itrust-updater"
	DefaultStateDirLinux  = ".local/state/itrust-updater"
	SystemConfigDirLinux  = "/etc/itrust-updater"
	SystemStateDirLinux   = "/var/lib/itrust-updater"
)

func GetPaths(customConfigDir, customStateDir string) (string, string) {
	var configDir, stateDir string
	if customConfigDir != "" {
		configDir = customConfigDir
	} else {
		configDir = GetDefaultConfigDir()
	}
	if customStateDir != "" {
		stateDir = customStateDir
	} else {
		stateDir = GetDefaultStateDir()
	}
	return configDir, stateDir
}

func GetDefaultConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "itrust-updater")
	}
	if os.Getuid() == 0 {
		return SystemConfigDirLinux
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultConfigDirLinux)
}

func GetDefaultStateDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "itrust-updater")
	}
	if os.Getuid() == 0 {
		return SystemStateDirLinux
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, DefaultStateDirLinux)
}
