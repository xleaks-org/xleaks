package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/xleaks-org/xleaks/pkg/storage"
)

// feedPartial returns feed items as an htmx partial.
func (h *Handler) feedPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if !h.identity.IsUnlocked() {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Not logged in.</p></div>`)
		return
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
