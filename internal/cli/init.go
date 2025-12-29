package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/secrets"
)

type InitCmd struct {
	Profile          string `arg:"" help:"Profile name."`
	BaseURL          string `required:"" help:"Repository base URL."`
	AppID            string `required:"" help:"Application ID."`
	Channel          string `default:"stable" help:"Update channel."`
	RepoPubkeySha256 string `name:"repo-pubkey-sha256" required:"" help:"Expected SHA256 of repository public key."`
	Dest             string `required:"" help:"Destination path for artifact."`
	Backend          string `default:"nexus" help:"Repository backend type."`
	RepoID           string `help:"Repository ID."`
	NexusUser        string `help:"Nexus username."`
	StoreCredentials bool   `help:"Store credentials in OS keyring."`
	NexusPassword    string `help:"Nexus password (used with --store-credentials)."`
}

func (c *InitCmd) Run(g *Globals) error {
	return handleInit(c.Profile, c.BaseURL, c.AppID, c.Channel, c.RepoPubkeySha256, c.Dest, c.Backend, c.RepoID, c.NexusUser, c.NexusPassword, c.StoreCredentials, g.NonInteractive)
}

func handleInit(profile, baseURL, appId, channel, pubkeySha, dest, backendType, repoID, user, password string, storeCreds, nonInteractive bool) error {
	configDir := support.GetDefaultConfigDir()
	profilePath := filepath.Join(configDir, "apps", profile+".env")
	logger.Infof("Initializing profile %s at %s", profile, profilePath)

	if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	content := fmt.Sprintf("ITRUST_BASE_URL=%s\nITRUST_APP_ID=%s\nITRUST_CHANNEL=%s\nITRUST_REPO_PUBKEY_SHA256=%s\nITRUST_DEST=%s\nITRUST_BACKEND=%s\n",
		baseURL, appId, channel, pubkeySha, dest, backendType)
	if repoID != "" {
		content += fmt.Sprintf("ITRUST_REPO_ID=%s\n", repoID)
	}
	if user != "" {
		content += fmt.Sprintf("ITRUST_NEXUS_USERNAME=%s\n", user)
	}

	if storeCreds {
		if repoID == "" {
			return fmt.Errorf("--repo-id is required when using --store-credentials")
		}
		pass := password
		if pass == "" && !nonInteractive {
			var err error
			pass, err = support.ReadPassword(fmt.Sprintf("Enter password for %s: ", user))
			if err != nil {
				return fmt.Errorf("failed to read password: %w", err)
			}
		}
		if pass == "" {
			return fmt.Errorf("password is required for --store-credentials (provide via --nexus-password or interactive prompt)")
		}

		logger.Debug("Storing credentials in OS keyring")
		ss := &secrets.KeyringSecretStore{}
		if err := ss.Set("itrust-updater", "nexus:"+repoID+":username", user); err != nil {
			return fmt.Errorf("failed to store username in keyring: %w", err)
		}
		if err := ss.Set("itrust-updater", "nexus:"+repoID+":password", pass); err != nil {
			return fmt.Errorf("failed to store password in keyring: %w", err)
		}
		fmt.Println("Credentials stored in OS keyring.")
	}

	if err := os.WriteFile(profilePath, []byte(content), 0600); err != nil {
		return fmt.Errorf("failed to write profile: %w", err)
	}
	fmt.Printf("Profile %s initialized at %s\n", profile, profilePath)
	logger.Infof("Successfully initialized profile %s", profile)
	return nil
}
