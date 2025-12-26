package secrets

import "github.com/zalando/go-keyring"

type KeyringSecretStore struct{}

func (k *KeyringSecretStore) Get(service, key string) (string, error) {
	return keyring.Get(service, key)
}

func (k *KeyringSecretStore) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func (k *KeyringSecretStore) Delete(service, key string) error {
	return keyring.Delete(service, key)
}
