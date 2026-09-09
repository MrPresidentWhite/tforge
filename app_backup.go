package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tforge/internal/backup"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// ChooseBackupTarget asks where to write a backup.
func (a *App) ChooseBackupTarget() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("context not initialised")
	}
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Backup speichern",
		DefaultFilename: "tforge-vaults.tfbak",
		Filters: []runtime.FileFilter{
			{DisplayName: "TForge-Backup", Pattern: "*.tfbak"},
		},
	})
}

// ChooseBackupSource asks which backup to restore.
func (a *App) ChooseBackupSource() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("context not initialised")
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Backup öffnen",
		Filters: []runtime.FileFilter{
			{DisplayName: "TForge-Backup", Pattern: "*.tfbak"},
			{DisplayName: "Alle Dateien", Pattern: "*.*"},
		},
	})
}

// BackupVaults writes every vault to a passphrase-protected file.
//
// The file is read back and decrypted before this reports success: a backup
// that cannot be restored is worse than none, because it is trusted.
func (a *App) BackupVaults(path, passphrase string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("no target file chosen")
	}

	vaults := a.vaults.ListVaults()
	if len(vaults) == 0 {
		return "", fmt.Errorf("there are no vaults to back up")
	}

	sealed, err := backup.Seal(vaults, []byte(passphrase))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return "", fmt.Errorf("write %s: %w", path, err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("verify backup: %w", err)
	}
	if _, err := backup.Open(written, []byte(passphrase)); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("verify backup: the file just written cannot be decrypted: %w", err)
	}

	entries := 0
	for _, v := range vaults {
		entries += len(v.Entries)
	}
	return fmt.Sprintf("%d Vaults mit %d Keys gesichert", len(vaults), entries), nil
}

// RestoreResult describes what a restore did, so the UI can be specific.
type RestoreResult struct {
	Added   []string `json:"added"`
	Skipped []string `json:"skipped"`
}

// RestoreVaults reads a backup back into the vault list.
//
// Merging is the default: a vault whose ID or name already exists is skipped
// and reported. The agent resolves a reference by ID or name and takes the
// first match, so importing a second vault under an existing name would make
// that reference ambiguous.
func (a *App) RestoreVaults(path, passphrase string, replace bool) (*RestoreResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}

	restored, err := backup.Open(data, []byte(passphrase))
	if err != nil {
		return nil, err
	}

	result := &RestoreResult{}

	if replace {
		a.vaults.SetAll(restored)
		for _, v := range restored {
			result.Added = append(result.Added, v.Name)
		}
	} else {
		existing := a.vaults.ListVaults()
		byID := map[string]bool{}
		byName := map[string]bool{}
		for _, v := range existing {
			byID[v.ID] = true
			byName[v.Name] = true
		}

		final := append([]*vault.Vault(nil), existing...)
		for _, v := range restored {
			if v == nil {
				continue
			}
			if byID[v.ID] {
				result.Skipped = append(result.Skipped,
					fmt.Sprintf("%s (ein Vault mit derselben ID existiert bereits)", v.Name))
				continue
			}
			if byName[v.Name] {
				result.Skipped = append(result.Skipped,
					fmt.Sprintf("%s (ein anderer Vault mit diesem Namen existiert bereits)", v.Name))
				continue
			}
			byID[v.ID] = true
			byName[v.Name] = true
			final = append(final, v)
			result.Added = append(result.Added, v.Name)
		}
		a.vaults.SetAll(final)
	}

	if err := a.persistVaults(); err != nil {
		// Put the state on disk back into memory rather than leaving the two
		// out of step after a failed write.
		if reloaded, loadErr := storage.LoadVaults(a.protector); loadErr == nil && reloaded != nil {
			a.vaults.SetAll(reloaded)
		}
		return nil, err
	}

	return result, nil
}

// MinBackupPassphraseLength lets the UI show and enforce the same minimum the
// backup container does, without hardcoding a second copy of the number.
func (a *App) MinBackupPassphraseLength() int {
	return backup.MinPassphraseLen
}
