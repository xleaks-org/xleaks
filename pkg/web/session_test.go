package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionCookieSecureFlagMatchesRequestScheme(t *testing.T) {
	t.Parallel()

	sm := NewSessionManager()
	defer sm.Stop()

	httpReq := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	httpResp := httptest.NewRecorder()
	sm.SetCookie(httpResp, httpReq, "token-http")

	httpCookie := findResponseCookie(httpResp.Result().Cookies(), sessionCookieName)
	if httpCookie == nil {
		t.Fatal("expected http session cookie")
	}
	if httpCookie.Secure {
		t.Fatal("expected http session cookie to be non-secure")
	}

	httpsReq := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	httpsResp := httptest.NewRecorder()
	sm.SetCookie(httpsResp, httpsReq, "token-https")

	httpsCookie := findResponseCookie(httpsResp.Result().Cookies(), sessionCookieName)
	if httpsCookie == nil {
		t.Fatal("expected https session cookie")
	}
	if !httpsCookie.Secure {
		t.Fatal("expected https session cookie to be secure")
	}
}
