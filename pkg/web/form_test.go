package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesRejectOversizedFormBodyDuringCSRFFromForm(t *testing.T) {
	t.Parallel()

	sessions := NewSessionManager()
	defer sessions.Stop()

	handler, err := NewHandler(nil, nil, nil, sessions)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	token := strings.Repeat("a", csrfTokenLen*2)
	body := "csrf_token=" + token + "&passphrase=" + strings.Repeat("x", maxFormBodyBytes)
	req := httptest.NewRequest(http.MethodPost, "/unlock", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestRoutesRejectOversizedFormBodyAfterHeaderCSRF(t *testing.T) {
	t.Parallel()

	sessions := NewSessionManager()
	defer sessions.Stop()

	handler, err := NewHandler(nil, nil, nil, sessions)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	token := strings.Repeat("b", csrfTokenLen*2)
	body := "content=" + strings.Repeat("y", maxFormBodyBytes)
	req := httptest.NewRequest(http.MethodPost, "/web/post", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set(csrfHeaderName, token)
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.Routes().ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}
