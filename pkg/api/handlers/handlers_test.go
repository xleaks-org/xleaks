package handlers

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/social"
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

	cas, err := content.NewContentStore(filepath.Join(dir, "cas"))
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

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

	h := New(db, cas, kp, nil, nil, nil, nil, nil, nil, fm, tl)
	h.SetIdentityHolder(idHolder)

	return h, db
}

func wireServiceBackedHandler(h *Handler, db *storage.DB) {
	if h == nil || db == nil {
		return
	}

	h.feed = feed.NewManager(db)
	h.timeline = feed.NewTimeline(db, h.identity)
	h.posts = social.NewPostService(db, h.cas, h.currentKeyPair())
	h.reactions = social.NewReactionService(db, h.currentKeyPair())
	h.profiles = social.NewProfileService(db, h.currentKeyPair())
	h.dms = social.NewDMService(db, h.currentKeyPair())
	h.notifs = social.NewNotificationService(db)
	h.follows = social.NewFollowService(db, h.feed, h.currentKeyPair())
}

func testStoredIdentityHandler(t *testing.T, passphrase string) (*Handler, *storage.DB, *identity.Holder, *identity.KeyPair) {
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

	cas, err := content.NewContentStore(filepath.Join(dir, "cas"))
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

	holder := identity.NewHolder(filepath.Join(dir, "identities"))
	kp, _, err := holder.CreateAndSave(passphrase)
	if err != nil {
		t.Fatalf("CreateAndSave: %v", err)
	}
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	fm := feed.NewManager(db)
	tl := feed.NewTimeline(db, holder)
	h := New(db, cas, kp, nil, nil, nil, nil, nil, nil, fm, tl)
	h.SetIdentityHolder(holder)

	return h, db, holder, kp
}

func captureDefaultJSONLogger(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	previous := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() {
		slog.SetDefault(previous)
	})
	return &buf
}

func decodeSingleJSONLogLine(t *testing.T, logLine string) map[string]any {
	t.Helper()

	if strings.TrimSpace(logLine) == "" {
		t.Fatal("expected log output")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(logLine), &payload); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}
	return payload
}

func assertJSONErrorResponse(t *testing.T, body []byte, want string, disallowed ...string) {
	t.Helper()

	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if got := payload["error"]; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
	for _, fragment := range disallowed {
		if fragment != "" && strings.Contains(string(body), fragment) {
			t.Fatalf("response leaked %q in %s", fragment, string(body))
		}
	}
}

func testPNGWithDimensions(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})

	writeChunk := func(name string, data []byte) {
		_ = binary.Write(&buf, binary.BigEndian, uint32(len(data)))
		buf.WriteString(name)
		buf.Write(data)
		crc := crc32.ChecksumIEEE(append([]byte(name), data...))
		_ = binary.Write(&buf, binary.BigEndian, crc)
	}

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)

	return buf.Bytes()
}

func testPNGImage(width, height int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 17) % 255),
				G: uint8((y * 19) % 255),
				B: 120,
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
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
		w := httptest.NewRecorder()

		var req createPostRequest
		if err := parseJSON(w, r, &req); err != nil {
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
		w := httptest.NewRecorder()

		var req createPostRequest
		if err := parseJSON(w, r, &req); err == nil {
			t.Fatal("expected error for invalid JSON")
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader(`{"content":"hello","unexpected":true}`)
		r := httptest.NewRequest("POST", "/test", body)
		w := httptest.NewRecorder()

		var req createPostRequest
		if err := parseJSON(w, r, &req); err == nil {
			t.Fatal("expected error for unknown field")
		}
	})

	t.Run("rejects trailing data", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader(`{"content":"hello"}{"content":"again"}`)
		r := httptest.NewRequest("POST", "/test", body)
		w := httptest.NewRecorder()

		var req createPostRequest
		if err := parseJSON(w, r, &req); err == nil {
			t.Fatal("expected error for trailing JSON data")
		}
	})

	t.Run("rejects oversized bodies", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader(`{"content":"` + strings.Repeat("a", maxJSONBodyBytes) + `"}`)
		r := httptest.NewRequest("POST", "/test", body)
		w := httptest.NewRecorder()

		var req createPostRequest
		err := parseJSON(w, r, &req)
		if err == nil {
			t.Fatal("expected error for oversized JSON body")
		}
		if err.Error() != "JSON body too large" {
			t.Fatalf("error = %q, want %q", err.Error(), "JSON body too large")
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

func TestExportIdentityReturnsAttachmentOnPost(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cas, err := content.NewContentStore(filepath.Join(dir, "cas"))
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

	holder := identity.NewHolder(filepath.Join(dir, "identities"))
	kp, _, err := holder.CreateAndSave("passphrase")
	if err != nil {
		t.Fatalf("CreateAndSave: %v", err)
	}
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	fm := feed.NewManager(db)
	tl := feed.NewTimeline(db, holder)
	h := New(db, cas, kp, nil, nil, nil, nil, nil, nil, fm, tl)
	h.SetIdentityHolder(holder)

	req := httptest.NewRequest(http.MethodPost, "/api/identity/export", nil)
	rr := httptest.NewRecorder()

	h.ExportIdentity(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	if got := rr.Header().Get("Content-Disposition"); !strings.Contains(got, ".xleaks-key.json") {
		t.Fatalf("Content-Disposition = %q, want export attachment", got)
	}

	var payload struct {
		Pubkey string `json:"pubkey"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload.Pubkey != hex.EncodeToString(kp.PublicKeyBytes()) {
		t.Fatalf("pubkey = %q, want %q", payload.Pubkey, hex.EncodeToString(kp.PublicKeyBytes()))
	}
}

func TestUnlockIdentityWrongPassphraseDoesNotLeakInternalError(t *testing.T) {
	t.Parallel()

	h, _, holder, _ := testStoredIdentityHandler(t, "correct horse battery staple")
	holder.Lock()

	req := httptest.NewRequest(http.MethodPost, "/api/identity/unlock", strings.NewReader(`{"passphrase":"wrong passphrase"}`))
	rr := httptest.NewRecorder()

	h.UnlockIdentity(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := payload["error"]; got != "failed to unlock identity" {
		t.Fatalf("error = %q, want %q", got, "failed to unlock identity")
	}
	if strings.Contains(rr.Body.String(), "decrypt") || strings.Contains(rr.Body.String(), "authentication") {
		t.Fatalf("response leaked internal unlock error: %s", rr.Body.String())
	}
}

func TestSwitchIdentityWrongPassphraseDoesNotLeakInternalError(t *testing.T) {
	t.Parallel()

	h, _, _, kp := testStoredIdentityHandler(t, "correct horse battery staple")
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())

	req := httptest.NewRequest(http.MethodPut, "/api/identity/switch/"+pubkeyHex, strings.NewReader(`{"passphrase":"wrong passphrase"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("pubkey", pubkeyHex)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr := httptest.NewRecorder()

	h.SwitchIdentity(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var payload map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := payload["error"]; got != "failed to switch identity" {
		t.Fatalf("error = %q, want %q", got, "failed to switch identity")
	}
	if strings.Contains(rr.Body.String(), "decrypt") || strings.Contains(rr.Body.String(), pubkeyHex) {
		t.Fatalf("response leaked internal switch error: %s", rr.Body.String())
	}
}

func TestExportIdentityAuditLogRedactsIdentity(t *testing.T) {
	buf := captureDefaultJSONLogger(t)
	h, _, _, kp := testStoredIdentityHandler(t, "correct horse battery staple")
	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	req := httptest.NewRequest(http.MethodPost, "/api/identity/export", nil)
	rr := httptest.NewRecorder()

	h.ExportIdentity(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	logLine := strings.TrimSpace(buf.String())
	if strings.Contains(logLine, pubkeyHex) || strings.Contains(logLine, address) {
		t.Fatalf("log line leaked raw identity details: %s", logLine)
	}

	payload := decodeSingleJSONLogLine(t, logLine)
	if got := payload["msg"]; got != "identity exported" {
		t.Fatalf("msg = %v, want %q", got, "identity exported")
	}
	identityField, ok := payload["identity"].(map[string]any)
	if !ok {
		t.Fatalf("identity = %T, want object", payload["identity"])
	}
	if got := identityField["fingerprint"]; got == "" {
		t.Fatalf("identity.fingerprint = %v, want non-empty fingerprint", got)
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

func TestCreatePostRejectsTooLongContent(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	body := strings.NewReader(`{"content":"` + strings.Repeat("a", 5001) + `"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/posts", body)
	h.CreatePost(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCreatePostInternalFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)
	wireServiceBackedHandler(h, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	body := strings.NewReader(`{"content":"hello world"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/posts", body)
	h.CreatePost(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	assertJSONErrorResponse(t, w.Body.Bytes(), "failed to create post", "sql", "closed")
}

func TestUploadMediaRejectsOversizedImageDimensions(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "huge.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fileWriter.Write(testPNGWithDimensions(content.MaxImageWidth, content.MaxImageHeight)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/media", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.UploadMedia(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if !strings.Contains(w.Body.String(), "invalid image") {
		t.Fatalf("body = %q, want invalid image error", w.Body.String())
	}
}

func TestUploadMediaStoresStreamedUpload(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)

	png := testPNGWithDimensions(640, 480)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "upload.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fileWriter.Write(png); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/media", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.UploadMedia(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusCreated)
	}

	var resp struct {
		CID      string `json:"cid"`
		MimeType string `json:"mime_type"`
		Size     int64  `json:"size"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if resp.MimeType != "image/png" {
		t.Fatalf("mime_type = %q, want %q", resp.MimeType, "image/png")
	}
	if resp.Size != int64(len(png)) {
		t.Fatalf("size = %d, want %d", resp.Size, len(png))
	}
	if resp.Filename != "upload.png" {
		t.Fatalf("filename = %q, want %q", resp.Filename, "upload.png")
	}

	cid, err := content.HexToCID(resp.CID)
	if err != nil {
		t.Fatalf("HexToCID() error: %v", err)
	}
	media, err := db.GetMediaObject(cid)
	if err != nil {
		t.Fatalf("GetMediaObject() error: %v", err)
	}
	if media == nil {
		t.Fatal("expected uploaded media object in DB")
	}
	if media.Size != uint64(len(png)) {
		t.Fatalf("media size = %d, want %d", media.Size, len(png))
	}
}

func TestUploadMediaRejectsConfiguredUploadLimit(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	h.cfg = config.DefaultConfig()
	h.cfg.Media.MaxUploadSizeMB = 1

	fileData := bytes.Repeat([]byte("a"), 1024*1024+1)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	fileWriter, err := writer.CreateFormFile("file", "big.bin")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := fileWriter.Write(fileData); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/media", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.UploadMedia(w, r)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(w.Body.String(), "uploaded file too large") {
		t.Fatalf("body = %q, want upload-too-large error", w.Body.String())
	}
}

func TestGetMediaServesStoredContent(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)
	png := testPNGWithDimensions(320, 240)
	cid, err := content.ComputeCID(png)
	if err != nil {
		t.Fatalf("ComputeCID() error: %v", err)
	}
	if err := h.cas.Put(cid, png); err != nil {
		t.Fatalf("CAS Put() error: %v", err)
	}
	if err := db.InsertMediaObject(cid, h.kp.PublicKeyBytes(), "image/png", uint64(len(png)), 1, 320, 240, 0, nil, time.Now().UnixMilli()); err != nil {
		t.Fatalf("InsertMediaObject() error: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/media/"+hex.EncodeToString(cid), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cid", hex.EncodeToString(cid))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.GetMedia(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("Content-Type = %q, want %q", got, "image/png")
	}
	if !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatal("GetMedia() body did not match stored content")
	}
}

func TestGetMediaThumbnailGeneratesFromStoredImage(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	png := testPNGImage(320, 240)
	cid, err := content.ComputeCID(png)
	if err != nil {
		t.Fatalf("ComputeCID() error: %v", err)
	}
	if err := h.cas.Put(cid, png); err != nil {
		t.Fatalf("CAS Put() error: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/media/"+hex.EncodeToString(cid)+"/thumbnail", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cid", hex.EncodeToString(cid))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.GetMediaThumbnail(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want %q", got, "image/jpeg")
	}
	if w.Body.Len() == 0 {
		t.Fatal("expected generated thumbnail bytes")
	}
}

func TestGetMediaStatusReadsStoredContentWithoutMediaRow(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	png := testPNGWithDimensions(640, 480)
	cid, err := content.ComputeCID(png)
	if err != nil {
		t.Fatalf("ComputeCID() error: %v", err)
	}
	if err := h.cas.Put(cid, png); err != nil {
		t.Fatalf("CAS Put() error: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/media/"+hex.EncodeToString(cid)+"/status", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("cid", hex.EncodeToString(cid))
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))

	h.GetMediaStatus(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		MimeType string `json:"mime_type"`
		Size     int64  `json:"size"`
		Width    uint32 `json:"width"`
		Height   uint32 `json:"height"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if resp.MimeType != "image/png" {
		t.Fatalf("mime_type = %q, want %q", resp.MimeType, "image/png")
	}
	if resp.Size != int64(len(png)) {
		t.Fatalf("size = %d, want %d", resp.Size, len(png))
	}
	if resp.Width != 640 || resp.Height != 480 {
		t.Fatalf("dimensions = %dx%d, want 640x480", resp.Width, resp.Height)
	}
}

func TestUpdateProfileRejectsTooLongDisplayName(t *testing.T) {
	t.Parallel()
	h, _ := testHandler(t)

	body := strings.NewReader(`{"display_name":"` + strings.Repeat("b", 51) + `"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/profile", body)
	h.UpdateProfile(w, r)

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

func TestGetFeedInternalFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/feed", nil)
	h.GetFeed(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	assertJSONErrorResponse(t, w.Body.Bytes(), "failed to load feed", "sql", "closed")
}

func TestGetNotificationsInternalFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)
	wireServiceBackedHandler(h, db)
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/notifications", nil)
	h.GetNotifications(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	assertJSONErrorResponse(t, w.Body.Bytes(), "failed to load notifications", "sql", "closed")
}

func TestCreateBackupFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	h, db := testHandler(t)
	dbPath := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/node/backup", nil)
	h.CreateBackup(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	assertJSONErrorResponse(t, w.Body.Bytes(), "backup failed", "sql", "closed", dbPath)
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
	h.SetAPITokenConfigured(true)

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
		"enable_web_ui": false,
		"allow_remote_web_ui": false,
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
	if cfg.API.EnableWebUI {
		t.Fatal("expected web UI to be disabled")
	}
	if cfg.API.AllowRemoteWebUI {
		t.Fatal("expected remote web UI exposure to remain disabled")
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

func TestUpdateNodeConfigRejectsRemoteWebUIWithoutOptIn(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.API.ListenAddress = "0.0.0.0:7470"
	cfg.API.EnableWebUI = false
	h.SetConfig(cfg, "")
	h.SetAPITokenConfigured(true)

	body := strings.NewReader(`{"enable_web_ui": true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if cfg.API.EnableWebUI {
		t.Fatal("enable_web_ui mutated after rejected update")
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(resp["error"], "allow_remote_web_ui") {
		t.Fatalf("error = %q, want to mention allow_remote_web_ui", resp["error"])
	}
}

func TestUpdateNodeConfigAllowsRemoteWebUIWithTokenAndOptIn(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.API.ListenAddress = "0.0.0.0:7470"
	cfg.API.EnableWebUI = false
	h.SetConfig(cfg, "")
	h.SetAPITokenConfigured(true)

	body := strings.NewReader(`{"enable_web_ui": true, "allow_remote_web_ui": true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !cfg.API.EnableWebUI {
		t.Fatal("expected web UI to be enabled")
	}
	if !cfg.API.AllowRemoteWebUI {
		t.Fatal("expected remote web UI exposure to be enabled")
	}
}

func TestUpdateNodeConfigSaveFailureDoesNotLeakBackendError(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	h.SetConfig(cfg, t.TempDir())

	body := strings.NewReader(`{"enable_mdns":true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPut, "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	assertJSONErrorResponse(t, w.Body.Bytes(), "failed to save config", "regular file", h.cfgPath)
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

func TestUpdateNodeConfigRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	h, _ := testHandler(t)
	cfg := config.DefaultConfig()
	cfg.Network.MaxPeers = 42
	h.SetConfig(cfg, "")

	body := strings.NewReader(`{"max_connections": 100, "surprise": true}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/node/config", body)
	h.UpdateNodeConfig(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if cfg.Network.MaxPeers != 42 {
		t.Fatalf("max peers mutated to %d after rejected update", cfg.Network.MaxPeers)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !strings.Contains(resp["error"], "surprise") {
		t.Fatalf("error = %q, want to mention surprise", resp["error"])
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
