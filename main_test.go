package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tforge/internal/storage"
)

// TestMain isolates the whole package from the real vault file.
//
// Tests here construct an App and call methods that persist, and App.persistVaults
// goes straight to storage.SaveVaults, which resolves its path from the user's
// config directory. Without this, a single test that successfully creates a
// vault overwrites the developer's own vaults.bin -- which is exactly what
// happened once, sealed with a test stub, before this guard existed.
//
// Both spellings of the Windows variable are set. Only one of them is read,
// but relying on which is a needless risk when the cost of setting both is
// nothing.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "tforge-app-test-")
	if err != nil {
		panic("cannot create a temporary config dir: " + err.Error())
	}

	for _, key := range []string{"AppData", "APPDATA", "XDG_CONFIG_HOME", "HOME"} {
		if err := os.Setenv(key, dir); err != nil {
			panic("cannot isolate " + key + ": " + err.Error())
		}
	}

	// Refuse to run rather than risk writing to a real vault file.
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
