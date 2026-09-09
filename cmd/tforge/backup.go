package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"

	"tforge/internal/backup"
	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// passphraseEnvVar lets automation supply the passphrase without a terminal.
// It is a deliberate trade-off: an environment variable is visible to other
// processes of the same user, so the interactive prompt stays the default.
const passphraseEnvVar = "TFORGE_BACKUP_PASSPHRASE"

// runBackup writes every vault to an encrypted, passphrase-protected file.
func runBackup(path string, force bool) error {
	if path == "" {
		return errors.New("missing backup file path")
	}

	if _, err := os.Stat(path); err == nil && !force {
		return fmt.Errorf("%s already exists; pass --force to overwrite it", path)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check %s: %w", path, err)
	}

	vaults, err := loadAllVaults()
	if err != nil {
		return err
	}
	if len(vaults) == 0 {
		return errors.New("there are no vaults to back up")
	}

	passphrase, err := readNewPassphrase()
	if err != nil {
		return err
	}
	defer zero(passphrase)

	sealed, err := backup.Seal(vaults, passphrase)
	if err != nil {
		return err
	}

	if err := writeFileAtomic(path, sealed); err != nil {
		return err
	}

	// A backup nobody can restore is worse than no backup, because it is
	// trusted. Read it back and decrypt it before reporting success.
	if err := verifyBackup(path, passphrase); err != nil {
		if rmErr := os.Remove(path); rmErr != nil {
			return fmt.Errorf("%w (and the unusable file at %s could not be removed: %v)", err, path, rmErr)
		}
		return err
	}

	entries := 0
	for _, v := range vaults {
		entries += len(v.Entries)
	}
	fmt.Printf("Backed up %d vaults (%d keys) to %s\n", len(vaults), entries, path)
	fmt.Println("Keep the passphrase somewhere separate from this file. Without it the backup cannot be restored, by you or anyone else.")
	return nil
}

// verifyBackup re-reads a freshly written backup and decrypts it.
func verifyBackup(path string, passphrase []byte) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("verify backup: re-read failed: %w", err)
	}
	if _, err := backup.Open(data, passphrase); err != nil {
		return fmt.Errorf("verify backup: the file just written cannot be decrypted: %w", err)
	}
	return nil
}

// runRestore reads an encrypted backup back into local storage.
func runRestore(path string, replace, skipConfirm bool) error {
	if path == "" {
		return errors.New("missing backup file path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	passphrase, err := readPassphrase("Passphrase: ")
	if err != nil {
		return err
	}
	defer zero(passphrase)

	restored, err := backup.Open(data, passphrase)
	if err != nil {
		return err
	}

	existing, err := loadAllVaults()
	if err != nil {
		return err
	}

	var result mergeResult
	if replace {
		if len(existing) > 0 && !skipConfirm {
			ok, err := confirm(fmt.Sprintf(
				"Replace all %d existing vaults with the %d from the backup? This cannot be undone. [y/N]: ",
				len(existing), len(restored)))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Aborted; nothing was changed.")
				return nil
			}
		}
		result = mergeResult{Final: restored, Added: restored}
	} else {
		result = mergeVaults(existing, restored)
	}

	prot, err := defaultProtector()
	if err != nil {
		return err
	}
	if err := storage.SaveVaults(prot, result.Final); err != nil {
		return fmt.Errorf("save vaults: %w", err)
	}

	_ = triggerAgentReload()

	fmt.Printf("Restored %d vaults from %s\n", len(result.Added), path)
	for _, s := range result.Skipped {
		fmt.Printf("  skipped %q: %s\n", s.Name, s.Reason)
	}
	if len(result.Skipped) > 0 {
		fmt.Println("Use --replace to overwrite the local vaults with the backup instead.")
	}
	return nil
}

// skippedVault records a vault from the backup that was not imported.
type skippedVault struct {
	Name   string
	Reason string
}

// mergeResult is the outcome of merging a backup into the existing vaults.
type mergeResult struct {
	Final   []*vault.Vault
	Added   []*vault.Vault
	Skipped []skippedVault
}

// mergeVaults adds the vaults from a backup that do not collide with what is
// already stored.
//
// A colliding ID means the same vault is already present. A colliding *name*
// matters just as much: the agent resolves a reference by ID or name and takes
// the first match, so importing a second "MyVault" would make `tforge @MyVault`
// ambiguous. Both are skipped and reported rather than guessed at -- the user
// can then use --replace, or rename one side first.
func mergeVaults(existing, restored []*vault.Vault) mergeResult {
	byID := make(map[string]bool, len(existing))
	byName := make(map[string]bool, len(existing))
	for _, v := range existing {
		if v == nil {
			continue
		}
		byID[v.ID] = true
		byName[v.Name] = true
	}

	result := mergeResult{Final: append([]*vault.Vault(nil), existing...)}

	for _, v := range restored {
		if v == nil {
			continue
		}
		switch {
		case byID[v.ID]:
			result.Skipped = append(result.Skipped, skippedVault{
				Name:   v.Name,
				Reason: "a vault with the same ID already exists",
			})
		case byName[v.Name]:
			result.Skipped = append(result.Skipped, skippedVault{
				Name:   v.Name,
				Reason: "a different vault with the same name already exists",
			})
		default:
			byID[v.ID] = true
			byName[v.Name] = true
			result.Final = append(result.Final, v)
			result.Added = append(result.Added, v)
		}
	}
	return result
}

// --- helpers -------------------------------------------------------------

func defaultProtector() (secure.Protector, error) {
	cfgDir, err := storage.ConfigDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}
	prot, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		return nil, fmt.Errorf("init protector: %w", err)
	}
	return prot, nil
}

func loadAllVaults() ([]*vault.Vault, error) {
	prot, err := defaultProtector()
	if err != nil {
		return nil, err
	}
	vaults, err := storage.LoadVaults(prot)
	if err != nil {
		return nil, fmt.Errorf("load vaults: %w", err)
	}
	return vaults, nil
}

// writeFileAtomic writes through a temporary file in the same directory, so an
// interrupted write cannot leave a half-finished backup under the final name.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tforge-backup-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}

	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		cleanup()
		return fmt.Errorf("set permissions on %s: %w", tmpName, err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename to %s: %w", path, err)
	}
	return nil
}

// readNewPassphrase prompts twice and checks the two entries match.
func readNewPassphrase() ([]byte, error) {
	if fromEnv := os.Getenv(passphraseEnvVar); fromEnv != "" {
		return []byte(fromEnv), nil
	}

	first, err := readPassphrase(fmt.Sprintf("Passphrase for the backup (at least %d characters): ", backup.MinPassphraseLen))
	if err != nil {
		return nil, err
	}
	if len(first) < backup.MinPassphraseLen {
		zero(first)
		return nil, backup.ErrPassphraseTooShort
	}

	second, err := readPassphrase("Repeat passphrase: ")
	if err != nil {
		zero(first)
		return nil, err
	}
	defer zero(second)

	if string(first) != string(second) {
		zero(first)
		return nil, errors.New("the two passphrases do not match")
	}
	return first, nil
}

// readPassphrase reads a passphrase without echoing it.
func readPassphrase(prompt string) ([]byte, error) {
	if fromEnv := os.Getenv(passphraseEnvVar); fromEnv != "" {
		return []byte(fromEnv), nil
	}

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, fmt.Errorf(
			"cannot prompt for a passphrase: stdin is not a terminal. Set %s for automation",
			passphraseEnvVar,
		)
	}

	fmt.Print(prompt)
	pw, err := term.ReadPassword(fd)
	fmt.Println()
	if err != nil {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}
	return pw, nil
}

func confirm(prompt string) (bool, error) {
	fmt.Print(prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read confirmation: %w", err)
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "y" || line == "yes", nil
}

func zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
