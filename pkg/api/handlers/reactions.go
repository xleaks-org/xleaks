package handlers

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// createReactionRequest is the JSON body for POST /api/reactions.
type createReactionRequest struct {
	Target string `json:"target"`
}

// createRepostRequest is the JSON body for POST /api/repost.
type createRepostRequest struct {
	PostCID string `json:"post_cid"`
}

// CreateReaction handles POST /api/reactions.
func (h *Handler) CreateReaction(w http.ResponseWriter, r *http.Request) {
	var req createReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Target == "" {
		respondError(w, http.StatusBadRequest, "target is required")
		return
	}

	targetCID, err := hex.DecodeString(req.Target)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid target hex")
		return
	}

	reaction, err := h.reactions.CreateReaction(r.Context(), targetCID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	reactionData := map[string]interface{}{
		"id":            hex.EncodeToString(reaction.Id),
		"author":        hex.EncodeToString(reaction.Author),
		"target":        hex.EncodeToString(reaction.Target),
		"reaction_type": reaction.ReactionType,
		"timestamp":     reaction.Timestamp,
	}
	h.emit("new_reaction", reactionData)
	respondJSON(w, http.StatusCreated, reactionData)
}

// CreateRepost handles POST /api/repost.
func (h *Handler) CreateRepost(w http.ResponseWriter, r *http.Request) {
	var req createRepostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.PostCID == "" {
		respondError(w, http.StatusBadRequest, "post_cid is required")
		return
	}

	postCID, err := hex.DecodeString(req.PostCID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post_cid hex")
		return
	}

	post, err := h.posts.CreateRepost(r.Context(), postCID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        hex.EncodeToString(post.Id),
		"author":    hex.EncodeToString(post.Author),
		"repost_of": hex.EncodeToString(post.RepostOf),
		"timestamp": post.Timestamp,
	})
}
