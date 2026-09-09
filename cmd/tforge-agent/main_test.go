package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"tforge/internal/vault"
)

func TestIsLoopbackHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1:5959", true},
		{"localhost:5959", true},
		{"LOCALHOST:5959", true},
		{"[::1]:5959", true},
		{"127.0.0.1", true},
		{"localhost", true},

		{"", false},
		{"evil.com:5959", false},
		{"evil.com", false},
		// A rebinding target that resolves to loopback still carries the
		// attacker's hostname in the Host header.
		{"attacker.example:5959", false},
		{"192.168.1.10:5959", false},
		{"127.0.0.1:8080", false},
	}

	for _, c := range cases {
		if got := isLoopbackHost(c.host); got != c.want {
			t.Errorf("isLoopbackHost(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestOnlyLocalClientsRejectsBrowserAndForeignHosts(t *testing.T) {
	handler := onlyLocalClients(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	cases := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{"plain local client", "127.0.0.1:5959", nil, http.StatusOK},
		{"localhost", "localhost:5959", nil, http.StatusOK},
		{"dns rebinding", "evil.example:5959", nil, http.StatusForbidden},
		{"browser with origin", "127.0.0.1:5959", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"browser fetch metadata", "127.0.0.1:5959", map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-origin browser page", "127.0.0.1:5959", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusForbidden},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:5959/unlock", nil)
			req.Host = c.host
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != c.want {
				t.Errorf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func newTestAgent(t *testing.T, locked bool, entries []vault.Entry) *Agent {
	t.Helper()

	svc := vault.NewService()
	v := svc.CreateVault("TestVault", "")
	v.Entries = entries
	svc.UpdateVault(v)

	return &Agent{
		svc:          svc,
		locked:       locked,
		lastActivity: time.Now(),
	}
}

func TestHandleEnvRefusesWhileLocked(t *testing.T) {
	agent := newTestAgent(t, true, []vault.Entry{{Key: "SECRET", ValueDev: "shh"}})

	req := httptest.NewRequest(http.MethodGet, "/env?vault=TestVault&env=dev", nil)
	rec := httptest.NewRecorder()
	agent.handleEnv(rec, req)

	if rec.Code != http.StatusLocked {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusLocked)
	}
	if body := rec.Body.String(); strings.Contains(body, "shh") {
		t.Errorf("locked response leaked a secret value: %q", body)
	}
}

func TestHandleEnvReturnsValuesWhenUnlocked(t *testing.T) {
	agent := newTestAgent(t, false, []vault.Entry{
		{Key: "DB_HOST", ValueDev: "dev-host", ValueProd: "prod-host"},
		{Key: "ONLY_PROD", ValueProd: "prod-only"},
		{Key: "", ValueDev: "no-key"},
	})

	req := httptest.NewRequest(http.MethodGet, "/env?vault=TestVault&env=dev", nil)
	rec := httptest.NewRecorder()
	agent.handleEnv(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	var got envResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Env["DB_HOST"] != "dev-host" {
		t.Errorf("DB_HOST = %q, want dev-host", got.Env["DB_HOST"])
	}
	if _, ok := got.Env["ONLY_PROD"]; ok {
		t.Error("an entry with no dev value should be omitted from the dev env")
	}
	if _, ok := got.Env[""]; ok {
		t.Error("an entry with an empty key should be omitted")
	}
}

func TestHandleEnvRejectsNonGet(t *testing.T) {
	agent := newTestAgent(t, false, nil)

	req := httptest.NewRequest(http.MethodPost, "/env?vault=TestVault", nil)
	rec := httptest.NewRecorder()
	agent.handleEnv(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestBuildEnvForVaultSelectsEnvironment(t *testing.T) {
	v := &vault.Vault{Entries: []vault.Entry{
		{Key: "K", ValueDev: "d", ValueStage: "s", ValueProd: "p"},
	}}

	cases := map[string]string{
		"dev":      "d",
		"staging":  "s",
		"prod":     "p",
		"":         "d", // unknown targets fall back to dev
		"nonsense": "d",
	}

	for target, want := range cases {
		if got := buildEnvForVault(v, target)["K"]; got != want {
			t.Errorf("buildEnvForVault(%q)[K] = %q, want %q", target, got, want)
		}
	}
}

func TestLockStateIsRaceFree(t *testing.T) {
	agent := newTestAgent(t, true, nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			agent.setLocked(i%2 == 0)
			_ = agent.isLocked()
		}(i)
	}
	wg.Wait()
}
