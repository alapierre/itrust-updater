package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alecthomas/kong"
)

var logger = logging.Component("internal/cli")

type Globals struct {
	Verbose        bool   `help:"Enable verbose logging." short:"v"`
	NonInteractive bool   `help:"Disable interactive prompts."`
	UseKeyring     bool   `help:"Use OS keyring for secrets."`
	LogToFile      bool   `help:"Enable logging to file." env:"ITRUST_LOG_TO_FILE"`
	LogFilePath    string `help:"Override default log file path." env:"ITRUST_LOG_FILE"`
}

type CLI struct {
	Globals `embed:""`

	Init     InitCmd     `cmd:"" help:"Initialize a new profile."`
	Get      GetCmd      `cmd:"" help:"Install or update an application."`
	Status   StatusCmd   `cmd:"" help:"Show installation status."`
	Push     PushCmd     `cmd:"" help:"Publish a new release (publisher mode)."`
	Manifest ManifestCmd `cmd:"" help:"Manifest utilities."`
	Repo     RepoCmd     `cmd:"" help:"Repository management."`
	Version  VersionCmd  `cmd:"" help:"Show application version."`
}

func Main() {
	var cli CLI
	kctx := kong.Parse(&cli,
		kong.Name("itrust-updater"),
		kong.Description("Secure application updater for artifacts"),
		kong.UsageOnError(),
		kong.Vars{
			"default_os":   runtime.GOOS,
			"default_arch": runtime.GOARCH,
		},
	)

	logPath := cli.Globals.LogFilePath
	if (cli.Globals.LogToFile || os.Getenv("ITRUST_LOG_TO_FILE") == "true") && logPath == "" {
		logPath = filepath.Join(logging.GetDefaultLogDir(), "itrust-updater.log")
	}

	logging.SetupLogging(cli.Globals.Verbose, logPath)

	err := kctx.Run(&cli.Globals)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
