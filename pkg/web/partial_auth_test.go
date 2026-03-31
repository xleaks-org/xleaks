package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func assertSessionExpiredPartial(t *testing.T, body string) {
	t.Helper()
	if !strings.Contains(body, "Session expired") {
		t.Fatalf("body = %q, want session expired prompt", body)
	}
	if !strings.Contains(body, "Sign in again") {
		t.Fatalf("body = %q, want sign-in link", body)
	}
}

func TestSearchResultsPartialRequiresSession(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	author := handler.sessions.Get(token).KeyPair.PublicKeyBytes()
	cid := make([]byte, 32)
	cid[0] = 0x31
	if err := db.InsertPost(cid, author, "hello world", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/search-results?q=hello", nil)
	rr := httptest.NewRecorder()

	handler.searchResultsPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	assertSessionExpiredPartial(t, body)
	if strings.Contains(body, "hello world") {
		t.Fatalf("body leaked search results without a session: %q", body)
	}
}

func TestTrendingTagsPartialRequiresSession(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	author := handler.sessions.Get(token).KeyPair.PublicKeyBytes()
	cid := make([]byte, 32)
	cid[0] = 0x32
	if err := db.InsertPost(cid, author, "hello #secret", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.InsertPostTags(cid, []string{"secret"}); err != nil {
		t.Fatalf("InsertPostTags: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/trending-tags", nil)
	rr := httptest.NewRecorder()

	handler.trendingTagsPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	assertSessionExpiredPartial(t, body)
	if strings.Contains(body, "#secret") {
		t.Fatalf("body leaked trending tag data without a session: %q", body)
	}
}

func TestTrendingPostsPartialRequiresSession(t *testing.T) {
	t.Parallel()

	handler, db, token := testFeedPartialHandler(t)
	author := handler.sessions.Get(token).KeyPair.PublicKeyBytes()
	cid := make([]byte, 32)
	cid[0] = 0x33
	if err := db.InsertPost(cid, author, "trending secret post", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/web/trending-posts", nil)
	rr := httptest.NewRecorder()

	handler.trendingPostsPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	body := rr.Body.String()
	assertSessionExpiredPartial(t, body)
	if strings.Contains(body, "trending secret post") {
		t.Fatalf("body leaked trending posts without a session: %q", body)
	}
}

func TestNodeStatusPartialRequiresSession(t *testing.T) {
	t.Parallel()

	handler, _, _ := testFeedPartialHandler(t)
	handler.nodeStatus = func() (int, float64, int64, int64, int) {
		t.Fatal("nodeStatus should not be called without a session")
		return 0, 0, 0, 0, 0
	}

	req := httptest.NewRequest(http.MethodGet, "/web/node-status", nil)
	rr := httptest.NewRecorder()

	handler.nodeStatusPartial(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	assertSessionExpiredPartial(t, rr.Body.String())
}
