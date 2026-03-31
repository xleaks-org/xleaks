package web

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/identity"
	xlog "github.com/xleaks-org/xleaks/pkg/logging"
	"github.com/xleaks-org/xleaks/pkg/social"
)

// getKeyPair returns the key pair from the current session, or nil if not authenticated.
func (h *Handler) getKeyPair(r *http.Request) *identity.KeyPair {
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		return sess.KeyPair
	}
	return nil
}

// settingsPage serves the settings page.
func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := h.pageData(r, "settings", "Settings")

	displayName := ""
	bio := ""
	website := ""
	avatarCID := ""
	bannerCID := ""
	kp := h.getKeyPair(r)
	if kp != nil {
		profile, err := h.db.GetProfile(kp.PublicKeyBytes())
		if err == nil && profile != nil {
			displayName = profile.DisplayName
			bio = profile.Bio
			website = profile.Website
			avatarCID = hex.EncodeToString(profile.AvatarCID)
			bannerCID = hex.EncodeToString(profile.BannerCID)
		}
	}
	if displayName == "" && data["User"] != nil {
		if user, ok := data["User"].(*UserInfo); ok {
			displayName = user.DisplayName
		}
	}
	if displayName == "" {
		displayName = "Anonymous"
	}
	data["DisplayName"] = displayName
	data["Bio"] = bio
	data["Website"] = website
	data["AvatarCID"] = avatarCID
	data["BannerCID"] = bannerCID
	data["Error"] = r.URL.Query().Get("error")
	data["Success"] = r.URL.Query().Get("success")

	cookie, err := r.Cookie("theme")
	data["DarkMode"] = err != nil || cookie.Value != "light"

	if h.identity != nil {
		if ids, listErr := h.identity.ListIdentities(); listErr == nil {
			views := make([]IdentityView, 0, len(ids))
			for _, id := range ids {
				display := id.DisplayName
				if display == "" {
					display = "Anonymous"
				}
				views = append(views, IdentityView{
					Pubkey:      id.PubkeyHex,
					ShortPubkey: shortenHex(id.PubkeyHex),
					Address:     id.Address,
					DisplayName: display,
					IsActive:    id.IsActive,
				})
			}
			data["Identities"] = views
		}
	}

	if h.nodeStatus != nil {
		peers, uptimeSecs, storageUsed, storageLimit, subscriptions := h.nodeStatus()
		storagePct := 0
		if storageLimit > 0 {
			storagePct = int((storageUsed * 100) / storageLimit)
		}
		data["NodeStatus"] = StatusData{
			Peers:         peers,
			Uptime:        formatDuration(uptimeSecs),
			StorageUsed:   formatBytes(storageUsed),
			StorageMax:    formatBytes(storageLimit),
			Subscriptions: subscriptions,
		}
		data["StoragePct"] = storagePct
	}

	h.renderPage(w, "settings.html", data)
}

// messagesPage serves the messages/DM page.
func (h *Handler) messagesPage(w http.ResponseWriter, r *http.Request) {
	kp := h.getKeyPair(r)
	if kp == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	convos, err := h.db.GetConversations(kp.PublicKeyBytes())
	if err != nil {
		slog.Error("failed to get conversations", "error", err)
	}

	views := make([]ConversationView, 0, len(convos))
	for _, c := range convos {
		peerHex := hex.EncodeToString(c.PeerPubkey)
		peerName := peerHex
		if len(peerName) > 16 {
			peerName = peerName[:16] + "..."
		}
		peerProfile, err := h.db.GetProfile(c.PeerPubkey)
		if err == nil && peerProfile != nil && peerProfile.DisplayName != "" {
			peerName = peerProfile.DisplayName
		}
		preview := ""
		if c.LastTimestamp > 0 {
			msgs, convErr := h.db.GetConversation(kp.PublicKeyBytes(), c.PeerPubkey, 0, 1)
			if convErr == nil && len(msgs) > 0 {
				preview = decryptDMContent(kp, msgs[0])
			}
		}
		if preview == "" {
			preview = "(encrypted)"
		}
		views = append(views, ConversationView{
			PeerPubkey:   peerHex,
			PeerName:     peerName,
			PeerInitial:  getInitial(peerName),
			Preview:      preview,
			RelativeTime: formatRelativeTime(c.LastTimestamp),
			UnreadCount:  c.UnreadCount,
		})
	}

	data := h.pageData(r, "messages", "Messages")
	data["Conversations"] = views
	h.renderPage(w, "messages.html", data)
}

// conversationPage serves a DM conversation detail page.
func (h *Handler) conversationPage(w http.ResponseWriter, r *http.Request) {
	kp := h.getKeyPair(r)
	if kp == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	peerHex := chi.URLParam(r, "pubkey")
	peerBytes, err := hex.DecodeString(peerHex)
	if err != nil {
		http.Error(w, "Invalid public key", http.StatusBadRequest)
		return
	}

	ownPubkey := kp.PublicKeyBytes()
	msgs, err := h.db.GetConversation(ownPubkey, peerBytes, 0, 50)
	if err != nil {
		slog.Error("failed to get conversation", "error", err)
	}
	for _, msg := range msgs {
		if bytes.Equal(msg.Recipient, ownPubkey) && !msg.Read {
			if err := h.db.MarkDMRead(msg.CID); err != nil {
				slog.Warn("failed to mark message read", "error", err)
			}
		}
	}

	peerName := peerHex
	if len(peerName) > 16 {
		peerName = peerName[:16] + "..."
	}
	peerProfile, err := h.db.GetProfile(peerBytes)
	if err == nil && peerProfile != nil && peerProfile.DisplayName != "" {
		peerName = peerProfile.DisplayName
	}

	data := h.pageData(r, "messages", peerName)
	data["PeerPubkey"] = peerHex
	data["PeerName"] = peerName
	data["PeerShortPubkey"] = shortenHex(peerHex)
	data["Messages"] = buildMessageViews(kp, msgs)
	h.renderPage(w, "conversation.html", data)
}

// handleSendDM handles the POST /web/send-dm form submission.
func (h *Handler) handleSendDM(w http.ResponseWriter, r *http.Request) {
	kp := h.getKeyPair(r)
	if kp == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	if h.sendDM == nil {
		http.Error(w, "Direct messaging not configured", http.StatusInternalServerError)
		return
	}
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	recipient := r.FormValue("recipient")
	content := strings.TrimSpace(r.FormValue("content"))
	if recipient == "" || content == "" {
		http.Error(w, "Recipient and content are required", http.StatusBadRequest)
		return
	}
	recipientPubkey, err := hex.DecodeString(recipient)
	if err != nil {
		http.Error(w, "Invalid recipient public key", http.StatusBadRequest)
		return
	}
	if err := h.sendDM(r.Context(), kp, recipientPubkey, content); err != nil {
		slog.Error("failed to send direct message", "error", err)
		http.Error(w, "Failed to send direct message", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	escaped := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(content)
	fmt.Fprint(w, `<div class="flex justify-end"><div class="max-w-[75%] rounded-2xl px-4 py-2 bg-blue-600 text-white">`+
		`<p class="text-sm">`+escaped+`</p><p class="text-xs text-blue-200 mt-1">just now</p></div></div>`)
}

// handleUpdateProfile handles the POST /settings/profile form submission.
func (h *Handler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	kp := h.getKeyPair(r)
	if kp == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.updateProfile == nil {
		http.Error(w, "Profile updates not configured", http.StatusInternalServerError)
		return
	}
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	bio := strings.TrimSpace(r.FormValue("bio"))
	website := strings.TrimSpace(r.FormValue("website"))
	avatarHex := strings.TrimSpace(r.FormValue("avatar_cid"))
	bannerHex := strings.TrimSpace(r.FormValue("banner_cid"))
	if displayName == "" {
		displayName = "Anonymous"
	}
	if err := social.ValidateProfileFields(displayName, bio, website); err != nil {
		http.Redirect(w, r, "/settings?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	var avatarCID, bannerCID []byte
	var err error
	if avatarHex != "" {
		avatarCID, err = hex.DecodeString(avatarHex)
		if err != nil {
			http.Error(w, "Invalid avatar CID", http.StatusBadRequest)
			return
		}
	}
	if bannerHex != "" {
		bannerCID, err = hex.DecodeString(bannerHex)
		if err != nil {
			http.Error(w, "Invalid banner CID", http.StatusBadRequest)
			return
		}
	}

	if err := h.updateProfile(r.Context(), kp, displayName, bio, website, avatarCID, bannerCID); err != nil {
		slog.Error("failed to update profile", "error", err)
		http.Error(w, "Failed to update profile", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/settings?success=Profile+updated", http.StatusSeeOther)
}

// handleExportIdentity exports the encrypted key for the current web session's
// identity instead of relying on whatever identity is globally active.
func (h *Handler) handleExportIdentity(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	sess := h.sessions.GetFromRequest(r)
	if sess == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.identity == nil {
		http.Redirect(w, r, "/settings?error=identity+system+not+available", http.StatusSeeOther)
		return
	}

	enc, err := h.identity.ExportIdentity(sess.PubkeyHex)
	if err != nil {
		slog.Warn("failed to export session identity", "identity", xlog.RedactIdentifier(sess.PubkeyHex), "error", err)
		http.Redirect(w, r, "/settings?error=failed+to+export+identity", http.StatusSeeOther)
		return
	}

	body, filename, err := identity.MarshalExportIdentity(sess.PubkeyHex, enc)
	if err != nil {
		slog.Error("failed to render exported identity", "identity", xlog.RedactIdentifier(sess.PubkeyHex), "error", err)
		http.Redirect(w, r, "/settings?error=failed+to+export+identity", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// handleSwitchIdentity switches the active identity and replaces the current web session.
func (h *Handler) handleSwitchIdentity(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.identity == nil {
		http.Redirect(w, r, "/settings?error=identity+system+not+available", http.StatusSeeOther)
		return
	}
	if err := parseRequestForm(r); err != nil {
		if formBodyTooLarge(err) {
			http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Redirect(w, r, "/settings?error=invalid+form+data", http.StatusSeeOther)
		return
	}

	pubkeyHex := strings.TrimSpace(r.FormValue("pubkey"))
	passphrase := r.FormValue("passphrase")
	if pubkeyHex == "" || passphrase == "" {
		http.Redirect(w, r, "/settings?error=pubkey+and+passphrase+are+required", http.StatusSeeOther)
		return
	}

	if err := h.identity.SwitchIdentity(pubkeyHex, passphrase); err != nil {
		http.Redirect(w, r, "/settings?error="+url.QueryEscape("failed to switch identity"), http.StatusSeeOther)
		return
	}
	h.notifyIdentityChange()
	h.ensureProfile()

	if _, err := h.sessions.RotateForRequest(w, r, h.identity.Get()); err != nil {
		slog.Error("failed to create session after identity switch", "error", err)
		http.Redirect(w, r, "/settings?error=failed+to+create+session", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/settings?success=identity+switched", http.StatusSeeOther)
}

// handleToggleTheme toggles the theme cookie between dark and light.
func (h *Handler) handleToggleTheme(w http.ResponseWriter, r *http.Request) {
	theme := "light"
	cookie, err := r.Cookie("theme")
	if err == nil && cookie.Value == "light" {
		theme = "dark"
	}
	http.SetCookie(w, &http.Cookie{
		Name: "theme", Value: theme, Path: "/",
		MaxAge: 365 * 24 * 60 * 60, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode,
	})
	referer := r.Header.Get("Referer")
	redirectTo := "/settings"
	if referer != "" {
		if u, err := url.Parse(referer); err == nil && u.Path != "" {
			redirectTo = u.Path
		}
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}
