package web

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

func decodeJSON(r io.Reader, v interface{}) error {
	return json.NewDecoder(r).Decode(v)
}

//go:embed templates/*.html
var templateFS embed.FS

// UserInfo contains display information about the current user for templates.
type UserInfo struct {
	DisplayName string
	Address     string
	Pubkey      string
	ShortPubkey string
}

// PostView is a template-friendly representation of a post.
type PostView struct {
	ID            string
	AuthorName    string
	AuthorInitial string
	ShortPubkey   string
	Content       string
	RelativeTime  string
	LikeCount     int
	ReplyCount    int
	RepostCount   int
}

// NotificationView is a template-friendly representation of a notification.
type NotificationView struct {
	Type         string
	ActorName    string
	ActorInitial string
	RelativeTime string
	Read         bool
}

// ConversationView is a template-friendly representation of a DM conversation.
type ConversationView struct {
	PeerPubkey  string
	PeerName    string
	PeerInitial string
	Preview     string
	RelativeTime string
	UnreadCount int
}

// ProfileView holds profile data for the profile page template.
type ProfileView struct {
	DisplayName string
	Pubkey      string
	ShortPubkey string
	Initial     string
	Bio         string
}

// CreatePostFunc is a callback to create a post, avoiding direct dependency on social package.
type CreatePostFunc func(ctx context.Context, content string) (id string, err error)

// Handler serves the web UI HTML pages.
type Handler struct {
	pages      map[string]*template.Template
	partials   *template.Template
	db         *storage.DB
	identity   *identity.Holder
	timeline   *feed.Timeline
	createPost CreatePostFunc
}

// SetCreatePost sets the post creation callback.
func (h *Handler) SetCreatePost(fn CreatePostFunc) {
	h.createPost = fn
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
	}
}

// NewHandler creates a new web UI handler.
func NewHandler(db *storage.DB, idHolder *identity.Holder, tl *feed.Timeline) (*Handler, error) {
	funcMap := templateFuncMap()

	// Parse partials (templates that don't use layout).
	partials, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/feed_items.html", "templates/status_partial.html")
	if err != nil {
		return nil, fmt.Errorf("parse partials: %w", err)
	}

	// For each page template, create a combined layout+page template.
	// This avoids the problem of multiple "content" definitions conflicting.
	pageFiles := []string{
		"home.html",
		"onboarding.html",
		"settings.html",
		"post.html",
		"profile.html",
		"notifications.html",
		"messages.html",
		"search.html",
	}

	pages := make(map[string]*template.Template, len(pageFiles))
	for _, pf := range pageFiles {
		t, err := template.New("layout.html").Funcs(funcMap).ParseFS(
			templateFS,
			"templates/layout.html",
			"templates/"+pf,
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

	// Pages
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
	r.Get("/search", h.searchPage)

	// htmx partials
	r.Get("/web/feed", h.feedPartial)
	r.Post("/web/post", h.handlePost)
	r.Get("/web/search-results", h.searchResultsPartial)
	r.Get("/web/node-status", h.nodeStatusPartial)

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
		short = short[:8] + "..." + short[len(short)-4:]
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

// homePage serves the main feed page, or redirects to onboarding if needed.
func (h *Handler) homePage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.HasIdentity() {
		http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
		return
	}

	if !h.identity.IsUnlocked() {
		// Show unlock form
		data := h.pageData("", "Unlock")
		data["Locked"] = true
		h.renderPage(w, "onboarding.html", data)
		return
	}

	data := h.pageData("home", "Home")
	h.renderPage(w, "home.html", data)
}

// onboardingPage serves the create/import identity page.
func (h *Handler) onboardingPage(w http.ResponseWriter, r *http.Request) {
	if h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := h.pageData("", "Get Started")
	if h.identity.HasIdentity() {
		data["Locked"] = true
	} else {
		data["NeedsOnboarding"] = true
	}
	h.renderPage(w, "onboarding.html", data)
}

// handleCreate processes the identity creation form.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", true)
		return
	}

	passphrase := r.FormValue("passphrase")
	confirm := r.FormValue("confirm")

	if len(passphrase) < 8 {
		h.renderOnboardingError(w, "Passphrase must be at least 8 characters", true)
		return
	}

	if passphrase != confirm {
		h.renderOnboardingError(w, "Passphrases do not match", true)
		return
	}

	_, mnemonic, err := h.identity.CreateAndSave(passphrase)
	if err != nil {
		h.renderOnboardingError(w, fmt.Sprintf("Failed to create identity: %v", err), true)
		return
	}

	// Pick 3 random positions for confirmation
	positions := pickRandomPositions(24, 3)

	data := h.pageData("", "Save Seed Phrase")
	data["SeedPhrase"] = mnemonic
	data["SeedWords"] = strings.Fields(mnemonic)
	data["ConfirmPos1"] = positions[0]
	data["ConfirmPos2"] = positions[1]
	data["ConfirmPos3"] = positions[2]
	h.renderPage(w, "onboarding.html", data)
}

// pickRandomPositions returns n unique random positions from 0 to total-1.
func pickRandomPositions(total, n int) []int {
	perm := rand.Perm(total)
	result := perm[:n]
	// Sort for display
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i] > result[j] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// WordSlot represents a word in the seed confirmation grid.
type WordSlot struct {
	Word  string
	Blank bool
}

// handleVerifyStep shows the seed confirmation page (words hidden, blanks for 3 positions).
func (h *Handler) handleVerifyStep(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	seed := r.FormValue("seed")
	words := strings.Fields(seed)
	if len(words) != 24 {
		h.renderOnboardingError(w, "Invalid seed phrase", true)
		return
	}

	positions := pickRandomPositions(24, 3)
	blankSet := map[int]bool{positions[0]: true, positions[1]: true, positions[2]: true}

	slots := make([]WordSlot, 24)
	for i, word := range words {
		if blankSet[i] {
			slots[i] = WordSlot{Blank: true}
		} else {
			slots[i] = WordSlot{Word: word}
		}
	}

	posStr := fmt.Sprintf("%d,%d,%d", positions[0], positions[1], positions[2])

	data := h.pageData("", "Verify Seed Phrase")
	data["ConfirmSeed"] = true
	data["HiddenSeed"] = seed
	data["WordSlots"] = slots
	data["BlankPositions"] = posStr
	h.renderPage(w, "onboarding.html", data)
}

// handleConfirmSeed verifies the user filled in the correct words.
func (h *Handler) handleConfirmSeed(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	seed := r.FormValue("seed")
	words := strings.Fields(seed)

	posStr := r.FormValue("blank_positions")
	posParts := strings.Split(posStr, ",")

	allCorrect := true
	for _, ps := range posParts {
		pos, err := strconv.Atoi(strings.TrimSpace(ps))
		if err != nil || pos >= len(words) {
			allCorrect = false
			break
		}
		entered := strings.TrimSpace(strings.ToLower(r.FormValue(fmt.Sprintf("word_%d", pos))))
		if entered != words[pos] {
			allCorrect = false
			break
		}
	}

	if !allCorrect {
		// Re-show confirm page with error — generate new blank positions
		positions := pickRandomPositions(24, 3)
		blankSet := map[int]bool{positions[0]: true, positions[1]: true, positions[2]: true}
		slots := make([]WordSlot, 24)
		for i, word := range words {
			if blankSet[i] {
				slots[i] = WordSlot{Blank: true}
			} else {
				slots[i] = WordSlot{Word: word}
			}
		}
		posNewStr := fmt.Sprintf("%d,%d,%d", positions[0], positions[1], positions[2])

		data := h.pageData("", "Verify Seed Phrase")
		data["ConfirmSeed"] = true
		data["HiddenSeed"] = seed
		data["WordSlots"] = slots
		data["BlankPositions"] = posNewStr
		data["Error"] = "Some words don't match. Try again."
		h.renderPage(w, "onboarding.html", data)
		return
	}

	// Success — show profile setup
	data := h.pageData("", "Set Your Name")
	data["SetProfile"] = true
	h.renderPage(w, "onboarding.html", data)
}

// handleSetProfile sets the user's display name.
func (h *Handler) handleSetProfile(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	if displayName == "" {
		displayName = "Anonymous"
	}

	if h.identity.IsUnlocked() {
		kp := h.identity.Get()
		if kp != nil {
			h.db.UpsertProfile(kp.PublicKeyBytes(), displayName, "", nil, nil, "", 1, time.Now().UnixMilli())
		}
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handlePost creates a new post from form data using the callback.
func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		http.Error(w, "Post content is required", http.StatusBadRequest)
		return
	}

	if !h.identity.IsUnlocked() {
		http.Error(w, "Identity not unlocked", http.StatusUnauthorized)
		return
	}

	if h.createPost == nil {
		http.Error(w, "Post creation not configured", http.StatusInternalServerError)
		return
	}

	postID, err := h.createPost(r.Context(), content)
	if err != nil {
		log.Printf("Post creation failed: %v", err)
		http.Error(w, "Failed to create post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	user := h.currentUser()
	authorName := "Anonymous"
	authorInitial := "A"
	shortPubkey := ""
	if user != nil {
		authorName = user.DisplayName
		authorInitial = string([]rune(user.DisplayName)[0])
		shortPubkey = user.ShortPubkey
	}

	post := PostView{
		ID:            postID,
		AuthorName:    authorName,
		AuthorInitial: authorInitial,
		ShortPubkey:   shortPubkey,
		Content:       content,
		RelativeTime:  "just now",
		LikeCount:     0,
		ReplyCount:    0,
		RepostCount:   0,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.partials.ExecuteTemplate(w, "feed_items.html", struct{ Posts []PostView }{Posts: []PostView{post}})
}

// handleImport processes the identity import form.
func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", true)
		return
	}

	mnemonic := strings.TrimSpace(r.FormValue("mnemonic"))
	passphrase := r.FormValue("passphrase")

	if mnemonic == "" {
		h.renderOnboardingError(w, "Seed phrase is required", true)
		return
	}

	if len(passphrase) < 8 {
		h.renderOnboardingError(w, "Passphrase must be at least 8 characters", true)
		return
	}

	_, err := h.identity.ImportAndSave(mnemonic, passphrase)
	if err != nil {
		h.renderOnboardingError(w, fmt.Sprintf("Failed to import identity: %v", err), true)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleUnlock processes the unlock form.
func (h *Handler) handleUnlock(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderOnboardingError(w, "Invalid form data", false)
		return
	}

	passphrase := r.FormValue("passphrase")
	if passphrase == "" {
		h.renderOnboardingError(w, "Passphrase is required", false)
		return
	}

	_, err := h.identity.Unlock(passphrase)
	if err != nil {
		data := h.pageData("", "Unlock")
		data["Locked"] = true
		data["Error"] = "Incorrect passphrase"
		h.renderPage(w, "onboarding.html", data)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleLogout locks the identity and redirects to onboarding.
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	h.identity.Lock()
	http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
}

// settingsPage serves the settings page.
func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	data := h.pageData("settings", "Settings")
	h.renderPage(w, "settings.html", data)
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

	short := pubkeyHex
	if len(short) > 16 {
		short = short[:8] + "..." + short[len(short)-4:]
	}

	initial := "?"
	if len(displayName) > 0 {
		initial = string([]rune(displayName)[:1])
	}

	isOwn := false
	kp := h.identity.Get()
	if kp != nil {
		ownPubkey := hex.EncodeToString(kp.PublicKeyBytes())
		isOwn = ownPubkey == pubkeyHex
	}

	data := h.pageData("", displayName)
	data["ProfileUser"] = &ProfileView{
		DisplayName: displayName,
		Pubkey:      pubkeyHex,
		ShortPubkey: short,
		Initial:     initial,
		Bio:         bio,
	}
	data["IsOwnProfile"] = isOwn
	data["PostCount"] = 0 // Could count posts, but keeping it simple.
	h.renderPage(w, "profile.html", data)
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

	views := make([]NotificationView, 0, len(notifs))
	for _, n := range notifs {
		actorName := hex.EncodeToString(n.Actor)
		if len(actorName) > 16 {
			actorName = actorName[:16] + "..."
		}
		actorProfile, err := h.db.GetProfile(n.Actor)
		if err == nil && actorProfile != nil && actorProfile.DisplayName != "" {
			actorName = actorProfile.DisplayName
		}

		initial := "?"
		if len(actorName) > 0 {
			initial = string([]rune(actorName)[:1])
		}

		views = append(views, NotificationView{
			Type:         n.Type,
			ActorName:    actorName,
			ActorInitial: initial,
			RelativeTime: formatRelativeTime(n.Timestamp),
			Read:         n.Read,
		})
	}

	data := h.pageData("notifications", "Notifications")
	data["Notifications"] = views
	h.renderPage(w, "notifications.html", data)
}

// messagesPage serves the messages/DM page.
func (h *Handler) messagesPage(w http.ResponseWriter, r *http.Request) {
	if !h.identity.IsUnlocked() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	kp := h.identity.Get()
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

		initial := "?"
		if len(peerName) > 0 {
			initial = string([]rune(peerName)[:1])
		}

		views = append(views, ConversationView{
			PeerPubkey:   peerHex,
			PeerName:     peerName,
			PeerInitial:  initial,
			Preview:      "(encrypted)",
			RelativeTime: formatRelativeTime(c.LastTimestamp),
			UnreadCount:  c.UnreadCount,
		})
	}

	data := h.pageData("messages", "Messages")
	data["Conversations"] = views
	h.renderPage(w, "messages.html", data)
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

// searchResultsPartial returns search results as an htmx partial.
func (h *Handler) searchResultsPartial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p class="text-sm">Enter a search term.</p></div>`)
		return
	}

	// Search is handled via the API/indexer; for now, show a placeholder.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<div class="text-center py-12 text-gray-400"><p class="text-lg mb-2">No results for "%s"</p><p class="text-sm">Try a different search term.</p></div>`, template.HTMLEscapeString(q))
}

// feedPartial returns feed items as an htmx partial.
func (h *Handler) feedPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if !h.identity.IsUnlocked() {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Not logged in.</p></div>`)
		return
	}

	entries, err := h.timeline.GetFeed(0, 50)
	if err != nil {
		log.Printf("web: failed to get feed: %v", err)
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Failed to load feed.</p></div>`)
		return
	}

	posts := make([]PostView, 0, len(entries))
	for _, e := range entries {
		posts = append(posts, h.entryToView(&e))
	}

	data := map[string]interface{}{
		"Posts": posts,
	}

	if err := h.partials.ExecuteTemplate(w, "feed_items.html", data); err != nil {
		log.Printf("web: template error rendering feed_items: %v", err)
	}
}

// entryToView converts a feed.TimelineEntry to a PostView.
func (h *Handler) entryToView(e *feed.TimelineEntry) PostView {
	cidHex := hex.EncodeToString(e.Post.CID)
	authorHex := hex.EncodeToString(e.Post.Author)

	short := authorHex
	if len(short) > 16 {
		short = short[:8] + "..." + short[len(short)-4:]
	}

	authorName := e.AuthorName
	initial := "?"
	if len(authorName) > 0 {
		initial = string([]rune(authorName)[:1])
	}

	return PostView{
		ID:            cidHex,
		AuthorName:    authorName,
		AuthorInitial: initial,
		ShortPubkey:   short,
		Content:       e.Post.Content,
		RelativeTime:  formatRelativeTime(e.Post.Timestamp),
		LikeCount:     e.LikeCount,
		ReplyCount:    e.ReplyCount,
		RepostCount:   e.RepostCount,
	}
}

// postRowToView converts a storage.PostRow to a PostView (fetching profile data).
func (h *Handler) postRowToView(p *storage.PostRow) PostView {
	cidHex := hex.EncodeToString(p.CID)
	authorHex := hex.EncodeToString(p.Author)

	short := authorHex
	if len(short) > 16 {
		short = short[:8] + "..." + short[len(short)-4:]
	}

	authorName := authorHex[:16] + "..."
	profile, err := h.db.GetProfile(p.Author)
	if err == nil && profile != nil && profile.DisplayName != "" {
		authorName = profile.DisplayName
	}

	initial := "?"
	if len(authorName) > 0 {
		initial = string([]rune(authorName)[:1])
	}

	likeCount, _ := h.db.GetReactionCount(p.CID)

	return PostView{
		ID:            cidHex,
		AuthorName:    authorName,
		AuthorInitial: initial,
		ShortPubkey:   short,
		Content:       p.Content,
		RelativeTime:  formatRelativeTime(p.Timestamp),
		LikeCount:     likeCount,
		ReplyCount:    0,
		RepostCount:   0,
	}
}

// renderOnboardingError renders the onboarding page with an error message.
func (h *Handler) renderOnboardingError(w http.ResponseWriter, errMsg string, needsOnboarding bool) {
	data := h.pageData("", "Get Started")
	if needsOnboarding {
		data["NeedsOnboarding"] = true
	} else {
		data["Locked"] = true
	}
	data["Error"] = errMsg
	h.renderPage(w, "onboarding.html", data)
}

// formatRelativeTime converts a Unix millisecond timestamp to a human-readable
// relative time string (e.g., "2m", "3h", "5d").
func formatRelativeTime(timestampMs int64) string {
	t := time.UnixMilli(timestampMs)
	d := time.Since(t)

	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

// StatusData holds formatted node status for the template.
type StatusData struct {
	Peers         int
	Uptime        string
	StorageUsed   string
	StorageMax    string
	Subscriptions int
}

func formatBytes(b int64) string {
	switch {
	case b < 1024:
		return fmt.Sprintf("%d B", b)
	case b < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	case b < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	default:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	}
}

func formatDuration(secs float64) string {
	d := time.Duration(secs) * time.Second
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h < 24 {
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := h / 24
	rh := h % 24
	if rh > 0 {
		return fmt.Sprintf("%dd %dh", days, rh)
	}
	return fmt.Sprintf("%dd", days)
}

func (h *Handler) nodeStatusPartial(w http.ResponseWriter, r *http.Request) {
	// Get node status from the P2P host via the API internally
	// For simplicity, we query our own API endpoint
	peers := 0
	var uptimeSecs float64
	var storageUsed, storageLimit int64
	subscriptions := 0

	// Read from database directly
	if h.db != nil {
		if count, err := h.db.CountSubscriptions(); err == nil {
			subscriptions = count
		}
	}

	// Get P2P stats if available — call internal API
	resp, err := http.Get("http://127.0.0.1:7470/api/node/status")
	if err == nil {
		defer resp.Body.Close()
		var result struct {
			Peers    int     `json:"peers"`
			Uptime   float64 `json:"uptime"`
			Storage  struct {
				Used  int64 `json:"used"`
				Limit int64 `json:"limit"`
			} `json:"storage"`
			Subscriptions int `json:"subscriptions"`
		}
		if err := decodeJSON(resp.Body, &result); err == nil {
			peers = result.Peers
			uptimeSecs = result.Uptime
			storageUsed = result.Storage.Used
			storageLimit = result.Storage.Limit
			if result.Subscriptions > 0 {
				subscriptions = result.Subscriptions
			}
		}
	}

	data := StatusData{
		Peers:         peers,
		Uptime:        formatDuration(uptimeSecs),
		StorageUsed:   formatBytes(storageUsed),
		StorageMax:    formatBytes(storageLimit),
		Subscriptions: subscriptions,
	}

	tmpl := h.partials.Lookup("status_partial")
	if tmpl == nil {
		http.Error(w, "status template not found", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, data)
}
