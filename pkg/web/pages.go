package web

import (
	"encoding/hex"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// homePage serves the main feed page, or redirects to onboarding if needed.
func (h *Handler) homePage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.HasIdentity() {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}
	if !h.identity.IsUnlocked() {
		data := h.pageData("", "Unlock")
		data["Locked"] = true
		h.renderPage(w, "onboarding.html", data)
		return
	}
	data := h.pageData("home", "Home")
	h.renderPage(w, "home.html", data)
}

// postPage serves a single post detail page.
func (h *Handler) postPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cidHex := chi.URLParam(r, "id")
	cidBytes, err := hex.DecodeString(cidHex)
	if err != nil {
		data := h.pageData("", "Post")
		data["Post"] = nil
		h.renderPage(w, "post.html", data)
		return
	}

	post, err := h.db.GetPost(cidBytes)
	if err != nil || post == nil {
		data := h.pageData("", "Post")
		data["Post"] = nil
		h.renderPage(w, "post.html", data)
		return
	}

	pv := h.postRowToView(post)
	data := h.pageData("", "Post")
	data["Post"] = &pv
	h.renderPage(w, "post.html", data)
}

// profilePage serves a user's profile page.
func (h *Handler) profilePage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	pubkeyHex := chi.URLParam(r, "pubkey")
	pubkeyBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		http.Error(w, "Invalid public key", http.StatusBadRequest)
		return
	}

	displayName := pubkeyHex[:16] + "..."
	bio := ""
	profile, err := h.db.GetProfile(pubkeyBytes)
	if err == nil && profile != nil {
		if profile.DisplayName != "" {
			displayName = profile.DisplayName
		}
		bio = profile.Bio
	}

	isOwn := false
	kp := h.identity.Get()
	if kp != nil {
		isOwn = hex.EncodeToString(kp.PublicKeyBytes()) == pubkeyHex
	}

	data := h.pageData("", displayName)
	data["ProfileUser"] = &ProfileView{
		DisplayName: displayName,
		Pubkey:      pubkeyHex,
		ShortPubkey: shortenHex(pubkeyHex),
		Initial:     getInitial(displayName),
		Bio:         bio,
	}
	data["IsOwnProfile"] = isOwn
	postCount, _ := h.db.CountPostsByAuthor(pubkeyBytes)
	data["PostCount"] = postCount
	h.renderPage(w, "profile.html", data)
}

// trendingPage serves the trending page.
func (h *Handler) trendingPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData("trending", "Trending")
	h.renderPage(w, "trending.html", data)
}

// notificationsPage serves the notifications page.
func (h *Handler) notificationsPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	notifs, err := h.db.GetNotifications(0, 50)
	if err != nil {
		log.Printf("web: failed to get notifications: %v", err)
	}

	// Local profile cache to avoid querying the same actor multiple times (N+1 fix).
	profileNameCache := make(map[string]string, len(notifs))

	views := make([]NotificationView, 0, len(notifs))
	for _, n := range notifs {
		actorHex := hex.EncodeToString(n.Actor)
		actorName, cached := profileNameCache[actorHex]
		if !cached {
			actorName = actorHex
			if len(actorName) > 16 {
				actorName = actorName[:16] + "..."
			}
			actorProfile, err := h.db.GetProfile(n.Actor)
			if err == nil && actorProfile != nil && actorProfile.DisplayName != "" {
				actorName = actorProfile.DisplayName
			}
			profileNameCache[actorHex] = actorName
		}
		views = append(views, NotificationView{
			Type:         n.Type,
			ActorName:    actorName,
			ActorInitial: getInitial(actorName),
			RelativeTime: formatRelativeTime(n.Timestamp),
			Read:         n.Read,
		})
	}

	data := h.pageData("notifications", "Notifications")
	data["Notifications"] = views
	h.renderPage(w, "notifications.html", data)
}

// searchPage serves the search page.
func (h *Handler) searchPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData("search", "Search")
	data["Query"] = ""
	data["Results"] = []PostView(nil)
	h.renderPage(w, "search.html", data)
}
