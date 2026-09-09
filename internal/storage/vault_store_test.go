package storage

import (
	"os"
	"path/filepath"
	"testing"

	"tforge/internal/secure"
	"tforge/internal/vault"
)

// isolateConfigDir points ConfigDir at a temporary directory so tests never
// touch the developer's real vault file.
func isolateConfigDir(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	// os.UserConfigDir reads AppData on Windows and XDG_CONFIG_HOME (falling
	// back to HOME/.config) elsewhere.
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)

	return dir
}

// fakeProtector is a deliberately trivial Protector: it lets the storage tests
// assert on framing and file handling without depending on any real crypto.
type fakeProtector struct {
	failUnseal bool
	kind       secure.Kind
}

// Kind defaults to the software protector so existing tests need no changes.
func (f *fakeProtector) Kind() secure.Kind {
	if f.kind == secure.KindUnknown {
		return secure.KindSoftware
	}
	return f.kind
}

func (f *fakeProtector) Seal(plaintext []byte) ([]byte, error) {
	return append([]byte("sealed:"), plaintext...), nil
}

func (f *fakeProtector) Unseal(ciphertext []byte) ([]byte, error) {
	if f.failUnseal {
		return nil, os.ErrInvalid
	}
	return ciphertext[len("sealed:"):], nil
}

func TestLoadVaultsReturnsNilWhenFileMissing(t *testing.T) {
	isolateConfigDir(t)

	got, err := LoadVaults(&fakeProtector{})
	if err != nil {
		t.Fatalf("LoadVaults on a fresh config dir: %v", err)
	}
	if got != nil {
		t.Errorf("got %v, want nil for a missing vault file", got)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	isolateConfigDir(t)
	p := &fakeProtector{}

	want := []*vault.Vault{{
		ID:   "id-1",
		Name: "Vault",
		Entries: []vault.Entry{
			{Key: "DB_HOST", ValueDev: "localhost", Type: vault.EntryTypeSecret},
		},
	}}

	if err := SaveVaults(p, want); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	got, err := LoadVaults(p)
	if err != nil {
		t.Fatalf("LoadVaults: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d vaults, want 1", len(got))
	}
	if got[0].ID != "id-1" || got[0].Name != "Vault" {
		t.Errorf("vault metadata not preserved: %+v", got[0])
	}
	if len(got[0].Entries) != 1 || got[0].Entries[0].ValueDev != "localhost" {
		t.Errorf("entries not preserved: %+v", got[0].Entries)
	}
}

func TestSaveVaultsWritesEncryptedBytesOnly(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{}

	vaults := []*vault.Vault{{
		ID:      "id-1",
		Name:    "Vault",
		Entries: []vault.Entry{{Key: "TOKEN", ValueDev: "super-secret-value"}},
	}}

	if err := SaveVaults(p, vaults); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	path := filepath.Join(dir, "TForge", "vaults.bin")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vaults.bin: %v", err)
	}

	_, payload, framed := splitFile(data)
	if !framed {
		t.Fatal("vaults.bin was written without the format header")
	}
	if len(payload) < len("sealed:") || string(payload[:len("sealed:")]) != "sealed:" {
		t.Error("the payload was not written through the protector")
	}

	// The temp file used for the atomic rename must not be left behind.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("the .tmp file from the atomic write was not cleaned up")
	}
}

func TestLoadVaultsFailsLoudlyOnUndecryptableFile(t *testing.T) {
	isolateConfigDir(t)

	if err := SaveVaults(&fakeProtector{}, []*vault.Vault{{ID: "id-1", Name: "V"}}); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	// A protector that cannot decrypt the file must produce an error, never an
	// empty vault list: callers treat "nil, nil" as "nothing stored yet" and
	// would happily overwrite the file.
	got, err := LoadVaults(&fakeProtector{failUnseal: true})
	if err == nil {
		t.Fatal("expected an error when the file cannot be decrypted")
	}
	if got != nil {
		t.Errorf("got %v alongside the error, want nil", got)
	}
}
