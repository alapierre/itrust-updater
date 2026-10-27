package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringStoreDistinguishesMissingSecretFromFailure(t *testing.T) {
	keyring.MockInit()
	t.Cleanup(keyring.MockInit)
	store := &KeyringSecretStore{}
	if _, err := store.Get("test", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing secret not normalized: %v", err)
	}
	if err := store.Set("test", "key", "value"); err != nil {
		t.Fatal(err)
	}
	if value, err := store.Get("test", "key"); err != nil || value != "value" {
		t.Fatalf("roundtrip: %q, %v", value, err)
	}
	unavailable := errors.New("keyring unavailable")
	keyring.MockInitWithError(unavailable)
	if _, err := store.Get("test", "key"); !errors.Is(err, unavailable) || errors.Is(err, ErrNotFound) {
		t.Fatalf("keyring failure mistaken for a missing key: %v", err)
	}
}
