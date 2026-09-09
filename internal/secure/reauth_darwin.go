//go:build darwin

package secure

// RequireOSReauth on macOS is intended to use the LocalAuthentication
// framework (Touch ID / password) in a follow-up iteration. For now
// it is a stub that always succeeds so we can evolve the flow without
// breaking the CLI/agent integration.
func RequireOSReauth() error {
	return nil
}

