package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/install"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/zalando/go-keyring"
)

type StatusCmd struct {
	Profile string `arg:"" help:"Profile name."`
}

func (c *StatusCmd) Run(g *Globals) error {
	return handleStatus(context.Background(), c.Profile, g.NonInteractive, g.UseKeyring)
}

func handleStatus(ctx context.Context, profile string, nonInteractive, useKeyring bool) error {
	logger.Infof("Checking status for profile %s", profile)
	configDir, stateDir := support.GetPaths("", "")
	cfg := support.LoadMergedConfig(configDir, profile, logger)

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID != "" {
		logger.Debugf("Loading repo config for %s", repoID)
		rc, err := repo.LoadRepoConfig(configDir, repoID)
		if err == nil {
			if cfg.Get("ITRUST_BASE_URL", "") == "" {
				cfg["ITRUST_BASE_URL"] = rc.BaseURL
			}
			if cfg.Get("ITRUST_REPO_PUBKEY_SHA256", "") == "" {
				cfg["ITRUST_REPO_PUBKEY_SHA256"] = rc.PubkeySha256
			}
			if cfg.Get("ITRUST_REPO_PUBKEY_PATH", "") == "" {
				cfg["ITRUST_REPO_PUBKEY_PATH"] = rc.PubkeyPath
			}
		}
	}

	baseURL := cfg.Get("ITRUST_BASE_URL", "")
	appId := cfg.Get("ITRUST_APP_ID", "")
	channel := cfg.Get("ITRUST_CHANNEL", "stable")
	expectedPubkeySha := cfg.Get("ITRUST_REPO_PUBKEY_SHA256", "")
	backendType := cfg.Get("ITRUST_BACKEND", "nexus")
	pubkeyPath := cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub")

	st, err := install.LoadState(stateDir, profile)
	if err != nil || st == nil {
		fmt.Printf("Profile %s is not installed or state is missing.\n", profile)
		logger.Infof("Profile %s is not installed or state is missing", profile)
		// We can still try to check remote if config is present
		if baseURL == "" || appId == "" {
			return nil
		}
	} else {
		fmt.Printf("Profile:           %s\n", st.Profile)
		fmt.Printf("App ID:            %s\n", st.AppID)
		fmt.Printf("Channel:           %s\n", st.Channel)
		fmt.Printf("Installed Version: %s\n", st.InstalledVersion)
		fmt.Printf("Installed At:      %s\n", st.InstalledAt.Local().Format(time.RFC3339))
		fmt.Printf("Destination:       %s\n", st.Dest)
	}

	if baseURL == "" || appId == "" || expectedPubkeySha == "" {
		fmt.Println("Latest Version:    unverified (missing configuration for secure check)")
		logger.Debug("Missing configuration for secure check")
		return nil
	}

	username := cfg.Get("ITRUST_NEXUS_USERNAME", "")
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		logger.Debug("Attempting to get credentials from keyring")
		ss := &secrets.KeyringSecretStore{}
		if username == "" {
			username, _ = ss.Get("itrust-updater", "nexus:"+repoID+":username")
		}
		password, _ = ss.Get("itrust-updater", "nexus:"+repoID+":password")
	}

	// Backward compatibility for non-multi-repo keyring
	if password == "" && useKeyring && username != "" {
		logger.Debug("Attempting to get credentials from keyring (fallback)")
		password, _ = keyring.Get("itrust-updater", username)
	}

	if password == "" && !nonInteractive {
		if username == "" {
			fmt.Print("Enter Nexus username: ")
			fmt.Scanln(&username)
		}
		if username != "" {
			var err error
			password, err = support.ReadPassword(fmt.Sprintf("Enter Nexus password for %s: ", username))
			if err != nil {
				fmt.Printf("Latest Version:    unverified (failed to read password: %v)\n", err)
				logger.Errorf("Failed to read password: %v", err)
				return nil
			}
		}
	}

	if password == "" && username != "" && nonInteractive {
		fmt.Println("Latest Version:    unverified (missing credentials)")
		logger.Debug("Missing credentials in non-interactive mode")
		return nil
	}

	var b backend.Backend
	if backendType == "nexus" {
		b = backend.NewNexusBackend(baseURL, username, password)
	} else {
		logger.Errorf("Unsupported backend: %s", backendType)
		return nil
	}

	logger.Infof("Fetching manifest to check for updates")
	m, _, err := support.FetchAndVerifyManifest(ctx, b, appId, channel, "", pubkeyPath, expectedPubkeySha)
	if err != nil {
		fmt.Printf("Latest Version:    unverified (%v)\n", err)
		logger.Errorf("Failed to fetch/verify manifest: %v", err)
		return nil
	}

	fmt.Printf("Latest Version:    %s\n", m.Payload.Latest.Version)
	if st != nil {
		if st.InstalledVersion != m.Payload.Latest.Version {
			fmt.Println("\nUpdate available!")
		} else {
			fmt.Println("\nApplication is up to date.")
		}
	}
	return nil
}
