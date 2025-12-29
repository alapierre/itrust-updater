package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/natefinch/lumberjack"
	"github.com/sirupsen/logrus"
)

func Component(name string) *logrus.Entry {
	return logrus.WithField("component", name)
}

func App(name string) *logrus.Entry {
	return Component(name)
}

// SetupLogging konfiguruje logrus do logowania na stdout LUB do pliku z rotacją.
func SetupLogging(verbose bool, logFile string) {
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	if verbose {
		logrus.SetLevel(logrus.DebugLevel)
	} else {
		logrus.SetLevel(logrus.InfoLevel)
	}

	if logFile != "" {
		// Upewnij się, że katalog na logi istnieje
		logDir := filepath.Dir(logFile)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create log directory %s: %v\n", logDir, err)
		}

		lumberjackLogger := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    10, // megabytes
			MaxBackups: 3,
			MaxAge:     28,   //days
			Compress:   true, // disabled by default
		}
		logrus.SetOutput(lumberjackLogger)
	} else {
		logrus.SetOutput(os.Stdout)
	}
}

// GetDefaultLogDir zwraca domyślny katalog dla logów w zależności od systemu operacyjnego.
func GetDefaultLogDir() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "itrust-updater", "logs")
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library/Logs", "itrust-updater")
	default: // Linux i inne Unixowe
		if os.Getuid() == 0 {
			return "/var/log/itrust-updater"
		}
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".local/state/itrust-updater/logs")
	}
}
