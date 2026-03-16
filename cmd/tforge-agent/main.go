package main

import (
	"encoding/json"
	"log"
	"net/http"

	"tforge/internal/secure"
	"tforge/internal/storage"
	"tforge/internal/vault"
)

// Agent holds in-memory vault service and crypto protector.
type Agent struct {
	svc       *vault.Service
	protector secure.Protector
}

func main() {
	cfgDir, err := storage.ConfigDir()
	if err != nil {
		log.Fatalf("config dir: %v", err)
	}

	protector, err := secure.NewDefaultProtector(cfgDir)
	if err != nil {
		log.Fatalf("protector init: %v", err)
	}

	agent := &Agent{
		svc:       vault.NewService(),
		protector: protector,
	}

	// Load existing vaults.
	vaults, err := storage.LoadVaults(protector)
	if err != nil {
		log.Fatalf("load vaults: %v", err)
	}
	if vaults != nil {
		agent.svc.SetAll(vaults)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", agent.handleHealth)
	mux.HandleFunc("/env", agent.handleEnv)

	server := &http.Server{
		Addr:    "127.0.0.1:5959",
		Handler: mux,
	}

	log.Println("tforge-agent listening on http://127.0.0.1:5959")
	log.Fatal(server.ListenAndServe())
}

func (a *Agent) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

type envResponse struct {
	Env map[string]string `json:"env"`
}

func (a *Agent) handleEnv(w http.ResponseWriter, r *http.Request) {
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

