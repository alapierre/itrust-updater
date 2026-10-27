package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
)

type RepoCmd struct {
	Init   RepoInitCmd   `cmd:"" help:"Initialize a new repository."`
	Config RepoConfigCmd `cmd:"" help:"Show repository configuration."`
	Export RepoExportCmd `cmd:"" help:"Export repository configuration and secrets."`
	Import RepoImportCmd `cmd:"" help:"Import repository configuration and secrets."`
}

type RepoInitCmd struct {
	RepoID        string `required:"" help:"Repository ID."`
	BaseURL       string `required:"" help:"Repository base URL."`
	NexusUser     string `required:"" help:"Nexus username."`
	NexusPassword string `help:"Nexus password (prompted if missing)."`
	PubkeyPath    string `default:"repo/public-keys/ed25519.pub" help:"Path in repository for public key."`
}

func (c *RepoInitCmd) Run(g *Globals) error {
	return handleRepoInit(context.Background(), c.RepoID, c.BaseURL, c.NexusUser, c.NexusPassword, c.PubkeyPath, g.NonInteractive, g.UseKeyring)
}

type RepoConfigCmd struct {
	RepoID string `required:"" help:"Repository ID."`
}

func (c *RepoConfigCmd) Run(g *Globals) error {
	return handleRepoConfig(c.RepoID)
}

type RepoExportCmd struct {
	RepoID       string `required:"" help:"Repository ID."`
	IncludeSeed  bool   `default:"true" help:"Include signing seed in export."`
	IncludeNexus bool   `default:"false" help:"Include Nexus credentials in export."`
	Out          string `help:"Output file path (default: stdout)."`
}

func (c *RepoExportCmd) Run(g *Globals) error {
	return handleRepoExport(c.RepoID, c.IncludeSeed, c.IncludeNexus, c.Out, g.UseKeyring)
}

type RepoImportCmd struct {
	In              string `help:"Input file path (default: stdin)."`
	WriteRepoConfig bool   `default:"true" help:"Write repo configuration file."`
}

func (c *RepoImportCmd) Run(g *Globals) error {
	return handleRepoImport(c.In, c.WriteRepoConfig, g.UseKeyring)
}

func handleRepoInit(ctx context.Context, repoID, baseURL, user, pass, pubkeyPath string, nonInteractive, useKeyring bool) error {
	rc := &repo.RepoConfig{RepoID: repoID, BaseURL: baseURL, PubkeyPath: pubkeyPath}
	if err := rc.Validate(); err != nil {
		return err
	}
	var err error
	pass, err = promptForPassword(user, pass, nonInteractive)
	if err != nil {
		return err
	}
	if pass == "" && !nonInteractive {
		return fmt.Errorf("password is required")
	}
	var store secrets.SecretStore
	if useKeyring {
		store = &secrets.KeyringSecretStore{}
	}
	options := repoInitOptions{ConfigDir: support.GetDefaultConfigDir(), Repo: rc, Username: user, Password: pass}
	return initializeRepo(ctx, options, backend.NewNexusBackend(baseURL, user, pass), store, os.Stdout)
}

func handleRepoConfig(repoID string) error {
	configDir := support.GetDefaultConfigDir()
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		return fmt.Errorf("failed to load repo config for %s: %w", repoID, err)
	}

	fmt.Printf("Repo config snippet for %s:\n", repoID)
	fmt.Println("-------------------------------------------")
	fmt.Print(repo.ToEnvSnippet(rc))
	fmt.Println("-------------------------------------------")
	return nil
}

func handleRepoExport(repoID string, includeSeed, includeNexus bool, outPath string, useKeyring bool) error {
	logger.Infof("Exporting repo config for %s", repoID)
	configDir := support.GetDefaultConfigDir()
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		return fmt.Errorf("failed to load repo config: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(repo.ToEnvSnippet(rc))

	if useKeyring {
		ss := &secrets.KeyringSecretStore{}
		if includeSeed {
			seed, err := ss.Get("itrust-updater", "signing:"+repoID+":ed25519-seed-b64")
			if err == nil {
				sb.WriteString(fmt.Sprintf("ITRUST_REPO_SIGNING_ED25519_SEED_B64=%s\n", seed))
			} else {
				logger.Warnf("Could not find seed in keyring for %s", repoID)
			}
		}
		if includeNexus {
			user, _ := ss.Get("itrust-updater", "nexus:"+repoID+":username")
			pass, _ := ss.Get("itrust-updater", "nexus:"+repoID+":password")
			if user != "" {
				sb.WriteString(fmt.Sprintf("ITRUST_NEXUS_USERNAME=%s\n", user))
			}
			if pass != "" {
				sb.WriteString(fmt.Sprintf("ITRUST_NEXUS_PASSWORD=%s\n", pass))
			}
		}
	}

	output := sb.String()
	if outPath != "" {
		if err := os.WriteFile(outPath, []byte(output), 0600); err != nil {
			return fmt.Errorf("failed to write export: %w", err)
		}
		fmt.Printf("Repo %s exported to %s\n", repoID, outPath)
	} else {
		fmt.Println("WARNING: Bundle contains secrets!")
		fmt.Println("-------------------------------------------")
		fmt.Print(output)
		fmt.Println("-------------------------------------------")
	}
	return nil
}

func handleRepoImport(inPath string, writeConfig, useKeyring bool) error {
	var data []byte
	var err error
	if inPath != "" {
		logger.Infof("Importing repo config from %s", inPath)
		data, err = os.ReadFile(inPath)
	} else {
		logger.Infof("Importing repo config from stdin")
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	var store secrets.SecretStore
	if useKeyring {
		store = &secrets.KeyringSecretStore{}
	}
	return importRepoConfig(data, support.GetDefaultConfigDir(), writeConfig, store)
}

func importRepoConfig(data []byte, configDir string, writeConfig bool, store secrets.SecretStore) error {
	cfg, err := config.Parse(strings.NewReader(string(data)))
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if err := config.ValidateIdentifier("repository ID", repoID); err != nil {
		return err
	}
	rc := &repo.RepoConfig{
		RepoID:       repoID,
		BaseURL:      cfg.Get("ITRUST_BASE_URL", ""),
		PubkeyPath:   cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub"),
		PubkeySha256: cfg.Get("ITRUST_REPO_PUBKEY_SHA256", ""),
	}
	if err := rc.Validate(); err != nil {
		return err
	}
	if writeConfig {
		if err := repo.SaveRepoConfig(configDir, rc); err != nil {
			return fmt.Errorf("failed to save repo config: %w", err)
		}
		fmt.Printf("Repo config for %s saved.\n", repoID)
	}
	if store == nil {
		fmt.Println("Keyring not enabled, secrets not imported.")
		return nil
	}
	for _, item := range []struct{ name, key, value string }{
		{"signing seed", "signing:" + repoID + ":ed25519-seed-b64", cfg.Get("ITRUST_REPO_SIGNING_ED25519_SEED_B64", "")},
		{"Nexus username", "nexus:" + repoID + ":username", cfg.Get("ITRUST_NEXUS_USERNAME", "")},
		{"Nexus password", "nexus:" + repoID + ":password", cfg.Get("ITRUST_NEXUS_PASSWORD", "")},
	} {
		if item.value != "" {
			if err := saveVerifiedSecret(store, item.name, item.key, item.value); err != nil {
				return err
			}
		}
	}
	fmt.Println("Repository secrets imported to keyring.")
	return nil
}
