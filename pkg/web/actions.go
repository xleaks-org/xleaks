package web

import (
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/identity"
)

// getKeyPair returns the key pair from the session or falls back to the global identity.
func (h *Handler) getKeyPair(r *http.Request) *identity.KeyPair {
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		return sess.KeyPair
	}
	return h.identity.Get()
}

// settingsPage serves the settings page.
func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := h.pageData(r, "settings", "Settings")

	bio := ""
	kp := h.getKeyPair(r)
	if kp != nil {
		profile, err := h.db.GetProfile(kp.PublicKeyBytes())
		if err == nil && profile != nil {
			bio = profile.Bio
		}
	}
	data["Bio"] = bio

	cookie, err := r.Cookie("theme")
	data["DarkMode"] = err != nil || cookie.Value != "light"

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
		log.Printf("web: failed to get conversations: %v", err)
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
		views = append(views, ConversationView{
			PeerPubkey:   peerHex,
			PeerName:     peerName,
			PeerInitial:  getInitial(peerName),
			Preview:      "(encrypted)",
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
		log.Printf("web: failed to get conversation: %v", err)
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
	data["Messages"] = buildMessageViews(msgs, ownPubkey)
	h.renderPage(w, "conversation.html", data)
}

// handleSendDM handles the POST /web/send-dm form submission.
func (h *Handler) handleSendDM(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	r.ParseForm()
	recipient := r.FormValue("recipient")
	content := strings.TrimSpace(r.FormValue("content"))
	if recipient == "" || content == "" {
		http.Error(w, "Recipient and content are required", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<div class="flex justify-end"><div class="max-w-[75%] rounded-2xl px-4 py-2 bg-blue-600 text-white">`+
		`<p class="text-sm">(encrypted)</p><p class="text-xs text-blue-200 mt-1">just now</p></div></div>`)
}

// handleUpdateProfile handles the POST /settings/profile form submission.
func (h *Handler) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	kp := h.getKeyPair(r)
	if kp == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	r.ParseForm()
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	bio := strings.TrimSpace(r.FormValue("bio"))
	if displayName == "" {
		displayName = "Anonymous"
	}
	var version uint64 = 1
	profile, err := h.db.GetProfile(kp.PublicKeyBytes())
	if err == nil && profile != nil {
		version = profile.Version + 1
	}
	h.db.UpsertProfile(kp.PublicKeyBytes(), displayName, bio, nil, nil, "", version, time.Now().UnixMilli())
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
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
		MaxAge: 365 * 24 * 60 * 60, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	referer := r.Header.Get("Referer")
	if referer == "" {
		referer = "/settings"
	}
	http.Redirect(w, r, referer, http.StatusSeeOther)
}
