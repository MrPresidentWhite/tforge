package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tforge/internal/storage"
)

// TestMain isolates the package from the real vault file.
//
// Individual tests already point the config directory at a temp dir, but the
// backup, restore, import and delete paths all reach storage directly, so a
// test that forgets to isolate would silently write to the developer's own
// vaults.bin. This makes forgetting harmless rather than destructive.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tforge-cli-test-")
	if err != nil {
		panic("cannot create a temporary config dir: " + err.Error())
	}

	for _, key := range []string{"AppData", "APPDATA", "XDG_CONFIG_HOME", "HOME"} {
		if err := os.Setenv(key, dir); err != nil {
			panic("cannot isolate " + key + ": " + err.Error())
		}
	}

	cfg, err := storage.ConfigDir()
	if err != nil {
		panic("cannot resolve the isolated config dir: " + err.Error())
	}
	if rel, err := filepath.Rel(dir, cfg); err != nil || strings.HasPrefix(rel, "..") {
		panic("config dir " + cfg + " is not inside " + dir +
			"; refusing to run tests that could overwrite real vault data")
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
