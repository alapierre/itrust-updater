package cli

import (
	"fmt"

	"github.com/alapierre/itrust-updater/version"
)

type VersionCmd struct {
}

func (c *VersionCmd) Run(g *Globals) error {
	handleVersion()
	return nil
}

func handleVersion() {
	fmt.Printf("itrust-updater\n")
	fmt.Printf("Copyrights ITrust sp. z o.o.\n")
	fmt.Printf("Version: %s\n", version.Version)
}
