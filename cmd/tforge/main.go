package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// agentBaseURL is where the local tforge-agent listens. The agent binds to
// loopback only, so this is not configurable on either side.
const agentBaseURL = "http://127.0.0.1:5959"

// agentClient talks to the local agent. A timeout matters even on loopback:
// an unlock request can sit waiting for a Windows Hello prompt the user never
// answers, and without this the CLI would hang forever.
var agentClient = &http.Client{Timeout: 2 * time.Minute}

type envResponse struct {
	Env map[string]string `json:"env"`
}

type statusResponse struct {
	Locked         bool  `json:"locked"`
	TimeoutSeconds int64 `json:"timeoutSeconds"`
}

func main() {
	envFlag := flag.String("env", "dev", "environment to use (dev|staging|prod)")
	exportMode := flag.Bool("export", false, "print env as KEY=VALUE lines instead of running a command")
	exportFormat := flag.String("export-format", "raw", "how --export quotes values (raw|shell); use shell for `eval $(tforge --export ...)`")

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

	// Ensure the agent is unlocked (may trigger an OS re-auth prompt on
	// supported platforms) before requesting env data.
	if err := ensureAgentUnlocked(); err != nil {
		log.Fatalf("unlock agent: %v", err)
	}

	envMap, err := fetchEnvFromAgent(vaultRef, *envFlag)
	if err != nil {
		log.Fatalf("fetch env from agent: %v", err)
	}

	if *exportMode {
		out, err := formatExport(envMap, *exportFormat)
		if err != nil {
			log.Fatalf("export: %v", err)
		}
		fmt.Print(out)
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
	resp, err := agentClient.Get(agentBaseURL + "/env?" + q.Encode())
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

	// Reject an unknown --duplicate-to before doing any work, so the user is
	// not told about it only after the file has been read.
	switch strings.ToLower(duplicateTo) {
	case "", "prod", "production", "staging", "stage":
	default:
		return fmt.Errorf("unsupported --duplicate-to %q (use staging|prod)", duplicateTo)
	}

	// Parse the env-style file.
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	pairs, err := parseEnvFile(f)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	entries := make([]vault.Entry, 0, len(pairs))
	for _, kv := range pairs {
		e := vault.Entry{
			Key:      kv.Key,
			ValueDev: kv.Value,
			Type:     et,
		}

		switch strings.ToLower(duplicateTo) {
		case "prod", "production":
			e.ValueProd = kv.Value
		case "staging", "stage":
			e.ValueStage = kv.Value
		}

		entries = append(entries, e)
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

	// The agent resolves a vault reference by ID *or* name and takes the first
	// match, so two vaults sharing a name make `tforge @Name` ambiguous.
	for _, ev := range existing {
		if ev != nil && ev.Name == name {
			return fmt.Errorf("a vault named %q already exists (ID %s); pick another name", name, ev.ID)
		}
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
	req, err := http.NewRequest(http.MethodPost, agentBaseURL+"/reload", nil)
	if err != nil {
		return err
	}
	resp, err := agentClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent reload: %s", resp.Status)
	}
	return nil
}

// ensureAgentUnlocked unlocks the local agent if, and only if, it is currently
// locked. The status check is what keeps this cheap: unlocking triggers an
// OS re-authentication prompt (Windows Hello, Touch ID), and sending it
// unconditionally would ask the user to authenticate on every single command
// even though the agent was already unlocked.
func ensureAgentUnlocked() error {
	locked, err := agentLocked()
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}

	req, err := http.NewRequest(http.MethodPost, agentBaseURL+"/unlock", nil)
	if err != nil {
		return err
	}
	resp, err := agentClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent unlock: %s", resp.Status)
	}
	return nil
}

// agentLocked reports whether the agent currently refuses env access.
func agentLocked() (bool, error) {
	resp, err := agentClient.Get(agentBaseURL + "/status")
	if err != nil {
		return false, fmt.Errorf("connect agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("agent status: %s", resp.Status)
	}

	var sr statusResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return false, fmt.Errorf("decode status: %w", err)
	}
	return sr.Locked, nil
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

// envPair is one KEY=VALUE assignment read from an env-style file.
type envPair struct {
	Key   string
	Value string
}

// parseEnvFile reads env-style input (KEY=VALUE, # comments) and returns the
// pairs in file order. Beyond the original minimal parsing it also handles the
// three things real .env files almost always contain:
//
//   - a leading "export " on the key,
//   - values wrapped in matching single or double quotes,
//   - surrounding whitespace around unquoted values.
//
// When a key appears more than once the last assignment wins, which matches
// how shells and dotenv loaders behave.
func parseEnvFile(r io.Reader) ([]envPair, error) {
	var pairs []envPair
	indexByKey := make(map[string]int)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
		if key == "" {
			continue
		}

		value = unquoteEnvValue(value)

		if i, ok := indexByKey[key]; ok {
			pairs[i].Value = value
			continue
		}
		indexByKey[key] = len(pairs)
		pairs = append(pairs, envPair{Key: key, Value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return pairs, nil
}

// unquoteEnvValue trims a value and removes one layer of matching quotes.
// Whitespace inside quotes is significant and is preserved.
func unquoteEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// formatExport renders the env map for --export.
//
// "raw" prints plain KEY=VALUE lines. "shell" single-quotes each value so the
// output survives `eval $(tforge --export ...)`: without quoting, a value
// containing a space, newline or semicolon would be split by the shell or, in
// the worst case, executed as a command of its own.
//
// Keys are sorted in both modes; Go randomises map iteration order, so the
// output was previously different on every run.
func formatExport(env map[string]string, format string) (string, error) {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	switch strings.ToLower(format) {
	case "", "raw":
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, env[k])
		}
	case "shell":
		for _, k := range keys {
			fmt.Fprintf(&b, "%s=%s\n", k, shellQuote(env[k]))
		}
	default:
		return "", fmt.Errorf("unsupported --export-format %q (use raw|shell)", format)
	}
	return b.String(), nil
}

// shellQuote wraps s in single quotes for POSIX shells, escaping any single
// quote it contains using the standard POSIX close-escape-reopen idiom.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
