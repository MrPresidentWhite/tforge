package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
	groups, ungrouped := vault.DetectGroups(keys)

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

// --- Nachträglicher Import in einen bestehenden Vault ---------------------

// VaultEnvAnalysis checks one env file against a vault that already exists.
//
// Here the vault itself is the structure, not a .env.example: the user is
// filling in an environment they did not have values for yet.
type VaultEnvAnalysis struct {
	// Values holds the non-empty assignments read from the file.
	Values map[string]string `json:"values"`
	// Missing lists vault keys the file supplies no value for.
	Missing []string `json:"missing"`
	// Unknown lists keys the file supplies that the vault does not have.
	Unknown []string `json:"unknown"`
	// Overwrite lists keys where the target environment already holds a value
	// that this file would replace.
	Overwrite []string `json:"overwrite"`
}

// envField maps an environment name to the field it fills.
func envField(name string) (func(*vault.Entry) *string, error) {
	switch name {
	case "dev":
		return func(e *vault.Entry) *string { return &e.ValueDev }, nil
	case "staging":
		return func(e *vault.Entry) *string { return &e.ValueStage }, nil
	case "prod":
		return func(e *vault.Entry) *string { return &e.ValueProd }, nil
	default:
		return nil, fmt.Errorf("unknown environment %q", name)
	}
}

// AnalyseEnvForVault reports what importing text into one environment of an
// existing vault would do.
func (a *App) AnalyseEnvForVault(vaultID, envName, text string) (*VaultEnvAnalysis, error) {
	field, err := envField(envName)
	if err != nil {
		return nil, err
	}

	v, ok := a.vaults.GetVault(vaultID)
	if !ok {
		return nil, fmt.Errorf("vault not found")
	}

	pairs, err := envfile.ParseString(text)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(pairs) == 0 {
		return nil, fmt.Errorf("the file contains no KEY=VALUE lines")
	}

	keys := make([]string, 0, len(v.Entries))
	current := make(map[string]string, len(v.Entries))
	for i := range v.Entries {
		e := &v.Entries[i]
		keys = append(keys, e.Key)
		current[e.Key] = *field(e)
	}

	report := envfile.Validate(keys, pairs)
	values := envfile.Values(pairs)

	analysis := &VaultEnvAnalysis{
		Values:  values,
		Missing: report.Missing,
		Unknown: report.Unknown,
	}
	for _, k := range keys {
		if values[k] != "" && current[k] != "" {
			analysis.Overwrite = append(analysis.Overwrite, k)
		}
	}
	sort.Strings(analysis.Overwrite)

	return analysis, nil
}

// ApplyEnvValues writes values into one environment of an existing vault.
//
// Only the values handed in are written; an empty one is skipped rather than
// clearing what is already stored, since importing a file is meant to add
// values, not to erase them. Keys listed in addKeys are created first and
// inherit the prefix of a matching existing group.
func (a *App) ApplyEnvValues(vaultID, envName string, values map[string]string, addKeys []string) (*vault.Vault, error) {
	field, err := envField(envName)
	if err != nil {
		return nil, err
	}

	v, ok := a.vaults.GetVault(vaultID)
	if !ok {
		return nil, fmt.Errorf("vault not found")
	}

	existing := make(map[string]bool, len(v.Entries))
	for _, e := range v.Entries {
		existing[e.Key] = true
	}

	for _, key := range addKeys {
		key = strings.TrimSpace(key)
		if key == "" || existing[key] {
			continue
		}
		existing[key] = true
		// The group prefix is left empty on purpose: UpdateVault normalises
		// grouping, so a new key lands in the group its name belongs to.
		v.Entries = append(v.Entries, vault.Entry{
			Key:  key,
			Type: vault.EntryTypeSecret,
		})
	}

	applied := 0
	for i := range v.Entries {
		e := &v.Entries[i]
		value, ok := values[e.Key]
		if !ok || value == "" {
			continue
		}
		*field(e) = value
		applied++
	}
	if applied == 0 {
		return nil, fmt.Errorf("no values to apply")
	}

	if err := a.UpdateVault(v); err != nil {
		return nil, err
	}

	fresh, _ := a.vaults.GetVault(vaultID)
	return fresh, nil
}
