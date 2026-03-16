package main

import (
	"context"
	"encoding/base64"
	"fmt"
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
		fmt.Println("config dir error:", err)
		return
	}

	protector, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		fmt.Println("protector init error:", err)
		return
	}
	a.protector = protector

	// Bestehende Vaults laden (falls vorhanden).
	vaults, err := storage.LoadVaults(a.protector)
	if err != nil {
		fmt.Println("load vaults error:", err)
		return
	}
	if vaults != nil {
		a.vaults.SetAll(vaults)
	}
}

// Greet returns a greeting for the given name
func (a *App) Greet(name string) string {
	return fmt.Sprintf("Hello %s, It's show time!", name)
}

// Vault API exposed to the frontend.

func (a *App) ListVaults() []*vault.Vault {
	return a.vaults.ListVaults()
}

func (a *App) CreateVault(name, description string) *vault.Vault {
	v := a.vaults.CreateVault(name, description)
	a.persistVaults()
	return v
}

func (a *App) GetVault(id string) (*vault.Vault, error) {
	v, ok := a.vaults.GetVault(id)
	if !ok {
		return nil, fmt.Errorf("vault not found")
	}
	return v, nil
}

func (a *App) UpdateVault(v *vault.Vault) error {
	if ok := a.vaults.UpdateVault(v); !ok {
		return fmt.Errorf("vault not found")
	}
	a.persistVaults()
	return nil
}

func (a *App) DeleteVault(id string) error {
	if ok := a.vaults.DeleteVault(id); !ok {
		return fmt.Errorf("vault not found")
	}
	a.persistVaults()
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
func (a *App) persistVaults() {
	if a.protector == nil {
		// Protector noch nicht initialisiert (z.B. Startup-Fehler) – dann keine Persistenz.
		return
	}
	if err := storage.SaveVaults(a.protector, a.vaults.ListVaults()); err != nil {
		fmt.Println("save vaults error:", err)
	}
}
