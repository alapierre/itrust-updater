package cli

import (
	"fmt"

	"github.com/alapierre/itrust-updater/internal/support"
	"github.com/alapierre/itrust-updater/pkg/secrets"
	"github.com/zalando/go-keyring"
)

func loadNexusCredentialsFromKeyring(username, password, repoID string, useKeyring bool) (string, string) {
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

	return username, password
}

func promptForNexusCredentials(username, password string, nonInteractive bool) (string, string, error) {
	if password != "" || nonInteractive {
		return username, password, nil
	}

	if username == "" {
		fmt.Print("Enter Nexus username: ")
		fmt.Scanln(&username)
	}

	if username == "" {
		return username, password, nil
	}

	var err error
	password, err = support.ReadPassword(fmt.Sprintf("Enter Nexus password for %s: ", username))
	if err != nil {
		return username, password, fmt.Errorf("failed to read password: %w", err)
	}

	return username, password, nil
}

func promptForPassword(username, password string, nonInteractive bool) (string, error) {
	if password != "" || nonInteractive {
		return password, nil
	}

	pass, err := support.ReadPassword(fmt.Sprintf("Enter password for %s: ", username))
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	return pass, nil
}
