package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func pass(p string) func() (string, error) { return func() (string, error) { return p, nil } }

func newFile(t *testing.T, path, passphrase string) Store {
	t.Helper()
	s, err := NewEncryptedFile(path, pass(passphrase))
	if err != nil {
		t.Fatalf("NewEncryptedFile: %v", err)
	}
	return s
}

func TestFileStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secrets.enc")
	s := newFile(t, path, "correct horse")

	if err := s.Set("playpresence", "jellyfin", "tok-123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.Get("playpresence", "jellyfin")
	if err != nil || got != "tok-123" {
		t.Fatalf("Get = %q, %v", got, err)
	}

	if err := s.Delete("playpresence", "jellyfin"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("playpresence", "jellyfin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: err = %v, want ErrNotFound", err)
	}
}

func TestFileStoreMissingKeyAndIdempotentDelete(t *testing.T) {
	s := newFile(t, filepath.Join(t.TempDir(), "s.enc"), "pw")

	if _, err := s.Get("svc", "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get missing: err = %v, want ErrNotFound", err)
	}
	if err := s.Delete("svc", "nope"); err != nil {
		t.Errorf("Delete missing: err = %v, want nil", err)
	}
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.enc")
	if err := newFile(t, path, "pw").Set("svc", "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := newFile(t, path, "pw").Set("svc", "k2", "v2"); err != nil {
		t.Fatal(err)
	}

	s := newFile(t, path, "pw")
	for key, want := range map[string]string{"k": "v1", "k2": "v2"} {
		if got, err := s.Get("svc", key); err != nil || got != want {
			t.Errorf("Get(%q) = %q, %v, want %q", key, got, err, want)
		}
	}
}

func TestFileStoreWrongPassphraseFailsAndLeaksNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.enc")
	const secret = "super-secret-token-value"
	if err := newFile(t, path, "right").Set("svc", "k", secret); err != nil {
		t.Fatal(err)
	}

	_, err := newFile(t, path, "wrong").Get("svc", "k")

	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want a decryption error distinct from ErrNotFound", err)
	}
	raw, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Error("secret found in plaintext in the file")
	}
}

func TestFileStoreWorksInPathsWithSpacesAndAccents(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mon dossier é", "sous dossier")
	path := filepath.Join(dir, "secrets.enc")
	s := newFile(t, path, "pw")

	if err := s.Set("svc", "k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got, err := s.Get("svc", "k"); err != nil || got != "v" {
		t.Fatalf("Get = %q, %v", got, err)
	}
}

func TestFileStoreFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	path := filepath.Join(t.TempDir(), "s.enc")
	s := newFile(t, path, "pw")
	if err := s.Set("svc", "k", "v"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

func TestFileStoreRejectsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.enc")
	if err := os.WriteFile(path, []byte("not json at all"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := newFile(t, path, "pw").Get("svc", "k"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want a corruption error", err)
	}
}

func TestNewEncryptedFileValidatesArguments(t *testing.T) {
	if _, err := NewEncryptedFile("", pass("pw")); err == nil {
		t.Error("empty path must be rejected")
	}
	if _, err := NewEncryptedFile("x", nil); err == nil {
		t.Error("nil passphrase source must be rejected")
	}
}
