package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

const testSeedKey = "signing:test-repo:ed25519-seed-b64"

type initBackend struct {
	key         []byte
	puts        int
	onPut       func([]byte) error
	responseErr error
}

func (b *initBackend) Exists(context.Context, string) (bool, error) { return b.key != nil, nil }
func (b *initBackend) Get(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.key)), nil
}
func (b *initBackend) Put(_ context.Context, _ string, open func() (io.ReadCloser, error), _ string) error {
	b.puts++
	r, err := open()
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if b.onPut != nil {
		if err := b.onPut(data); err != nil {
			return err
		}
	}
	b.key = data
	return b.responseErr
}

type faultSecretStore struct {
	*secrets.InMemorySecretStore
	failSet, failReadback, wrongReadback string
	failInitialRead                      bool
	writes                               int
}

func newFaultStore() *faultSecretStore {
	return &faultSecretStore{InMemorySecretStore: secrets.NewInMemorySecretStore()}
}
func (s *faultSecretStore) Set(service, key, value string) error {
	s.writes++
	if key == s.failSet {
		return errors.New("keyring write unavailable")
	}
	return s.InMemorySecretStore.Set(service, key, value)
}
func (s *faultSecretStore) Get(service, key string) (string, error) {
	if s.failInitialRead {
		return "", errors.New("keyring read unavailable")
	}
	value, err := s.InMemorySecretStore.Get(service, key)
	if err == nil && key == s.failReadback {
		return "", errors.New("readback unavailable")
	}
	if err == nil && key == s.wrongReadback {
		return "wrong value", nil
	}
	return value, err
}

func initOptions(t *testing.T) repoInitOptions {
	t.Helper()
	return repoInitOptions{ConfigDir: t.TempDir(), Repo: &repo.RepoConfig{
		RepoID: "test-repo", BaseURL: "https://example.invalid", PubkeyPath: "repo/key.pub",
	}, Username: "test-user", Password: "test-password"}
}

func TestRepoInitAbortsBeforePublicationOnKeyringFailure(t *testing.T) {
	for _, scenario := range []string{"initial read", "seed write", "seed readback", "seed mismatch", "username write", "password write", "password readback"} {
		t.Run(scenario, func(t *testing.T) {
			opts, b, s := initOptions(t), &initBackend{}, newFaultStore()
			switch scenario {
			case "initial read":
				s.failInitialRead = true
			case "seed write":
				s.failSet = testSeedKey
			case "seed readback":
				s.failReadback = testSeedKey
			case "seed mismatch":
				s.wrongReadback = testSeedKey
			case "username write":
				s.failSet = "nexus:test-repo:username"
			case "password write":
				s.failSet = "nexus:test-repo:password"
			case "password readback":
				s.failReadback = "nexus:test-repo:password"
			}
			var output bytes.Buffer
			if err := initializeRepo(context.Background(), opts, b, s, &output); err == nil {
				t.Fatal("keyring failure reported success")
			}
			if b.puts != 0 || b.key != nil {
				t.Fatal("public key published without verified secrets")
			}
			path, _ := repo.GetRepoConfigPath(opts.ConfigDir, opts.Repo.RepoID)
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("config written after keyring failure: %v", err)
			}
			if output.Len() != 0 {
				t.Fatal("failure printed success or leaked seed")
			}
		})
	}
}

func TestRepoInitVerifiesSecretsBeforePublishing(t *testing.T) {
	opts, b, s := initOptions(t), &initBackend{}, newFaultStore()
	b.onPut = func(pub []byte) error {
		seed, err := s.Get("itrust-updater", testSeedKey)
		if err != nil {
			return err
		}
		expected, err := sign.SeedToPubKey(seed)
		if err != nil || !bytes.Equal(pub, expected) {
			return fmt.Errorf("published key differs from stored seed")
		}
		for key, want := range map[string]string{"nexus:test-repo:username": opts.Username, "nexus:test-repo:password": opts.Password} {
			got, err := s.Get("itrust-updater", key)
			if err != nil || got != want {
				return fmt.Errorf("credential %s was not preserved", key)
			}
		}
		return nil
	}
	var output bytes.Buffer
	if err := initializeRepo(context.Background(), opts, b, s, &output); err != nil {
		t.Fatal(err)
	}
	rc, err := repo.LoadRepoConfig(opts.ConfigDir, opts.Repo.RepoID)
	if err != nil || rc.PubkeySha256 != sign.SHA256(b.key) {
		t.Fatalf("incorrect saved config: %v, %v", rc, err)
	}
	seed, _ := s.Get("itrust-updater", testSeedKey)
	if strings.Contains(output.String(), seed) {
		t.Fatal("keyring mode exposed seed")
	}
	if !strings.Contains(output.String(), "initialized") {
		t.Fatal("missing success result")
	}
	puts, writes := b.puts, s.writes
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err == nil {
		t.Fatal("reinitialized an existing repository")
	}
	if b.puts != puts || s.writes != writes {
		t.Fatal("reinitialization changed existing secrets or key")
	}
}

func TestRepoInitRetryRetainsSeedAfterLostUploadResponse(t *testing.T) {
	opts, b, s := initOptions(t), &initBackend{responseErr: errors.New("response lost")}, newFaultStore()
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err == nil {
		t.Fatal("expected upload response failure")
	}
	seed, err := s.Get("itrust-updater", testSeedKey)
	if err != nil {
		t.Fatal("seed lost after upload failure")
	}
	b.responseErr = nil
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err != nil {
		t.Fatal(err)
	}
	retried, _ := s.Get("itrust-updater", testSeedKey)
	if seed != retried || b.puts != 1 {
		t.Fatal("retry rotated the key instead of completing configuration")
	}
}

func TestRepoInitRetryRetainsSeedAfterConfigFailure(t *testing.T) {
	opts, b, s := initOptions(t), &initBackend{}, newFaultStore()
	path, _ := repo.GetRepoConfigPath(opts.ConfigDir, opts.Repo.RepoID)
	b.onPut = func([]byte) error { return os.Mkdir(path, 0700) }
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err == nil || !strings.Contains(err.Error(), "failed to save repo config") {
		t.Fatalf("expected config failure, got %v", err)
	}
	seed, err := s.Get("itrust-updater", testSeedKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	b.onPut = nil
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err != nil {
		t.Fatal(err)
	}
	retried, _ := s.Get("itrust-updater", testSeedKey)
	if retried != seed || b.puts != 1 {
		t.Fatal("configuration retry replaced key")
	}
}

func TestRepoInitDoesNotOverwriteDifferentRemoteKey(t *testing.T) {
	opts, b, s := initOptions(t), &initBackend{key: []byte("existing public key")}, newFaultStore()
	if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err == nil {
		t.Fatal("overwrote existing remote key")
	}
	if b.puts != 0 || s.writes != 0 {
		t.Fatal("conflicting remote key caused writes")
	}
}

func TestRepoInitDoesNotReplaceInvalidStoredSeed(t *testing.T) {
	for _, value := range []string{"", "invalid-seed"} {
		t.Run(value, func(t *testing.T) {
			opts, b, s := initOptions(t), &initBackend{}, newFaultStore()
			if err := s.InMemorySecretStore.Set("itrust-updater", testSeedKey, value); err != nil {
				t.Fatal(err)
			}
			if err := initializeRepo(context.Background(), opts, b, s, io.Discard); err == nil {
				t.Fatal("invalid stored seed silently replaced")
			}
			if s.writes != 0 || b.puts != 0 {
				t.Fatal("invalid stored seed caused key replacement")
			}
		})
	}
}

type brokenOutput struct{}

func (brokenOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestRepoInitWithoutKeyringOutputsSeedBeforePublication(t *testing.T) {
	opts, b := initOptions(t), &initBackend{}
	var output bytes.Buffer
	b.onPut = func(pub []byte) error {
		for _, line := range strings.Split(output.String(), "\n") {
			derived, err := sign.SeedToPubKey(line)
			if err == nil && bytes.Equal(derived, pub) {
				return nil
			}
		}
		return fmt.Errorf("seed not output before publishing")
	}
	if err := initializeRepo(context.Background(), opts, b, nil, &output); err != nil {
		t.Fatal(err)
	}
	opts, b = initOptions(t), &initBackend{}
	if err := initializeRepo(context.Background(), opts, b, nil, brokenOutput{}); err == nil {
		t.Fatal("ignored seed output failure")
	}
	if b.puts != 0 {
		t.Fatal("published without delivering seed")
	}
}

func TestRepoImportRejectsTraversalBeforeAnyEffects(t *testing.T) {
	for _, id := range []string{"../apps/victim", "../../victim", `..\apps\victim`, "/tmp/victim", `C:\temp\victim`, "CON", "name.", ""} {
		for _, writeConfig := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/write=%v", id, writeConfig), func(t *testing.T) {
				root := t.TempDir()
				configDir := filepath.Join(root, "config")
				victim := filepath.Join(root, "victim.env")
				if err := os.WriteFile(victim, []byte("unchanged"), 0600); err != nil {
					t.Fatal(err)
				}
				data := []byte("ITRUST_REPO_ID=" + id + "\nITRUST_REPO_SIGNING_ED25519_SEED_B64=test-seed\n")
				s := newFaultStore()
				if err := importRepoConfig(data, configDir, writeConfig, s); err == nil {
					t.Fatal("unsafe repository ID accepted")
				}
				if s.writes != 0 {
					t.Fatal("invalid ID reached keyring")
				}
				if _, err := os.Stat(configDir); !os.IsNotExist(err) {
					t.Fatal("invalid ID created configuration")
				}
				content, _ := os.ReadFile(victim)
				if string(content) != "unchanged" {
					t.Fatal("victim modified")
				}
			})
		}
	}
}

func TestRepoImportReportsConfigAndSecretFailures(t *testing.T) {
	data := []byte("ITRUST_REPO_ID=test-repo\nITRUST_NEXUS_USERNAME=user\n")
	dir := t.TempDir()
	blocked := filepath.Join(dir, "file")
	if err := os.WriteFile(blocked, []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	s := newFaultStore()
	if err := importRepoConfig(data, blocked, true, s); err == nil {
		t.Fatal("config failure reported success")
	}
	if s.writes != 0 {
		t.Fatal("keyring changed after config failure")
	}
	s.failSet = "nexus:test-repo:username"
	if err := importRepoConfig(data, dir, false, s); err == nil {
		t.Fatal("secret failure reported success")
	}
	s.failSet = ""
	if err := importRepoConfig(data, dir, true, s); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.LoadRepoConfig(dir, "test-repo"); err != nil {
		t.Fatal(err)
	}
}
