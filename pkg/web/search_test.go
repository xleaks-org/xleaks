package web

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearchResultsPartialPromptsForWhitespaceOnlyQuery(t *testing.T) {
	t.Parallel()

	handler, _, token := testFeedPartialHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/web/search-results?q=+++", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	rr := httptest.NewRecorder()

	handler.searchResultsPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Enter a search term.") {
		t.Fatalf("body = %q, want search prompt", body)
	}
	if strings.Contains(body, "No results for") {
		t.Fatalf("body = %q, should not render a no-results state for whitespace query", body)
	}
}

func TestPerformSearchTrimsContentQuery(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	author := handler.sessions.Get(token).KeyPair.PublicKeyBytes()
	cid := make([]byte, 32)
	cid[0] = 0x11

	if err := db.InsertPost(cid, author, "hello world", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	results := handler.performSearch("   hello   ", 20)

	if len(results.Posts) != 1 {
		t.Fatalf("len(results.Posts) = %d, want 1", len(results.Posts))
	}
	if results.Posts[0].ID != hex.EncodeToString(cid) {
		t.Fatalf("results.Posts[0].ID = %q, want %q", results.Posts[0].ID, hex.EncodeToString(cid))
	}
}

func TestPerformSearchFindsWhitespaceWrappedHashtag(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	author := handler.sessions.Get(token).KeyPair.PublicKeyBytes()
	cid := make([]byte, 32)
	cid[0] = 0x22

	if err := db.InsertPost(cid, author, "hello #test world", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.InsertPostTags(cid, []string{"test"}); err != nil {
		t.Fatalf("InsertPostTags: %v", err)
	}

	results := handler.performSearch("   #test   ", 20)

	if len(results.Posts) != 1 {
		t.Fatalf("len(results.Posts) = %d, want 1", len(results.Posts))
	}
	if results.Posts[0].ID != hex.EncodeToString(cid) {
		t.Fatalf("results.Posts[0].ID = %q, want %q", results.Posts[0].ID, hex.EncodeToString(cid))
	}
}
