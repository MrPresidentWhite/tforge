package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

type envResponse struct {
	Env map[string]string `json:"env"`
}

func main() {
	envFlag := flag.String("env", "dev", "environment to use (dev|staging|prod)")
	exportMode := flag.Bool("export", false, "print env as KEY=VALUE lines instead of running a command")

	createVault := flag.String("create-vault", "", "create a new vault by importing from an env file")
	duplicateTo := flag.String("duplicate-to", "", "duplicate imported dev values to another environment (staging|prod)")
	importFile := flag.String("file", "", "path to an env-style file to import")
	entryType := flag.String("type", "secrets", "entry type for imported keys (secrets|env|note)")

	deleteVault := flag.Bool("delete", false, "delete a vault by name or ID")
	skipConfirm := flag.Bool("y", false, "skip confirmation when deleting a vault")

	flag.Parse()

	// Import mode: create a new vault from an env file.
	if *createVault != "" {
		if err := importEnvFileAsVault(*createVault, *duplicateTo, *importFile, *entryType); err != nil {
			log.Fatalf("import vault from file: %v", err)
		}
		return
	}

	args := flag.Args()
	if len(args) == 0 {
		if *deleteVault {
			log.Fatalf("usage: tforge --delete @VaultName [-y]")
		}
		log.Fatalf("usage: tforge [--env dev|staging|prod] @VaultName [command ...]")
	}

	// Extract vault reference (strip optional leading "@").
	vaultRef := strings.TrimPrefix(args[0], "@")

	// Delete mode: remove a vault from storage.
	if *deleteVault {
		if err := deleteVaultByRef(vaultRef, *skipConfirm); err != nil {
			log.Fatalf("delete vault: %v", err)
		}
		return
	}

	var cmdArgs []string
	if len(args) > 1 {
		cmdArgs = args[1:]
		// Support optional `--` separator: tforge @Vault -- npm run dev
		if len(cmdArgs) > 0 && cmdArgs[0] == "--" {
			cmdArgs = cmdArgs[1:]
		}
	}

	if len(cmdArgs) == 0 && !*exportMode {
		log.Fatalf("no command specified (or use --export)")
	}

	envMap, err := fetchEnvFromAgent(vaultRef, *envFlag)
	if err != nil {
		log.Fatalf("fetch env from agent: %v", err)
	}

	if *exportMode {
		for k, v := range envMap {
			fmt.Printf("%s=%s\n", k, v)
		}
		return
	}

	// Run child process with merged environment.
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	baseEnv := os.Environ()
	for k, v := range envMap {
		baseEnv = append(baseEnv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = baseEnv

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		log.Fatalf("command failed: %v", err)
	}
}

func fetchEnvFromAgent(vaultRef, env string) (map[string]string, error) {
	if env == "" {
		env = "dev"
	}
	q := url.Values{}
	q.Set("vault", vaultRef)
	q.Set("env", env)
	u := url.URL{
		Scheme:   "http",
		Host:     "127.0.0.1:5959",
		Path:     "/env",
		RawQuery: q.Encode(),
	}

	resp, err := http.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("connect agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("agent returned %s", resp.Status)
	}

	var er envResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return er.Env, nil
}

func importEnvFileAsVault(name, duplicateTo, filePath, entryType string) error {
	if filePath == "" {
		return fmt.Errorf("missing --file for env import")
	}
	if name == "" {
		return fmt.Errorf("missing --create-vault name")
	}

	// Normalise entry type.
	var et vault.EntryType
	switch strings.ToLower(entryType) {
	case "", "secret", "secrets":
		et = vault.EntryTypeSecret
	case "env", "environment":
		et = vault.EntryTypeEnv
	case "note", "notes":
		et = vault.EntryTypeNote
	default:
		return fmt.Errorf("unsupported --type %q (use secrets|env|note)", entryType)
	}

	// Parse the env-style file.
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	entries := make([]vault.Entry, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Very simple KEY=VALUE parsing; no escaping for now.
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := parts[1]
		if key == "" {
			continue
		}

		e := vault.Entry{
			Key:      key,
			ValueDev: value,
			Type:     et,
		}

		switch strings.ToLower(duplicateTo) {
		case "prod", "production":
			e.ValueProd = value
		case "staging", "stage":
			e.ValueStage = value
		case "":
			// no duplication
		default:
			return fmt.Errorf("unsupported --duplicate-to %q (use staging|prod)", duplicateTo)
		}

		entries = append(entries, e)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Initialise storage/protector and persist the new vault.
	cfgDir, err := storage.ConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}

	prot, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		return fmt.Errorf("init protector: %w", err)
	}

	existing, err := storage.LoadVaults(prot)
	if err != nil {
		return fmt.Errorf("load existing vaults: %w", err)
	}

	svc := vault.NewService()
	if existing != nil {
		svc.SetAll(existing)
	}

	v := svc.CreateVault(name, "")
	v.Entries = entries
	// Persist back to disk.
	if err := storage.SaveVaults(prot, svc.ListVaults()); err != nil {
		return fmt.Errorf("save vaults: %w", err)
	}

	// Tell the agent to reload vaults from disk so it sees the new vault
	// without a restart (no-op if agent is not running).
	_ = triggerAgentReload()

	fmt.Printf("Created vault %q with %d entries from %s\n", name, len(entries), filePath)
	return nil
}

func triggerAgentReload() error {
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:5959/reload", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent reload: %s", resp.Status)
	}
	return nil
}

// deleteVaultByRef deletes a vault identified by name or ID from persistent storage.
// If skipConfirm is false, it will ask the user for explicit confirmation.
func deleteVaultByRef(ref string, skipConfirm bool) error {
	if ref == "" {
		return fmt.Errorf("missing vault reference to delete")
	}

	cfgDir, err := storage.ConfigDir()
	if err != nil {
		return fmt.Errorf("config dir: %w", err)
	}

	prot, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		return fmt.Errorf("init protector: %w", err)
	}

	existing, err := storage.LoadVaults(prot)
	if err != nil {
		return fmt.Errorf("load existing vaults: %w", err)
	}
	if len(existing) == 0 {
		return fmt.Errorf("no vaults found")
	}

	// Find the index of the vault by ID or Name.
	idx := -1
	for i, v := range existing {
		if v.ID == ref || v.Name == ref {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("vault %q not found", ref)
	}

	v := existing[idx]

	if !skipConfirm {
		fmt.Printf("Really delete vault %q (ID: %s)? This cannot be undone. [y/N]: ", v.Name, v.ID)
		reader := bufio.NewReader(os.Stdin)
		line, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("read confirmation: %w", err)
		}
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			fmt.Println("Aborted; vault not deleted.")
			return nil
		}
	}

	// Remove the vault from the slice.
	existing = append(existing[:idx], existing[idx+1:]...)

	if err := storage.SaveVaults(prot, existing); err != nil {
		return fmt.Errorf("save vaults: %w", err)
	}

	// Tell the agent to reload so it sees the deletion without restart.
	_ = triggerAgentReload()

	fmt.Printf("Deleted vault %q (ID: %s)\n", v.Name, v.ID)
	return nil
}


