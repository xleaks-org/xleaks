package handlers

import (
	"encoding/hex"
	"net/http"

	"github.com/xleaks-org/xleaks/pkg/p2p"
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
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	var req createReactionRequest
	if err := parseJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
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
		respondInternalError(w, "failed to create reaction", err, "failed to create reaction")
		return
	}
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.ReactionsTopic(hex.EncodeToString(targetCID)))
	}

	reactionData := map[string]interface{}{
		"id":            hex.EncodeToString(reaction.Id),
		"author":        hex.EncodeToString(reaction.Author),
		"target":        hex.EncodeToString(reaction.Target),
		"reaction_type": reaction.ReactionType,
		"timestamp":     reaction.Timestamp,
	}
	h.emit(EventNewReaction, reactionData)
	respondJSON(w, http.StatusCreated, reactionData)
}

// CreateRepost handles POST /api/repost.
func (h *Handler) CreateRepost(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	var req createRepostRequest
	if err := parseJSON(w, r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
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
		respondInternalError(w, "failed to create repost", err, "failed to create repost")
		return
	}

	repostData := map[string]interface{}{
		"id":        hex.EncodeToString(post.Id),
		"author":    hex.EncodeToString(post.Author),
		"repost_of": hex.EncodeToString(post.RepostOf),
		"timestamp": post.Timestamp,
	}
	h.emit(EventNewRepost, repostData)
	respondJSON(w, http.StatusCreated, repostData)
}
