package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
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

	resp, err = client.Get(testServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics without token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /metrics without token status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
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

	req, err = http.NewRequest(http.MethodGet, testServer.URL+"/metrics", nil)
	if err != nil {
		t.Fatalf("NewRequest(authenticated metrics) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /metrics with token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics with token status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestMountedServerMetricsRemainLocalWithoutToken(t *testing.T) {
	t.Parallel()

	testServer := newMountedTestServer(t, "")
	defer testServer.Close()

	resp, err := testServer.Client().Get(testServer.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestMountedServerAllowsForwardedHTTPSBrowserFlowWithTokenAuth(t *testing.T) {
	t.Parallel()

	const (
		apiToken   = "integration-token"
		publicHost = "app.example"
		publicURL  = "https://" + publicHost
	)

	testServer := newMountedTestServer(t, apiToken)
	defer testServer.Close()

	client := testServer.Client()

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/signup", nil)
	if err != nil {
		t.Fatalf("NewRequest(GET /signup) error = %v", err)
	}
	req.Host = publicHost
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("X-Forwarded-Proto", "https")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /signup via forwarded https error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /signup via forwarded https status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	csrfCookie := findCookie(resp.Cookies(), "xleaks_csrf")
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("expected xleaks_csrf cookie from forwarded https web route")
	}
	if !csrfCookie.Secure {
		t.Fatal("expected forwarded https csrf cookie to be secure")
	}

	createBody := `{"passphrase":"correct horse battery staple"}`
	req, err = http.NewRequest(http.MethodPost, testServer.URL+"/api/identity/create", strings.NewReader(createBody))
	if err != nil {
		t.Fatalf("NewRequest(POST /api/identity/create) error = %v", err)
	}
	req.Host = publicHost
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", publicURL)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)
	req.AddCookie(&http.Cookie{Name: "xleaks_csrf", Value: csrfCookie.Value})

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/identity/create via forwarded https error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /api/identity/create via forwarded https status = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestMountedServerWebSocketTicketFlowWithTokenAuth(t *testing.T) {
	t.Parallel()

	const apiToken = "integration-token"
	testServer := newMountedTestServerWithWebSocket(t, apiToken, true)
	defer testServer.Close()

	client := testServer.Client()

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/signup", nil)
	if err != nil {
		t.Fatalf("NewRequest(GET /signup) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /signup error = %v", err)
	}
	defer resp.Body.Close()

	csrfCookie := findCookie(resp.Cookies(), "xleaks_csrf")
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("expected csrf cookie before requesting websocket ticket")
	}

	req, err = http.NewRequest(http.MethodPost, testServer.URL+"/api/ws-ticket", nil)
	if err != nil {
		t.Fatalf("NewRequest(POST /api/ws-ticket) error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("Origin", testServer.URL)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/ws-ticket error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ws-ticket status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var ticketResp struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ticketResp); err != nil {
		t.Fatalf("decode websocket ticket response error = %v", err)
	}
	if ticketResp.Ticket == "" {
		t.Fatal("expected websocket ticket in response")
	}

	wsURL := "ws" + strings.TrimPrefix(testServer.URL, "http") + "/ws?ws_ticket=" + url.QueryEscape(ticketResp.Ticket)
	header := http.Header{}
	header.Set("Origin", testServer.URL)

	conn, wsResp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		if wsResp != nil {
			t.Fatalf("Dial websocket error = %v, status = %d", err, wsResp.StatusCode)
		}
		t.Fatalf("Dial websocket error = %v", err)
	}
	_ = conn.Close()

	_, wsResp, err = websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Fatal("expected one-time websocket ticket replay to fail")
	}
	if wsResp == nil {
		t.Fatal("expected HTTP response for failed websocket replay")
	}
	if wsResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("replayed websocket ticket status = %d, want %d", wsResp.StatusCode, http.StatusUnauthorized)
	}
}

func TestMountedServerBrowserAuthBootstrapSupportsProtectedWebUIAndAPI(t *testing.T) {
	t.Parallel()

	const apiToken = "integration-token"
	testServer := newMountedTestServerWithWebSocket(t, apiToken, true)
	defer testServer.Close()

	client := testServer.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/", nil)
	if err != nil {
		t.Fatalf("NewRequest(GET /) error = %v", err)
	}
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET / status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if got := resp.Header.Get("Location"); got != "/auth/token?next=%2F" {
		t.Fatalf("GET / Location = %q, want %q", got, "/auth/token?next=%2F")
	}

	resp, err = client.Get(testServer.URL + "/auth/token?next=%2Fsignup")
	if err != nil {
		t.Fatalf("GET /auth/token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /auth/token status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	form := url.Values{
		"token": []string{apiToken},
		"next":  []string{"/signup"},
	}
	resp, err = client.PostForm(testServer.URL+"/auth/token", form)
	if err != nil {
		t.Fatalf("POST /auth/token error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /auth/token status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if got := resp.Header.Get("Location"); got != "/signup" {
		t.Fatalf("POST /auth/token Location = %q, want %q", got, "/signup")
	}

	resp, err = client.Get(testServer.URL + "/signup")
	if err != nil {
		t.Fatalf("GET /signup after browser auth error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /signup after browser auth status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	csrfCookie := findCookie(resp.Cookies(), "xleaks_csrf")
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("expected csrf cookie from browser-authenticated signup page")
	}

	resp, err = client.Get(testServer.URL + "/api/node/status")
	if err != nil {
		t.Fatalf("GET /api/node/status with browser auth error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/node/status with browser auth status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req, err = http.NewRequest(http.MethodPost, testServer.URL+"/api/ws-ticket", nil)
	if err != nil {
		t.Fatalf("NewRequest(POST /api/ws-ticket) error = %v", err)
	}
	req.Header.Set("Origin", testServer.URL)
	req.Header.Set("X-CSRF-Token", csrfCookie.Value)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /api/ws-ticket with browser auth error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ws-ticket with browser auth status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestMountedServerBrowserAuthLoginRateLimited(t *testing.T) {
	t.Parallel()

	const apiToken = "integration-token"
	testServer := newMountedTestServerWithWebSocket(t, apiToken, false)
	defer testServer.Close()

	client := testServer.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	for i := 0; i < 5; i++ {
		resp, err := client.PostForm(testServer.URL+"/auth/token", url.Values{
			"token": []string{"wrong-token"},
			"next":  []string{"/"},
		})
		if err != nil {
			t.Fatalf("POST /auth/token attempt %d error = %v", i+1, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST /auth/token attempt %d status = %d, want %d", i+1, resp.StatusCode, http.StatusSeeOther)
		}
	}

	resp, err := client.PostForm(testServer.URL+"/auth/token", url.Values{
		"token": []string{"wrong-token"},
		"next":  []string{"/"},
	})
	if err != nil {
		t.Fatalf("POST /auth/token rate-limited attempt error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("POST /auth/token rate-limited status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
	}
}

func TestMountedServerBrowserAuthLogoutRevokesAccessAndAcceptsSameOriginWebForm(t *testing.T) {
	t.Parallel()

	const apiToken = "integration-token"
	testServer := newMountedTestServerWithWebSocket(t, apiToken, false)
	defer testServer.Close()

	client := testServer.Client()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	resp, err := client.PostForm(testServer.URL+"/auth/token", url.Values{
		"token": []string{apiToken},
		"next":  []string{"/signup"},
	})
	if err != nil {
		t.Fatalf("POST /auth/token error = %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /auth/token status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}

	resp, err = client.Get(testServer.URL + "/signup")
	if err != nil {
		t.Fatalf("GET /signup error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /signup status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	csrfCookie := findCookie(resp.Cookies(), "xleaks_csrf")
	if csrfCookie == nil || csrfCookie.Value == "" {
		t.Fatal("expected csrf cookie from signup page")
	}

	logoutBody := strings.NewReader(url.Values{
		"csrf_token": []string{csrfCookie.Value},
	}.Encode())
	req, err := http.NewRequest(http.MethodPost, testServer.URL+"/logout", logoutBody)
	if err != nil {
		t.Fatalf("NewRequest(POST /logout) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", testServer.URL)
	req.Header.Set("Referer", testServer.URL+"/signup")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /logout error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /logout status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if got := resp.Header.Get("Location"); got != "/auth/token?error=logged_out&next=%2F" {
		t.Fatalf("POST /logout Location = %q, want %q", got, "/auth/token?error=logged_out&next=%2F")
	}

	browserAuthCookie := findCookie(resp.Cookies(), "xleaks_browser_auth")
	if browserAuthCookie == nil {
		t.Fatal("expected cleared browser auth cookie on logout")
	}
	if browserAuthCookie.MaxAge != -1 {
		t.Fatalf("browser auth cookie MaxAge = %d, want %d", browserAuthCookie.MaxAge, -1)
	}

	resp, err = client.Get(testServer.URL + "/api/node/status")
	if err != nil {
		t.Fatalf("GET /api/node/status after logout error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/node/status after logout status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}

	req, err = http.NewRequest(http.MethodGet, testServer.URL+"/signup", nil)
	if err != nil {
		t.Fatalf("NewRequest(GET /signup) error = %v", err)
	}
	req.Header.Set("Accept", "text/html")

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET /signup after logout error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("GET /signup after logout status = %d, want %d", resp.StatusCode, http.StatusSeeOther)
	}
	if got := resp.Header.Get("Location"); got != "/auth/token?next=%2Fsignup" {
		t.Fatalf("GET /signup after logout Location = %q, want %q", got, "/auth/token?next=%2Fsignup")
	}
}

func newMountedTestServer(t *testing.T, apiToken string) *httptest.Server {
	return newMountedTestServerWithWebSocket(t, apiToken, false)
}

func newMountedTestServerWithWebSocket(t *testing.T, apiToken string, enableWebSocket bool) *httptest.Server {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	cfg := config.DefaultConfig()
	cfg.Node.DataDir = t.TempDir()
	cfg.API.ListenAddress = "127.0.0.1:0"
	cfg.API.EnableWebSocket = enableWebSocket
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
	deps := buildAPIDeps(db, cas, kp, idHolder, svc, nil, cfg, cfgPath, webRoutes, apiToken != "", func(*identity.KeyPair) {}, nil)
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
