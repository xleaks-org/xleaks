package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnsureCSRFCookieSetsTokenOnSafeRequest(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rr := httptest.NewRecorder()

	handler := ensureCSRFCookie(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := csrfTokenFromRequest(r)
		if !validCSRFToken(token) {
			t.Fatalf("csrf token %q is invalid", token)
		}
	}))
	handler.ServeHTTP(rr, req)

	res := rr.Result()
	defer res.Body.Close()

	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == csrfCookieName {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("expected csrf cookie to be set")
	}
	if !validCSRFToken(cookie.Value) {
		t.Fatalf("csrf cookie %q is invalid", cookie.Value)
	}
}

func TestRequireCSRFRejectsMissingToken(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/web/post", strings.NewReader("content=test"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	handler := requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("request should not have reached wrapped handler")
	}))
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}
}

func TestRequireCSRFAllowsMatchingFormToken(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("a", csrfTokenLen*2)
	req := httptest.NewRequest(http.MethodPost, "/settings/profile", strings.NewReader("csrf_token="+token))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rr := httptest.NewRecorder()

	called := false
	handler := requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestRequireCSRFAllowsMatchingHeaderToken(t *testing.T) {
	t.Parallel()

	token := strings.Repeat("b", csrfTokenLen*2)
	req := httptest.NewRequest(http.MethodPost, "/web/like", nil)
	req.Header.Set(csrfHeaderName, token)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rr := httptest.NewRecorder()

	called := false
	handler := requireCSRF(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	handler.ServeHTTP(rr, req)

	if !called {
		t.Fatal("expected wrapped handler to be called")
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}
