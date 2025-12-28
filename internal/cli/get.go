package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/install"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/zalando/go-keyring"
)

type GetCmd struct {
	Profile   string `arg:"" help:"Profile name."`
	Version   string `help:"Specific version to install (v1 supports 'latest' only via channel manifest)."`
	Dest      string `help:"Override destination path."`
	Os        string `default:"${default_os}" help:"Override operating system."`
	Arch      string `default:"${default_arch}" help:"Override architecture."`
	ConfigDir string `help:"Override configuration directory."`
	StateDir  string `help:"Override state directory."`
	Force     bool   `help:"Force download and installation."`
}

func (c *GetCmd) Run(g *Globals) error {
	return handleGet(context.Background(), c.Profile, c.Version, c.Dest, c.Os, c.Arch, c.ConfigDir, c.StateDir, c.Force, g.NonInteractive, g.UseKeyring)
}

func handleGet(ctx context.Context, profile, version, destOverride, goos, goarch, customConfigDir, customStateDir string, force, nonInteractive, useKeyring bool) error {
	configDir, stateDir := support.GetPaths(customConfigDir, customStateDir)
	logger.Infof("Starting get for profile %s, version %s", profile, version)
	logger.Debugf("Config dir: %s, state dir: %s", configDir, stateDir)

	cfg := support.LoadMergedConfig(configDir, profile, logger)

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID != "" {
		logger.Debugf("Loading repo config for %s", repoID)
		rc, err := repo.LoadRepoConfig(configDir, repoID)
		if err == nil {
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
		}
	}

	baseURL := cfg.Get("ITRUST_BASE_URL", "")
	appId := cfg.Get("ITRUST_APP_ID", "")
	channel := cfg.Get("ITRUST_CHANNEL", "stable")
	expectedPubkeySha := cfg.Get("ITRUST_REPO_PUBKEY_SHA256", "")
	dest := cfg.Get("ITRUST_DEST", "")
	backendType := cfg.Get("ITRUST_BACKEND", "nexus")
	pubkeyPath := cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub")

	if destOverride != "" {
		dest = destOverride
	}

	if baseURL == "" || appId == "" || expectedPubkeySha == "" || dest == "" {
		return fmt.Errorf("missing required configuration (ITRUST_BASE_URL, ITRUST_APP_ID, ITRUST_REPO_PUBKEY_SHA256, ITRUST_DEST)")
	}

	username := cfg.Get("ITRUST_NEXUS_USERNAME", "")
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		logger.Debug("Attempting to get credentials from keyring for repo")
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
				return fmt.Errorf("failed to read password: %w", err)
			}
		}
	}

	if password == "" && username != "" && nonInteractive {
		return fmt.Errorf("Nexus password is required but not provided (use ITRUST_NEXUS_PASSWORD or init --store-credentials)")
	}

	if password == "" && username == "" && nonInteractive {
		logger.Debug("No Nexus credentials provided, proceeding without auth")
	}

	var b backend.Backend
	if backendType == "nexus" {
		logger.Debugf("Using Nexus backend at %s", baseURL)
		b = backend.NewNexusBackend(baseURL, username, password)
	} else {
		return fmt.Errorf("unsupported backend: %s", backendType)
	}

	logger.Infof("Fetching manifest for %s (channel: %s, version: %s)", appId, channel, version)
	m, _, err := support.FetchAndVerifyManifest(ctx, b, appId, channel, version, pubkeyPath, expectedPubkeySha)
	if err != nil {
		return fmt.Errorf("failed to fetch/verify manifest: %w", err)
	}

	artifact, err := m.FindArtifact(goos, goarch)
	if err != nil {
		return fmt.Errorf("artifact not found: %w", err)
	}
	logger.Debugf("Found artifact: %s", artifact.URL)

	// Resolve destination if it's a directory
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		name := appId
		ext := filepath.Ext(artifact.URL)
		if ext != "" {
			name += ext
		}
		dest = filepath.Join(dest, name)
	}
	logger.Debugf("Resolved destination path: %s", dest)

	// 3. Check state
	st, err := install.LoadState(stateDir, profile)
	if err == nil && st != nil && !force {
		if st.InstalledVersion == m.Payload.Latest.Version && st.InstalledSha256 == artifact.Sha256 {
			if _, err := os.Stat(dest); err == nil {
				fmt.Printf("Application %s is up to date (version %s)\n", appId, st.InstalledVersion)
				logger.Infof("Application %s is up to date (version %s)", appId, st.InstalledVersion)
				return nil
			}
		}
	}

	// 4. Download and install
	fmt.Printf("Downloading %s version %s...\n", appId, m.Payload.Latest.Version)
	logger.Infof("Downloading %s version %s from %s", appId, m.Payload.Latest.Version, artifact.URL)
	artifactReader, err := b.Get(ctx, artifact.URL)
	if err != nil {
		return fmt.Errorf("failed to download artifact: %w", err)
	}
	defer artifactReader.Close()

	logger.Infof("Installing artifact to %s", dest)
	actualSha, err := install.InstallArtifact(artifactReader, dest, artifact.Sha256, stateDir, profile, artifact.Type)
	if err != nil {
		return fmt.Errorf("installation failed: %w", err)
	}

	// 5. Save state
	newState := &install.State{
		Profile:          profile,
		AppID:            appId,
		Channel:          channel,
		InstalledVersion: m.Payload.Latest.Version,
		InstalledSha256:  actualSha,
		InstalledAt:      time.Now().UTC(),
		Dest:             dest,
		OS:               goos,
		Arch:             goarch,
		SourceURL:        artifact.URL,
		BackendInfo:      backendType,
	}
	if err := install.SaveState(stateDir, profile, newState); err != nil {
		logger.Errorf("Failed to save state: %v", err)
	}

	fmt.Printf("Successfully installed %s version %s to %s\n", appId, m.Payload.Latest.Version, dest)
	logger.Infof("Successfully installed %s version %s to %s", appId, m.Payload.Latest.Version, dest)
	return nil
}
