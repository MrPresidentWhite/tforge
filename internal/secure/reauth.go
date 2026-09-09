package secure

// RequireOSReauth enforces a short re-authentication step using the
// platform's native mechanisms (e.g. Windows Hello, macOS LocalAuthentication).
// Implementations are provided in the platform-specific reauth_*.go files.
