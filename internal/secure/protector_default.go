//go:build !windows

package secure

// NewDefaultProtector returns the recommended Protector implementation for
// the current platform. On non-Windows platforms this is currently the
// SoftwareProtector.
//
// On Windows, a Windows-specific implementation is selected in
// protector_default_windows.go.
func NewDefaultProtector(configDir string) (Protector, error) {
	return NewSoftwareProtector(configDir)
}

