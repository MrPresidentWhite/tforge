package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	"github.com/wailsapp/mimetype"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// App struct
type App struct {
	ctx       context.Context
	vaults    *vault.Service
	protector secure.Protector

	// loadErr is set when an existing vaults.bin could not be read or
	// decrypted at startup. While it is set, persistence is disabled so a
	// half-initialised (empty) state can never overwrite the encrypted file
	// on disk. See persistVaults.
	loadErr error
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		vaults: vault.NewService(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Initialisiere Crypto/Storage-Layer.
	cfgDir, err := storage.ConfigDir()
	if err != nil {
		a.loadErr = fmt.Errorf("config dir: %w", err)
		log.Println("config dir error:", err)
		return
	}

	protector, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		a.loadErr = fmt.Errorf("protector init: %w", err)
		log.Println("protector init error:", err)
		return
	}
	a.protector = protector

	// Bestehende Vaults laden (falls vorhanden).
	vaults, err := storage.LoadVaults(a.protector)
	if err != nil {
		// The vault file exists but is unreadable, most likely because it was
		// encrypted with a different protector (e.g. a legacy master.key
		// installation now running under DPAPI). Keep the error so
		// persistVaults refuses to write and the frontend can warn the user.
		a.loadErr = err
		log.Println("load vaults error:", err)
		return
	}
	if vaults != nil {
		a.vaults.SetAll(vaults)
	}
}

// StartupError reports a startup problem that makes the vault state unsafe to
// write, as a human readable string. It returns an empty string when
// everything loaded normally. The frontend uses this to show a warning and to
// stop the user from editing a state that cannot be persisted.
func (a *App) StartupError() string {
	if a.loadErr == nil {
		return ""
	}
	return a.loadErr.Error()
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// Vault API exposed to the frontend.

func (a *App) ListVaults() []*vault.Vault {
	return a.vaults.ListVaults()
}

func (a *App) CreateVault(name, description string) (*vault.Vault, error) {
	v := a.vaults.CreateVault(name, description)
	if err := a.persistVaults(); err != nil {
		// Roll the in-memory creation back so the UI does not show a vault
		// that never made it to disk.
		a.vaults.DeleteVault(v.ID)
		return nil, err
	}
	return v, nil
}

func (a *App) GetVault(id string) (*vault.Vault, error) {
	v, ok := a.vaults.GetVault(id)
	if !ok {
		return nil, fmt.Errorf("vault not found")
	}
	return v, nil
}

func (a *App) UpdateVault(v *vault.Vault) error {
	if v == nil {
		return fmt.Errorf("vault is nil")
	}
	previous, ok := a.vaults.GetVault(v.ID)
	if !ok {
		return fmt.Errorf("vault not found")
	}
	if ok := a.vaults.UpdateVault(v); !ok {
		return fmt.Errorf("vault not found")
	}
	if err := a.persistVaults(); err != nil {
		// Restore the previous state so memory and disk stay in sync.
		a.vaults.UpdateVault(previous)
		return err
	}
	return nil
}

func (a *App) DeleteVault(id string) error {
	previous, ok := a.vaults.GetVault(id)
	if !ok {
		return fmt.Errorf("vault not found")
	}
	if ok := a.vaults.DeleteVault(id); !ok {
		return fmt.Errorf("vault not found")
	}
	if err := a.persistVaults(); err != nil {
		// Put the vault back; the deletion never reached disk.
		a.vaults.RestoreVault(previous)
		return err
	}
	return nil
}

// ChooseVaultIcon öffnet einen Dateidialog, mit dem der Benutzer ein Bild für ein Vault-Icon auswählen kann.
// Es wird der ausgewählte Pfad zurückgegeben oder ein leerer String, wenn der Dialog abgebrochen wurde.
func (a *App) ChooseVaultIcon() (string, error) {
	if a.ctx == nil {
		return "", fmt.Errorf("context not initialised")
	}

	result, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Icon für Vault auswählen",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Bilder",
				Pattern:     "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.ico",
			},
		},
	})
	if err != nil {
		return "", err
	}
	if result == "" {
		// Abgebrochen.
		return "", nil
	}

	// Bild einlesen und als data: URL (Base64) zurückgeben,
	// damit das WebView es zuverlässig anzeigen kann.
	data, err := os.ReadFile(result)
	if err != nil {
		return "", fmt.Errorf("read icon: %w", err)
	}

	mt := mimetype.Detect(data)
	mimeType := mt.String()
	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, encoded)
	return dataURL, nil
}

// persistVaults schreibt den aktuellen Vault-State verschlüsselt auf Disk.
func (a *App) persistVaults() error {
	if a.protector == nil {
		// Protector noch nicht initialisiert (z.B. Startup-Fehler) – dann keine Persistenz.
		return fmt.Errorf("storage not initialised")
	}
	if a.loadErr != nil {
		// Existing data on disk could not be decrypted. Writing now would
		// replace it with whatever is in memory (usually nothing) and destroy
		// the user's vaults, so refuse instead.
		return fmt.Errorf("refusing to save: existing vault data could not be loaded (%v)", a.loadErr)
	}
	if err := storage.SaveVaults(a.protector, a.vaults.ListVaults()); err != nil {
		log.Println("save vaults error:", err)
		return fmt.Errorf("save vaults: %w", err)
	}
	return nil
}
