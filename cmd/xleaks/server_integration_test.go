package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/api"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/identity"
)

func TestMountedServerSharesWebCSRFCookieWithAPI(t *testing.T) {
	t.Parallel()

	testServer := newMountedTestServer(t, "")
	defer testServer.Close()

	client := testServer.Client()

	resp, err := client.Get(testServer.URL + "/signup")
	if err != nil {
		t.Fatalf("GET /signup error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /signup status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Cache-Control"); got != "no-store, max-age=0" {
		t.Fatalf("GET /signup Cache-Control = %q, want %q", got, "no-store, max-age=0")
	}
	if got := resp.Header.Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("GET /signup X-Frame-Options = %q, want %q", got, "DENY")
	}

	csrfCookie := findCookie(resp.Cookies(), "xleaks_csrf")
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("expected xleaks_csrf cookie from web route")
	}

	createBody := `{"passphrase":"correct horse battery staple"}`

	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/api/identity/create", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("NewRequest(missing csrf) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testServer.URL)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/identity/create without csrf error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST /api/identity/create without csrf status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}

	req, err = http.NewRequest(http.MethodPost, testServer.URL+"/api/identity/create", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("NewRequest(valid csrf) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testServer.URL)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/identity/create with csrf error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/identity/create with csrf status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}

	var created struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response error = %v", err)
	}
	if created.Pubkey == "" {
		t.Fatal("expected created pubkey in create response")
	}

	resp, err = client.Get(testServer.URL + "/api/identity/active")
	if err != nil {
		t.Fatalf("GET /api/identity/active error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/identity/active status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var active struct {
		Active bool   `json:"active"`
		Pubkey string `json:"pubkey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&active); err != nil {
		t.Fatalf("decode active response error = %v", err)
	}
	if !active.Active {
		t.Fatal("expected active identity after mounted create flow")
	}
	if active.Pubkey != created.Pubkey {
		t.Fatalf("active pubkey = %q, want %q", active.Pubkey, created.Pubkey)
	}
}

func TestMountedServerHealthBypassesTokenAuthButAPIRequiresIt(t *testing.T) {
	t.Parallel()

	const apiToken = "integration-token"
	testServer := newMountedTestServer(t, apiToken)
	defer testServer.Close()

	client := testServer.Client()

	resp, err := client.Get(testServer.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = client.Get(testServer.URL + "/api/node/status")
	if err != nil {
		t.Fatalf("GET /api/node/status without token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/node/status without token status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/node/status", nil)
	if err != nil {
		t.Fatalf("NewRequest(authenticated status) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/node/status with token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/node/status with token status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func newMountedTestServer(t *testing.T, apiToken string) *httptest.Server {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := config.DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	cfg.API.ListenAddress = "127.0.0.1:0"
	cfg.API.EnableWebSocket = false
	cfg.Network.EnableMDNS = false
	cfg.Network.EnableRelay = false
	cfg.Network.EnableHolePunching = false
	cfg.Network.ListenAddresses = nil
	cfg.Network.BootstrapPeers = nil
	cfg.Indexer.KnownIndexers = nil
	cfg.Logging.File = filepath.Join(cfg.Node.DataDir, "logs", "xleaks.log")

	db, cas, err := setupDatabase(cfg)
	if err != nil {
		t.Fatalf("setupDatabase() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	idHolder, kp := setupIdentity(cfg.DataDir(), db)
	svc := setupServices(ctx, db, cas, kp, idHolder)
	webRoutes := setupWebHandler(db, idHolder, svc, cfg, nil, cfg.DataDir(), nil, func(*identity.KeyPair) {}, nil)
	cfgPath := filepath.Join(cfg.DataDir(), "config.toml")
	deps := buildAPIDeps(db, cas, kp, idHolder, svc, nil, cfg, cfgPath, webRoutes, func(*identity.KeyPair) {}, nil)
	server := api.NewServerWithConfig(api.ServerConfig{
		ListenAddr:      cfg.API.ListenAddress,
		APIToken:        apiToken,
		EnableWebSocket: cfg.API.EnableWebSocket,
	}, deps)

	handler := server.Handler()
	if handler == nil {
		t.Fatal("expected server handler")
	}

	ts := httptest.NewServer(handler)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New() error = %v", err)
	}
	ts.Client().Jar = jar
	return ts
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
