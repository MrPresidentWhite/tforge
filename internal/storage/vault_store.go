package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"tforge/internal/secure"
	"tforge/internal/vault"
)

const (
	appDirName = "TForge"
	vaultsFile = "vaults.bin"
)

// ConfigDir liefert den Basis-Konfigpfad für TForge, z.B. %APPDATA%\TForge.
func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("user config dir: %w", err)
	}
	dir := filepath.Join(base, appDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// LoadVaults lädt den verschlüsselten Vault-State aus dem Dateisystem.
// Wenn keine Datei existiert, wird (nil, nil) zurückgegeben.
func LoadVaults(p secure.Protector) ([]*vault.Vault, error) {
	cfgDir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(cfgDir, vaultsFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read vaults: %w", err)
	}

	hdr, payload, framed := splitFile(data)

	// A newer format is not something to guess at, so stop before decrypting.
	if framed && hdr.Version > formatVersion {
		return nil, &UnsupportedVersionError{Found: hdr.Version, Supported: formatVersion}
	}

	plaintext, err := p.Unseal(payload)
	if err != nil {
		// Turn the most common cause into an error that actually says what
		// happened, instead of a bare "cipher: message authentication failed".
		if framed && hdr.Kind != p.Kind() {
			return nil, &ProtectorMismatchError{
				Sealed:  hdr.Kind,
				Current: p.Kind(),
				Err:     err,
			}
		}
		return nil, fmt.Errorf("unseal vaults: %w", err)
	}

	var vaults []*vault.Vault
	if err := json.Unmarshal(plaintext, &vaults); err != nil {
		return nil, fmt.Errorf("decode vaults: %w", err)
	}
	return vaults, nil
}

// SaveVaults speichert alle Vaults als verschlüsselten Blob.
func SaveVaults(p secure.Protector, vaults []*vault.Vault) error {
	cfgDir, err := ConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(cfgDir, vaultsFile)

	data, err := json.MarshalIndent(vaults, "", "  ")
	if err != nil {
		return fmt.Errorf("encode vaults: %w", err)
	}

	ciphertext, err := p.Seal(data)
	if err != nil {
		return fmt.Errorf("seal vaults: %w", err)
	}

	// Every write emits the current framing, so a legacy file upgrades itself
	// the first time it is saved -- no separate migration step needed.
	out := append(encodeHeader(p.Kind()), ciphertext...)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return fmt.Errorf("write tmp vaults: %w", err)
	}

	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename vaults: %w", err)
	}
	return nil
}
