package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBrowserGuardRejectsCrossOriginRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:7470/api/node/config", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("cross-origin request should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestBrowserGuardAllowsUnsafeCLIRequestWithoutBrowserHeaders(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPut, "http://127.0.0.1:7470/api/node/config", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	called := false

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected non-browser request to reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBrowserGuardRejectsSameOriginUnsafeRequestWithoutCSRF(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/api/media", nil)
	req.Header.Set("Origin", "http://127.0.0.1:7470")
	rr := httptest.NewRecorder()

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("missing-csrf request should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestBrowserGuardAllowsSameOriginUnsafeRequestWithCSRF(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("a", browserCSRFTokenLen*2)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/api/media", nil)
	req.Header.Set("Origin", "http://127.0.0.1:7470")
	req.Header.Set(browserCSRFHeaderName, token)
	req.AddCookie(&http.Cookie{Name: browserCSRFCookieName, Value: token})
	rr := httptest.NewRecorder()
	called := false

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected same-origin csrf-protected request to reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBrowserGuardAllowsSameOriginUnsafeWebFormRequestWithoutAPIHeaderCSRF(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/logout", nil)
	req.Header.Set("Origin", "http://127.0.0.1:7470")
	req.AddCookie(&http.Cookie{Name: browserCSRFCookieName, Value: strings.Repeat("b", browserCSRFTokenLen*2)})
	rr := httptest.NewRecorder()
	called := false

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected same-origin web form request to reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBrowserGuardAllowsForwardedHTTPSOriginWithCSRF(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("c", browserCSRFTokenLen*2)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/api/media", nil)
	req.Host = "app.example"
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set(browserCSRFHeaderName, token)
	req.AddCookie(&http.Cookie{Name: browserCSRFCookieName, Value: token})
	rr := httptest.NewRecorder()
	called := false

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected forwarded https csrf-protected request to reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBrowserGuardAllowsForwardedHostFromStandardHeader(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("d", browserCSRFTokenLen*2)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:7470/api/media", nil)
	req.Host = "127.0.0.1:7470"
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("Forwarded", `proto=https;host="app.example"`)
	req.Header.Set(browserCSRFHeaderName, token)
	req.AddCookie(&http.Cookie{Name: browserCSRFCookieName, Value: token})
	rr := httptest.NewRecorder()
	called := false

	BrowserGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected forwarded host/proto request to reach handler")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestCORSAllowsSameOriginPreflight(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:7470/api/media", nil)
	req.Header.Set("Origin", "http://127.0.0.1:7470")
	rr := httptest.NewRecorder()

	CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should return before hitting handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:7470" {
		t.Fatalf("allow-origin = %q, want %q", got, "http://127.0.0.1:7470")
	}
	if !strings.Contains(rr.Header().Get("Access-Control-Allow-Headers"), browserCSRFHeaderName) {
		t.Fatalf("allow-headers = %q, want to contain %q", rr.Header().Get("Access-Control-Allow-Headers"), browserCSRFHeaderName)
	}
}

func TestCORSAllowsForwardedHTTPSPreflight(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:7470/api/media", nil)
	req.Host = "app.example"
	req.Header.Set("Origin", "https://app.example")
	req.Header.Set("X-Forwarded-Proto", "https")
	rr := httptest.NewRecorder()

	CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("preflight should return before hitting handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example" {
		t.Fatalf("allow-origin = %q, want %q", got, "https://app.example")
	}
}

func TestCORSRejectsCrossOriginPreflight(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:7470/api/media", nil)
	req.Header.Set("Origin", "https://evil.example")
	rr := httptest.NewRecorder()

	CORS()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("cross-origin preflight should not reach handler")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}
