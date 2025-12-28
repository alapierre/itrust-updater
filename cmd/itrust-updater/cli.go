package main

import (
	"runtime"
)

type Globals struct {
	Verbose        bool `help:"Enable verbose logging." short:"v"`
	NonInteractive bool `help:"Disable interactive prompts."`
	UseKeyring     bool `help:"Use OS keyring for secrets."`
}

type CLI struct {
	Globals

	Init     InitCmd     `cmd:"" help:"Initialize a new profile."`
	Get      GetCmd      `cmd:"" help:"Install or update an application."`
	Status   StatusCmd   `cmd:"" help:"Show installation status."`
	Push     PushCmd     `cmd:"" help:"Publish a new release (publisher mode)."`
	Manifest ManifestCmd `cmd:"" help:"Manifest utilities."`
	Repo     RepoCmd     `cmd:"" help:"Repository management."`
	Version  VersionCmd  `cmd:"" help:"Show application version."`
}

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

type StatusCmd struct {
	Profile string `arg:"" help:"Profile name."`
}

type PushCmd struct {
	Config       string `default:"./itrust-updater.project.env" help:"Project configuration file."`
	ArtifactPath string `help:"Path to the artifact to push."`
	RepoID       string `help:"Repository ID."`
	AppID        string `help:"Application ID."`
	Version      string `help:"Version to push."`
	RunHooks     bool   `default:"true" help:"Run pre-push hooks."`
	Force        bool   `help:"Allow overwriting an existing release (dangerous)."`
}

type ManifestCmd struct {
	Verify ManifestVerifyCmd `cmd:"" help:"Verify a manifest file."`
	Sign   ManifestSignCmd   `cmd:"" help:"Sign a payload."`
}

type ManifestVerifyCmd struct {
	File             string `required:"" help:"Manifest file to verify."`
	RepoPubkey       string `required:"" help:"Path to repository public key."`
	RepoPubkeySha256 string `name:"repo-pubkey-sha256" help:"Expected SHA256 of public key."`
}

type ManifestSignCmd struct {
	Payload string `required:"" help:"Payload JSON file."`
	Out     string `required:"" help:"Output signed manifest file."`
	KeyID   string `required:"" help:"Key ID for the signature."`
}

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

type RepoConfigCmd struct {
	RepoID string `required:"" help:"Repository ID."`
}

type RepoExportCmd struct {
	RepoID       string `required:"" help:"Repository ID."`
	IncludeSeed  bool   `default:"true" help:"Include signing seed in export."`
	IncludeNexus bool   `default:"false" help:"Include Nexus credentials in export."`
	Out          string `help:"Output file path (default: stdout)."`
}

type RepoImportCmd struct {
	In              string `help:"Input file path (default: stdin)."`
	WriteRepoConfig bool   `default:"true" help:"Write repo configuration file."`
}

type VersionCmd struct {
}

func GetDefaultOs() string {
	return runtime.GOOS
}

func GetDefaultArch() string {
	return runtime.GOARCH
}
