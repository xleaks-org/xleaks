package handlers

import (
	"encoding/hex"
	"net/http"

	"github.com/xleaks-org/xleaks/pkg/p2p"
)

// Follow handles POST /api/follow/{pubkey}.
func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := h.follows.Follow(r.Context(), pubkey); err != nil {
		respondInternalError(w, "failed to follow user", err, "failed to follow user")
		return
	}
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.FollowsTopic(hex.EncodeToString(pubkey)))
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "followed",
		"pubkey": hex.EncodeToString(pubkey),
	})
}

// Unfollow handles DELETE /api/follow/{pubkey}.
func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := h.follows.Unfollow(r.Context(), pubkey); err != nil {
		respondInternalError(w, "failed to unfollow user", err, "failed to unfollow user")
		return
	}
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.FollowsTopic(hex.EncodeToString(pubkey)))
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "unfollowed",
		"pubkey": hex.EncodeToString(pubkey),
	})
}

// GetFollowing handles GET /api/following.
func (h *Handler) GetFollowing(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	following, err := h.db.GetFollowing(kp.PublicKeyBytes())
	if err != nil {
		respondInternalError(w, "failed to load following", err, "failed to load following")
		return
	}

	result := hexSlice(following)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"following": result,
		"count":     len(result),
	})
}

// GetFollowers handles GET /api/users/{pubkey}/followers.
func (h *Handler) GetFollowers(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	followers, err := h.db.GetFollowers(pubkey)
	if err != nil {
		respondInternalError(w, "failed to load followers", err, "failed to load followers")
		return
	}

	result := hexSlice(followers)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"followers": result,
		"count":     len(result),
	})
}
