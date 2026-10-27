package secrets

type InMemorySecretStore struct {
	secrets map[string]string
}

func NewInMemorySecretStore() *InMemorySecretStore {
	return &InMemorySecretStore{
		secrets: make(map[string]string),
	}
}

func (i *InMemorySecretStore) Get(service, key string) (string, error) {
	val, ok := i.secrets[service+":"+key]
	if !ok {
		return "", ErrNotFound
	}
	return val, nil
}

func (i *InMemorySecretStore) Set(service, key, value string) error {
	i.secrets[service+":"+key] = value
	return nil
}

func (i *InMemorySecretStore) Delete(service, key string) error {
	delete(i.secrets, service+":"+key)
	return nil
}
