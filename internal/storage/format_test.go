package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"tforge/internal/secure"
	"tforge/internal/vault"
)

func vaultPath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(dir, "TForge", "vaults.bin")
}

func sampleVaults() []*vault.Vault {
	return []*vault.Vault{{
		ID:      "id-1",
		Name:    "Vault",
		Entries: []vault.Entry{{Key: "TOKEN", ValueDev: "value"}},
	}}
}

func TestSaveWritesHeader(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{kind: secure.KindDPAPI}

	if err := SaveVaults(p, sampleVaults()); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	data, err := os.ReadFile(vaultPath(t, dir))
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if string(data[:len(fileMagic)]) != fileMagic {
		t.Fatalf("file does not start with the magic: %q", data[:len(fileMagic)])
	}

	hdr, payload, framed := splitFile(data)
	if !framed {
		t.Fatal("splitFile did not recognise the header it just wrote")
	}
	if hdr.Version != formatVersion {
		t.Errorf("version = %d, want %d", hdr.Version, formatVersion)
	}
	if hdr.Kind != secure.KindDPAPI {
		t.Errorf("kind = %v, want %v", hdr.Kind, secure.KindDPAPI)
	}
	if len(payload) != len(data)-headerLen {
		t.Error("payload length does not match the data after the header")
	}
}

func TestLegacyFileWithoutHeaderStillLoads(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{}

	// Write a file the way versions before the header did: the sealed payload
	// with nothing in front of it.
	sealed, err := p.Seal([]byte(`[{"id":"id-1","name":"Legacy","entries":[]}]`))
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	path := vaultPath(t, dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadVaults(p)
	if err != nil {
		t.Fatalf("LoadVaults on a legacy file: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Legacy" {
		t.Fatalf("legacy vault not read back: %+v", got)
	}
}

func TestLegacyFileUpgradesOnNextSave(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{}

	sealed, _ := p.Seal([]byte(`[{"id":"id-1","name":"Legacy","entries":[]}]`))
	path := vaultPath(t, dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadVaults(p)
	if err != nil {
		t.Fatalf("LoadVaults: %v", err)
	}
	if err := SaveVaults(p, loaded); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	data, _ := os.ReadFile(path)
	if _, _, framed := splitFile(data); !framed {
		t.Error("saving a legacy file did not add the header")
	}

	// And it must still be readable afterwards.
	again, err := LoadVaults(p)
	if err != nil {
		t.Fatalf("LoadVaults after upgrade: %v", err)
	}
	if len(again) != 1 || again[0].Name != "Legacy" {
		t.Errorf("data changed during the upgrade: %+v", again)
	}
}

func TestProtectorMismatchIsReportedPrecisely(t *testing.T) {
	isolateConfigDir(t)

	// Sealed under DPAPI ...
	if err := SaveVaults(&fakeProtector{kind: secure.KindDPAPI}, sampleVaults()); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	// ... and opened by a build using the software protector, which cannot
	// decrypt it. This is the legacy-master.key-under-DPAPI case in reverse.
	_, err := LoadVaults(&fakeProtector{kind: secure.KindSoftware, failUnseal: true})
	if err == nil {
		t.Fatal("expected an error")
	}

	var mismatch *ProtectorMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("got %T (%v), want *ProtectorMismatchError", err, err)
	}
	if mismatch.Sealed != secure.KindDPAPI {
		t.Errorf("Sealed = %v, want %v", mismatch.Sealed, secure.KindDPAPI)
	}
	if mismatch.Current != secure.KindSoftware {
		t.Errorf("Current = %v, want %v", mismatch.Current, secure.KindSoftware)
	}
	if mismatch.Err == nil {
		t.Error("the underlying decrypt error was dropped")
	}
}

func TestSameProtectorFailureIsNotReportedAsMismatch(t *testing.T) {
	isolateConfigDir(t)

	if err := SaveVaults(&fakeProtector{kind: secure.KindSoftware}, sampleVaults()); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	// Same kind, but the decrypt fails anyway -- that is a corrupt file, and
	// blaming the protector would be misleading.
	_, err := LoadVaults(&fakeProtector{kind: secure.KindSoftware, failUnseal: true})
	if err == nil {
		t.Fatal("expected an error")
	}

	var mismatch *ProtectorMismatchError
	if errors.As(err, &mismatch) {
		t.Error("a same-protector failure was reported as a protector mismatch")
	}
}

func TestFutureFormatVersionIsRejected(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{}

	if err := SaveVaults(p, sampleVaults()); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	path := vaultPath(t, dir)
	data, _ := os.ReadFile(path)
	data[len(fileMagic)] = formatVersion + 1
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := LoadVaults(p)
	if err == nil {
		t.Fatal("expected an error for a newer format version")
	}

	var unsupported *UnsupportedVersionError
	if !errors.As(err, &unsupported) {
		t.Fatalf("got %T (%v), want *UnsupportedVersionError", err, err)
	}
	if unsupported.Found != formatVersion+1 || unsupported.Supported != formatVersion {
		t.Errorf("unexpected versions in %v", unsupported)
	}
}

func TestSplitFile(t *testing.T) {
	valid := append(encodeHeader(secure.KindKeyring), []byte("payload")...)

	cases := []struct {
		name      string
		in        []byte
		wantOK    bool
		wantKind  secure.Kind
		wantPload string
	}{
		{"header written by us", valid, true, secure.KindKeyring, "payload"},
		{"legacy payload", []byte("\x8a\x1f\x00\xd3 random nonce bytes"), false, secure.KindUnknown, "\x8a\x1f\x00\xd3 random nonce bytes"},
		{"too short for a header", []byte("TFV"), false, secure.KindUnknown, "TFV"},
		{"magic but implausible version", append([]byte(fileMagic), 0x00, 0x01), false, secure.KindUnknown, fileMagic + "\x00\x01"},
		{"empty", []byte{}, false, secure.KindUnknown, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hdr, payload, ok := splitFile(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if hdr.Kind != c.wantKind {
				t.Errorf("kind = %v, want %v", hdr.Kind, c.wantKind)
			}
			if string(payload) != c.wantPload {
				t.Errorf("payload = %q, want %q", payload, c.wantPload)
			}
		})
	}
}

func TestUnknownKindStillLoadsWhenDecryptSucceeds(t *testing.T) {
	dir := isolateConfigDir(t)
	p := &fakeProtector{}

	if err := SaveVaults(p, sampleVaults()); err != nil {
		t.Fatalf("SaveVaults: %v", err)
	}

	// A kind this build does not know about must not block a payload that
	// decrypts perfectly well -- the header is a hint, not a gate.
	path := vaultPath(t, dir)
	data, _ := os.ReadFile(path)
	data[len(fileMagic)+1] = 99
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LoadVaults(p)
	if err != nil {
		t.Fatalf("LoadVaults with an unknown kind: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d vaults, want 1", len(got))
	}
}
