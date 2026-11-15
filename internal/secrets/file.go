package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	fileVersion = 1
	saltLen     = 16
	keyLen      = 32
)

// fileFormat is the on-disk layout; Salt, Nonce and Data are base64 in JSON.
type fileFormat struct {
	Version int    `json:"version"`
	Salt    []byte `json:"salt"`
	Nonce   []byte `json:"nonce"`
	Data    []byte `json:"data"`
}

type fileStore struct {
	path       string
	passphrase func() (string, error)

	mu      sync.Mutex
	key     []byte
	keySalt []byte
}

// NewEncryptedFile returns a Store that keeps every secret in one file,
// encrypted with AES-256-GCM under a key derived from the passphrase with
// Argon2id. The passphrase is requested lazily and must never be hardcoded.
func NewEncryptedFile(path string, passphrase func() (string, error)) (Store, error) {
	if path == "" {
		return nil, errors.New("secrets: empty file path")
	}
	if passphrase == nil {
		return nil, errors.New("secrets: nil passphrase source")
	}
	return &fileStore{path: path, passphrase: passphrase}, nil
}

func (s *fileStore) Get(service, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := data[entry(service, key)]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *fileStore) Set(service, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, salt, err := s.load()
	if err != nil {
		return err
	}
	data[entry(service, key)] = value
	return s.save(data, salt)
}

func (s *fileStore) Delete(service, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, salt, err := s.load()
	if err != nil {
		return err
	}
	if _, ok := data[entry(service, key)]; !ok {
		return nil
	}
	delete(data, entry(service, key))
	return s.save(data, salt)
}

// load decrypts the file, or returns an empty map and a fresh salt when it
// does not exist yet.
func (s *fileStore) load() (map[string]string, []byte, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		salt := make([]byte, saltLen)
		if _, err := rand.Read(salt); err != nil {
			return nil, nil, fmt.Errorf("secrets: generate salt: %w", err)
		}
		return map[string]string{}, salt, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("secrets: read %s: %w", s.path, err)
	}

	var f fileFormat
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != fileVersion || len(f.Salt) == 0 {
		return nil, nil, errors.New("secrets: file is corrupt or has an unsupported format")
	}
	aead, err := s.aead(f.Salt)
	if err != nil {
		return nil, nil, err
	}
	if len(f.Nonce) != aead.NonceSize() {
		return nil, nil, errors.New("secrets: file is corrupt or has an unsupported format")
	}
	plain, err := aead.Open(nil, f.Nonce, f.Data, nil)
	if err != nil {
		return nil, nil, errors.New("secrets: cannot decrypt file (wrong passphrase or corrupt file)")
	}
	data := map[string]string{}
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, nil, errors.New("secrets: decrypted content is corrupt")
	}
	return data, f.Salt, nil
}

func (s *fileStore) save(data map[string]string, salt []byte) error {
	plain, err := json.Marshal(data)
	if err != nil {
		return err
	}
	aead, err := s.aead(salt)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("secrets: generate nonce: %w", err)
	}
	out, err := json.Marshal(fileFormat{
		Version: fileVersion,
		Salt:    salt,
		Nonce:   nonce,
		Data:    aead.Seal(nil, nonce, plain, nil),
	})
	if err != nil {
		return err
	}
	return writeAtomic(s.path, out)
}

// aead derives (and caches) the key for a salt and builds the cipher.
func (s *fileStore) aead(salt []byte) (cipher.AEAD, error) {
	if s.key == nil || string(s.keySalt) != string(salt) {
		pass, err := s.passphrase()
		if err != nil {
			return nil, fmt.Errorf("secrets: passphrase: %w", err)
		}
		if pass == "" {
			return nil, errors.New("secrets: empty passphrase")
		}
		s.key = argon2.IDKey([]byte(pass), salt, 1, 64*1024, 4, keyLen)
		s.keySalt = append([]byte(nil), salt...)
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// writeAtomic writes data to a temporary file in the same directory (created
// owner-only) and renames it over path.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("secrets: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".secrets-*")
	if err != nil {
		return fmt.Errorf("secrets: temp file: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("secrets: replace %s: %w", path, err)
	}
	return nil
}
