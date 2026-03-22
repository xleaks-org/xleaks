package handlers

import (
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// GetNotifications handles GET /api/notifications?before=TIMESTAMP.
func (h *Handler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	var before int64
	var err error
	if b := r.URL.Query().Get("before"); b != "" {
		before, err = strconv.ParseInt(b, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid before timestamp")
			return
		}
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil || limit <= 0 {
			limit = 20
		}
	}

	notifs, err := h.notifs.GetNotifications(before, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]map[string]interface{}, 0, len(notifs))
	for _, n := range notifs {
		entry := map[string]interface{}{
			"type":               n.Type,
			"actor_pubkey":       hex.EncodeToString(n.ActorPubkey),
			"actor_display_name": n.ActorDisplayName,
			"actor_avatar_cid":   hexOrEmpty(n.ActorAvatarCID),
			"target_post_cid":    hexOrEmpty(n.TargetPostCID),
			"related_cid":        hexOrEmpty(n.RelatedCID),
			"timestamp":          n.Timestamp,
			"read":               n.Read,
		}

		// Enrich: resolve actor profile for pubkey display.
		if h.db != nil && len(n.ActorPubkey) > 0 {
			profile, profileErr := h.db.GetProfile(n.ActorPubkey)
			if profileErr == nil && profile != nil {
				// Override with freshest profile data from DB.
				if profile.DisplayName != "" {
					entry["actor_display_name"] = profile.DisplayName
				}
				if len(profile.AvatarCID) > 0 {
					entry["actor_avatar_cid"] = hex.EncodeToString(profile.AvatarCID)
				}
				entry["actor_pubkey_hex"] = hex.EncodeToString(profile.Pubkey)
			}
		}

		// Enrich: for like/reply/repost, include a snippet of the target post.
		switch n.Type {
		case "like", "repost":
			if h.db != nil && len(n.TargetPostCID) > 0 {
				post, postErr := h.db.GetPost(n.TargetPostCID)
				if postErr == nil && post != nil {
					entry["target_post_snippet"] = truncate(post.Content, 100)
				}
			}

		case "reply":
			// Include a snippet of the target post (the post being replied to).
			if h.db != nil && len(n.TargetPostCID) > 0 {
				post, postErr := h.db.GetPost(n.TargetPostCID)
				if postErr == nil && post != nil {
					entry["target_post_snippet"] = truncate(post.Content, 100)
				}
			}
			// Include a snippet of the reply itself (the related CID is the reply).
			if h.db != nil && len(n.RelatedCID) > 0 {
				reply, replyErr := h.db.GetPost(n.RelatedCID)
				if replyErr == nil && reply != nil {
					entry["reply_snippet"] = truncate(reply.Content, 100)
				}
			}

		case "dm":
			// For DM notifications, only indicate a message was sent.
			// Do not expose any decrypted content.
			entry["summary"] = "sent you a message"

		case "follow":
			entry["summary"] = "started following you"
		}

		result = append(result, entry)
	}

	respondJSON(w, http.StatusOK, result)
}

// MarkAllNotificationsRead handles PUT /api/notifications/read.
func (h *Handler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	if err := h.db.MarkAllRead(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkNotificationRead handles PUT /api/notifications/{id}/read.
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		respondError(w, http.StatusBadRequest, "missing notification id")
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid notification id")
		return
	}

	if err := h.db.MarkRead(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetUnreadCount handles GET /api/notifications/unread-count.
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	count, err := h.db.UnreadCount()
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"unread_count": count,
	})
}

// truncate returns the first n characters of s (by rune), appending "..." if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "..."
}
