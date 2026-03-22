package handlers

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

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

// EventBroadcaster is a callback that broadcasts real-time events to WebSocket clients.
type EventBroadcaster func(eventType string, data interface{})

// Handler holds all dependencies for HTTP API handlers.
type Handler struct {
	db             *storage.DB
	cas            *content.ContentStore
	kp             *identity.KeyPair
	identity       *identity.Holder
	posts          *social.PostService
	reactions      *social.ReactionService
	profiles       *social.ProfileService
	dms            *social.DMService
	notifs         *social.NotificationService
	feed           *feed.Manager
	timeline       *feed.Timeline
	indexerClient  *indexer.IndexerClient
	p2pHost        *p2p.Host
	cfg            *config.Config
	cfgPath        string
	broadcast      EventBroadcaster
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

// updateIdentity propagates a new key pair to all services that need it.
func (h *Handler) updateIdentity(kp *identity.KeyPair) {
	h.kp = kp
	if h.posts != nil {
		h.posts.SetIdentity(kp)
	}
	if h.reactions != nil {
		h.reactions.SetIdentity(kp)
	}
	if h.profiles != nil {
		h.profiles.SetIdentity(kp)
	}
	if h.dms != nil {
		h.dms.SetIdentity(kp)
	}
}

// New creates a new Handler with all dependencies.
func New(db *storage.DB, cas *content.ContentStore, kp *identity.KeyPair, posts *social.PostService, reactions *social.ReactionService, profiles *social.ProfileService, dms *social.DMService, notifs *social.NotificationService, feed *feed.Manager, timeline *feed.Timeline) *Handler {
	return &Handler{
		db:        db,
		cas:       cas,
		kp:        kp,
		posts:     posts,
		reactions: reactions,
		profiles:  profiles,
		dms:       dms,
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
