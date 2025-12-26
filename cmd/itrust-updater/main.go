package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/akamensky/argparse"
	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/config"
	"github.com/alapierre/itrust-updater/pkg/install"
	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/repo"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/alapierre/itrust-updater/pkg/sign"
	"github.com/alapierre/itrust-updater/version"
	"github.com/sirupsen/logrus"
	"github.com/zalando/go-keyring"
	"golang.org/x/term"
)

var logger = logging.Component("cmd/itrust-updater")

const (
	defaultConfigDirLinux = ".config/itrust-updater"
	defaultStateDirLinux  = ".local/state/itrust-updater"
	systemConfigDirLinux  = "/etc/itrust-updater"
	systemStateDirLinux   = "/var/lib/itrust-updater"
)

func main() {
	parser := argparse.NewParser("itrust-updater", "Secure application updater for artifacts")

	// Global flags
	verbose := parser.Flag("v", "verbose", &argparse.Options{Help: "Enable verbose logging"})
	nonInteractive := parser.Flag("", "non-interactive", &argparse.Options{Help: "Disable interactive prompts"})
	useKeyring := parser.Flag("", "use-keyring", &argparse.Options{Help: "Use OS keyring for secrets"})

	// Init command
	initCmd := parser.NewCommand("init", "Initialize a new profile")
	initProfile := initCmd.StringPositional(&argparse.Options{Required: true, Help: "Profile name"})
	initBaseURL := initCmd.String("", "base-url", &argparse.Options{Required: true, Help: "Repository base URL"})
	initAppID := initCmd.String("", "app-id", &argparse.Options{Required: true, Help: "Application ID"})
	initChannel := initCmd.String("", "channel", &argparse.Options{Default: "stable", Help: "Update channel"})
	initPubkeySha := initCmd.String("", "repo-pubkey-sha256", &argparse.Options{Required: true, Help: "Expected SHA256 of repository public key"})
	initDest := initCmd.String("", "dest", &argparse.Options{Required: true, Help: "Destination path for artifact"})
	initBackend := initCmd.String("", "backend", &argparse.Options{Default: "nexus", Help: "Repository backend type"})
	initRepoID := initCmd.String("", "repo-id", &argparse.Options{Help: "Repository ID"})
	initUser := initCmd.String("", "nexus-user", &argparse.Options{Help: "Nexus username"})
	initStoreCreds := initCmd.Flag("", "store-credentials", &argparse.Options{Help: "Store credentials in OS keyring"})
	initPass := initCmd.String("", "nexus-password", &argparse.Options{Help: "Nexus password (used with --store-credentials)"})

	// Get command
	getCmd := parser.NewCommand("get", "Install or update an application")
	getProfile := getCmd.StringPositional(&argparse.Options{Required: true, Help: "Profile name"})
	getVersion := getCmd.String("", "version", &argparse.Options{Help: "Specific version to install (v1 supports 'latest' only via channel manifest)"})
	getDest := getCmd.String("", "dest", &argparse.Options{Help: "Override destination path"})
	getOS := getCmd.String("", "os", &argparse.Options{Default: runtime.GOOS, Help: "Override operating system"})
	getArch := getCmd.String("", "arch", &argparse.Options{Default: runtime.GOARCH, Help: "Override architecture"})
	getConfigDir := getCmd.String("", "config-dir", &argparse.Options{Help: "Override configuration directory"})
	getStateDir := getCmd.String("", "state-dir", &argparse.Options{Help: "Override state directory"})
	getForce := getCmd.Flag("", "force", &argparse.Options{Help: "Force download and installation"})

	// Status command
	statusCmd := parser.NewCommand("status", "Show installation status")
	statusProfile := statusCmd.StringPositional(&argparse.Options{Required: true, Help: "Profile name"})

	// Push command
	pushCmd := parser.NewCommand("push", "Publish a new release (publisher mode)")
	pushConfig := pushCmd.String("", "config", &argparse.Options{Default: "./itrust-updater.project.env", Help: "Project configuration file"})
	pushArtifactPath := pushCmd.String("", "artifact-path", &argparse.Options{Help: "Path to the artifact to push"})
	pushRepoID := pushCmd.String("", "repo-id", &argparse.Options{Help: "Repository ID"})
	pushAppID := pushCmd.String("", "app-id", &argparse.Options{Help: "Application ID"})
	pushVersion := pushCmd.String("", "version", &argparse.Options{Help: "Version to push"})
	pushRunHooks := pushCmd.Flag("", "run-hooks", &argparse.Options{Default: true, Help: "Run pre-push hooks"})
	pushForce := pushCmd.Flag("", "force", &argparse.Options{Help: "Allow overwriting an existing release (dangerous)"})

	// Manifest command
	manCmd := parser.NewCommand("manifest", "Manifest utilities")
	manVerify := manCmd.NewCommand("verify", "Verify a manifest file")
	manVerifyFile := manVerify.String("", "file", &argparse.Options{Required: true, Help: "Manifest file to verify"})
	manVerifyPubKey := manVerify.String("", "repo-pubkey", &argparse.Options{Required: true, Help: "Path to repository public key"})
	manVerifyPubKeySha := manVerify.String("", "repo-pubkey-sha256", &argparse.Options{Help: "Expected SHA256 of public key"})

	manSign := manCmd.NewCommand("sign", "Sign a payload")
	manSignPayload := manSign.String("", "payload", &argparse.Options{Required: true, Help: "Payload JSON file"})
	manSignOut := manSign.String("", "out", &argparse.Options{Required: true, Help: "Output signed manifest file"})
	manSignKeyID := manSign.String("", "key-id", &argparse.Options{Required: true, Help: "Key ID for the signature"})

	// Repo command
	repoCmd := parser.NewCommand("repo", "Repository management")
	repoInit := repoCmd.NewCommand("init", "Initialize a new repository")
	repoInitID := repoInit.String("", "repo-id", &argparse.Options{Required: true, Help: "Repository ID"})
	repoInitURL := repoInit.String("", "base-url", &argparse.Options{Required: true, Help: "Repository base URL"})
	repoInitUser := repoInit.String("", "nexus-user", &argparse.Options{Required: true, Help: "Nexus username"})
	repoInitPass := repoInit.String("", "nexus-password", &argparse.Options{Help: "Nexus password (prompted if missing)"})
	repoInitPubKeyPath := repoInit.String("", "pubkey-path", &argparse.Options{Default: "repo/public-keys/ed25519.pub", Help: "Path in repository for public key"})

	repoConfig := repoCmd.NewCommand("config", "Show repository configuration")
	repoConfigID := repoConfig.String("", "repo-id", &argparse.Options{Required: true, Help: "Repository ID"})

	repoExport := repoCmd.NewCommand("export", "Export repository configuration and secrets")
	repoExportID := repoExport.String("", "repo-id", &argparse.Options{Required: true, Help: "Repository ID"})
	repoExportIncludeSeed := repoExport.Flag("", "include-seed", &argparse.Options{Default: true, Help: "Include signing seed in export"})
	repoExportIncludeNexus := repoExport.Flag("", "include-nexus", &argparse.Options{Default: false, Help: "Include Nexus credentials in export"})
	repoExportOut := repoExport.String("", "out", &argparse.Options{Help: "Output file path (default: stdout)"})

	repoImport := repoCmd.NewCommand("import", "Import repository configuration and secrets")
	repoImportIn := repoImport.String("", "in", &argparse.Options{Help: "Input file path (default: stdin)"})
	repoImportWriteConfig := repoImport.Flag("", "write-repo-config", &argparse.Options{Default: true, Help: "Write repo configuration file"})

	// Version command
	versionCmd := parser.NewCommand("version", "Show application version")

	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Print(parser.Usage(err))
		os.Exit(1)
	}

	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})
	if *verbose {
		logrus.SetLevel(logrus.DebugLevel)
	}

	ctx := context.Background()

	switch {
	case versionCmd.Happened():
		handleVersion()
	case initCmd.Happened():
		handleInit(*initProfile, *initBaseURL, *initAppID, *initChannel, *initPubkeySha, *initDest, *initBackend, *initRepoID, *initUser, *initPass, *initStoreCreds, *nonInteractive)
	case getCmd.Happened():
		handleGet(ctx, *getProfile, *getVersion, *getDest, *getOS, *getArch, *getConfigDir, *getStateDir, *getForce, *nonInteractive, *useKeyring)
	case statusCmd.Happened():
		handleStatus(ctx, *statusProfile, *nonInteractive, *useKeyring)
	case pushCmd.Happened():
		handlePush(ctx, *pushConfig, *pushArtifactPath, *pushRepoID, *pushAppID, *pushVersion, *pushRunHooks, *pushForce, *nonInteractive, *useKeyring)
	case manVerify.Happened():
		handleManifestVerify(*manVerifyFile, *manVerifyPubKey, *manVerifyPubKeySha)
	case manSign.Happened():
		handleManifestSign(*manSignPayload, *manSignOut, *manSignKeyID, *useKeyring)
	case repoInit.Happened():
		handleRepoInit(ctx, *repoInitID, *repoInitURL, *repoInitUser, *repoInitPass, *repoInitPubKeyPath, *nonInteractive, *useKeyring)
	case repoConfig.Happened():
		handleRepoConfig(*repoConfigID)
	case repoExport.Happened():
		handleRepoExport(*repoExportID, *repoExportIncludeSeed, *repoExportIncludeNexus, *repoExportOut, *useKeyring)
	case repoImport.Happened():
		handleRepoImport(*repoImportIn, *repoImportWriteConfig, *useKeyring)
	}
}

func handleVersion() {
	fmt.Printf("itrust-updater\n")
	fmt.Printf("Copyrights ITrust sp. z o.o.\n")
	fmt.Printf("Version: %s\n", version.Version)
}

func readPassword(prompt string) (string, error) {
	fmt.Print(prompt)
	bytePassword, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", err
	}
	fmt.Println() // Add a newline after the password entry
	return strings.TrimSpace(string(bytePassword)), nil
}

func getPaths(customConfigDir, customStateDir string) (string, string) {
	var configDir, stateDir string
	if customConfigDir != "" {
		configDir = customConfigDir
	} else {
		configDir = getDefaultConfigDir()
	}
	if customStateDir != "" {
		stateDir = customStateDir
	} else {
		stateDir = getDefaultStateDir()
	}
	return configDir, stateDir
}

func getDefaultConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "itrust-updater")
	}
	if os.Getuid() == 0 {
		return systemConfigDirLinux
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, defaultConfigDirLinux)
}

func getDefaultStateDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "itrust-updater")
	}
	if os.Getuid() == 0 {
		return systemStateDirLinux
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, defaultStateDirLinux)
}

func handleInit(profile, baseURL, appId, channel, pubkeySha, dest, backendType, repoID, user, password string, storeCreds, nonInteractive bool) {
	configDir := getDefaultConfigDir()
	profilePath := filepath.Join(configDir, "apps", profile+".env")
	if err := os.MkdirAll(filepath.Dir(profilePath), 0755); err != nil {
		logger.Fatalf("Failed to create config directory: %v", err)
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
			logger.Fatal("--repo-id is required when using --store-credentials")
		}
		pass := password
		if pass == "" && !nonInteractive {
			var err error
			pass, err = readPassword(fmt.Sprintf("Enter password for %s: ", user))
			if err != nil {
				logger.Fatalf("Failed to read password: %v", err)
			}
		}
		if pass == "" {
			logger.Fatal("Password is required for --store-credentials (provide via --nexus-password or interactive prompt)")
		}

		ss := &secrets.KeyringSecretStore{}
		if err := ss.Set("itrust-updater", "nexus:"+repoID+":username", user); err != nil {
			logger.Fatalf("Failed to store username in keyring: %v", err)
		}
		if err := ss.Set("itrust-updater", "nexus:"+repoID+":password", pass); err != nil {
			logger.Fatalf("Failed to store password in keyring: %v", err)
		}
		fmt.Println("Credentials stored in OS keyring.")
	}

	if err := os.WriteFile(profilePath, []byte(content), 0600); err != nil {
		logger.Fatalf("Failed to write profile: %v", err)
	}
	fmt.Printf("Profile %s initialized at %s\n", profile, profilePath)
}

func handleGet(ctx context.Context, profile, version, destOverride, goos, goarch, customConfigDir, customStateDir string, force, nonInteractive, useKeyring bool) {
	configDir, stateDir := getPaths(customConfigDir, customStateDir)
	cfg := loadMergedConfig(configDir, profile)

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID != "" {
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
		logger.Fatal("Missing required configuration (ITRUST_BASE_URL, ITRUST_APP_ID, ITRUST_REPO_PUBKEY_SHA256, ITRUST_DEST)")
	}

	// Resolve destination if it's a directory
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() {
		name := appId
		if goos == "windows" {
			name += ".exe"
		}
		dest = filepath.Join(dest, name)
	}

	username := cfg.Get("ITRUST_NEXUS_USERNAME", "")
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		ss := &secrets.KeyringSecretStore{}
		if username == "" {
			username, _ = ss.Get("itrust-updater", "nexus:"+repoID+":username")
		}
		password, _ = ss.Get("itrust-updater", "nexus:"+repoID+":password")
	}

	// Backward compatibility for non-multi-repo keyring
	if password == "" && useKeyring && username != "" {
		password, _ = keyring.Get("itrust-updater", username)
	}

	if password == "" && !nonInteractive {
		if username == "" {
			fmt.Print("Enter Nexus username: ")
			fmt.Scanln(&username)
		}
		if username != "" {
			var err error
			password, err = readPassword(fmt.Sprintf("Enter Nexus password for %s: ", username))
			if err != nil {
				logger.Fatalf("Failed to read password: %v", err)
			}
		}
	}

	if password == "" && username != "" && nonInteractive {
		logger.Fatal("Nexus password is required but not provided (use ITRUST_NEXUS_PASSWORD or init --store-credentials)")
	}

	if password == "" && username == "" && nonInteractive {
		logger.Debug("No Nexus credentials provided, proceeding without auth")
	}

	var b backend.Backend
	if backendType == "nexus" {
		b = backend.NewNexusBackend(baseURL, username, password)
	} else {
		logger.Fatalf("Unsupported backend: %s", backendType)
	}

	m, _, err := fetchAndVerifyManifest(ctx, b, appId, channel, version, pubkeyPath, expectedPubkeySha)
	if err != nil {
		logger.Fatalf("Failed to fetch/verify manifest: %v", err)
	}

	artifact, err := m.FindArtifact(goos, goarch)
	if err != nil {
		logger.Fatalf("Artifact not found: %v", err)
	}

	// 3. Check state
	st, err := install.LoadState(stateDir, profile)
	if err == nil && st != nil && !force {
		if st.InstalledVersion == m.Payload.Latest.Version && st.InstalledSha256 == artifact.Sha256 {
			if _, err := os.Stat(dest); err == nil {
				fmt.Printf("Application %s is up to date (version %s)\n", appId, st.InstalledVersion)
				return
			}
		}
	}

	// 4. Download and install
	fmt.Printf("Downloading %s version %s...\n", appId, m.Payload.Latest.Version)
	artifactReader, err := b.Get(ctx, artifact.URL)
	if err != nil {
		logger.Fatalf("Failed to download artifact: %v", err)
	}
	defer artifactReader.Close()

	actualSha, err := install.InstallArtifact(artifactReader, dest, artifact.Sha256, stateDir, profile)
	if err != nil {
		logger.Fatalf("Installation failed: %v", err)
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
}

func fetchAndVerifyManifest(ctx context.Context, b backend.Backend, appId, channel, version, pubkeyPath, expectedPubkeySha string) (*manifest.Manifest, []byte, error) {
	// 1. Get and verify pubkey
	pubKeyReader, err := b.Get(ctx, pubkeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get repository public key: %v", err)
	}
	pubKey, err := io.ReadAll(pubKeyReader)
	pubKeyReader.Close()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key: %v", err)
	}

	if err := sign.VerifyFingerprint(pubKey, expectedPubkeySha); err != nil {
		return nil, nil, fmt.Errorf("public key verification failed: %v", err)
	}

	// 2. Get manifest
	manifestPath := fmt.Sprintf("apps/%s/channels/%s.json", appId, channel)
	if version != "" && version != "latest" {
		manifestPath = fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	}

	manifestReader, err := b.Get(ctx, manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get manifest: %v", err)
	}
	var m manifest.Manifest
	if err := json.NewDecoder(manifestReader).Decode(&m); err != nil {
		manifestReader.Close()
		return nil, nil, fmt.Errorf("failed to decode manifest: %v", err)
	}
	manifestReader.Close()

	if err := m.Verify(pubKey); err != nil {
		return nil, nil, fmt.Errorf("manifest signature verification failed: %v", err)
	}

	return &m, pubKey, nil
}

func handleStatus(ctx context.Context, profile string, nonInteractive, useKeyring bool) {
	configDir, stateDir := getPaths("", "")
	cfg := loadMergedConfig(configDir, profile)

	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID != "" {
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
		// We can still try to check remote if config is present
		if baseURL == "" || appId == "" {
			return
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
		return
	}

	username := cfg.Get("ITRUST_NEXUS_USERNAME", "")
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		ss := &secrets.KeyringSecretStore{}
		if username == "" {
			username, _ = ss.Get("itrust-updater", "nexus:"+repoID+":username")
		}
		password, _ = ss.Get("itrust-updater", "nexus:"+repoID+":password")
	}

	// Backward compatibility for non-multi-repo keyring
	if password == "" && useKeyring && username != "" {
		password, _ = keyring.Get("itrust-updater", username)
	}

	if password == "" && !nonInteractive {
		if username == "" {
			fmt.Print("Enter Nexus username: ")
			fmt.Scanln(&username)
		}
		if username != "" {
			var err error
			password, err = readPassword(fmt.Sprintf("Enter Nexus password for %s: ", username))
			if err != nil {
				fmt.Printf("Latest Version:    unverified (failed to read password: %v)\n", err)
				return
			}
		}
	}

	if password == "" && username != "" && nonInteractive {
		fmt.Println("Latest Version:    unverified (missing credentials)")
		return
	}

	var b backend.Backend
	if backendType == "nexus" {
		b = backend.NewNexusBackend(baseURL, username, password)
	} else {
		return
	}

	m, _, err := fetchAndVerifyManifest(ctx, b, appId, channel, "", pubkeyPath, expectedPubkeySha)
	if err != nil {
		fmt.Printf("Latest Version:    unverified (%v)\n", err)
		return
	}

	fmt.Printf("Latest Version:    %s\n", m.Payload.Latest.Version)
	if st != nil {
		if st.InstalledVersion != m.Payload.Latest.Version {
			fmt.Println("\nUpdate available!")
		} else {
			fmt.Println("\nApplication is up to date.")
		}
	}
}

func handlePush(ctx context.Context, configPath, artifactPathFlag, repoIDFlag, appIDFlag, versionFlag string, runHooks, force, nonInteractive, useKeyring bool) {
	cfg, err := config.LoadFile(configPath)
	if err != nil {
		logger.Fatalf("Failed to load project config: %v", err)
	}
	cfg.Merge(config.GetEnvConfig())

	// Priority: Flag > ENV/Config
	repoID := repoIDFlag
	if repoID == "" {
		repoID = cfg.Get("ITRUST_REPO_ID", "")
	}

	if repoID != "" {
		configDir := getDefaultConfigDir()
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
		logrus.Fatal("Missing required project configuration (base-url, app-id, version, artifact-path)")
	}

	// Secrety
	username := cfg.Get("ITRUST_NEXUS_USERNAME", os.Getenv("ITRUST_NEXUS_USERNAME"))
	password := os.Getenv("ITRUST_NEXUS_PASSWORD")

	if password == "" && useKeyring && repoID != "" {
		ss := &secrets.KeyringSecretStore{}
		if username == "" {
			username, _ = ss.Get("itrust-updater", "nexus:"+repoID+":username")
		}
		password, _ = ss.Get("itrust-updater", "nexus:"+repoID+":password")
	}

	if password == "" && useKeyring && username != "" {
		password, _ = keyring.Get("itrust-updater", username)
	}
	if password == "" && !nonInteractive {
		var err error
		password, err = readPassword(fmt.Sprintf("Enter password for %s: ", username))
		if err != nil {
			logger.Fatalf("Failed to read password: %v", err)
		}
	}

	seed := cfg.Get("ITRUST_REPO_SIGNING_ED25519_SEED_B64", os.Getenv("ITRUST_REPO_SIGNING_ED25519_SEED_B64"))
	if seed == "" && useKeyring && repoID != "" {
		ss := &secrets.KeyringSecretStore{}
		seed, _ = ss.Get("itrust-updater", "signing:"+repoID+":ed25519-seed-b64")
	}
	if seed == "" && useKeyring {
		seed, _ = keyring.Get("itrust-updater-sign", repoID)
	}
	if seed == "" {
		logger.Fatal("Repository signing seed missing (ITRUST_REPO_SIGNING_ED25519_SEED_B64)")
	}

	// Hook
	hook := cfg.Get("ITRUST_PREPUSH_HOOK", "")
	if hook != "" && runHooks {
		fmt.Printf("Running pre-push hook: %s\n", hook)
		cmdParts := strings.Fields(hook)
		cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
		cmd.Env = append(os.Environ(), "ITRUST_ARTIFACT_PATH="+artifactPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			logger.Fatalf("Pre-push hook failed: %v", err)
		}
	}

	// 1. Calculate artifact metadata
	sha256, err := sign.FileSHA256(artifactPath)
	if err != nil {
		logger.Fatalf("Failed to calculate artifact SHA256: %v", err)
	}
	fi, err := os.Stat(artifactPath)
	if err != nil {
		logger.Fatalf("Failed to stat artifact: %v", err)
	}

	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	remoteArtifactPath := fmt.Sprintf("apps/%s/releases/v%s/%s/%s/%s_%s_%s_%s%s", appId, version, goos, goarch, appId, version, goos, goarch, ext)

	var b backend.Backend
	if backendType == "nexus" {
		b = backend.NewNexusBackend(baseURL, username, password)
	}

	// 1.5 Check if release already exists
	versionManifestPath := fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	exists, err := b.Exists(ctx, versionManifestPath)
	if err != nil {
		logger.Fatalf("Failed to check if release exists: %v", err)
	}
	if exists && !force {
		logger.Fatalf("Release v%s already exists. Use --force to overwrite.", version)
	}

	// 2. Upload artifact
	fmt.Printf("Uploading artifact to %s...\n", remoteArtifactPath)
	openArtifact := func() (io.ReadCloser, error) {
		return os.Open(artifactPath)
	}
	if err := b.Put(ctx, remoteArtifactPath, openArtifact, "application/octet-stream"); err != nil {
		logger.Fatalf("Upload failed: %v", err)
	}

	// 3. Upload SHA256
	openSha := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(sha256)), nil
	}
	if err := b.Put(ctx, remoteArtifactPath+".sha256", openSha, "text/plain"); err != nil {
		logger.Errorf("Failed to upload SHA256: %v", err)
	}

	// 4. Create and upload artifacts.json (version manifest)
	art := manifest.Artifact{
		OS:     goos,
		Arch:   goarch,
		Type:   "binary",
		URL:    remoteArtifactPath,
		Size:   fi.Size(),
		Sha256: sha256,
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
			Artifacts:   []manifest.Artifact{art},
		},
	}

	m, err := manifest.SignManifest(payload, seed, "repo-key-"+time.Now().Format("2006-01"))
	if err != nil {
		logger.Fatalf("Failed to sign manifest: %v", err)
	}

	mJson, _ := json.MarshalIndent(m, "", "  ")
	openManifest := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(mJson))), nil
	}

	versionManifestPath = fmt.Sprintf("apps/%s/releases/v%s/artifacts.json", appId, version)
	if err := b.Put(ctx, versionManifestPath, openManifest, "application/json"); err != nil {
		logger.Fatalf("Failed to upload version manifest: %v", err)
	}

	// 5. Update channel manifest
	channelManifestPath := fmt.Sprintf("apps/%s/channels/%s.json", appId, channel)
	if err := b.Put(ctx, channelManifestPath, openManifest, "application/json"); err != nil {
		logger.Fatalf("Failed to upload channel manifest: %v", err)
	}

	fmt.Println("Push successful!")
}

func handleManifestVerify(filePath, pubKeyPath, expectedSha string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		logger.Fatalf("Failed to read manifest: %v", err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		logger.Fatalf("Failed to parse manifest: %v", err)
	}

	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		logger.Fatalf("Failed to read public key: %v", err)
	}

	if expectedSha != "" {
		if err := sign.VerifyFingerprint(pubKey, expectedSha); err != nil {
			logger.Fatalf("Public key verification failed: %v", err)
		}
	}

	if err := m.Verify(pubKey); err != nil {
		logger.Fatalf("Verification failed: %v", err)
	}
	fmt.Println("Manifest verified successfully.")
}

func handleManifestSign(payloadPath, outPath, keyId string, useKeyring bool) {
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		logger.Fatalf("Failed to read payload: %v", err)
	}
	var p manifest.Payload
	if err := json.Unmarshal(data, &p); err != nil {
		logger.Fatalf("Failed to parse payload: %v", err)
	}

	seed := os.Getenv("ITRUST_REPO_SIGNING_ED25519_SEED_B64")
	if seed == "" && useKeyring {
		seed, _ = keyring.Get("itrust-updater-sign", p.Repo.ID)
	}
	if seed == "" {
		logger.Fatal("Signing seed missing (ITRUST_REPO_SIGNING_ED25519_SEED_B64)")
	}

	m, err := manifest.SignManifest(p, seed, keyId)
	if err != nil {
		logger.Fatalf("Signing failed: %v", err)
	}

	mJson, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(outPath, mJson, 0644); err != nil {
		logger.Fatalf("Failed to write signed manifest: %v", err)
	}
	fmt.Printf("Manifest signed and saved to %s\n", outPath)
}

func handleRepoInit(ctx context.Context, repoID, baseURL, user, pass, pubkeyPath string, nonInteractive, useKeyring bool) {
	if pass == "" && !nonInteractive {
		var err error
		pass, err = readPassword(fmt.Sprintf("Enter password for %s: ", user))
		if err != nil {
			logger.Fatalf("Failed to read password: %v", err)
		}
	}
	if pass == "" && !nonInteractive {
		logger.Fatal("Password is required")
	}

	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		logger.Fatalf("Failed to generate seed: %v", err)
	}
	seedB64 := base64.StdEncoding.EncodeToString(seed)
	pubKey, err := sign.SeedToPubKey(seedB64)
	if err != nil {
		logger.Fatalf("Failed to derive public key: %v", err)
	}
	pubKeySha := sign.SHA256(pubKey)

	b := backend.NewNexusBackend(baseURL, user, pass)
	openPubKey := func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(pubKey))), nil
	}
	if err := b.Put(ctx, pubkeyPath, openPubKey, "application/octet-stream"); err != nil {
		logger.Fatalf("Failed to upload public key: %v", err)
	}

	rc := &repo.RepoConfig{
		RepoID:       repoID,
		BaseURL:      baseURL,
		PubkeyPath:   pubkeyPath,
		PubkeySha256: pubKeySha,
	}

	configDir := getDefaultConfigDir()
	if err := repo.SaveRepoConfig(configDir, rc); err != nil {
		logger.Fatalf("Failed to save repo config: %v", err)
	}

	if useKeyring {
		ss := &secrets.KeyringSecretStore{}
		_ = ss.Set("itrust-updater", "nexus:"+repoID+":username", user)
		_ = ss.Set("itrust-updater", "nexus:"+repoID+":password", pass)
		_ = ss.Set("itrust-updater", "signing:"+repoID+":ed25519-seed-b64", seedB64)
		fmt.Println("Secrets stored in keyring.")
	} else {
		fmt.Printf("\nIMPORTANT: Store this signing seed securely (it will NOT be saved to disk):\n%s\n", seedB64)
	}

	fmt.Printf("\nRepository %s initialized.\n", repoID)
	fmt.Println("\nClient config snippet (use for profile init):")
	fmt.Println("-------------------------------------------")
	fmt.Printf("ITRUST_REPO_ID=%s\n", repoID)
	fmt.Printf("ITRUST_BASE_URL=%s\n", baseURL)
	fmt.Printf("ITRUST_REPO_PUBKEY_SHA256=%s\n", pubKeySha)
	fmt.Printf("ITRUST_REPO_PUBKEY_PATH=%s\n", pubkeyPath)
	fmt.Println("-------------------------------------------")
}

func handleRepoConfig(repoID string) {
	configDir := getDefaultConfigDir()
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		logger.Fatalf("Failed to load repo config for %s: %v", repoID, err)
	}

	fmt.Printf("Repo config snippet for %s:\n", repoID)
	fmt.Println("-------------------------------------------")
	fmt.Print(repo.ToEnvSnippet(rc))
	fmt.Println("-------------------------------------------")
}

func handleRepoExport(repoID string, includeSeed, includeNexus bool, outPath string, useKeyring bool) {
	configDir := getDefaultConfigDir()
	rc, err := repo.LoadRepoConfig(configDir, repoID)
	if err != nil {
		logger.Fatalf("Failed to load repo config: %v", err)
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
			logger.Fatalf("Failed to write export: %v", err)
		}
		fmt.Printf("Repo %s exported to %s\n", repoID, outPath)
	} else {
		fmt.Println("WARNING: Bundle contains secrets!")
		fmt.Println("-------------------------------------------")
		fmt.Print(output)
		fmt.Println("-------------------------------------------")
	}
}

func handleRepoImport(inPath string, writeConfig, useKeyring bool) {
	var data []byte
	var err error
	if inPath != "" {
		data, err = os.ReadFile(inPath)
	} else {
		data, err = io.ReadAll(os.Stdin)
	}
	if err != nil {
		logger.Fatalf("Failed to read input: %v", err)
	}

	cfg, _ := config.Parse(strings.NewReader(string(data)))
	repoID := cfg.Get("ITRUST_REPO_ID", "")
	if repoID == "" {
		logger.Fatal("Input does not contain ITRUST_REPO_ID")
	}

	if writeConfig {
		rc := &repo.RepoConfig{
			RepoID:       repoID,
			BaseURL:      cfg.Get("ITRUST_BASE_URL", ""),
			PubkeyPath:   cfg.Get("ITRUST_REPO_PUBKEY_PATH", "repo/public-keys/ed25519.pub"),
			PubkeySha256: cfg.Get("ITRUST_REPO_PUBKEY_SHA256", ""),
		}
		configDir := getDefaultConfigDir()
		if err := repo.SaveRepoConfig(configDir, rc); err != nil {
			logger.Errorf("Failed to save repo config: %v", err)
		} else {
			fmt.Printf("Repo config for %s saved.\n", repoID)
		}
	}

	if useKeyring {
		ss := &secrets.KeyringSecretStore{}
		seed := cfg.Get("ITRUST_REPO_SIGNING_ED25519_SEED_B64", "")
		if seed != "" {
			_ = ss.Set("itrust-updater", "signing:"+repoID+":ed25519-seed-b64", seed)
			fmt.Println("Signing seed imported to keyring.")
		}
		user := cfg.Get("ITRUST_NEXUS_USERNAME", "")
		pass := cfg.Get("ITRUST_NEXUS_PASSWORD", "")
		if user != "" {
			_ = ss.Set("itrust-updater", "nexus:"+repoID+":username", user)
			if pass != "" {
				_ = ss.Set("itrust-updater", "nexus:"+repoID+":password", pass)
			}
			fmt.Println("Nexus credentials imported to keyring.")
		}
	} else {
		fmt.Println("Keyring not enabled, secrets not imported.")
	}
}

func loadMergedConfig(configDir, profile string) config.Config {
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
