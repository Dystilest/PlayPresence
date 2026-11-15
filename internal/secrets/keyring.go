package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

type keyringStore struct{}

// NewKeyring returns a Store backed by the OS credential store (Windows
// Credential Manager, macOS Keychain, Secret Service on Linux).
func NewKeyring() Store { return keyringStore{} }

func (keyringStore) Get(service, key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (keyringStore) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func (keyringStore) Delete(service, key string) error {
	err := keyring.Delete(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
