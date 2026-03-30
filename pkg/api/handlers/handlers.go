package handlers

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/p2p"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// DefaultDisplayName is the fallback name for users who haven't set a profile.
const DefaultDisplayName = "Anonymous"

const maxJSONBodyBytes = 1 << 20

// WebSocket event type constants.
const (
	EventNewPost         = "new_post"
	EventNewReaction     = "new_reaction"
	EventNewRepost       = "new_repost"
	EventNewNotification = "new_notification"
	EventNewDM           = "new_dm"
)

// EventBroadcaster is a callback that broadcasts real-time events to WebSocket clients.
type EventBroadcaster func(eventType string, data interface{})

// Handler holds all dependencies for HTTP API handlers.
type Handler struct {
	db                 *storage.DB
	cas                *content.ContentStore
	kp                 *identity.KeyPair
	identity           *identity.Holder
	posts              *social.PostService
	reactions          *social.ReactionService
	profiles           *social.ProfileService
	dms                *social.DMService
	follows            *social.FollowService
	notifs             *social.NotificationService
	feed               *feed.Manager
	timeline           *feed.Timeline
	indexerClient      *indexer.IndexerClient
	p2pHost            *p2p.Host
	cfg                *config.Config
	cfgPath            string
	apiTokenConfigured bool
	broadcast          EventBroadcaster
	onIdentityChange   func(*identity.KeyPair)
	ensureTopic        func(string) error
}

// SetBroadcaster sets the WebSocket event broadcaster.
func (h *Handler) SetBroadcaster(b EventBroadcaster) {
	h.broadcast = b
}

func (h *Handler) emit(eventType string, data interface{}) {
	if h.broadcast != nil {
		h.broadcast(eventType, data)
	}
}

// updateIdentity updates the handler's active key pair and invokes the shared
// identity-change hook used by the runtime.
func (h *Handler) updateIdentity(kp *identity.KeyPair) {
	h.kp = kp
	if h.onIdentityChange != nil {
		h.onIdentityChange(kp)
	}
}

// New creates a new Handler with all dependencies.
func New(db *storage.DB, cas *content.ContentStore, kp *identity.KeyPair, posts *social.PostService, reactions *social.ReactionService, profiles *social.ProfileService, dms *social.DMService, follows *social.FollowService, notifs *social.NotificationService, feed *feed.Manager, timeline *feed.Timeline) *Handler {
	return &Handler{
		db:        db,
		cas:       cas,
		kp:        kp,
		posts:     posts,
		reactions: reactions,
		profiles:  profiles,
		dms:       dms,
		follows:   follows,
		notifs:    notifs,
		feed:      feed,
		timeline:  timeline,
	}
}

// SetIdentityHolder sets the shared identity holder for create/unlock/lock operations.
func (h *Handler) SetIdentityHolder(holder *identity.Holder) {
	h.identity = holder
}

// SetIndexerClient sets the indexer client used for search and trending queries.
func (h *Handler) SetIndexerClient(client *indexer.IndexerClient) {
	h.indexerClient = client
}

// SetP2PHost sets the P2P host for node status and peer queries.
func (h *Handler) SetP2PHost(host *p2p.Host) {
	h.p2pHost = host
}

// SetConfig sets the node configuration and its file path for config endpoints.
func (h *Handler) SetConfig(cfg *config.Config, cfgPath string) {
	h.cfg = cfg
	h.cfgPath = cfgPath
}

// SetAPITokenConfigured records whether the running server has token auth enabled.
func (h *Handler) SetAPITokenConfigured(configured bool) {
	h.apiTokenConfigured = configured
}

// SetIdentityChangeFunc registers the shared runtime hook for create, unlock,
// switch, and lock transitions.
func (h *Handler) SetIdentityChangeFunc(fn func(*identity.KeyPair)) {
	h.onIdentityChange = fn
}

// SetTopicSubscriber registers a best-effort callback for joining runtime
// subscriptions needed by API-driven views.
func (h *Handler) SetTopicSubscriber(fn func(string) error) {
	h.ensureTopic = fn
}

func (h *Handler) currentKeyPair() *identity.KeyPair {
	if h.identity != nil {
		if kp := h.identity.Get(); isUsableKeyPair(kp) {
			return kp
		}
	}
	if isUsableKeyPair(h.kp) {
		return h.kp
	}
	return nil
}

func (h *Handler) requireIdentity(w http.ResponseWriter) (*identity.KeyPair, bool) {
	kp := h.currentKeyPair()
	if kp == nil {
		respondError(w, http.StatusUnauthorized, "identity is locked or not initialized")
		return nil, false
	}
	return kp, true
}

func (h *Handler) passphraseMinLen() int {
	if h.cfg != nil {
		return h.cfg.PassphraseMinLen()
	}
	return config.DefaultConfig().PassphraseMinLen()
}

func isUsableKeyPair(kp *identity.KeyPair) bool {
	if kp == nil {
		return false
	}
	pubkey := kp.PublicKeyBytes()
	if len(pubkey) == 0 {
		return false
	}
	for _, b := range pubkey {
		if b != 0 {
			return true
		}
	}
	return false
}

// respondJSON writes a JSON response with the given status code.
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// respondError writes a JSON error response with the given status code and message.
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// parseHexParam extracts a URL parameter by name and decodes it from hex to bytes.
func parseHexParam(r *http.Request, name string) ([]byte, error) {
	param := chi.URLParam(r, name)
	if param == "" {
		return nil, fmt.Errorf("missing parameter: %s", name)
	}
	b, err := hex.DecodeString(param)
	if err != nil {
		return nil, fmt.Errorf("invalid hex for %s: %w", name, err)
	}
	return b, nil
}

// parsePagination extracts "before" and "limit" query parameters for cursor-based pagination.
func parsePagination(r *http.Request, defaultLimit int) (before int64, limit int) {
	limit = defaultLimit
	if b := r.URL.Query().Get("before"); b != "" {
		if v, err := strconv.ParseInt(b, 10, 64); err == nil {
			before = v
		}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	return
}

// parseJSON decodes the request body as JSON into v.
func parseJSON(w http.ResponseWriter, r *http.Request, v interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return fmt.Errorf("JSON body too large")
		}
		return fmt.Errorf("invalid JSON body")
	}
	var trailing struct{}
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("invalid JSON body")
	}
	return nil
}

// hexSlice encodes a slice of byte slices to a slice of hex strings.
func hexSlice(items [][]byte) []string {
	result := make([]string, len(items))
	for i, item := range items {
		result[i] = hex.EncodeToString(item)
	}
	return result
}
