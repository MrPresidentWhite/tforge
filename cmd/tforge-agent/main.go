package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fsnotify "github.com/fsnotify/fsnotify"

	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// Agent holds in-memory vault service and crypto protector.
type Agent struct {
	svc       *vault.Service
	protector secure.Protector

	mu           sync.RWMutex
	locked       bool
	lastActivity time.Time
	timeout      time.Duration
}

// listenAddr is the loopback address the agent binds to. It is deliberately
// not configurable: the CLI and the security model both assume loopback only.
const listenAddr = "127.0.0.1:5959"

func main() {
	lockTimeout := flag.Duration("lock-timeout", 0, "inactivity timeout before the agent auto-locks (0 = disabled)")
	flag.Parse()

	cfgDir, err := storage.ConfigDir()
	if err != nil {
		log.Fatalf("config dir: %v", err)
	}

	protector, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		log.Fatalf("protector init: %v", err)
	}

	agent := &Agent{
		svc:          vault.NewService(),
		protector:    protector,
		timeout:      *lockTimeout,
		lastActivity: time.Now(),
		locked:       true, // start locked by default; requires explicit unlock
	}

	agent.startInactivityWatcher()

	// Load existing vaults.
	vaults, err := storage.LoadVaults(protector)
	if err != nil {
		log.Fatalf("load vaults: %v", err)
	}
	if vaults != nil {
		agent.svc.SetAll(vaults)
	}

	agent.startVaultWatcher(cfgDir)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", agent.handleHealth)
	mux.HandleFunc("/env", agent.handleEnv)
	mux.HandleFunc("/lock", agent.handleLock)
	mux.HandleFunc("/unlock", agent.handleUnlock)
	mux.HandleFunc("/status", agent.handleStatus)
	mux.HandleFunc("/reload", agent.handleReload)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: onlyLocalClients(mux),
		// Without these a single stalled connection can hold a handler
		// goroutine open indefinitely.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("tforge-agent listening on http://%s", listenAddr)
	log.Fatal(server.ListenAndServe())
}

// onlyLocalClients rejects requests that did not come from a local, non-browser
// client. Binding to loopback alone is not enough: any web page the user visits
// can send a request to 127.0.0.1 from their browser. Without this guard, a
// malicious page could POST /unlock and pop a Windows Hello prompt, and a DNS
// rebinding attack could go on to read /env.
//
// Two checks cover both cases:
//   - a Host header that is not our loopback address means the request was
//     addressed to some other name that happens to resolve here (rebinding),
//   - an Origin or Sec-Fetch-Site header means a browser sent it; nothing that
//     legitimately talks to the agent runs in a browser.
func onlyLocalClients(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackHost(r.Host) {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			http.Error(w, "browser requests are not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLoopbackHost reports whether a request's Host header names this agent on
// the loopback interface.
func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}

	hostname, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port in the header; treat the whole value as the hostname.
		hostname = host
		port = ""
	}
	if port != "" && port != agentPort() {
		return false
	}

	if strings.EqualFold(hostname, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(hostname, "[]"))
	return ip != nil && ip.IsLoopback()
}

func agentPort() string {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return ""
	}
	return port
}

func (a *Agent) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.touchActivity()
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleLock puts the agent into a locked state where env access is disabled.
// Locking never requires re-authentication; only unlocking does.
func (a *Agent) handleLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.touchActivity()
	a.setLocked(true)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("locked"))
}

// handleUnlock returns the agent to the unlocked state so env access works
// again. It enforces a short OS-level re-authentication step via the
// secure.RequireOSReauth hook before actually unlocking.
func (a *Agent) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.touchActivity()

	if err := secure.RequireOSReauth(); err != nil {
		http.Error(w, "re-auth failed: "+err.Error(), http.StatusUnauthorized)
		return
	}

	a.setLocked(false)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("unlocked"))
}

func (a *Agent) touchActivity() {
	if a.timeout <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastActivity = time.Now()
}

func (a *Agent) startInactivityWatcher() {
	if a.timeout <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for range ticker.C {
			a.mu.Lock()
			relocked := false
			if !a.locked && !a.lastActivity.IsZero() && time.Since(a.lastActivity) > a.timeout {
				a.locked = true
				relocked = true
			}
			a.mu.Unlock()

			if relocked {
				log.Printf("auto-locked after %s of inactivity", a.timeout)
			}
		}
	}()
}

// startVaultWatcher watches the vaults.bin file for changes and reloads the
// in-memory vault list when it changes. This keeps the agent in sync with
// external writers (e.g. GUI or CLI import) without requiring a restart.
func (a *Agent) startVaultWatcher(cfgDir string) {
	vaultPath := filepath.Join(cfgDir, "vaults.bin")
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("vault watcher init error: %v", err)
		return
	}

	dir := filepath.Dir(vaultPath)
	if err := watcher.Add(dir); err != nil {
		log.Printf("vault watcher add error: %v", err)
		_ = watcher.Close()
		return
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if ev.Name == vaultPath && (ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) != 0) {
					if err := a.reloadVaultsFromDisk(); err != nil && !errors.Is(err, errNoVaultFile) {
						log.Printf("vault reload error: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("vault watcher error: %v", err)
			}
		}
	}()
}

func (a *Agent) setLocked(v bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.locked = v
}

func (a *Agent) isLocked() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.locked
}

type envResponse struct {
	Env map[string]string `json:"env"`
}

type statusResponse struct {
	Locked         bool  `json:"locked"`
	TimeoutSeconds int64 `json:"timeoutSeconds"`
}

// reloadVaultsFromDisk loads vaults from disk and replaces the in-memory state.
//
// A missing vault file is reported as an error rather than applied: LoadVaults
// returns (nil, nil) in that case, and blindly applying it would silently drop
// every vault the agent still holds, for example while the file is being
// replaced or if it was removed by accident.
func (a *Agent) reloadVaultsFromDisk() error {
	vaults, err := storage.LoadVaults(a.protector)
	if err != nil {
		return err
	}
	if vaults == nil {
		return errNoVaultFile
	}
	a.svc.SetAll(vaults)
	return nil
}

// errNoVaultFile signals that there is currently no vault file on disk.
var errNoVaultFile = errors.New("no vault file on disk; keeping current state")

// handleReload re-reads vaults from disk and replaces the in-memory state.
// Useful after the CLI (or another process) has created or updated vaults
// so the agent sees them without a restart.
func (a *Agent) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.touchActivity()

	if err := a.reloadVaultsFromDisk(); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNoVaultFile) {
			status = http.StatusNotFound
		}
		http.Error(w, "reload: "+err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("reloaded"))
}

func (a *Agent) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.mu.RLock()
	locked := a.locked
	var timeoutSeconds int64
	if a.timeout > 0 {
		timeoutSeconds = int64(a.timeout.Seconds())
	}
	a.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(statusResponse{
		Locked:         locked,
		TimeoutSeconds: timeoutSeconds,
	}); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
}

func (a *Agent) handleEnv(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	a.touchActivity()
	if a.isLocked() {
		http.Error(w, "agent is locked; env access disabled", http.StatusLocked)
		return
	}

	q := r.URL.Query()
	vaultRef := q.Get("vault")
	if vaultRef == "" {
		http.Error(w, "missing vault parameter", http.StatusBadRequest)
		return
	}
	target := q.Get("env")
	if target == "" {
		target = "dev"
	}

	v := findVault(a.svc, vaultRef)
	if v == nil {
		http.Error(w, "vault not found", http.StatusNotFound)
		return
	}

	env := buildEnvForVault(v, target)

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(envResponse{Env: env}); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
}

// findVault resolves by ID or name (first match).
func findVault(svc *vault.Service, ref string) *vault.Vault {
	for _, v := range svc.ListVaults() {
		if v.ID == ref || v.Name == ref {
			return v
		}
	}
	return nil
}

// buildEnvForVault converts a vault into an env map for given environment.
func buildEnvForVault(v *vault.Vault, target string) map[string]string {
	env := make(map[string]string)
	for _, e := range v.Entries {
		if e.Key == "" {
			continue
		}

		var val string
		switch target {
		case "prod":
			val = e.ValueProd
		case "staging":
			val = e.ValueStage
		default:
			val = e.ValueDev
		}

		if val == "" {
			continue
		}

		env[e.Key] = val
	}
	return env
}
