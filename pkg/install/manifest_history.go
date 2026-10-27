package install

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alapierre/itrust-updater/pkg/manifest"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

// ManifestHistory is separate from installation state: installing an explicitly
// selected old release must never erase the newest authenticated metadata.
type ManifestHistory struct {
	Records map[string]ManifestRecord `json:"records"`
}

type ManifestRecord struct {
	PubkeySHA256  string    `json:"pubkeySha256"`
	AppID         string    `json:"appId"`
	Channel       string    `json:"channel"`
	GeneratedAt   time.Time `json:"generatedAt"`
	PayloadSHA256 string    `json:"payloadSha256"`
}

func manifestScope(pubkeySHA256, appID, channel string) string {
	data, _ := json.Marshal([]string{pubkeySHA256, appID, channel})
	return sign.SHA256(data)
}

// Accept must only receive a manifest whose signature and identity were verified.
// explicitVersion permits historical releases without moving the checkpoint back.
func (h *ManifestHistory) Accept(m *manifest.Manifest, pubkeySHA256 string, explicitVersion bool) (bool, error) {
	if m.Payload.GeneratedAt.IsZero() {
		return false, fmt.Errorf("manifest is missing signed generatedAt; cannot check replay protection")
	}
	key := manifestScope(pubkeySHA256, m.Payload.App.ID, m.Payload.Channel)
	if previous, ok := h.Records[key]; ok {
		older := m.Payload.GeneratedAt.Before(previous.GeneratedAt)
		conflict := m.Payload.GeneratedAt.Equal(previous.GeneratedAt) && m.Signature.PayloadSha256 != previous.PayloadSHA256
		if older || conflict {
			if explicitVersion {
				return false, nil
			}
			return false, fmt.Errorf("manifest replay rejected: channel %q metadata (%s) is older than or conflicts with the last accepted manifest (%s); use --version <version> only for an intentional installation",
				m.Payload.Channel, m.Payload.GeneratedAt.Format(time.RFC3339Nano), previous.GeneratedAt.Format(time.RFC3339Nano))
		}
		if m.Payload.GeneratedAt.Equal(previous.GeneratedAt) {
			return false, nil
		}
	}
	if h.Records == nil {
		h.Records = make(map[string]ManifestRecord)
	}
	h.Records[key] = ManifestRecord{
		PubkeySHA256: pubkeySHA256, AppID: m.Payload.App.ID, Channel: m.Payload.Channel,
		GeneratedAt: m.Payload.GeneratedAt, PayloadSHA256: m.Signature.PayloadSha256,
	}
	return true, nil
}

func manifestHistoryPath(stateDir, profile string) string {
	return filepath.Join(stateDir, "manifest-history", sign.SHA256([]byte(profile))+".json")
}

// A missing file returns nil; malformed history is an error, never a fresh start.
func LoadManifestHistory(stateDir, profile string) (*ManifestHistory, error) {
	data, err := os.ReadFile(manifestHistoryPath(stateDir, profile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var h ManifestHistory
	if err := json.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("invalid manifest history: %w", err)
	}
	if len(h.Records) == 0 {
		return nil, fmt.Errorf("manifest history has no checkpoints")
	}
	for key, record := range h.Records {
		if record.GeneratedAt.IsZero() || len(record.PayloadSHA256) != 64 || key != manifestScope(record.PubkeySHA256, record.AppID, record.Channel) {
			return nil, fmt.Errorf("invalid manifest history checkpoint")
		}
	}
	return &h, nil
}

// SaveManifestHistory requires the profile lock and runs before downloading or
// replacing an artifact. Failure to persist protection must abort installation.
func SaveManifestHistory(stateDir, profile string, h *ManifestHistory) error {
	path := manifestHistoryPath(stateDir, profile)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := json.NewEncoder(f).Encode(h); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// LockProfile serializes updates across processes, including checkpoint writes
// and artifact replacement. A lock left by a killed process is not auto-removed:
// guessing that it is stale could let an older update overtake a newer one.
func LockProfile(stateDir, profile string) (func() error, error) {
	dir := filepath.Join(stateDir, "locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, sign.SHA256([]byte(profile))+".lock")
	if err := os.Mkdir(path, 0700); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("profile %q is locked; if an updater crashed, remove %s only after confirming no update is running", profile, path)
		}
		return nil, err
	}
	return func() error { return os.Remove(path) }, nil
}
