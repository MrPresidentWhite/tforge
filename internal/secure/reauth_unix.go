//go:build !windows && !darwin

package secure

// RequireOSReauth on generic Unix (e.g. Linux, *BSD) currently does not
// integrate with any specific desktop authentication mechanism. It simply
// succeeds so that the higher-level lock/unlock flow works without
// additional prompts on these platforms for now.
func RequireOSReauth() error {
	return nil
}

