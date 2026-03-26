package web

import (
	"encoding/hex"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/p2p"
)

// ExploreUser holds data for a single user card on the explore page.
type ExploreUser struct {
	Pubkey      string
	DisplayName string
	ShortPubkey string
	Initial     string
	Bio         string
}

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
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.ReactionsTopic(cidHex))
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
	if h.ensureTopic != nil {
		_ = h.ensureTopic(p2p.FollowsTopic(pubkeyHex))
	}

	displayName := pubkeyHex[:16] + "..."
	bio := ""
	website := ""
	avatarURL := ""
	bannerURL := ""
	profile, err := h.db.GetProfile(pubkeyBytes)
	if err == nil && profile != nil {
		if profile.DisplayName != "" {
			displayName = profile.DisplayName
		}
		bio = profile.Bio
		website = profile.Website
		if len(profile.AvatarCID) > 0 {
			avatarURL = "/api/media/" + hex.EncodeToString(profile.AvatarCID)
		}
		if len(profile.BannerCID) > 0 {
			bannerURL = "/api/media/" + hex.EncodeToString(profile.BannerCID)
		}
	}

	isOwn := user.Pubkey == pubkeyHex

	data := h.pageData(r, "", displayName)
	data["ProfileUser"] = &ProfileView{
		DisplayName: displayName,
		Pubkey:      pubkeyHex,
		ShortPubkey: shortenHex(pubkeyHex),
		Initial:     getInitial(displayName),
		Bio:         bio,
		Website:     website,
		AvatarURL:   avatarURL,
		BannerURL:   bannerURL,
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
	isFollowing := false
	if kp := h.getKeyPair(r); kp != nil {
		isFollowing = h.db.IsSubscribed(kp.PublicKeyBytes(), pubkeyBytes)
	}
	data["IsFollowing"] = isFollowing

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

	kp := h.getKeyPair(r)
	if kp == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	notifs, err := h.db.GetNotifications(kp.PublicKeyBytes(), 0, 50)
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

	if q != "" {
		results := h.performSearch(q, 20)
		data["PostResults"] = results.Posts
		data["UserResults"] = results.Users
		data["HasResults"] = len(results.Posts) > 0 || len(results.Users) > 0
	} else {
		data["HasResults"] = false
	}
	h.renderPage(w, "search.html", data)
}

// explorePage serves the user directory / explore page.
func (h *Handler) explorePage(w http.ResponseWriter, r *http.Request) {
	if h.currentUser(r) == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Get current user's pubkey to exclude from explore list
	var ownPubkey string
	if user := h.currentUser(r); user != nil {
		ownPubkey = user.Pubkey
	}

	users := make([]ExploreUser, 0, 32)
	seen := make(map[string]struct{}, 32)

	if h.indexerClient != nil && h.indexerClient.Available() {
		publishers, err := h.indexerClient.GetExplorePublishers(32)
		if err != nil {
			log.Printf("web: failed to get explore publishers: %v", err)
		} else {
			for _, publisher := range publishers {
				if publisher.Pubkey == "" || publisher.Pubkey == ownPubkey {
					continue
				}
				if _, ok := seen[publisher.Pubkey]; ok {
					continue
				}

				displayName := publisher.DisplayName
				if displayName == "" {
					displayName = shortenHex(publisher.Pubkey)
				}

				users = append(users, ExploreUser{
					Pubkey:      publisher.Pubkey,
					DisplayName: displayName,
					ShortPubkey: shortenHex(publisher.Pubkey),
					Initial:     getInitial(displayName),
					Bio:         publisher.Bio,
				})
				seen[publisher.Pubkey] = struct{}{}
			}
		}
	}

	profiles, err := h.db.GetAllProfiles()
	if err != nil {
		log.Printf("web: failed to get all profiles: %v", err)
	}
	for _, p := range profiles {
		pubkeyHex := hex.EncodeToString(p.Pubkey)
		if pubkeyHex == ownPubkey {
			continue
		}
		if _, ok := seen[pubkeyHex]; ok {
			continue
		}

		displayName := p.DisplayName
		if displayName == "" {
			displayName = shortenHex(pubkeyHex)
		}
		users = append(users, ExploreUser{
			Pubkey:      pubkeyHex,
			DisplayName: displayName,
			ShortPubkey: shortenHex(pubkeyHex),
			Initial:     getInitial(displayName),
			Bio:         p.Bio,
		})
		seen[pubkeyHex] = struct{}{}
	}
	data := h.pageData(r, "explore", "Explore")
	data["Users"] = users
	h.renderPage(w, "explore.html", data)
}
