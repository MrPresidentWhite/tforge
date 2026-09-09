package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tforge/internal/storage"
	"tforge/internal/vault"
)

func v(id, name string, entries ...vault.Entry) *vault.Vault {
	return &vault.Vault{ID: id, Name: name, Entries: entries}
}

func names(vs []*vault.Vault) []string {
	out := make([]string, 0, len(vs))
	for _, x := range vs {
		out = append(out, x.Name)
	}
	return out
}

func TestMergeVaultsIntoEmptyStorage(t *testing.T) {
	restored := []*vault.Vault{v("id-1", "Shop"), v("id-2", "Blog")}

	got := mergeVaults(nil, restored)

	if len(got.Added) != 2 || len(got.Final) != 2 {
		t.Fatalf("added %d, final %d; want 2 and 2", len(got.Added), len(got.Final))
	}
	if len(got.Skipped) != 0 {
		t.Errorf("unexpected skips: %+v", got.Skipped)
	}
}

func TestMergeVaultsSkipsSameID(t *testing.T) {
	existing := []*vault.Vault{v("id-1", "Shop")}
	restored := []*vault.Vault{v("id-1", "Shop (old copy)")}

	got := mergeVaults(existing, restored)

	if len(got.Added) != 0 {
		t.Errorf("added %v, want nothing", names(got.Added))
	}
	if len(got.Skipped) != 1 || !strings.Contains(got.Skipped[0].Reason, "ID") {
		t.Fatalf("skips = %+v, want one ID collision", got.Skipped)
	}
	if len(got.Final) != 1 {
		t.Errorf("final holds %d vaults, want 1", len(got.Final))
	}
}

func TestMergeVaultsSkipsSameName(t *testing.T) {
	existing := []*vault.Vault{v("id-1", "Shop")}
	restored := []*vault.Vault{v("id-999", "Shop")}

	got := mergeVaults(existing, restored)

	if len(got.Added) != 0 {
		t.Errorf("added %v, want nothing", names(got.Added))
	}
	if len(got.Skipped) != 1 || !strings.Contains(got.Skipped[0].Reason, "name") {
		t.Fatalf("skips = %+v, want one name collision", got.Skipped)
	}
}

func TestMergeVaultsAddsOnlyTheNonColliding(t *testing.T) {
	existing := []*vault.Vault{v("id-1", "Shop")}
	restored := []*vault.Vault{
		v("id-1", "Shop"),      // same ID
		v("id-2", "Shop"),      // same name
		v("id-3", "Analytics"), // fine
	}

	got := mergeVaults(existing, restored)

	if len(got.Added) != 1 || got.Added[0].Name != "Analytics" {
		t.Fatalf("added %v, want [Analytics]", names(got.Added))
	}
	if len(got.Skipped) != 2 {
		t.Errorf("skips = %+v, want 2", got.Skipped)
	}
	if len(got.Final) != 2 {
		t.Errorf("final holds %d vaults, want 2", len(got.Final))
	}
}

func TestMergeVaultsDoesNotMutateExisting(t *testing.T) {
	existing := []*vault.Vault{v("id-1", "Shop")}

	mergeVaults(existing, []*vault.Vault{v("id-2", "Blog")})

	if len(existing) != 1 {
		t.Errorf("the caller's slice grew to %d entries", len(existing))
	}
}

// isolate points storage.ConfigDir at a temporary directory and supplies the
// passphrase through the environment, since tests have no terminal.
func isolate(t *testing.T, passphrase string) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv(passphraseEnvVar, passphrase)

	return dir
}

func TestBackupAndRestoreRoundTrip(t *testing.T) {
	isolate(t, "a sufficiently long passphrase")

	prot, err := defaultProtector()
	if err != nil {
		t.Fatalf("protector: %v", err)
	}
	original := []*vault.Vault{
		v("id-1", "Shop", vault.Entry{Key: "TOKEN", ValueDev: "dev-value", ValueProd: "prod-value"}),
		v("id-2", "Blog"),
	}
	if err := storage.SaveVaults(prot, original); err != nil {
		t.Fatalf("seed vaults: %v", err)
	}

	path := filepath.Join(t.TempDir(), "vaults.tfbak")
	if err := runBackup(path, false); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup file is empty")
	}

	// Simulate the disaster: local storage is gone.
	if err := storage.SaveVaults(prot, nil); err != nil {
		t.Fatalf("wipe vaults: %v", err)
	}

	if err := runRestore(path, false, true); err != nil {
		t.Fatalf("runRestore: %v", err)
	}

	got, err := storage.LoadVaults(prot)
	if err != nil {
		t.Fatalf("LoadVaults: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("restored %d vaults, want 2", len(got))
	}

	var shop *vault.Vault
	for _, x := range got {
		if x.Name == "Shop" {
			shop = x
		}
	}
	if shop == nil {
		t.Fatal("the Shop vault was not restored")
	}
	if len(shop.Entries) != 1 || shop.Entries[0].ValueProd != "prod-value" {
		t.Errorf("entry values not restored: %+v", shop.Entries)
	}
}

func TestBackupRefusesToOverwriteWithoutForce(t *testing.T) {
	isolate(t, "a sufficiently long passphrase")

	prot, _ := defaultProtector()
	if err := storage.SaveVaults(prot, []*vault.Vault{v("id-1", "Shop")}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "vaults.tfbak")
	if err := os.WriteFile(path, []byte("existing backup"), 0o600); err != nil {
		t.Fatalf("pre-create: %v", err)
	}

	err := runBackup(path, false)
	if err == nil {
		t.Fatal("expected a refusal to overwrite")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should point at --force, got: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "existing backup" {
		t.Error("the existing file was overwritten anyway")
	}

	if err := runBackup(path, true); err != nil {
		t.Fatalf("runBackup with force: %v", err)
	}
}

func TestBackupRefusesWhenThereIsNothingToSave(t *testing.T) {
	isolate(t, "a sufficiently long passphrase")

	path := filepath.Join(t.TempDir(), "vaults.tfbak")
	if err := runBackup(path, false); err == nil {
		t.Fatal("expected an error when there are no vaults")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty backup file was left behind")
	}
}

func TestRestoreRejectsWrongPassphrase(t *testing.T) {
	isolate(t, "a sufficiently long passphrase")

	prot, _ := defaultProtector()
	if err := storage.SaveVaults(prot, []*vault.Vault{v("id-1", "Shop")}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "vaults.tfbak")
	if err := runBackup(path, false); err != nil {
		t.Fatalf("runBackup: %v", err)
	}

	t.Setenv(passphraseEnvVar, "an entirely different passphrase")
	if err := runRestore(path, false, true); err == nil {
		t.Fatal("a wrong passphrase was accepted")
	}
}

func TestRestoreRejectsShortPassphraseOnBackup(t *testing.T) {
	isolate(t, "short")

	prot, _ := defaultProtector()
	if err := storage.SaveVaults(prot, []*vault.Vault{v("id-1", "Shop")}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	path := filepath.Join(t.TempDir(), "vaults.tfbak")
	if err := runBackup(path, false); err == nil {
		t.Fatal("a passphrase below the minimum length was accepted")
	}
}
