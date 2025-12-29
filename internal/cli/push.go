package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/alapierre/itrust-updater/pkg/sign"
	"github.com/zalando/go-keyring"
)

type PushCmd struct {
	Config       string `default:"./itrust-updater.project.env" help:"Project configuration file."`
	ArtifactPath string `help:"Path to the artifact to push."`
	RepoID       string `help:"Repository ID."`
	AppID        string `help:"Application ID."`
	Version      string `help:"Version to push."`
	RunHooks     bool   `default:"true" help:"Run pre-push hooks."`
	Force        bool   `help:"Allow overwriting an existing release (dangerous)."`
}

func (c *PushCmd) Run(g *Globals) error {
	return handlePush(context.Background(), c.Config, c.ArtifactPath, c.RepoID, c.AppID, c.Version, c.RunHooks, c.Force, g.NonInteractive, g.UseKeyring)
}

func handlePush(ctx context.Context, configPath, artifactPathFlag, repoIDFlag, appIDFlag, versionFlag string, runHooks, force, nonInteractive, useKeyring bool) error {
	logger.Infof("Starting push with config: %s", configPath)
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load project config: %w", err)
	}
	cfg.Merge(config.GetEnvConfig())

	// Priority: Flag > ENV/Config
	repoID := repoIDFlag
	if repoID == "" {
		repoID = cfg.Get("ITRUST_REPO_ID", "")
	}

	if repoID != "" {
		configDir := support.GetDefaultConfigDir()
		logger.Debugf("Loading repo config for %s from %s", repoID, configDir)
		rc, err := repo.LoadRepoConfig(configDir, repoID)
		if err == nil {
			if cfg.Get("ITRUST_BASE_URL", "") == "" {
				cfg["ITRUST_BASE_URL"] = rc.BaseURL
			}
		}
	}

	baseURL := cfg.Get("ITRUST_BASE_URL", "")

	appId := appIDFlag
	if appId == "" {
		appId = cfg.Get("ITRUST_APP_ID", "")
	}

	channel := cfg.Get("ITRUST_CHANNEL", "stable")

	version := versionFlag
	if version == "" {
		version = cfg.Get("ITRUST_VERSION", "")
	}

	goos := cfg.Get("ITRUST_OS", runtime.GOOS)
	goarch := cfg.Get("ITRUST_ARCH", runtime.GOARCH)

	// Artifact path priority: CLI flag > ENV/Config
	artifactPath := artifactPathFlag
	if artifactPath == "" {
		artifactPath = cfg.Get("ITRUST_ARTIFACT_PATH", "")
	}

	backendType := cfg.Get("ITRUST_BACKEND", "nexus")
	repoName := cfg.Get("ITRUST_REPO_NAME", "Default Repo")
	appName := cfg.Get("ITRUST_APP_NAME", appId)

	if baseURL == "" || appId == "" || version == "" || artifactPath == "" {
		return fmt.Errorf("missing required project configuration (base-url, app-id, version, artifact-path)")
	}
	logger.Infof("Pushing app %s version %s to %s", appId, version, baseURL)

	// Secrety
	username := cfg.Get("ITRUST_NEXUS_USERNAME", os.Getenv("ITRUST_NEXUS_USERNAME"))
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		logger.Debug("Attempting to get credentials from keyring")
		ss := &secrets.KeyringSecretStore{}
		if username == "" {
			username, _ = ss.Get("itrust-updater", "nexus:"+repoID+":username")
		}
		password, _ = ss.Get("itrust-updater", "nexus:"+repoID+":password")
	}

	if password == "" && useKeyring && username != "" {
		logger.Debug("Attempting to get credentials from keyring (fallback)")
		password, _ = keyring.Get("itrust-updater", username)
	}
	if password == "" && !nonInteractive {
		var err error
		password, err = support.ReadPassword(fmt.Sprintf("Enter password for %s: ", username))
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
	}

	seed := cfg.Get("ITRUST_REPO_SIGNING_ED25519_SEED_B64", os.Getenv("ITRUST_REPO_SIGNING_ED25519_SEED_B64"))
	if seed == "" && useKeyring && repoID != "" {
		logger.Debug("Attempting to get signing seed from keyring")
		ss := &secrets.KeyringSecretStore{}
		seed, _ = ss.Get("itrust-updater", "signing:"+repoID+":ed25519-seed-b64")
	}
	if seed == "" && useKeyring {
		seed, _ = keyring.Get("itrust-updater-sign", repoID)
	}
	if seed == "" {
		return fmt.Errorf("repository signing seed missing (ITRUST_REPO_SIGNING_ED25519_SEED_B64)")
	}

	// Hook
	hook := cfg.Get("ITRUST_PREPUSH_HOOK", "")
	if hook != "" && runHooks {
		fmt.Printf("Running pre-push hook: %s\n", hook)
		logger.Infof("Running pre-push hook: %s", hook)
		cmdParts := strings.Fields(hook)
		cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
		cmd.Env = append(os.Environ(), "ITRUST_ARTIFACT_PATH="+artifactPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("pre-push hook failed: %w", err)
		}
	}

	// 1. Calculate artifact metadata
	logger.Debugf("Calculating metadata for artifact: %s", artifactPath)
	sha256, err := sign.FileSHA256(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to calculate artifact SHA256: %w", err)
	}
	fi, err := os.Stat(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to stat artifact: %w", err)
	}

	// Keep extension exactly as provided by the artifact path.
	// If the file has no extension, do NOT add one.
	ext := strings.ToLower(filepath.Ext(artifactPath))

	artifactType := "binary"
	switch ext {
	case ".jar":
		artifactType = "jar"
		goos = "any"
		goarch = "any"
	case ".zip":
		artifactType = "zip"
	case ".msi":
		artifactType = "msi"
	case ".exe":
		artifactType = "exe"
	}

	remoteArtifactPath := fmt.Sprintf("apps/%s/releases/v%s/%s/%s/%s_%s_%s_%s%s", appId, version, goos, goarch, appId, version, goos, goarch, ext)
	logger.Debugf("Remote artifact path: %s", remoteArtifactPath)

	var b backend.Backend
	if backendType == "nexus" {
		b = backend.NewNexusBackend(baseURL, username, password)
	}

	// 1.5 Check if release already exists
	versionManifestPath := fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	var existingArtifacts []manifest.Artifact
	exists, err := b.Exists(ctx, versionManifestPath)
	if err != nil {
		return fmt.Errorf("failed to check if release exists: %w", err)
	}
	if exists {
		logger.Debugf("Version manifest exists at %s, checking for conflicts", versionManifestPath)
		// Fetch existing manifest to check for OS/Arch conflict
		rc, err := b.Get(ctx, versionManifestPath)
		if err != nil {
			return fmt.Errorf("failed to fetch existing manifest: %w", err)
		}
		var m manifest.Manifest
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			rc.Close()
			return fmt.Errorf("failed to decode existing manifest: %w", err)
		}
		rc.Close()

		existingArtifacts = m.Payload.Latest.Artifacts
		for _, a := range existingArtifacts {
			if a.OS == goos && a.Arch == goarch {
				if !force {
					return fmt.Errorf("artifact for %s/%s in version v%s already exists. Use --force to overwrite", goos, goarch, version)
				}
				logger.Warnf("Overwriting existing artifact for %s/%s", goos, goarch)
				break
			}
		}
	}

	// 2. Upload artifact
	fmt.Printf("Uploading artifact to %s\n", remoteArtifactPath)
	logger.Infof("Uploading artifact to %s", remoteArtifactPath)
	openArtifact := func() (io.ReadCloser, error) {
		return os.Open(artifactPath)
	}
	if err := b.Put(ctx, remoteArtifactPath, openArtifact, "application/octet-stream"); err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}

	// 3. Upload SHA256
	logger.Debug("Uploading SHA256")
	openSha := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(sha256)), nil
	}
	if err := b.Put(ctx, remoteArtifactPath+".sha256", openSha, "text/plain"); err != nil {
		logger.Errorf("Failed to upload SHA256: %v", err)
	}

	// 4. Create and upload artifacts.json (version manifest)
	newArt := manifest.Artifact{
		OS:     goos,
		Arch:   goarch,
		Type:   artifactType,
		URL:    remoteArtifactPath,
		Size:   fi.Size(),
		Sha256: sha256,
	}

	// Merge with existing artifacts
	finalArtifacts := make([]manifest.Artifact, 0)
	found := false
	for _, a := range existingArtifacts {
		if a.OS == goos && a.Arch == goarch {
			finalArtifacts = append(finalArtifacts, newArt)
			found = true
		} else {
			finalArtifacts = append(finalArtifacts, a)
		}
	}
	if !found {
		finalArtifacts = append(finalArtifacts, newArt)
	}

	payload := manifest.Payload{
		SchemaVersion: 1,
		Repo:          manifest.RepoInfo{ID: repoID, Name: repoName},
		App:           manifest.AppInfo{ID: appId, Name: appName},
		Channel:       channel,
		GeneratedAt:   time.Now().UTC(),
		Latest: manifest.Release{
			Version:     version,
			ReleaseDate: time.Now().UTC(),
			Artifacts:   finalArtifacts,
		},
	}

	logger.Infof("Signing and uploading manifest")
	m, err := manifest.SignManifest(payload, seed, "repo-key-"+time.Now().Format("2006-01"))
	if err != nil {
		return fmt.Errorf("failed to sign manifest: %w", err)
	}

	mJson, _ := json.MarshalIndent(m, "", "  ")
	openManifest := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(mJson))), nil
	}

	versionManifestPath = fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	if err := b.Put(ctx, versionManifestPath, openManifest, "application/json"); err != nil {
		return fmt.Errorf("failed to upload version manifest: %w", err)
	}

	// 5. Update channel manifest
	channelManifestPath := fmt.Sprintf("apps/%s/channels/%s.json", appId, channel)
	logger.Infof("Updating channel manifest: %s", channelManifestPath)
	if err := b.Put(ctx, channelManifestPath, openManifest, "application/json"); err != nil {
		return fmt.Errorf("failed to upload channel manifest: %w", err)
	}

	fmt.Println("Push successful!")
	logger.Infof("Push successful for %s version %s", appId, version)
	return nil
}
