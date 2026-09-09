package main

import (
	"errors"
	"testing"

	"tforge/internal/secure"
	"tforge/internal/vault"
)

// stubProtector records whether anything was written through it.
type stubProtector struct {
	sealCalls int
}

func (s *stubProtector) Seal(plaintext []byte) ([]byte, error) {
	s.sealCalls++
	return append([]byte("sealed:"), plaintext...), nil
}

func (s *stubProtector) Unseal(ciphertext []byte) ([]byte, error) {
	return ciphertext[len("sealed:"):], nil
}

func (s *stubProtector) Kind() secure.Kind { return secure.KindSoftware }

// newAppWithLoadError builds an App in the state it ends up in when an existing
// vaults.bin could not be decrypted at startup.
func newAppWithLoadError() (*App, *stubProtector) {
	prot := &stubProtector{}
	return &App{
		vaults:    vault.NewService(),
		protector: prot,
		loadErr:   errors.New("unseal vaults: decrypt: cipher: message authentication failed"),
	}, prot
}

func TestPersistRefusedAfterFailedLoad(t *testing.T) {
	app, prot := newAppWithLoadError()

	if err := app.persistVaults(); err == nil {
		t.Fatal("persistVaults must refuse to write after a failed load")
	}
	if prot.sealCalls != 0 {
		t.Errorf("protector was used %d times; nothing may be written after a failed load", prot.sealCalls)
	}
}

func TestCreateVaultDoesNotOverwriteUnreadableData(t *testing.T) {
	app, prot := newAppWithLoadError()

	v, err := app.CreateVault("New", "")
	if err == nil {
		t.Fatal("CreateVault must fail while the existing vault file is unreadable")
	}
	if v != nil {
		t.Error("CreateVault returned a vault even though it failed")
	}
	if prot.sealCalls != 0 {
		t.Error("CreateVault wrote to disk despite the failed load")
	}
	if got := app.ListVaults(); len(got) != 0 {
		t.Errorf("in-memory state holds %d vaults; the failed creation was not rolled back", len(got))
	}
}

func TestDeleteVaultRolledBackWhenPersistFails(t *testing.T) {
	app, _ := newAppWithLoadError()

	// Seed a vault directly, bypassing the blocked CreateVault path.
	seeded := app.vaults.CreateVault("Existing", "")

	if err := app.DeleteVault(seeded.ID); err == nil {
		t.Fatal("DeleteVault must fail while persistence is blocked")
	}
	if _, ok := app.vaults.GetVault(seeded.ID); !ok {
		t.Error("the vault was removed from memory even though the deletion never reached disk")
	}
}

func TestUpdateVaultRolledBackWhenPersistFails(t *testing.T) {
	app, _ := newAppWithLoadError()

	seeded := app.vaults.CreateVault("Existing", "original")

	err := app.UpdateVault(&vault.Vault{ID: seeded.ID, Name: "Renamed", Description: "changed"})
	if err == nil {
		t.Fatal("UpdateVault must fail while persistence is blocked")
	}

	got, ok := app.vaults.GetVault(seeded.ID)
	if !ok {
		t.Fatal("vault disappeared")
	}
	if got.Name != "Existing" || got.Description != "original" {
		t.Errorf("in-memory vault = %q/%q; the failed update was not rolled back", got.Name, got.Description)
	}
}

func TestStartupErrorReporting(t *testing.T) {
	app, _ := newAppWithLoadError()
	if app.StartupError() == "" {
		t.Error("StartupError should describe the load failure")
	}

	healthy := &App{vaults: vault.NewService(), protector: &stubProtector{}}
	if got := healthy.StartupError(); got != "" {
		t.Errorf("StartupError = %q, want empty on a clean start", got)
	}
}

func TestPersistWithoutProtectorFails(t *testing.T) {
	app := &App{vaults: vault.NewService()}
	if err := app.persistVaults(); err == nil {
		t.Error("persistVaults must fail when no protector was initialised")
	}
}

func TestUpdateVaultRejectsNil(t *testing.T) {
	app := &App{vaults: vault.NewService(), protector: &stubProtector{}}
	if err := app.UpdateVault(nil); err == nil {
		t.Error("UpdateVault(nil) must return an error")
	}
}
