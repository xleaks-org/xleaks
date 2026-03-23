package web

import (
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// homePage serves the main feed page, or shows the landing page for unauthenticated visitors.
func (h *Handler) homePage(w http.ResponseWriter, r *http.Request) {
	sess := h.sessions.GetFromRequest(r)
	if sess == nil {
		// No session cookie — show landing page. No fallback to global identity.
		h.renderLanding(w)
		return
	}
	data := h.pageData(r, "home", "Home")
	h.renderPage(w, "home.html", data)
}

// postPage serves a single post detail page.
func (h *Handler) postPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	cidHex := chi.URLParam(r, "id")
	cidBytes, err := hex.DecodeString(cidHex)
	if err != nil {
		data := h.pageData(r, "", "Post")
		data["Post"] = nil
		h.renderPage(w, "post.html", data)
		return
	}

	post, err := h.db.GetPost(cidBytes)
	if err != nil || post == nil {
		data := h.pageData(r, "", "Post")
		data["Post"] = nil
		h.renderPage(w, "post.html", data)
		return
	}

	pv := h.postRowToView(post)

	// Enrich with full reaction counts (replies + reposts).
	_, replies, reposts, _ := h.db.GetFullReactionCounts(cidBytes)
	pv.ReplyCount = replies
	pv.RepostCount = reposts

	data := h.pageData(r, "", "Post")
	data["Post"] = &pv

	// If this post is a reply, load the parent post for context.
	if len(post.ReplyTo) > 0 {
		if parent, err := h.db.GetPost(post.ReplyTo); err == nil && parent != nil {
			parentView := h.postRowToView(parent)
			data["ParentPost"] = &parentView
		}
	}

	h.renderPage(w, "post.html", data)
}

// profilePage serves a user's profile page.
func (h *Handler) profilePage(w http.ResponseWriter, r *http.Request) {
	user := h.currentUser(r)
	if user == nil {
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

	isOwn := user.Pubkey == pubkeyHex

	data := h.pageData(r, "", displayName)
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

	// Follower and following counts.
	followers, _ := h.db.GetFollowers(pubkeyBytes)
	following, _ := h.db.GetFollowing(pubkeyBytes)
	data["FollowerCount"] = len(followers)
	data["FollowingCount"] = len(following)

	// Check if the current user is following this profile.
	data["IsFollowing"] = h.db.IsSubscribed(pubkeyBytes)

	h.renderPage(w, "profile.html", data)
}

// trendingPage serves the trending page.
func (h *Handler) trendingPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData(r, "trending", "Trending")
	h.renderPage(w, "trending.html", data)
}

// notificationsPage serves the notifications page.
func (h *Handler) notificationsPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
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

	data := h.pageData(r, "notifications", "Notifications")
	data["Notifications"] = views
	h.renderPage(w, "notifications.html", data)
}

// searchPage serves the search page.
func (h *Handler) searchPage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData(r, "search", "Search")

	q := r.URL.Query().Get("q")
	data["Query"] = q

	var results []PostView
	if q != "" {
		if strings.HasPrefix(q, "#") {
			tag := strings.TrimPrefix(q, "#")
			posts, err := h.db.GetPostsByTag(tag, 0, 20)
			if err == nil {
				for _, p := range posts {
					results = append(results, h.postRowToView(&p))
				}
			}
		} else {
			posts, err := h.db.SearchPostsByContent(q, 20)
			if err == nil {
				for _, p := range posts {
					results = append(results, h.postRowToView(&p))
				}
			}
		}
	}
	data["Results"] = results
	h.renderPage(w, "search.html", data)
}
