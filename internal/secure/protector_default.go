//go:build !windows

package secure

// NewDefaultProtector returns the recommended Protector implementation for
// the current platform.
//
// On non-Windows platforms we prefer an OS-backed keyring-based protector and
// fall back to the SoftwareProtector if the keyring is not available.
//
// On Windows, a Windows-specific implementation is selected in
// protector_default_windows.go.
func NewDefaultProtector(configDir string) (Protector, error) {
	if p, err := NewKeyringProtector(); err == nil {
		return p, nil
	}
	return NewSoftwareProtector(configDir)
}

