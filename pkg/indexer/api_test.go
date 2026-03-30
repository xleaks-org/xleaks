package indexer

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

func TestIndexerAPIHandlerSetsSecurityHeaders(t *testing.T) {
	t.Parallel()

	api, cleanup := newTestIndexerAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/explore/publishers", nil)
	rr := httptest.NewRecorder()

	api.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want %q", got, "DENY")
	}
	if got := rr.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want %q", got, "no-referrer")
	}
}

func TestIndexerAPIExplorePublishersCapsLimit(t *testing.T) {
	api, cleanup := newTestIndexerAPI(t)
	defer cleanup()

	for i := 0; i < maxIndexerLimit+10; i++ {
		if _, err := api.stats.db.Exec(
			`INSERT INTO follower_counts (pubkey, follower_count, following_count) VALUES (?, ?, ?)`,
			[]byte{byte(i), byte(i >> 8)},
			i,
			0,
		); err != nil {
			t.Fatalf("insert follower_counts row %d error = %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/explore/publishers?limit=1000", nil)
	rr := httptest.NewRecorder()

	api.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	var payload struct {
		Publishers []struct {
			Pubkey string `json:"pubkey"`
		} `json:"publishers"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := len(payload.Publishers); got != maxIndexerLimit {
		t.Fatalf("publisher count = %d, want %d", got, maxIndexerLimit)
	}
}

func TestParseLimitParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want int
	}{
		{name: "empty", in: "", want: defaultIndexerLimit},
		{name: "invalid", in: "abc", want: defaultIndexerLimit},
		{name: "negative", in: "-1", want: defaultIndexerLimit},
		{name: "cap", in: "1000", want: maxIndexerLimit},
		{name: "valid", in: "42", want: 42},
	}

	for _, tt := range tests {
		if got := parseLimitParam(tt.in); got != tt.want {
			t.Fatalf("%s: parseLimitParam(%q) = %d, want %d", tt.name, tt.in, got, tt.want)
		}
	}
}

func TestParsePageParamCapsLargeValues(t *testing.T) {
	t.Parallel()

	if got := parsePageParam("100000"); got != maxIndexerPage {
		t.Fatalf("parsePageParam() = %d, want %d", got, maxIndexerPage)
	}
}

func newTestIndexerAPI(t *testing.T) (*IndexerAPI, func()) {
	t.Helper()

	db, err := storage.NewDB(filepath.Join(t.TempDir(), "indexer.db"))
	if err != nil {
		t.Fatalf("storage.NewDB() error = %v", err)
	}
	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("db.Migrate() error = %v", err)
	}

	api := NewIndexerAPI(nil, nil, NewStatsCollector(db))
	cleanup := func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db.Close() error = %v", err)
		}
	}
	return api, cleanup
}
