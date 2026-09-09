//go:build !windows

package secure

import "log"

// NewDefaultProtector returns the recommended Protector implementation for
// the current platform.
//
// On non-Windows platforms we prefer an OS-backed keyring-based protector and
// fall back to the SoftwareProtector if the keyring is not available.
//
// The fallback is a security downgrade: it writes a master.key file next to
// the vault instead of keeping the key in the OS keyring, so it is logged
// rather than applied silently. On a headless machine without a Secret Service
// implementation this is the expected path.
//
// On Windows, a Windows-specific implementation is selected in
// protector_default_windows.go.
func NewDefaultProtector(configDir string) (Protector, error) {
	p, err := NewKeyringProtector()
	if err == nil {
		return p, nil
	}

	log.Printf("OS keyring unavailable (%v); falling back to a local master.key file", err)
	return NewSoftwareProtector(configDir)
}
