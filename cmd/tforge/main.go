package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

type envResponse struct {
	Env map[string]string `json:"env"`
}

func main() {
	envFlag := flag.String("env", "dev", "environment to use (dev|staging|prod)")
	exportMode := flag.Bool("export", false, "print env as KEY=VALUE lines instead of running a command")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		log.Fatalf("usage: tforge [--env dev|staging|prod] @VaultName [command ...]")
	}

	// Extract vault reference (strip optional leading "@").
	vaultRef := strings.TrimPrefix(args[0], "@")

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

