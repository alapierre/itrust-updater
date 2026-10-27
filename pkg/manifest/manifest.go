package manifest

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/alapierre/itrust-updater/pkg/jcs"
	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

var logger = logging.Component("pkg/manifest")

// MaxManifestSize applies to the entire JSON document, including whitespace.
const MaxManifestSize int64 = 1 << 20

func Read(r io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxManifestSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	if int64(len(data)) > MaxManifestSize {
		return nil, fmt.Errorf("manifest exceeds size limit of %d bytes", MaxManifestSize)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}
	return &m, nil
}

type Signature struct {
	Alg           string    `json:"alg"`
	KeyID         string    `json:"keyId"`
	CreatedAt     time.Time `json:"createdAt"`
	PayloadSha256 string    `json:"payloadSha256"`
	Sig           string    `json:"sig"`
}

type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	Sha256 string `json:"sha256"`
}

type Release struct {
	Version     string     `json:"version"`
	ReleaseDate time.Time  `json:"releaseDate"`
	Notes       string     `json:"notes,omitempty"`
	Artifacts   []Artifact `json:"artifacts"`
}

type Payload struct {
	SchemaVersion int       `json:"schemaVersion"`
	Repo          RepoInfo  `json:"repo"`
	App           AppInfo   `json:"app"`
	Channel       string    `json:"channel"`
	GeneratedAt   time.Time `json:"generatedAt"`
	Latest        Release   `json:"latest"`
}

type RepoInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AppInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Manifest struct {
	Payload   Payload   `json:"payload"`
	Signature Signature `json:"signature"`
}

// Identity binds authenticated metadata to the requested repository and app.
// Version selects a release independently of its original channel. Without a
// version, Channel must match. RepoID is checked when configured by the caller.
type Identity struct {
	RepoID  string
	AppID   string
	Channel string
	Version string
}

type ArtifactsList struct {
	Version   string     `json:"version"`
	Artifacts []Artifact `json:"artifacts"`
}

func SignManifest(payload Payload, seedB64 string, keyID string) (*Manifest, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	canonical, err := jcs.Transform(payloadBytes)
	if err != nil {
		return nil, err
	}
	payloadSha := sign.SHA256(canonical)
	sig, err := sign.Sign(canonical, seedB64)
	if err != nil {
		return nil, err
	}

	return &Manifest{
		Payload: payload,
		Signature: Signature{
			Alg:           "Ed25519",
			KeyID:         keyID,
			CreatedAt:     time.Now().UTC(),
			PayloadSha256: payloadSha,
			Sig:           sig,
		},
	}, nil
}

func (m *Manifest) Verify(pubKey []byte) error {
	payloadBytes, err := json.Marshal(m.Payload)
	if err != nil {
		return err
	}
	canonical, err := jcs.Transform(payloadBytes)
	if err != nil {
		return err
	}
	payloadSha := sign.SHA256(canonical)
	if payloadSha != m.Signature.PayloadSha256 {
		return fmt.Errorf("payload SHA256 mismatch")
	}
	return sign.Verify(canonical, m.Signature.Sig, pubKey)
}

// VerifyFor checks both authenticity and the context in which a manifest is used.
func (m *Manifest) VerifyFor(pubKey []byte, expected Identity) error {
	if err := m.Verify(pubKey); err != nil {
		return fmt.Errorf("manifest signature verification failed: %w", err)
	}
	if m.Signature.Alg != "Ed25519" {
		return fmt.Errorf("unsupported manifest signature algorithm: %q", m.Signature.Alg)
	}
	if m.Payload.SchemaVersion != 1 {
		return fmt.Errorf("unsupported manifest schema version: %d", m.Payload.SchemaVersion)
	}
	if expected.RepoID != "" && m.Payload.Repo.ID != expected.RepoID {
		return fmt.Errorf("manifest repo ID mismatch: expected %q, got %q", expected.RepoID, m.Payload.Repo.ID)
	}
	if expected.AppID == "" || m.Payload.App.ID != expected.AppID {
		return fmt.Errorf("manifest app ID mismatch: expected %q, got %q", expected.AppID, m.Payload.App.ID)
	}
	if m.Payload.Latest.Version == "" {
		return fmt.Errorf("manifest release version is missing")
	}
	if expected.Version != "" {
		if m.Payload.Latest.Version != expected.Version {
			return fmt.Errorf("manifest version mismatch: expected %q, got %q", expected.Version, m.Payload.Latest.Version)
		}
	} else if expected.Channel == "" || m.Payload.Channel != expected.Channel {
		return fmt.Errorf("manifest channel mismatch: expected %q, got %q", expected.Channel, m.Payload.Channel)
	}
	return nil
}

func (m *Manifest) FindArtifact(os, arch string) (*Artifact, error) {
	// First check for multiplatform JAR
	for _, a := range m.Payload.Latest.Artifacts {
		if a.Type == "jar" && a.OS == "any" && a.Arch == "any" {
			return &a, nil
		}
	}

	for _, a := range m.Payload.Latest.Artifacts {
		if a.OS == os && a.Arch == arch {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no artifact found for %s/%s", os, arch)
}
