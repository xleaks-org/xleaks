package handlers

import (
	"bytes"
	"encoding/hex"
	"net/http"

	"github.com/xleaks-org/xleaks/pkg/social"
	pb "github.com/xleaks-org/xleaks/proto/gen"
)

// sendDMRequest is the JSON body for POST /api/dm/{pubkey}.
type sendDMRequest struct {
	Content string `json:"content"`
}

// ListConversations handles GET /api/dm.
func (h *Handler) ListConversations(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	conversations, err := h.db.GetConversations(kp.PublicKeyBytes())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(conversations))
	for _, conv := range conversations {
		preview := ""
		if h.dms != nil && len(conv.EncryptedContent) > 0 {
			recipient := kp.PublicKeyBytes()
			if bytes.Equal(conv.LastAuthor, kp.PublicKeyBytes()) {
				recipient = conv.PeerPubkey
			}
			preview = decryptDMPreview(h.dms, &pb.DirectMessage{
				Author:           conv.LastAuthor,
				Recipient:        recipient,
				EncryptedContent: conv.EncryptedContent,
				Nonce:            conv.Nonce,
			})
		}
		result = append(result, map[string]interface{}{
			"peer":           hex.EncodeToString(conv.PeerPubkey),
			"last_timestamp": conv.LastTimestamp,
			"last_author":    hex.EncodeToString(conv.LastAuthor),
			"preview":        preview,
			"unread_count":   conv.UnreadCount,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

// GetConversation handles GET /api/dm/{pubkey}?before=TIMESTAMP.
func (h *Handler) GetConversation(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	peerPubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	before, limit := parsePagination(r, 50)

	messages, err := h.db.GetConversation(kp.PublicKeyBytes(), peerPubkey, before, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		if bytes.Equal(msg.Recipient, kp.PublicKeyBytes()) && !msg.Read {
			if err := h.db.MarkDMRead(msg.CID); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			msg.Read = true
		}
		content := ""
		if h.dms != nil {
			content = decryptDMPreview(h.dms, &pb.DirectMessage{
				Id:               msg.CID,
				Author:           msg.Author,
				Recipient:        msg.Recipient,
				EncryptedContent: msg.EncryptedContent,
				Nonce:            msg.Nonce,
			})
		}
		result = append(result, map[string]interface{}{
			"id":        hex.EncodeToString(msg.CID),
			"author":    hex.EncodeToString(msg.Author),
			"recipient": hex.EncodeToString(msg.Recipient),
			"content":   content,
			"timestamp": msg.Timestamp,
			"read":      msg.Read,
		})
	}

	respondJSON(w, http.StatusOK, result)
}

func decryptDMPreview(service *social.DMService, dm *pb.DirectMessage) string {
	if service == nil || dm == nil {
		return ""
	}
	plaintext, err := service.DecryptDM(dm)
	if err != nil {
		return ""
	}
	return plaintext
}

// SendDM handles POST /api/dm/{pubkey}.
func (h *Handler) SendDM(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireIdentity(w); !ok {
		return
	}

	recipientPubkey, err := parseHexParam(r, "pubkey")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req sendDMRequest
	if err := parseJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
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

	dmData := map[string]interface{}{
		"id":        hex.EncodeToString(dm.Id),
		"author":    hex.EncodeToString(dm.Author),
		"recipient": hex.EncodeToString(dm.Recipient),
		"timestamp": dm.Timestamp,
	}
	h.emit(EventNewDM, dmData)
	respondJSON(w, http.StatusCreated, dmData)
}
