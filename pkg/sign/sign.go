package sign

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/alapierre/itrust-updater/pkg/logging"
)

var logger = logging.Component("pkg/sign")

type Hasher struct {
	h hash.Hash
}

func NewHasher() *Hasher {
	return &Hasher{h: sha256.New()}
}

func (h *Hasher) Write(p []byte) (n int, err error) {
	return h.h.Write(p)
}

func (h *Hasher) Sum() string {
	return hex.EncodeToString(h.h.Sum(nil))
}

func FileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func SHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func VerifyFingerprint(pubKey []byte, expectedFingerprint string) error {
	actual := SHA256(pubKey)
	if actual != expectedFingerprint {
		return fmt.Errorf("public key fingerprint mismatch: expected %s, got %s", expectedFingerprint, actual)
	}
	return nil
}

func Sign(payload []byte, seedB64 string) (string, error) {
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode seed: %v", err)
	}
	if len(seed) != ed25519.SeedSize {
		return "", fmt.Errorf("invalid seed size: expected %d, got %d", ed25519.SeedSize, len(seed))
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	sig := ed25519.Sign(privKey, payload)
	return base64.StdEncoding.EncodeToString(sig), nil
}

func Verify(payload []byte, sigB64 string, pubKey []byte) error {
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %v", err)
	}
	if !ed25519.Verify(pubKey, payload, sig) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func SeedToPubKey(seedB64 string) ([]byte, error) {
	seed, err := base64.StdEncoding.DecodeString(seedB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode seed: %v", err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("invalid seed size")
	}
	privKey := ed25519.NewKeyFromSeed(seed)
	return privKey.Public().(ed25519.PublicKey), nil
}
