package web

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// feedPartial returns feed items as an htmx partial.
func (h *Handler) feedPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if !h.identity.IsUnlocked() {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Not logged in.</p></div>`)
		return
	}

	// Handle reply_to filter: load replies to a specific post.
	replyTo := r.URL.Query().Get("reply_to")
	if replyTo != "" {
		cidBytes, err := hex.DecodeString(replyTo)
		if err == nil {
			replies, err := h.db.GetThread(cidBytes)
			if err == nil {
				var posts []PostView
				for i := range replies {
					posts = append(posts, h.postRowToView(&replies[i]))
				}
				data := struct{ Posts []PostView }{Posts: posts}
				if err := h.partials.ExecuteTemplate(w, "feed_items.html", data); err != nil {
					log.Printf("web: template error rendering reply feed: %v", err)
				}
				return
			}
			log.Printf("web: failed to get thread for %s: %v", replyTo, err)
		}
	}

	const pageSize = 20
	var before int64
	if b := r.URL.Query().Get("before"); b != "" {
		before, _ = strconv.ParseInt(b, 10, 64)
	}

	entries, err := h.timeline.GetFeed(before, pageSize+1)
	if err != nil {
		log.Printf("web: failed to get feed: %v", err)
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Failed to load feed.</p></div>`)
		return
	}

	hasMore := len(entries) > pageSize
	if hasMore {
		entries = entries[:pageSize]
	}

	posts := make([]PostView, 0, len(entries))
	for _, e := range entries {
		posts = append(posts, h.entryToView(&e))
	}

	if err := h.partials.ExecuteTemplate(w, "feed_items.html", map[string]interface{}{"Posts": posts}); err != nil {
		log.Printf("web: template error rendering feed_items: %v", err)
	}

	if hasMore && len(entries) > 0 {
		lastTs := entries[len(entries)-1].Post.Timestamp
		fmt.Fprintf(w, `<div class="text-center py-4">`+
			`<button hx-get="/web/feed?before=%d" hx-target="closest div" hx-swap="outerHTML" `+
			`class="text-blue-500 hover:text-blue-400 text-sm">Load more</button></div>`, lastTs)
	}
}

// handlePost creates a new post from form data using the callback.
func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	content := strings.TrimSpace(r.FormValue("content"))
	replyTo := strings.TrimSpace(r.FormValue("reply_to"))
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

	postID, err := h.createPost(r.Context(), content, replyTo)
	if err != nil {
		log.Printf("Post creation failed: %v", err)
		http.Error(w, "Failed to create post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	post := h.buildNewPostView(postID, content)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.partials.ExecuteTemplate(w, "feed_items.html", struct{ Posts []PostView }{Posts: []PostView{post}})
}

// nodeStatusPartial returns the node status as an htmx partial.
func (h *Handler) nodeStatusPartial(w http.ResponseWriter, r *http.Request) {
	peers := 0
	var uptimeSecs float64
	var storageUsed, storageLimit int64
	subscriptions := 0

	if h.nodeStatus != nil {
		peers, uptimeSecs, storageUsed, storageLimit, subscriptions = h.nodeStatus()
	} else if h.db != nil {
		if count, err := h.db.CountSubscriptions(); err == nil {
			subscriptions = count
		}
	}

	tmpl := h.partials.Lookup("status_partial")
	if tmpl == nil {
		http.Error(w, "status template not found", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, StatusData{
		Peers: peers, Uptime: formatDuration(uptimeSecs),
		StorageUsed: formatBytes(storageUsed), StorageMax: formatBytes(storageLimit),
		Subscriptions: subscriptions,
	})
}

// searchResultsPartial returns search results as an htmx partial.
func (h *Handler) searchResultsPartial(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if q == "" {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p class="text-sm">Enter a search term.</p></div>`)
		return
	}

	var postRows []storage.PostRow
	if strings.HasPrefix(q, "#") {
		postRows, _ = h.db.GetPostsByTag(strings.TrimPrefix(q, "#"), 0, 20)
	} else {
		postRows, _ = h.db.SearchPostsByContent(q, 20)
	}

	if len(postRows) == 0 {
		fmt.Fprintf(w, `<div class="text-center py-12 text-gray-400">`+
			`<p class="text-lg mb-2">No results for "%s"</p>`+
			`<p class="text-sm">Try a different search term.</p></div>`,
			template.HTMLEscapeString(q))
		return
	}

	posts := make([]PostView, 0, len(postRows))
	for i := range postRows {
		posts = append(posts, h.postRowToView(&postRows[i]))
	}
	if err := h.partials.ExecuteTemplate(w, "feed_items.html", map[string]interface{}{"Posts": posts}); err != nil {
		log.Printf("web: template error rendering search results: %v", err)
	}
}

// trendingTagsPartial returns trending hashtags as an htmx partial.
func (h *Handler) trendingTagsPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tags, err := h.db.GetTrendingTags(10)
	if err != nil {
		log.Printf("web: failed to get trending tags: %v", err)
		fmt.Fprint(w, `<p class="text-gray-400 text-sm">Could not load trending topics.</p>`)
		return
	}
	if len(tags) == 0 {
		fmt.Fprint(w, `<p class="text-gray-400 text-sm">No trending topics yet.</p>`)
		return
	}
	for _, tag := range tags {
		fmt.Fprintf(w, `<a href="/search?q=%%23%s" `+
			`class="block py-2 border-b border-gray-800 last:border-0 hover:bg-gray-800/50 -mx-4 px-4 transition-colors">`+
			`<span class="font-semibold text-sm">#%s</span>`+
			`<span class="text-xs text-gray-500 ml-2">%d posts</span></a>`,
			template.HTMLEscapeString(tag.Tag), template.HTMLEscapeString(tag.Tag), tag.Count)
	}
}

// handleLike creates a like reaction and returns the updated button HTML.
func (h *Handler) handleLike(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	target := r.FormValue("target")
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	targetBytes, err := hex.DecodeString(target)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	kp := h.identity.Get()
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}

	// Compute a deterministic CID for the reaction so duplicates are ignored.
	cid, _ := content.ComputeCID(append(kp.PublicKeyBytes(), targetBytes...))
	if err := h.db.InsertReaction(cid, kp.PublicKeyBytes(), targetBytes, "like", time.Now().UnixMilli()); err != nil {
		log.Printf("web: failed to insert like reaction: %v", err)
	}
	if err := h.db.UpdateReactionCount(targetBytes); err != nil {
		log.Printf("web: failed to update reaction count: %v", err)
	}

	likes, _, _, _ := h.db.GetFullReactionCounts(targetBytes)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span class="text-red-400">`+
		`<svg class="w-4 h-4 inline" fill="currentColor" stroke="currentColor" viewBox="0 0 24 24">`+
		`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" `+
		`d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"/>`+
		`</svg> %d</span>`, likes)
}

// handleRepost creates a repost reaction and returns the updated button HTML.
func (h *Handler) handleRepost(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	target := r.FormValue("target")
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	targetBytes, err := hex.DecodeString(target)
	if err != nil {
		http.Error(w, "invalid target", http.StatusBadRequest)
		return
	}

	kp := h.identity.Get()
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}

	// Insert a repost as a reaction (type "repost").
	repostData := append([]byte("repost:"), append(kp.PublicKeyBytes(), targetBytes...)...)
	cid, _ := content.ComputeCID(repostData)
	if err := h.db.InsertReaction(cid, kp.PublicKeyBytes(), targetBytes, "repost", time.Now().UnixMilli()); err != nil {
		log.Printf("web: failed to insert repost reaction: %v", err)
	}
	if err := h.db.UpdateReactionCount(targetBytes); err != nil {
		log.Printf("web: failed to update reaction count: %v", err)
	}

	_, _, reposts, _ := h.db.GetFullReactionCounts(targetBytes)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span class="text-green-400">`+
		`<svg class="w-4 h-4 inline" fill="none" stroke="currentColor" viewBox="0 0 24 24">`+
		`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" `+
		`d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/>`+
		`</svg> %d</span>`, reposts)
}

// handleFollow subscribes to a user and redirects back to their profile.
func (h *Handler) handleFollow(w http.ResponseWriter, r *http.Request) {
	pubkeyHex := chi.URLParam(r, "pubkey")
	pubkeyBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}

	kp := h.identity.Get()
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}

	if err := h.db.AddSubscription(pubkeyBytes, time.Now().UnixMilli()); err != nil {
		log.Printf("web: failed to follow: %v", err)
	}

	http.Redirect(w, r, "/user/"+pubkeyHex, http.StatusSeeOther)
}

// handleUnfollow removes a subscription and redirects back to the profile.
func (h *Handler) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	pubkeyHex := chi.URLParam(r, "pubkey")
	pubkeyBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}

	kp := h.identity.Get()
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}

	if err := h.db.RemoveSubscription(pubkeyBytes); err != nil {
		log.Printf("web: failed to unfollow: %v", err)
	}

	http.Redirect(w, r, "/user/"+pubkeyHex, http.StatusSeeOther)
}
