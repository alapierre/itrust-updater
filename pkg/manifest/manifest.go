package manifest

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/alapierre/itrust-updater/pkg/jcs"
	"github.com/alapierre/itrust-updater/pkg/logging"
	"github.com/alapierre/itrust-updater/pkg/sign"
)

var logger = logging.Component("pkg/manifest")

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

func (m *Manifest) FindArtifact(os, arch string) (*Artifact, error) {
	for _, a := range m.Payload.Latest.Artifacts {
		if a.OS == os && a.Arch == arch {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("no artifact found for %s/%s", os, arch)
}
