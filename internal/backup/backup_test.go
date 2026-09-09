package backup

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"

	"tforge/internal/vault"
)

const testPassphrase = "correct horse battery staple"

func sample() []*vault.Vault {
	return []*vault.Vault{
		{
			ID:          "id-1",
			Name:        "Shop",
			Description: "backend",
			Entries: []vault.Entry{
				{Key: "POSTGRES_HOST", ValueDev: "localhost", ValueProd: "db.internal", Type: vault.EntryTypeSecret, GroupPrefix: "POSTGRES_"},
				{Key: "API_TOKEN", ValueDev: "dev-token-value", Type: vault.EntryTypeSecret},
			},
		},
		{ID: "id-2", Name: "Empty", Entries: []vault.Entry{}},
	}
}

func TestRoundTrip(t *testing.T) {
	sealed, err := Seal(sample(), []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	got, err := Open(sealed, []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d vaults, want 2", len(got))
	}
	if got[0].Name != "Shop" || got[0].Description != "backend" {
		t.Errorf("vault metadata not preserved: %+v", got[0])
	}
	if len(got[0].Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(got[0].Entries))
	}
	e := got[0].Entries[0]
	if e.Key != "POSTGRES_HOST" || e.ValueDev != "localhost" || e.ValueProd != "db.internal" {
		t.Errorf("entry values not preserved: %+v", e)
	}
	if e.GroupPrefix != "POSTGRES_" || e.Type != vault.EntryTypeSecret {
		t.Errorf("entry metadata not preserved: %+v", e)
	}
	if len(got[1].Entries) != 0 {
		t.Errorf("empty vault gained entries: %+v", got[1])
	}
}

func TestSealHidesPlaintext(t *testing.T) {
	sealed, err := Seal(sample(), []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	for _, needle := range []string{"POSTGRES_HOST", "dev-token-value", "db.internal", "Shop"} {
		if bytes.Contains(sealed, []byte(needle)) {
			t.Errorf("%q appears in the backup in the clear", needle)
		}
	}
}

func TestSealIsRandomised(t *testing.T) {
	a, err := Seal(sample(), []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	b, err := Seal(sample(), []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Equal(a, b) {
		t.Error("two backups of the same data are byte-identical; salt or nonce is not random")
	}
}

func TestWrongPassphrase(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))

	_, err := Open(sealed, []byte("this is not the passphrase"))
	if !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("got %v, want ErrWrongPassphrase", err)
	}
}

func TestShortPassphraseRejected(t *testing.T) {
	_, err := Seal(sample(), []byte("short"))
	if !errors.Is(err, ErrPassphraseTooShort) {
		t.Fatalf("got %v, want ErrPassphraseTooShort", err)
	}
}

func TestNotABackup(t *testing.T) {
	for _, in := range [][]byte{
		nil,
		[]byte("hello"),
		[]byte("this is a long file but not a TForge backup at all, no magic here"),
	} {
		if _, err := Open(in, []byte(testPassphrase)); !errors.Is(err, ErrNotABackup) {
			t.Errorf("Open(%q) = %v, want ErrNotABackup", in, err)
		}
	}
}

func TestTruncatedBackup(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))

	if _, err := Open(sealed[:headerLen-1], []byte(testPassphrase)); !errors.Is(err, ErrNotABackup) {
		t.Error("a file shorter than the header should be rejected as not a backup")
	}
	if _, err := Open(sealed[:len(sealed)-5], []byte(testPassphrase)); err == nil {
		t.Error("a truncated ciphertext should not decrypt")
	}
}

func TestFutureVersionRejected(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))
	sealed[len(fileMagic)] = formatVersion + 1

	var unsupported *UnsupportedVersionError
	_, err := Open(sealed, []byte(testPassphrase))
	if !errors.As(err, &unsupported) {
		t.Fatalf("got %T (%v), want *UnsupportedVersionError", err, err)
	}
	if unsupported.Found != formatVersion+1 {
		t.Errorf("Found = %d, want %d", unsupported.Found, formatVersion+1)
	}
}

func TestTamperedCiphertextRejected(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))
	sealed[len(sealed)-1] ^= 0x01

	if _, err := Open(sealed, []byte(testPassphrase)); !errors.Is(err, ErrWrongPassphrase) {
		t.Error("a flipped bit in the ciphertext was accepted")
	}
}

// Weakening the KDF parameters is the attack the header authentication exists
// for: without it, an attacker could rewrite memory and time down to something
// trivial and brute-force the passphrase cheaply.
func TestWeakenedKdfParametersRejected(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))

	memOffset := len(fileMagic) + 1 + 1 + 4
	if got := binary.BigEndian.Uint32(sealed[memOffset : memOffset+4]); got != defaultMemory {
		t.Fatalf("memory field is not where the test expects it (found %d)", got)
	}
	binary.BigEndian.PutUint32(sealed[memOffset:memOffset+4], 8)

	if _, err := Open(sealed, []byte(testPassphrase)); err == nil {
		t.Error("a backup with downgraded KDF parameters was accepted")
	}
}

func TestTamperedSaltRejected(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))
	saltOffset := len(fileMagic) + 1 + 1 + 4 + 4 + 1 + 1
	sealed[saltOffset] ^= 0xff

	if _, err := Open(sealed, []byte(testPassphrase)); !errors.Is(err, ErrWrongPassphrase) {
		t.Error("a modified salt was accepted")
	}
}

func TestInvalidKdfParametersDoNotPanic(t *testing.T) {
	sealed, _ := Seal(sample(), []byte(testPassphrase))

	timeOffset := len(fileMagic) + 1 + 1
	binary.BigEndian.PutUint32(sealed[timeOffset:timeOffset+4], 0)

	// argon2.IDKey panics on a zero time cost, so this must be caught while
	// parsing rather than reaching the KDF.
	if _, err := Open(sealed, []byte(testPassphrase)); err == nil {
		t.Error("zero time cost was accepted")
	}
}

func TestEmptyVaultListRoundTrips(t *testing.T) {
	sealed, err := Seal(nil, []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	got, err := Open(sealed, []byte(testPassphrase))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d vaults, want 0", len(got))
	}
}
