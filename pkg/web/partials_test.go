package web

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestFeedPartialUsesSessionIdentityForLikeState(t *testing.T) {
	t.Parallel()

	handler, db, _ := testFeedPartialHandler(t)

	viewerKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(viewer): %v", err)
	}
	authorKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(author): %v", err)
	}
	if err := db.UpsertProfile(viewerKP.PublicKeyBytes(), "Viewer", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile(viewer): %v", err)
	}
	if err := db.UpsertProfile(authorKP.PublicKeyBytes(), "Author", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile(author): %v", err)
	}

	postCID := make([]byte, 32)
	postCID[0] = 0x71
	if err := db.InsertPost(postCID, authorKP.PublicKeyBytes(), "liked by session viewer", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	reactionCID := make([]byte, 32)
	reactionCID[0] = 0x72
	if err := db.InsertReaction(reactionCID, viewerKP.PublicKeyBytes(), postCID, "like", time.Now().UnixMilli()); err != nil {
		t.Fatalf("InsertReaction: %v", err)
	}

	token, err := handler.sessions.Create(viewerKP)
	if err != nil {
		t.Fatalf("Create(session): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/feed?all=1", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.feedPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	postID := hex.EncodeToString(postCID)
	if !strings.Contains(body, "liked by session viewer") {
		t.Fatalf("body = %q, want rendered post content", body)
	}
	if !strings.Contains(body, `text-red-400 flex items-center gap-1`) {
		t.Fatalf("body = %q, want liked-state markup", body)
	}
	if strings.Contains(body, `hx-post="/web/like" hx-vals='{"target":"`+postID+`"}'`) {
		t.Fatalf("body rendered unliked action for session-liked post: %q", body)
	}
}

func TestNodeStatusPartialUsesSessionIdentityForSubscriptions(t *testing.T) {
	t.Parallel()

	handler, db, _ := testFeedPartialHandler(t)

	viewerKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(viewer): %v", err)
	}
	targetKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair(target): %v", err)
	}
	if err := db.UpsertProfile(viewerKP.PublicKeyBytes(), "Viewer", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile(viewer): %v", err)
	}
	if err := db.UpsertProfile(targetKP.PublicKeyBytes(), "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile(target): %v", err)
	}
	if err := db.AddSubscription(viewerKP.PublicKeyBytes(), targetKP.PublicKeyBytes(), time.Now().UnixMilli()); err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}

	token, err := handler.sessions.Create(viewerKP)
	if err != nil {
		t.Fatalf("Create(session): %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/node-status", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.nodeStatusPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Following") {
		t.Fatalf("body = %q, want following row for session subscriptions", body)
	}
	if !strings.Contains(body, ">1<") {
		t.Fatalf("body = %q, want session subscription count", body)
	}
}
