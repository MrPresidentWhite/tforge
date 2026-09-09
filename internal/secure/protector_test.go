package secure

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSoftwareProtectorRoundTrip(t *testing.T) {
	p, err := NewSoftwareProtector(t.TempDir())
	if err != nil {
		t.Fatalf("NewSoftwareProtector: %v", err)
	}

	plaintext := []byte(`[{"id":"1","name":"Vault","entries":[{"key":"K","valueDev":"v"}]}]`)

	sealed, err := p.Seal(plaintext)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(sealed, plaintext) {
		t.Fatal("sealed output still contains the plaintext")
	}

	got, err := p.Unseal(sealed)
	if err != nil {
		t.Fatalf("Unseal: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("round trip = %q, want %q", got, plaintext)
	}
}

func TestSoftwareProtectorReusesStoredKey(t *testing.T) {
	dir := t.TempDir()

	first, err := NewSoftwareProtector(dir)
	if err != nil {
		t.Fatalf("first protector: %v", err)
	}
	sealed, err := first.Seal([]byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// A second protector over the same config dir must pick up master.key and
	// still be able to read what the first one wrote.
	second, err := NewSoftwareProtector(dir)
	if err != nil {
		t.Fatalf("second protector: %v", err)
	}
	got, err := second.Unseal(sealed)
	if err != nil {
		t.Fatalf("Unseal with reloaded key: %v", err)
	}
	if string(got) != "secret" {
		t.Errorf("got %q, want %q", got, "secret")
	}

	if _, err := os.Stat(filepath.Join(dir, "master.key")); err != nil {
		t.Errorf("master.key was not written: %v", err)
	}
}

func TestSoftwareProtectorDetectsTampering(t *testing.T) {
	p, err := NewSoftwareProtector(t.TempDir())
	if err != nil {
		t.Fatalf("NewSoftwareProtector: %v", err)
	}

	sealed, err := p.Seal([]byte("secret value"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// Flip a bit in the ciphertext body; AES-GCM must reject it.
	tampered := append([]byte(nil), sealed...)
	tampered[len(tampered)-1] ^= 0x01

	if _, err := p.Unseal(tampered); err == nil {
		t.Error("Unseal accepted tampered ciphertext")
	}
}

func TestSoftwareProtectorRejectsShortInput(t *testing.T) {
	p, err := NewSoftwareProtector(t.TempDir())
	if err != nil {
		t.Fatalf("NewSoftwareProtector: %v", err)
	}

	for _, input := range [][]byte{{}, {0x01}, make([]byte, 15)} {
		if _, err := p.Unseal(input); err == nil {
			t.Errorf("Unseal(%d bytes) should fail", len(input))
		}
	}
}

func TestSoftwareProtectorRejectsWrongKey(t *testing.T) {
	a, err := NewSoftwareProtector(t.TempDir())
	if err != nil {
		t.Fatalf("protector a: %v", err)
	}
	b, err := NewSoftwareProtector(t.TempDir())
	if err != nil {
		t.Fatalf("protector b: %v", err)
	}

	sealed, err := a.Seal([]byte("secret"))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	// This is exactly what happens when a vault encrypted under one protector
	// is opened by another (for example a legacy master.key vault under DPAPI):
	// it must fail loudly rather than yield garbage.
	if _, err := b.Unseal(sealed); err == nil {
		t.Error("a protector with a different key must not decrypt the data")
	}
}

func TestNewSoftwareProtectorRejectsEmptyDir(t *testing.T) {
	if _, err := NewSoftwareProtector(""); err == nil {
		t.Error("expected an error for an empty config dir")
	}
}
