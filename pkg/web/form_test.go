package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
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

func TestHandlePostInternalFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	sessions := NewSessionManager()
	defer sessions.Stop()

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	db, err := storage.NewDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	defer db.Close()
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, 1); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}
	token, err := sessions.Create(kp)
	if err != nil {
		t.Fatalf("Create(session): %v", err)
	}

	handler := &Handler{
		sessions: sessions,
		db:       db,
		createPost: func(_ context.Context, _ string, _ []string, _ string) (string, error) {
			return "", errors.New("sql: database is closed")
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/web/post", strings.NewReader("content=hello"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.handlePost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if body := rr.Body.String(); body != "Failed to create post\n" {
		t.Fatalf("body = %q, want %q", body, "Failed to create post\n")
	}
}
