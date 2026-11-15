// Package secrets stores credentials in the OS credential store, with an
// encrypted file as a fallback for headless machines.
package secrets

import (
	"errors"
	"sync"
)

// ErrNotFound is returned when a secret does not exist.
var ErrNotFound = errors.New("secrets: not found")

// Store keeps secrets addressed by (service, key).
type Store interface {
	Get(service, key string) (string, error)
	Set(service, key, value string) error
	// Delete removes a secret; deleting a missing one is not an error.
	Delete(service, key string) error
}

// Default returns the OS keyring when it works, otherwise an encrypted file
// at path unlocked with the passphrase returned by passphrase.
func Default(path string, passphrase func() (string, error)) Store {
	return choose(NewKeyring(), path, passphrase)
}

func choose(keyring Store, path string, passphrase func() (string, error)) Store {
	if probe(keyring) == nil {
		return keyring
	}
	fs, err := NewEncryptedFile(path, passphrase)
	if err != nil {
		return failedStore{err}
	}
	return fs
}

func probe(s Store) error {
	const svc, key = "playpresence-probe", "probe"
	if err := s.Set(svc, key, "1"); err != nil {
		return err
	}
	if got, err := s.Get(svc, key); err != nil || got != "1" {
		return errors.New("secrets: keyring probe failed")
	}
	return s.Delete(svc, key)
}

type failedStore struct{ err error }

func (f failedStore) Get(string, string) (string, error) { return "", f.err }
func (f failedStore) Set(string, string, string) error   { return f.err }
func (f failedStore) Delete(string, string) error        { return f.err }

// Memory is an in-memory Store, useful as a test double.
type Memory struct {
	mu   sync.Mutex
	data map[string]string
}

// NewMemory returns an empty in-memory store.
func NewMemory() *Memory { return &Memory{data: map[string]string{}} }

func entry(service, key string) string { return service + "\x00" + key }

// Get returns the stored value.
func (m *Memory) Get(service, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[entry(service, key)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set stores a value.
func (m *Memory) Set(service, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[entry(service, key)] = value
	return nil
}

// Delete removes a value.
func (m *Memory) Delete(service, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, entry(service, key))
	return nil
}
