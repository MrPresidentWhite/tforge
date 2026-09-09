//go:build windows

package secure

import "log"

// NewDefaultProtector returns the recommended Protector on Windows.
// It prefers a DPAPI-backed implementation and falls back to the
// software-based protector if DPAPI is not available.
//
// The fallback is a security downgrade: it writes a master.key file next to
// the vault instead of delegating key protection to the OS, so it is logged
// rather than applied silently.
func NewDefaultProtector(configDir string) (Protector, error) {
	p, err := NewDPAPIProtector()
	if err == nil {
		return p, nil
	}

	log.Printf("DPAPI unavailable (%v); falling back to a local master.key file", err)
	return NewSoftwareProtector(configDir)
}
