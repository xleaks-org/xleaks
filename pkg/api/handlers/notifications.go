package handlers

import (
	"encoding/hex"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// GetNotifications handles GET /api/notifications?before=TIMESTAMP.
func (h *Handler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	before, limit, err := parsePagination(r, 20)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	notifs, err := h.notifs.GetNotifications(kp.PublicKeyBytes(), before, limit)
	if err != nil {
		respondInternalError(w, "failed to load notifications", err, "failed to load notifications")
		return
	}

	// Build a local profile cache to avoid N+1 queries for the same actor.
	profileCache := make(map[string]*storage.ProfileRow)

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

		// Enrich: resolve actor profile for pubkey display (with cache).
		if h.db != nil && len(n.ActorPubkey) > 0 {
			key := hex.EncodeToString(n.ActorPubkey)
			if _, ok := profileCache[key]; !ok {
				p, _ := h.db.GetProfile(n.ActorPubkey)
				profileCache[key] = p
			}
			profile := profileCache[key]
			if profile != nil {
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
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	if err := h.db.MarkAllRead(kp.PublicKeyBytes()); err != nil {
		respondInternalError(w, "failed to mark notifications read", err, "failed to mark notifications read")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// MarkNotificationRead handles PUT /api/notifications/{id}/read.
func (h *Handler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

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

	if err := h.db.MarkRead(kp.PublicKeyBytes(), id); err != nil {
		respondInternalError(w, "failed to mark notification read", err, "failed to mark notification read")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GetUnreadCount handles GET /api/notifications/unread-count.
func (h *Handler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	count, err := h.db.UnreadCount(kp.PublicKeyBytes())
	if err != nil {
		respondInternalError(w, "failed to load unread notification count", err, "failed to load unread count")
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
