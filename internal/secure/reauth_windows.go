//go:build windows

package secure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// reauthTimeout bounds how long we wait for the user to answer the Windows
// Hello prompt. Without it a prompt nobody dismisses would block the agent's
// unlock handler, and with it the connection, forever.
const reauthTimeout = 60 * time.Second

// helperEnvVar names an explicit path to the Windows Hello helper. It exists
// for development, where the agent runs via `go run` from a temporary build
// directory and therefore has no helper next to its executable.
const helperEnvVar = "TFORGE_HELLO_HELPER"

// RequireOSReauth triggers a short Windows re-authentication (Windows Hello /
// OS login) by invoking the helper executable that ships next to the
// tforge-agent binary. The helper returns exit code 0 on successful
// verification; any non-zero exit code, timeout or execution error is treated
// as a failed or cancelled re-auth.
func RequireOSReauth() error {
	helperPath, err := findHelper()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), reauthTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, helperPath)
	// The helper renders its own OS dialog and needs no IO of its own.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("windows re-auth timed out after %s", reauthTimeout)
		}
		return fmt.Errorf("windows re-auth helper failed: %w", err)
	}
	return nil
}

// findHelper locates the Windows Hello helper.
//
// The search is deliberately limited to the agent's own directory plus an
// explicit environment override. It must never fall back to the working
// directory: the helper's exit code is what authorises unlocking the vault, so
// picking one up from wherever the agent happens to have been started would let
// anyone who can write to that directory plant a binary that exits 0 and
// bypass re-authentication entirely.
func findHelper() (string, error) {
	if override := os.Getenv(helperEnvVar); override != "" {
		if isExecutableFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("%s points to %q, which is not an executable file", helperEnvVar, override)
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve agent executable: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		// Installation layout: helper next to the agent.
		filepath.Join(exeDir, "tforge-hello-helper.exe"),
		// Repository layout: helper in a helper-bin subdirectory.
		filepath.Join(exeDir, "helper-bin", "tforge-hello-helper.exe"),
	}

	for _, c := range candidates {
		if isExecutableFile(c) {
			return c, nil
		}
	}

	return "", fmt.Errorf(
		"windows hello helper not found next to the agent (looked in %s); "+
			"build it with build-hello-helper.ps1, or set %s to its path",
		exeDir, helperEnvVar,
	)
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
