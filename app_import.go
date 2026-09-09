package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"tforge/internal/envfile"
	"tforge/internal/vault"
)

// Everything the create-vault import wizard and the backup/restore dialogs
// need from the backend.

// ImportGroup is a prefix group detected in the example file.
type ImportGroup struct {
	Prefix string   `json:"prefix"`
	Keys   []string `json:"keys"`
}

// ImportEnvReport is the validation outcome for one environment file.
type ImportEnvReport struct {
	Provided bool              `json:"provided"`
	Values   map[string]string `json:"values"`
	Missing  []string          `json:"missing"`
	Unknown  []string          `json:"unknown"`
}

// ImportAnalysis is the full picture the wizard renders: the structure taken
// from the example file, how it groups, and how each environment file lines up
// against it.
type ImportAnalysis struct {
	Keys      []string                   `json:"keys"`
	Groups    []ImportGroup              `json:"groups"`
	Ungrouped []string                   `json:"ungrouped"`
	Envs      map[string]ImportEnvReport `json:"envs"`
}

// ImportEntry is one finished row the wizard hands back for creation.
type ImportEntry struct {
	Key         string `json:"key"`
	GroupPrefix string `json:"groupPrefix"`
	ValueDev    string `json:"valueDev"`
	ValueStage  string `json:"valueStage"`
	ValueProd   string `json:"valueProd"`
	Type        string `json:"type"`
}

// AnalyseEnvImport turns the uploaded files into a plan for the wizard.
//
// The example file defines the structure; the three environment files only
// supply values for it. Everything is compared by key, never by position, so
// the files may list their variables in any order.
func (a *App) AnalyseEnvImport(exampleText, devText, stagingText, prodText string) (*ImportAnalysis, error) {
	examplePairs, err := envfile.ParseString(exampleText)
	if err != nil {
		return nil, fmt.Errorf("read example file: %w", err)
	}
	if len(examplePairs) == 0 {
		return nil, fmt.Errorf("the example file contains no KEY=VALUE lines")
	}

	keys := envfile.Keys(examplePairs)
	groups, ungrouped := envfile.DetectGroups(keys)

	analysis := &ImportAnalysis{
		Keys:      keys,
		Ungrouped: ungrouped,
		Envs:      make(map[string]ImportEnvReport, 3),
	}
	for _, g := range groups {
		analysis.Groups = append(analysis.Groups, ImportGroup{Prefix: g.Prefix, Keys: g.Keys})
	}

	for name, text := range map[string]string{
		"dev":     devText,
		"staging": stagingText,
		"prod":    prodText,
	} {
		report := ImportEnvReport{Values: map[string]string{}}

		if strings.TrimSpace(text) != "" {
			pairs, err := envfile.ParseString(text)
			if err != nil {
				return nil, fmt.Errorf("read %s file: %w", name, err)
			}
			validation := envfile.Validate(keys, pairs)
			report = ImportEnvReport{
				Provided: true,
				Values:   envfile.Values(pairs),
				Missing:  validation.Missing,
				Unknown:  validation.Unknown,
			}
		}

		analysis.Envs[name] = report
	}

	return analysis, nil
}

// CreateVaultFromImport creates a vault from the finished wizard rows.
//
// The wizard has already resolved every warning by this point, so the entries
// are taken as given rather than validated again.
func (a *App) CreateVaultFromImport(name, description, icon string, entries []ImportEntry) (*vault.Vault, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("the vault needs a name")
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("there is nothing to import")
	}

	v := a.vaults.CreateVault(name, description)
	v.Icon = icon
	v.Entries = make([]vault.Entry, 0, len(entries))

	for _, e := range entries {
		key := strings.TrimSpace(e.Key)
		if key == "" {
			continue
		}
		entryType := vault.EntryType(e.Type)
		switch entryType {
		case vault.EntryTypeEnv, vault.EntryTypeSecret, vault.EntryTypeNote:
		default:
			// Imported keys are secrets unless the wizard says otherwise.
			entryType = vault.EntryTypeSecret
		}
		v.Entries = append(v.Entries, vault.Entry{
			Key:         key,
			GroupPrefix: e.GroupPrefix,
			ValueDev:    e.ValueDev,
			ValueStage:  e.ValueStage,
			ValueProd:   e.ValueProd,
			Type:        entryType,
		})
	}

	if !a.vaults.UpdateVault(v) {
		return nil, fmt.Errorf("the vault disappeared while it was being filled")
	}

	if err := a.persistVaults(); err != nil {
		a.vaults.DeleteVault(v.ID)
		return nil, err
	}

	fresh, _ := a.vaults.GetVault(v.ID)
	return fresh, nil
}

// PickedFile is a file the user chose, with its contents already read.
type PickedFile struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

// ChooseEnvFile opens a file dialog for an env-style file and returns its
// contents. Reading happens in Go so the frontend never needs filesystem
// access of its own.
func (a *App) ChooseEnvFile(title string) (*PickedFile, error) {
	if a.ctx == nil {
		return nil, fmt.Errorf("context not initialised")
	}

	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: title,
		Filters: []runtime.FileFilter{
			{DisplayName: "Env-Dateien", Pattern: "*.env;*.example;.env*;*.txt"},
			{DisplayName: "Alle Dateien", Pattern: "*.*"},
		},
	})
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil // cancelled
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return &PickedFile{Path: path, Name: filepath.Base(path), Content: string(data)}, nil
}
