package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

type KeyringSecretStore struct{}

func (k *KeyringSecretStore) Get(service, key string) (string, error) {
	value, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return value, err
}

func (k *KeyringSecretStore) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func (k *KeyringSecretStore) Delete(service, key string) error {
	return keyring.Delete(service, key)
}
