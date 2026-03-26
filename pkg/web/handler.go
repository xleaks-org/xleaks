package web

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/indexer"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// IdentityChangeFunc is called when the active identity changes (create, unlock, import).
type IdentityChangeFunc func(kp *identity.KeyPair)

// Handler serves the web UI HTML pages.
type Handler struct {
	pages            map[string]*template.Template
	landing          *template.Template
	partials         *template.Template
	db               *storage.DB
	identity         *identity.Holder
	sessions         *SessionManager
	timeline         *feed.Timeline
	createPost       CreatePostFunc
	repostPost       RepostFunc
	createReaction   ReactFunc
	followUser       FollowFunc
	unfollowUser     FollowFunc
	updateProfile    UpdateProfileFunc
	sendDM           SendDMFunc
	nodeStatus       NodeStatusFunc
	onIdentityChange IdentityChangeFunc
	indexerClient    *indexer.IndexerClient
	ensureTopic      func(string) error
	enableWebSocket  bool
}

// SetRepost sets the repost callback.
func (h *Handler) SetRepost(fn RepostFunc) { h.repostPost = fn }

// SetCreatePost sets the post creation callback.
func (h *Handler) SetCreatePost(fn CreatePostFunc) {
	h.createPost = fn
}

// SetCreateReaction sets the like/reaction callback.
func (h *Handler) SetCreateReaction(fn ReactFunc) { h.createReaction = fn }

// SetFollow sets the follow callback.
func (h *Handler) SetFollow(fn FollowFunc) { h.followUser = fn }

// SetUnfollow sets the unfollow callback.
func (h *Handler) SetUnfollow(fn FollowFunc) { h.unfollowUser = fn }

// SetUpdateProfile sets the profile update callback.
func (h *Handler) SetUpdateProfile(fn UpdateProfileFunc) { h.updateProfile = fn }

// SetSendDM sets the direct message callback.
func (h *Handler) SetSendDM(fn SendDMFunc) { h.sendDM = fn }

// SetNodeStatus sets the node status callback.
func (h *Handler) SetNodeStatus(fn NodeStatusFunc) {
	h.nodeStatus = fn
}

// SetIndexerClient sets the indexer client for broader search capabilities.
func (h *Handler) SetIndexerClient(ic *indexer.IndexerClient) {
	h.indexerClient = ic
}

// SetTopicSubscriber sets the best-effort runtime topic subscription callback.
func (h *Handler) SetTopicSubscriber(fn func(string) error) {
	h.ensureTopic = fn
}

// SetWebSocketEnabled controls whether the web UI should connect to /ws.
func (h *Handler) SetWebSocketEnabled(enabled bool) {
	h.enableWebSocket = enabled
}

// SetOnIdentityChange sets the callback invoked when the user creates, imports, or unlocks an identity.
func (h *Handler) SetOnIdentityChange(fn IdentityChangeFunc) {
	h.onIdentityChange = fn
}

func (h *Handler) notifyIdentityChange() {
	if h.onIdentityChange != nil {
		h.onIdentityChange(h.identity.Get())
	}
}

// ensureProfile creates a default profile row in the DB if one doesn't exist.
// This is required because the posts table has a FOREIGN KEY to profiles.
func (h *Handler) ensureProfile() {
	kp := h.identity.Get()
	if kp == nil {
		return
	}
	profile, _ := h.db.GetProfile(kp.PublicKeyBytes())
	if profile == nil {
		h.db.UpsertProfile(kp.PublicKeyBytes(), "Anonymous", "", nil, nil, "", 1, time.Now().UnixMilli())
	}
}

// templateFuncMap returns the shared template function map.
func templateFuncMap() template.FuncMap {
	return template.FuncMap{
		"add": func(a, b int) int { return a + b },
		"slice": func(s string, start, end int) string {
			if len(s) == 0 {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			if start > len(s) {
				start = len(s)
			}
			return s[start:end]
		},
		"renderContent": renderContent,
		"truncate": func(s string, max int) string {
			if len(s) <= max {
				return s
			}
			return s[:max] + "..."
		},
	}
}

// NewHandler creates a new web UI handler.
func NewHandler(db *storage.DB, idHolder *identity.Holder, tl *feed.Timeline, sm *SessionManager) (*Handler, error) {
	funcMap := templateFuncMap()

	partials, err := template.New("").Funcs(funcMap).ParseFS(
		templateFS, "templates/feed_items.html", "templates/status_partial.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse partials: %w", err)
	}

	pageFiles := []string{
		"home.html", "onboarding.html", "settings.html", "post.html",
		"profile.html", "notifications.html", "messages.html",
		"search.html", "trending.html", "conversation.html", "terms.html",
		"explore.html",
	}

	pages := make(map[string]*template.Template, len(pageFiles))
	for _, pf := range pageFiles {
		t, err := template.New("layout.html").Funcs(funcMap).ParseFS(
			templateFS, "templates/layout.html", "templates/"+pf,
		)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", pf, err)
		}
		pages[pf] = t
	}

	// Parse the landing page as a standalone template (no layout).
	landing, err := template.New("landing.html").Funcs(funcMap).ParseFS(
		templateFS, "templates/landing.html",
	)
	if err != nil {
		return nil, fmt.Errorf("parse landing template: %w", err)
	}

	return &Handler{
		pages:    pages,
		landing:  landing,
		partials: partials,
		db:       db,
		identity: idHolder,
		sessions: sm,
		timeline: tl,
	}, nil
}

// Routes returns a chi.Router with all web UI routes.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.homePage)
	r.Get("/signup", h.signupPage)
	r.Get("/signin", h.signinPage)
	r.Get("/terms", h.termsPage)
	r.Post("/terms/accept", h.handleAcceptTerms)
	r.Get("/onboarding", h.onboardingPage)
	r.Post("/onboarding/create", h.handleCreate)
	r.Post("/onboarding/verify-step", h.handleVerifyStep)
	r.Post("/onboarding/confirm-seed", h.handleConfirmSeed)
	r.Post("/onboarding/set-profile", h.handleSetProfile)
	r.Post("/onboarding/import", h.handleImport)
	r.Post("/unlock", h.handleUnlock)
	r.Get("/logout", h.handleLogout)
	r.Get("/settings", h.settingsPage)
	r.Post("/settings/switch", h.handleSwitchIdentity)
	r.Get("/post/{id}", h.postPage)
	r.Get("/user/{pubkey}", h.profilePage)
	r.Get("/notifications", h.notificationsPage)
	r.Get("/messages", h.messagesPage)
	r.Get("/messages/{pubkey}", h.conversationPage)
	r.Get("/search", h.searchPage)
	r.Get("/explore", h.explorePage)
	r.Get("/trending", h.trendingPage)
	r.Post("/settings/profile", h.handleUpdateProfile)
	r.Get("/settings/toggle-theme", h.handleToggleTheme)

	r.Get("/web/feed", h.feedPartial)
	r.Post("/web/post", h.handlePost)
	r.Get("/web/search-results", h.searchResultsPartial)
	r.Get("/web/node-status", h.nodeStatusPartial)
	r.Get("/web/trending-tags", h.trendingTagsPartial)
	r.Post("/web/send-dm", h.handleSendDM)
	r.Post("/web/like", h.handleLike)
	r.Post("/web/repost", h.handleRepost)
	r.Post("/web/follow/{pubkey}", h.handleFollow)
	r.Post("/web/unfollow/{pubkey}", h.handleUnfollow)

	return r
}

// currentUser returns the UserInfo for the currently active identity, or nil.
// It first checks the session cookie, then falls back to the global identity holder.
func (h *Handler) currentUser(r *http.Request) *UserInfo {
	// Try session-based identity first.
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		pubkeyHex := sess.PubkeyHex
		address, _ := identity.PubKeyToAddress(sess.KeyPair.PublicKeyBytes())

		displayName := identity.DefaultDisplayName
		bio := ""
		website := ""
		avatarURL := ""
		if profile, err := h.db.GetProfile(sess.KeyPair.PublicKeyBytes()); err == nil && profile != nil {
			if profile.DisplayName != "" {
				displayName = profile.DisplayName
			}
			bio = profile.Bio
			website = profile.Website
			if len(profile.AvatarCID) > 0 {
				avatarURL = "/api/media/" + hex.EncodeToString(profile.AvatarCID)
			}
		}

		short := pubkeyHex
		if len(short) > 16 {
			short = shortenHex(short)
		}

		return &UserInfo{
			DisplayName: displayName,
			Address:     address,
			Pubkey:      pubkeyHex,
			ShortPubkey: short,
			Bio:         bio,
			Website:     website,
			AvatarURL:   avatarURL,
		}
	}

	// No session = not authenticated. No fallback to global identity.
	return nil
}

// pageData returns the base template data for a page.
func (h *Handler) pageData(r *http.Request, active, title string) map[string]interface{} {
	return map[string]interface{}{
		"Active":           active,
		"Title":            title,
		"User":             h.currentUser(r),
		"WebSocketEnabled": h.enableWebSocket,
	}
}

// renderLanding renders the standalone landing page for unauthenticated visitors.
func (h *Handler) renderLanding(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.landing.ExecuteTemplate(w, "landing", nil); err != nil {
		log.Printf("web: template error rendering landing: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// signupPage shows the create identity form (reuses onboarding).
func (h *Handler) signupPage(w http.ResponseWriter, r *http.Request) {
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData(r, "", "Join XLeaks")
	data["NeedsOnboarding"] = true
	h.renderPage(w, "onboarding.html", data)
}

// signinPage shows the import/unlock options.
func (h *Handler) signinPage(w http.ResponseWriter, r *http.Request) {
	sess := h.sessions.GetFromRequest(r)
	if sess != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	data := h.pageData(r, "", "Sign In")
	if h.identity.HasIdentity() {
		data["Locked"] = true
	} else {
		data["NeedsOnboarding"] = true
	}
	h.renderPage(w, "onboarding.html", data)
}

// renderPage renders a full page using the layout and the named content template.
func (h *Handler) renderPage(w http.ResponseWriter, tmplName string, data map[string]interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	t, ok := h.pages[tmplName]
	if !ok {
		log.Printf("web: template %s not found", tmplName)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := t.Execute(w, data); err != nil {
		log.Printf("web: template error rendering %s: %v", tmplName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
