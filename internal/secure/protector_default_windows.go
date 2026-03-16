//go:build windows

package secure

// NewDefaultProtector returns the recommended Protector on Windows.
// It prefers a DPAPI-backed implementation and falls back to the
// software-based protector if DPAPI is not available.
//
// Die eigentliche Migration von alten, mit master.key verschlüsselten
// Vaults erfolgt ausschließlich über das separate Tool
// `tforge-migrate-vaults` und nicht mehr in der Hauptanwendung.
func NewDefaultProtector(configDir string) (Protector, error) {
	if p, err := NewDPAPIProtector(); err == nil {
		return p, nil
	}
	// Fallback: reine Software-basierte Verschlüsselung.
	return NewSoftwareProtector(configDir)
}

