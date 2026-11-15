package secrets

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestMemoryStore(t *testing.T) {
	m := NewMemory()

	if _, err := m.Get("svc", "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing: err = %v", err)
	}
	if err := m.Set("svc", "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Get("svc", "k"); got != "v" {
		t.Errorf("Get = %q", got)
	}
	if err := m.Delete("svc", "k"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Get("svc", "k"); !errors.Is(err, ErrNotFound) {
		t.Errorf("after Delete: err = %v", err)
	}
}

func TestKeyringStoreMapsNotFound(t *testing.T) {
	keyring.MockInit()
	s := NewKeyring()

	if _, err := s.Get("svc", "k"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get missing: err = %v, want ErrNotFound", err)
	}
	if err := s.Set("svc", "k", "v"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get("svc", "k"); err != nil || got != "v" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := s.Delete("svc", "k"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("svc", "k"); err != nil {
		t.Errorf("Delete of a missing key: err = %v, want nil", err)
	}
}

type brokenStore struct{ Store }

func (brokenStore) Set(string, string, string) error { return errors.New("no keyring service") }

func TestChoosePrefersKeyringWhenItWorks(t *testing.T) {
	kr := NewMemory()

	got := choose(kr, t.TempDir()+"/s.enc", pass("pw"))

	if got != Store(kr) {
		t.Errorf("choose returned %T, want the keyring store", got)
	}
}

func TestChooseFallsBackToEncryptedFile(t *testing.T) {
	got := choose(brokenStore{NewMemory()}, t.TempDir()+"/s.enc", pass("pw"))

	if _, ok := got.(*fileStore); !ok {
		t.Fatalf("choose returned %T, want *fileStore", got)
	}
	if err := got.Set("svc", "k", "v"); err != nil {
		t.Fatalf("fallback store must work: %v", err)
	}
}
