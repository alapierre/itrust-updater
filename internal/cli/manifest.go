package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
	"github.com/zalando/go-keyring"
)

type ManifestCmd struct {
	Verify ManifestVerifyCmd `cmd:"" help:"Verify a manifest file."`
	Sign   ManifestSignCmd   `cmd:"" help:"Sign a payload."`
}

type ManifestVerifyCmd struct {
	File             string `required:"" help:"Manifest file to verify."`
	RepoPubkey       string `required:"" help:"Path to repository public key."`
	RepoPubkeySha256 string `name:"repo-pubkey-sha256" help:"Expected SHA256 of public key."`
}

func (c *ManifestVerifyCmd) Run(g *Globals) error {
	return handleManifestVerify(c.File, c.RepoPubkey, c.RepoPubkeySha256)
}

type ManifestSignCmd struct {
	Payload string `required:"" help:"Payload JSON file."`
	Out     string `required:"" help:"Output signed manifest file."`
	KeyID   string `required:"" help:"Key ID for the signature."`
}

func (c *ManifestSignCmd) Run(g *Globals) error {
	return handleManifestSign(c.Payload, c.Out, c.KeyID, g.UseKeyring)
}

func handleManifestVerify(filePath, pubKeyPath, expectedSha string) error {
	logger.Infof("Verifying manifest: %s", filePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	pubKey, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	if expectedSha != "" {
		logger.Debugf("Verifying public key fingerprint against %s", expectedSha)
		if err := sign.VerifyFingerprint(pubKey, expectedSha); err != nil {
			return fmt.Errorf("public key verification failed: %w", err)
		}
	}

	if err := m.Verify(pubKey); err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}
	fmt.Println("Manifest verified successfully.")
	return nil
}

func handleManifestSign(payloadPath, outPath, keyId string, useKeyring bool) error {
	logger.Infof("Signing payload %s to %s", payloadPath, outPath)
	data, err := os.ReadFile(payloadPath)
	if err != nil {
		return fmt.Errorf("failed to read payload: %w", err)
	}
	var p manifest.Payload
	if err := json.Unmarshal(data, &p); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	seed := os.Getenv("ITRUST_REPO_SIGNING_ED25519_SEED_B64")
	if seed == "" && useKeyring {
		logger.Debug("Attempting to get signing seed from keyring")
		seed, _ = keyring.Get("itrust-updater-sign", p.Repo.ID)
	}
	if seed == "" {
		return fmt.Errorf("signing seed missing (ITRUST_REPO_SIGNING_ED25519_SEED_B64)")
	}

	m, err := manifest.SignManifest(p, seed, keyId)
	if err != nil {
		return fmt.Errorf("signing failed: %w", err)
	}

	mJson, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(outPath, mJson, 0644); err != nil {
		return fmt.Errorf("failed to write signed manifest: %w", err)
	}
	fmt.Printf("Manifest signed and saved to %s\n", outPath)
	return nil
}
