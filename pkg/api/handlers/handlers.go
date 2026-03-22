package handlers

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/feed"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/social"
	"github.com/xleaks/xleaks/pkg/storage"
)

// Handler holds all dependencies for HTTP API handlers.
type Handler struct {
	db        *storage.DB
	cas       *content.ContentStore
	kp        *identity.KeyPair
	posts     *social.PostService
	reactions *social.ReactionService
	profiles  *social.ProfileService
	dms       *social.DMService
	notifs    *social.NotificationService
	feed      *feed.Manager
	timeline  *feed.Timeline
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
