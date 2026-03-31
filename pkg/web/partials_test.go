package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

func testFeedPartialHandler(t *testing.T) (*Handler, *storage.DB, string) {
	t.Helper()

	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("Migrate: %v", err)
	}

	holder := identity.NewHolder(filepath.Join(dir, "identities"))
	holder.SetDB(db)

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		db.Close()
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	holder.Set(kp)

	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, 1); err != nil {
		db.Close()
		t.Fatalf("UpsertProfile: %v", err)
	}

	sessions := NewSessionManager()
	handler, err := NewHandler(db, holder, feed.NewTimeline(db, holder), sessions)
	if err != nil {
		sessions.Stop()
		db.Close()
		t.Fatalf("NewHandler: %v", err)
	}

	token, err := sessions.Create(kp)
	if err != nil {
		handler.Close()
		db.Close()
		t.Fatalf("Create(session): %v", err)
	}

	t.Cleanup(func() {
		handler.Close()
		db.Close()
	})

	return handler, db, token
}

func TestFeedPartialRejectsInvalidReplyTarget(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/web/feed?reply_to=not-hex", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Invalid reply target.") {
		t.Fatalf("body = %q, want invalid reply target message", body)
	}
}

func TestFeedPartialRejectsInvalidBeforeCursor(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/web/feed?before=abc", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Invalid feed cursor.") {
		t.Fatalf("body = %q, want invalid feed cursor message", body)
	}
}

func TestFeedPartialRejectsNegativeBeforeCursor(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/web/feed?before=-1", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Invalid feed cursor.") {
		t.Fatalf("body = %q, want invalid feed cursor message", body)
	}
}

func TestFeedPartialRejectsInvalidAuthorKey(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/web/feed?author=not-hex", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
	if body := rr.Body.String(); !strings.Contains(body, "Invalid author key.") {
		t.Fatalf("body = %q, want invalid author key message", body)
	}
}

func TestFeedPartialReplyThreadFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/feed?reply_to="+strings.Repeat("a", 64), nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Failed to load replies.") {
		t.Fatalf("body = %q, want sanitized reply failure message", body)
	}
	if strings.Contains(strings.ToLower(body), "sql") || strings.Contains(strings.ToLower(body), "closed") {
		t.Fatalf("body leaked backend error details: %q", body)
	}
}
