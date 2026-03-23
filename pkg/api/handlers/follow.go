package handlers

import (
	"encoding/hex"
	"net/http"
	"time"
)

// Follow handles POST /api/follow/{pubkey}.
func (h *Handler) Follow(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	timestamp := time.Now().UnixMilli()
	if err := h.feed.Follow(r.Context(), pubkey, timestamp); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Also record the follow event for follower counts.
	if err := h.db.InsertFollowEvent(h.kp.PublicKeyBytes(), pubkey, "follow", timestamp); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "followed",
		"pubkey":  hex.EncodeToString(pubkey),
	})
}

// Unfollow handles DELETE /api/follow/{pubkey}.
func (h *Handler) Unfollow(w http.ResponseWriter, r *http.Request) {
	pubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.feed.Unfollow(pubkey); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Also record the unfollow event.
	timestamp := time.Now().UnixMilli()
	if err := h.db.InsertFollowEvent(h.kp.PublicKeyBytes(), pubkey, "unfollow", timestamp); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "unfollowed",
		"pubkey": hex.EncodeToString(pubkey),
	})
}

// GetFollowing handles GET /api/following.
func (h *Handler) GetFollowing(w http.ResponseWriter, r *http.Request) {
	following, err := h.db.GetFollowing(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := hexSlice(followers)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"followers": result,
		"count":     len(result),
	})
}
