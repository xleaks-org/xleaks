package handlers

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
)

// sendDMRequest is the JSON body for POST /api/dm/{pubkey}.
type sendDMRequest struct {
	Content string `json:"content"`
}

// ListConversations handles GET /api/dm.
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	conversations, err := h.db.GetConversations(h.kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(conversations))
	for _, conv := range conversations {
		result = append(result, map[string]interface{}{
			"peer":           hex.EncodeToString(conv.PeerPubkey),
			"last_timestamp": conv.LastTimestamp,
			"last_author":    hex.EncodeToString(conv.LastAuthor),
			"unread_count":   conv.UnreadCount,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

// GetConversation handles GET /api/dm/{pubkey}?before=TIMESTAMP.
func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	peerPubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var before int64
	if b := r.URL.Query().Get("before"); b != "" {
		before, err = strconv.ParseInt(b, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid before timestamp")
			return
		}
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil || limit <= 0 {
			limit = 50
		}
	}

	messages, err := h.db.GetConversation(h.kp.PublicKeyBytes(), peerPubkey, before, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		result = append(result, map[string]interface{}{
			"id":        hex.EncodeToString(msg.CID),
			"author":    hex.EncodeToString(msg.Author),
			"recipient": hex.EncodeToString(msg.Recipient),
			"timestamp": msg.Timestamp,
			"read":      msg.Read,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

// SendDM handles POST /api/dm/{pubkey}.
func (h *Handler) SendDM(w http.ResponseWriter, r *http.Request) {
	recipientPubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req sendDMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Content == "" {
		respondError(w, http.StatusBadRequest, "content is required")
		return
	}

	dm, err := h.dms.SendDM(r.Context(), recipientPubkey, req.Content)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"id":        hex.EncodeToString(dm.Id),
		"author":    hex.EncodeToString(dm.Author),
		"recipient": hex.EncodeToString(dm.Recipient),
		"timestamp": dm.Timestamp,
	})
}
