package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

type repoInitOptions struct {
	ConfigDir string
	Repo      *repo.RepoConfig
	Username  string
	Password  string
}

func initializeRepo(ctx context.Context, options repoInitOptions, b backend.Backend, store secrets.SecretStore, out io.Writer) error {
	rc := *options.Repo
	if err := rc.Validate(); err != nil {
		return err
	}
	unlock, err := repo.LockInitialization(options.ConfigDir, rc.RepoID)
	if err != nil {
		return err
	}
	defer func() {
		if err := unlock(); err != nil {
			logger.Errorf("Failed to release repository initialization lock: %v", err)
		}
	}()
	configPath, err := repo.GetRepoConfigPath(options.ConfigDir, rc.RepoID)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(configPath); err == nil {
		return fmt.Errorf("repository %q is already configured; repo init does not rotate existing keys", rc.RepoID)
	} else if !os.IsNotExist(err) {
		return err
	}

	seedKey := "signing:" + rc.RepoID + ":ed25519-seed-b64"
	var seedB64 string
	if store != nil {
		seedB64, err = store.Get("itrust-updater", seedKey)
		if err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return fmt.Errorf("failed to read signing seed from keyring: %w", err)
		}
		if err == nil && seedB64 == "" {
			return fmt.Errorf("keyring contains an empty signing seed; refusing to replace it")
		}
	}
	newSeed := seedB64 == ""
	if newSeed {
		seed := make([]byte, 32)
		if _, err := rand.Read(seed); err != nil {
			return fmt.Errorf("failed to generate seed: %w", err)
		}
		seedB64 = base64.StdEncoding.EncodeToString(seed)
	}
	pubKey, err := sign.SeedToPubKey(seedB64)
	if err != nil {
		return fmt.Errorf("invalid existing signing seed: %w", err)
	}
	rc.PubkeySha256 = sign.SHA256(pubKey)

	exists, err := b.Exists(ctx, rc.PubkeyPath)
	if err != nil {
		return fmt.Errorf("failed to check repository public key: %w", err)
	}
	if exists {
		r, err := b.Get(ctx, rc.PubkeyPath)
		if err != nil {
			return fmt.Errorf("failed to read existing repository public key: %w", err)
		}
		existing, err := io.ReadAll(io.LimitReader(r, int64(len(pubKey)+1)))
		r.Close()
		if err != nil {
			return err
		}
		if !bytes.Equal(existing, pubKey) {
			return fmt.Errorf("repository already has a different public key; refusing to overwrite it")
		}
	}

	// Preserve the seed even if any later step fails. A retry reuses it, including
	// when an upload succeeded remotely but its response was lost.
	if store != nil {
		if newSeed {
			if err := saveVerifiedSecret(store, "signing seed", seedKey, seedB64); err != nil {
				return err
			}
		}
		for _, item := range []struct{ name, key, value string }{
			{"Nexus username", "nexus:" + rc.RepoID + ":username", options.Username},
			{"Nexus password", "nexus:" + rc.RepoID + ":password", options.Password},
		} {
			if err := saveVerifiedSecret(store, item.name, item.key, item.value); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintf(out, "\nIMPORTANT: Store this signing seed securely (it will NOT be saved to disk):\n%s\n", seedB64); err != nil {
			return fmt.Errorf("failed to output signing seed; public key has not been published: %w", err)
		}
	}
	if !exists {
		openPubKey := func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(pubKey)), nil }
		if err := b.Put(ctx, rc.PubkeyPath, openPubKey, "application/octet-stream"); err != nil {
			return fmt.Errorf("failed to upload public key (signing seed has been preserved): %w", err)
		}
	}
	if err := repo.SaveRepoConfig(options.ConfigDir, &rc); err != nil {
		return fmt.Errorf("public key is published, but failed to save repo config (signing seed has been preserved): %w", err)
	}
	if store != nil {
		if _, err := fmt.Fprintln(out, "Secrets stored and verified in keyring."); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, "\nRepository %s initialized.\n\nClient config snippet:\n%s", rc.RepoID, repo.ToEnvSnippet(&rc))
	return err
}

func saveVerifiedSecret(store secrets.SecretStore, name, key, value string) error {
	if err := store.Set("itrust-updater", key, value); err != nil {
		return fmt.Errorf("failed to store %s in keyring: %w", name, err)
	}
	stored, err := store.Get("itrust-updater", key)
	if err != nil {
		return fmt.Errorf("failed to read back %s from keyring: %w", name, err)
	}
	if stored != value {
		return fmt.Errorf("keyring verification failed for %s: stored value differs", name)
	}
	return nil
}
