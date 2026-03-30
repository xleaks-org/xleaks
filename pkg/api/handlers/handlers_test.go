package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// testHandler creates a minimal Handler with a real SQLite DB for testing.
func testHandler(t *testing.T) (*Handler, *storage.DB) {
	t.Helper()
	dir := t.TempDir()

	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Profile needed for FK constraints.
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	idHolder := identity.NewHolder(dir)
	idHolder.Set(kp)

	fm := feed.NewManager(db)
	tl := feed.NewTimeline(db, idHolder)

	h := New(db, nil, kp, nil, nil, nil, nil, nil, nil, fm, tl)
	h.SetIdentityHolder(idHolder)

	return h, db
}

// ---------- Utility function tests ----------

func TestRespondJSON(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("body key = %q, want 'value'", body["key"])
	}
}

func TestRespondJSONNilData(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	respondJSON(w, http.StatusNoContent, nil)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body should be empty, got %q", w.Body.String())
	}
}

func TestRespondError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "something went wrong" {
		t.Errorf("error = %q, want 'something went wrong'", body["error"])
	}
}

func TestParsePagination(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		query      string
		defaultLim int
		wantBefore int64
		wantLimit  int
	}{
		{"defaults", "", 20, 0, 20},
		{"custom before", "before=1000", 20, 1000, 20},
		{"custom limit", "limit=50", 20, 0, 50},
		{"both", "before=5000&limit=10", 20, 5000, 10},
		{"limit too high", "limit=200", 20, 0, 20},
		{"limit zero", "limit=0", 20, 0, 20},
		{"invalid before", "before=abc", 20, 0, 20},
		{"invalid limit", "limit=abc", 20, 0, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			url := "/test"
			if tt.query != "" {
				url += "?" + tt.query
			}
			r := httptest.NewRequest("GET", url, nil)
			before, limit := parsePagination(r, tt.defaultLim)
			if before != tt.wantBefore {
				t.Errorf("before = %d, want %d", before, tt.wantBefore)
			}
			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
		})
	}
}

func TestParseJSON(t *testing.T) {
	t.Parallel()

	t.Run("valid JSON", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader(`{"content":"hello"}`)
		r := httptest.NewRequest("POST", "/test", body)

		var req createPostRequest
		if err := parseJSON(r, &req); err != nil {
			t.Fatalf("parseJSON: %v", err)
		}
		if req.Content != "hello" {
			t.Errorf("Content = %q, want 'hello'", req.Content)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader(`{broken`)
		r := httptest.NewRequest("POST", "/test", body)

		var req createPostRequest
		if err := parseJSON(r, &req); err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})
}

func TestHexOrEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{"nil", nil, ""},
		{"empty", []byte{}, ""},
		{"valid", []byte{0xde, 0xad}, "dead"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hexOrEmpty(tt.input)
			if got != tt.want {
				t.Errorf("hexOrEmpty(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestHexSlice(t *testing.T) {
	t.Parallel()

	input := [][]byte{{0xab, 0xcd}, {0x01, 0x02}}
	result := hexSlice(input)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
	if result[0] != "abcd" {
		t.Errorf("result[0] = %q, want 'abcd'", result[0])
	}
	if result[1] != "0102" {
		t.Errorf("result[1] = %q, want '0102'", result[1])
	}
}

func TestIsUsableKeyPair(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		if isUsableKeyPair(nil) {
			t.Error("nil key pair should not be usable")
		}
	})

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		kp, err := identity.GenerateKeyPair()
		if err != nil {
			t.Fatalf("GenerateKeyPair: %v", err)
		}
		if !isUsableKeyPair(kp) {
			t.Error("valid key pair should be usable")
		}
	})
}

func TestToStringSlice(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		input := []interface{}{"a", "b", "c"}
		result, ok := toStringSlice(input)
		if !ok {
			t.Fatal("expected ok = true")
		}
		if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
			t.Errorf("result = %v, want [a b c]", result)
		}
	})

	t.Run("not slice", func(t *testing.T) {
		t.Parallel()
		_, ok := toStringSlice("not a slice")
		if ok {
			t.Error("expected ok = false for non-slice input")
		}
	})

	t.Run("mixed types", func(t *testing.T) {
		t.Parallel()
		input := []interface{}{"a", 123}
		_, ok := toStringSlice(input)
		if ok {
			t.Error("expected ok = false for mixed-type slice")
		}
	})
}

// ---------- Handler endpoint tests ----------

func TestGetNodeStatusShape(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	cfg := config.DefaultConfig()
	h.SetConfig(cfg, "")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/status", nil)
	h.GetNodeStatus(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify required keys are present.
	requiredKeys := []string{"peers", "bandwidth", "storage", "uptime", "subscriptions", "identity", "node_id", "version"}
	for _, key := range requiredKeys {
		if _, ok := body[key]; !ok {
			t.Errorf("response missing key %q", key)
		}
	}

	// Bandwidth should be a nested object.
	bw, ok := body["bandwidth"].(map[string]interface{})
	if !ok {
		t.Fatal("bandwidth should be a JSON object")
	}
	for _, bwKey := range []string{"total_in", "total_out", "rate_in", "rate_out"} {
		if _, ok := bw[bwKey]; !ok {
			t.Errorf("bandwidth missing key %q", bwKey)
		}
	}

	// Storage should be a nested object.
	stor, ok := body["storage"].(map[string]interface{})
	if !ok {
		t.Fatal("storage should be a JSON object")
	}
	for _, sKey := range []string{"used", "limit"} {
		if _, ok := stor[sKey]; !ok {
			t.Errorf("storage missing key %q", sKey)
		}
	}

	// Identity should be a non-empty hex string (we have a keypair).
	identityStr, ok := body["identity"].(string)
	if !ok || identityStr == "" {
		t.Error("identity should be a non-empty hex string")
	}
}

func TestGetNodeStatusWithoutConfig(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)
	// No SetConfig call — cfg is nil.

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/status", nil)
	h.GetNodeStatus(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Storage should default to 0/0.
	stor := body["storage"].(map[string]interface{})
	if stor["used"].(float64) != 0 {
		t.Errorf("storage.used = %v, want 0", stor["used"])
	}
	if stor["limit"].(float64) != 0 {
		t.Errorf("storage.limit = %v, want 0", stor["limit"])
	}
}

func TestGetNodeStatusUsesConfiguredStorageLimit(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.Node.MaxStorageGB = 0
	h.SetConfig(cfg, "")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/status", nil)
	h.GetNodeStatus(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	stor := body["storage"].(map[string]interface{})
	if stor["limit"].(float64) != 0 {
		t.Errorf("storage.limit = %v, want 0", stor["limit"])
	}
}

func TestGetNodeConfigDefault(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)
	// No SetConfig — should return defaults.

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/config", nil)
	h.GetNodeConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if _, ok := body["enable_relay"]; !ok {
		t.Error("response missing enable_relay")
	}
}

func TestGetNodeConfigWithConfig(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 42
	h.SetConfig(cfg, "")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/config", nil)
	h.GetNodeConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if body["max_connections"].(float64) != 42 {
		t.Errorf("max_connections = %v, want 42", body["max_connections"])
	}
}

func TestSearchMissingQuery(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search", nil)
	h.Search(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(body["error"], "query parameter") {
		t.Errorf("error = %q, want to contain 'query parameter'", body["error"])
	}
}

func TestSearchPostsType(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?q=hello&type=posts", nil)
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["type"] != "posts" {
		t.Errorf("type = %v, want 'posts'", body["type"])
	}
	if body["query"] != "hello" {
		t.Errorf("query = %v, want 'hello'", body["query"])
	}
}

func TestSearchUsersType(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?q=alice&type=users", nil)
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["type"] != "users" {
		t.Errorf("type = %v, want 'users'", body["type"])
	}
}

func TestSearchInvalidType(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?q=test&type=invalid", nil)
	h.Search(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestSearchDefaultsToPostsType(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?q=test", nil)
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["type"] != "posts" {
		t.Errorf("type = %v, want 'posts' (default)", body["type"])
	}
}

func TestCreatePostNoIdentity(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	// Clear identity to simulate locked state.
	h.identity.Set(nil)
	h.kp = nil

	body := strings.NewReader(`{"content":"test post"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/posts", body)
	h.CreatePost(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestCreatePostInvalidJSON(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	body := strings.NewReader(`{broken json`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/posts", body)
	h.CreatePost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetFeedEmpty(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/feed", nil)
	h.GetFeed(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Empty feed should return empty array.
	if len(body) != 0 {
		t.Errorf("expected empty feed, got %d items", len(body))
	}
}

func TestGetNodePeersNilP2PHost(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/node/peers", nil)
	h.GetNodePeers(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body) != 0 {
		t.Errorf("expected empty peers list, got %d", len(body))
	}
}

func TestSearchHashtagLocalFallback(t *testing.T) {
	t.Parallel()
	h, db := testHandler(t)

	// Insert a post with a tag.
	cid := make([]byte, 32)
	cid[0] = 0x42
	kp := h.kp
	if err := db.InsertPost(cid, kp.PublicKeyBytes(), "Hello #test world", nil, nil, time.Now().UnixMilli(), make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.InsertPostTags(cid, []string{"test"}); err != nil {
		t.Fatalf("InsertPostTags: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/api/search?q=%23test&type=posts", nil)
	h.Search(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	results, ok := body["results"].([]interface{})
	if !ok {
		t.Fatal("results should be an array")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestUpdateNodeConfigNoConfig(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)
	// No SetConfig, so h.cfg is nil.

	body := strings.NewReader(`{"max_connections": 100}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestUpdateNodeConfigValidatedAndNormalized(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	h.SetConfig(cfg, "")

	idxClient := indexer.NewIndexerClient(context.Background())
	t.Cleanup(idxClient.Close)
	h.SetIndexerClient(idxClient)

	bootstrapPeer := config.DefaultBootstrapPeers()[0]
	body := strings.NewReader(fmt.Sprintf(`{
		"max_connections": 100,
		"storage_limit_gb": 0,
		"bandwidth_limit_mbps": 250,
		"enable_relay": false,
		"enable_mdns": false,
		"enable_hole_punching": false,
		"enable_websocket": false,
		"auto_fetch_media": true,
		"max_upload_size_mb": 256,
		"thumbnail_quality": 90,
		"bootstrap_peers": ["  %s  ", "%s"],
		"known_indexers": [" https://indexer.example.org:7471/ ", "https://indexer.example.org:7471/"],
		"log_level": "warning"
	}`, bootstrapPeer, bootstrapPeer))

	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if cfg.Network.MaxPeers != 100 {
		t.Fatalf("max peers = %d, want 100", cfg.Network.MaxPeers)
	}
	if cfg.Node.MaxStorageGB != 0 {
		t.Fatalf("storage_limit_gb = %d, want 0", cfg.Node.MaxStorageGB)
	}
	if cfg.Network.BandwidthLimitMbps != 250 {
		t.Fatalf("bandwidth_limit_mbps = %d, want 250", cfg.Network.BandwidthLimitMbps)
	}
	if cfg.Network.EnableRelay || cfg.Network.EnableMDNS || cfg.Network.EnableHolePunching {
		t.Fatal("expected relay, mdns, and hole punching to be disabled")
	}
	if cfg.API.EnableWebSocket {
		t.Fatal("expected websocket to be disabled")
	}
	if !cfg.Media.AutoFetchMedia {
		t.Fatal("expected auto_fetch_media to be enabled")
	}
	if cfg.Media.MaxUploadSizeMB != 256 {
		t.Fatalf("max_upload_size_mb = %d, want 256", cfg.Media.MaxUploadSizeMB)
	}
	if cfg.Media.ThumbnailQuality != 90 {
		t.Fatalf("thumbnail_quality = %d, want 90", cfg.Media.ThumbnailQuality)
	}
	if len(cfg.Network.BootstrapPeers) != 1 || cfg.Network.BootstrapPeers[0] != bootstrapPeer {
		t.Fatalf("bootstrap_peers = %v, want [%q]", cfg.Network.BootstrapPeers, bootstrapPeer)
	}
	if len(cfg.Indexer.KnownIndexers) != 1 || cfg.Indexer.KnownIndexers[0] != "https://indexer.example.org:7471" {
		t.Fatalf("known_indexers = %v, want [https://indexer.example.org:7471]", cfg.Indexer.KnownIndexers)
	}
	if cfg.Logging.Level != "warn" {
		t.Fatalf("log_level = %q, want %q", cfg.Logging.Level, "warn")
	}
	if !h.indexerClient.Available() {
		t.Fatal("expected runtime indexer client to refresh known indexers")
	}
}

func TestUpdateNodeConfigRejectsInvalidUpdateAtomically(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 42
	cfg.Node.MaxStorageGB = 5
	h.SetConfig(cfg, "")

	body := strings.NewReader(`{"max_connections": 100, "storage_limit_gb": -1}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if cfg.Network.MaxPeers != 42 {
		t.Fatalf("max peers mutated to %d after rejected update", cfg.Network.MaxPeers)
	}
	if cfg.Node.MaxStorageGB != 5 {
		t.Fatalf("storage_limit_gb mutated to %d after rejected update", cfg.Node.MaxStorageGB)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(resp["error"], "storage_limit_gb") {
		t.Fatalf("error = %q, want to mention storage_limit_gb", resp["error"])
	}
}

func TestUpdateNodeConfigRejectsFractionalNumbers(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 42
	h.SetConfig(cfg, "")

	body := strings.NewReader(`{"max_connections": 42.5}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if cfg.Network.MaxPeers != 42 {
		t.Fatalf("max peers mutated to %d after fractional update", cfg.Network.MaxPeers)
	}
}

func TestMediaCIDsForPostNilDB(t *testing.T) {
	t.Parallel()

	// Should not panic with nil DB.
	result := mediaCIDsForPost(nil, []byte{0x01})
	if result != nil {
		t.Errorf("expected nil for nil DB, got %v", result)
	}
}

func TestMediaCIDsForPostEmptyCID(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	result := h.mediaCIDsForPost(nil)
	if result != nil {
		t.Errorf("expected nil for empty CID, got %v", result)
	}
}
