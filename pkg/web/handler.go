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
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// IdentityChangeFunc is called when the active identity changes (create, unlock, import).
type IdentityChangeFunc func(kp *identity.KeyPair)

// Handler serves the web UI HTML pages.
type Handler struct {
	pages            map[string]*template.Template
	partials         *template.Template
	db               *storage.DB
	identity         *identity.Holder
	timeline         *feed.Timeline
	createPost       CreatePostFunc
	nodeStatus       NodeStatusFunc
	onIdentityChange IdentityChangeFunc
}

// SetCreatePost sets the post creation callback.
func (h *Handler) SetCreatePost(fn CreatePostFunc) {
	h.createPost = fn
}

// SetNodeStatus sets the node status callback.
func (h *Handler) SetNodeStatus(fn NodeStatusFunc) {
	h.nodeStatus = fn
}

// SetOnIdentityChange sets the callback invoked when the user creates, imports, or unlocks an identity.
func (h *Handler) SetOnIdentityChange(fn IdentityChangeFunc) {
	h.onIdentityChange = fn
}

func (h *Handler) notifyIdentityChange() {
	if h.onIdentityChange != nil {
		kp := h.identity.Get()
		if kp != nil {
			h.onIdentityChange(kp)
		}
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
	}
}

// NewHandler creates a new web UI handler.
func NewHandler(db *storage.DB, idHolder *identity.Holder, tl *feed.Timeline) (*Handler, error) {
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
		"search.html", "trending.html", "conversation.html",
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

	return &Handler{
		pages:    pages,
		partials: partials,
		db:       db,
		identity: idHolder,
		timeline: tl,
	}, nil
}

// Routes returns a chi.Router with all web UI routes.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.homePage)
	r.Get("/onboarding", h.onboardingPage)
	r.Post("/onboarding/create", h.handleCreate)
	r.Post("/onboarding/verify-step", h.handleVerifyStep)
	r.Post("/onboarding/confirm-seed", h.handleConfirmSeed)
	r.Post("/onboarding/set-profile", h.handleSetProfile)
	r.Post("/onboarding/import", h.handleImport)
	r.Post("/unlock", h.handleUnlock)
	r.Get("/logout", h.handleLogout)
	r.Get("/settings", h.settingsPage)
	r.Get("/post/{id}", h.postPage)
	r.Get("/user/{pubkey}", h.profilePage)
	r.Get("/notifications", h.notificationsPage)
	r.Get("/messages", h.messagesPage)
	r.Get("/messages/{pubkey}", h.conversationPage)
	r.Get("/search", h.searchPage)
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
func (h *Handler) currentUser() *UserInfo {
	kp := h.identity.Get()
	if kp == nil {
		return nil
	}

	pubkeyHex := hex.EncodeToString(kp.PublicKeyBytes())
	address, _ := identity.PubKeyToAddress(kp.PublicKeyBytes())

	displayName := identity.DefaultDisplayName
	profile, err := h.db.GetProfile(kp.PublicKeyBytes())
	if err == nil && profile != nil && profile.DisplayName != "" {
		displayName = profile.DisplayName
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
	}
}

// pageData returns the base template data for a page.
func (h *Handler) pageData(active, title string) map[string]interface{} {
	return map[string]interface{}{
		"Active": active,
		"Title":  title,
		"User":   h.currentUser(),
	}
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
