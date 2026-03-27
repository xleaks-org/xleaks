package web

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
)

// feedPartial returns feed items as an htmx partial.
func (h *Handler) feedPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if h.currentUser(r) == nil {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400">`+
			`<p class="mb-4">Session expired</p>`+
			`<a href="/" class="text-blue-500 hover:underline">Sign in again</a></div>`)
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
					slog.Error("template error rendering reply feed", "error", err)
				}
				return
			}
			slog.Warn("failed to get thread", "reply_to", replyTo, "error", err)
		}
	}

	const pageSize = 20
	var before int64
	if b := r.URL.Query().Get("before"); b != "" {
		before, _ = strconv.ParseInt(b, 10, 64)
	}

	// Handle author filter: load posts by a specific user (profile page).
	author := r.URL.Query().Get("author")
	if author != "" {
		authorBytes, err := hex.DecodeString(author)
		if err != nil {
			slog.Warn("invalid author pubkey", "author", author, "error", err)
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Invalid author key.</p></div>`)
			return
		}
		entries, err := h.timeline.GetUserPosts(authorBytes, before, pageSize+1)
		if err != nil {
			slog.Error("failed to get user posts", "error", err)
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Failed to load posts.</p></div>`)
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

		if len(posts) == 0 {
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>No posts yet.</p></div>`)
			return
		}

		if err := h.partials.ExecuteTemplate(w, "feed_items.html", map[string]interface{}{"Posts": posts}); err != nil {
			slog.Error("template error rendering user feed", "error", err)
		}

		if hasMore && len(entries) > 0 {
			lastTs := entries[len(entries)-1].Post.Timestamp
			fmt.Fprintf(w, `<div class="text-center py-4">`+
				`<button hx-get="/web/feed?author=%s&before=%d" hx-target="closest div" hx-swap="outerHTML" `+
				`class="text-blue-500 hover:text-blue-400 text-sm">Load more</button></div>`, author, lastTs)
		}
		return
	}

	// Use global feed when ?all=1 is present or when the user follows nobody.
	useGlobal := r.URL.Query().Get("all") == "1"
	var entries []feed.TimelineEntry

	if !useGlobal {
		var err error
		entries, err = h.timeline.GetFeed(before, pageSize+1)
		if err != nil {
			slog.Error("failed to get feed", "error", err)
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Failed to load feed.</p></div>`)
			return
		}
		// Fall back to global feed if the user's personal feed is empty.
		if len(entries) == 0 {
			useGlobal = true
		}
	}

	if useGlobal {
		var err error
		entries, err = h.timeline.GetGlobalFeed(before, pageSize+1)
		if err != nil {
			slog.Error("failed to get global feed", "error", err)
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Failed to load feed.</p></div>`)
			return
		}
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
		slog.Error("template error rendering feed_items", "error", err)
	}

	if hasMore && len(entries) > 0 {
		lastTs := entries[len(entries)-1].Post.Timestamp
		allParam := ""
		if useGlobal {
			allParam = "&all=1"
		}
		fmt.Fprintf(w, `<div class="text-center py-4">`+
			`<button hx-get="/web/feed?before=%d%s" hx-target="closest div" hx-swap="outerHTML" `+
			`class="text-blue-500 hover:text-blue-400 text-sm">Load more</button></div>`, lastTs, allParam)
	}
}

// handlePost creates a new post from form data using the callback.
func (h *Handler) handlePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	content := strings.TrimSpace(r.FormValue("content"))
	mediaCIDs := r.Form["media_cids"]
	replyTo := strings.TrimSpace(r.FormValue("reply_to"))
	if content == "" && len(mediaCIDs) == 0 {
		http.Error(w, "Post content or media is required", http.StatusBadRequest)
		return
	}
	if h.currentUser(r) == nil {
		http.Error(w, "Identity not unlocked", http.StatusUnauthorized)
		return
	}
	if h.createPost == nil {
		http.Error(w, "Post creation not configured", http.StatusInternalServerError)
		return
	}

	postID, err := h.createPost(r.Context(), content, mediaCIDs, replyTo)
	if err != nil {
		slog.Error("post creation failed", "error", err)
		http.Error(w, "Failed to create post: "+err.Error(), http.StatusInternalServerError)
		return
	}

	post := h.buildNewPostView(r, postID, content)
	if postCID, err := hex.DecodeString(postID); err == nil {
		if stored, dbErr := h.db.GetPost(postCID); dbErr == nil && stored != nil {
			post = h.postRowToView(stored)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.partials.ExecuteTemplate(w, "feed_items.html", struct{ Posts []PostView }{Posts: []PostView{post}}); err != nil {
		slog.Error("failed to render feed item", "error", err)
	}
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
		var ownerPubkey []byte
		if kp := h.identity.Get(); kp != nil {
			ownerPubkey = kp.PublicKeyBytes()
		}
		if count, err := h.db.CountSubscriptions(ownerPubkey); err == nil {
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

	results := h.performSearch(q, 20)

	if len(results.Posts) == 0 && len(results.Users) == 0 {
		fmt.Fprintf(w, `<div class="text-center py-12 text-gray-400">`+
			`<p class="text-lg mb-2">No results for "%s"</p>`+
			`<p class="text-sm">Try a different search term.</p></div>`,
			template.HTMLEscapeString(q))
		return
	}

	if len(results.Users) > 0 {
		fmt.Fprint(w, `<section class="border-b border-gray-800"><div class="px-4 py-3 text-xs uppercase tracking-[0.2em] text-gray-500">Users</div>`)
		for _, user := range results.Users {
			fmt.Fprintf(w, `<a href="/user/%s" class="block border-t border-gray-800 px-4 py-3 hover:bg-gray-900/50 transition-colors">`+
				`<div class="flex gap-3"><div class="w-10 h-10 rounded-full bg-gray-700 flex items-center justify-center text-sm font-bold">%s</div>`+
				`<div class="min-w-0 flex-1"><div class="flex items-center gap-2"><span class="font-semibold">%s</span><span class="text-xs font-mono text-gray-500">%s</span></div>`,
				template.HTMLEscapeString(user.Pubkey),
				template.HTMLEscapeString(user.Initial),
				template.HTMLEscapeString(user.DisplayName),
				template.HTMLEscapeString(user.ShortPubkey),
			)
			if user.Bio != "" {
				fmt.Fprintf(w, `<p class="mt-1 text-sm text-gray-400">%s</p>`, template.HTMLEscapeString(user.Bio))
			}
			if user.Website != "" {
				fmt.Fprintf(w, `<p class="mt-1 text-xs text-blue-400">%s</p>`, template.HTMLEscapeString(user.Website))
			}
			fmt.Fprint(w, `</div></div></a>`)
		}
		fmt.Fprint(w, `</section>`)
	}

	if len(results.Posts) > 0 {
		fmt.Fprint(w, `<section><div class="px-4 py-3 text-xs uppercase tracking-[0.2em] text-gray-500">Posts</div>`)
		if err := h.partials.ExecuteTemplate(w, "feed_items.html", map[string]interface{}{"Posts": results.Posts}); err != nil {
			slog.Error("template error rendering search results", "error", err)
		}
		fmt.Fprint(w, `</section>`)
	}
}

// trendingTagsPartial returns trending hashtags as an htmx partial.
func (h *Handler) trendingTagsPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	since := time.Now().Add(-24 * time.Hour).UnixMilli()
	tags, err := h.db.GetTrendingTagsSince(since, 10)
	if err != nil {
		slog.Error("failed to get trending tags", "error", err)
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

// trendingPostsPartial returns trending posts as an htmx partial.
func (h *Handler) trendingPostsPartial(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	posts := make([]PostView, 0, 20)
	if h.indexerClient != nil && h.indexerClient.Available() {
		resp, err := h.indexerClient.GetTrending("24h", 20)
		if err == nil && resp != nil {
			for _, hit := range resp.Posts {
				if hit.CID == "" {
					continue
				}
				posts = append(posts, h.trendingHitToView(hit))
			}
		}
	}

	if len(posts) == 0 {
		since := time.Now().Add(-24 * time.Hour).UnixMilli()
		localPosts, err := h.db.GetTrendingPosts(since, 20)
		if err != nil {
			slog.Error("failed to get trending posts", "error", err)
			fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>Could not load trending posts.</p></div>`)
			return
		}
		for i := range localPosts {
			posts = append(posts, h.postRowToView(&localPosts[i]))
		}
	}

	if len(posts) == 0 {
		fmt.Fprint(w, `<div class="text-center py-12 text-gray-400"><p>No trending posts yet.</p></div>`)
		return
	}

	if err := h.partials.ExecuteTemplate(w, "feed_items.html", map[string]interface{}{"Posts": posts}); err != nil {
		slog.Error("template error rendering trending posts", "error", err)
	}
}

// handleLike creates a like reaction and returns the updated button HTML.
func (h *Handler) handleLike(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
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

	kp := h.getKeyPair(r)
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}
	if h.createReaction == nil {
		http.Error(w, "reactions not configured", http.StatusInternalServerError)
		return
	}
	if err := h.createReaction(r.Context(), kp, targetBytes); err != nil {
		slog.Error("failed to create like reaction", "error", err)
		http.Error(w, "failed to create reaction", http.StatusInternalServerError)
		return
	}

	likes, _, _, _ := h.db.GetFullReactionCounts(targetBytes)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span class="text-red-400">`+
		`<svg class="w-4 h-4 inline" fill="currentColor" stroke="currentColor" viewBox="0 0 24 24">`+
		`<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" `+
		`d="M4.318 6.318a4.5 4.5 0 000 6.364L12 20.364l7.682-7.682a4.5 4.5 0 00-6.364-6.364L12 7.636l-1.318-1.318a4.5 4.5 0 00-6.364 0z"/>`+
		`</svg> %d</span>`, likes)
}

// handleRepost creates a repost (a new post with repost_of set) and returns updated button HTML.
// Per XLeaks protocol, reposts are immutable -- once reposted, it cannot be undone.
func (h *Handler) handleRepost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	target := r.FormValue("target")
	if target == "" {
		http.Error(w, "missing target", http.StatusBadRequest)
		return
	}

	targetBytes, err := hex.DecodeString(target)
	if err != nil {
		http.Error(w, "Invalid target", http.StatusBadRequest)
		return
	}

	kp := h.getKeyPair(r)
	if kp == nil || h.createPost == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}

	// Check if already reposted (immutable -- can't undo)
	if h.db.HasReacted(kp.PublicKeyBytes(), targetBytes, "repost") {
		// Already reposted -- just return the current state
		_, _, reposts, _ := h.db.GetFullReactionCounts(targetBytes)
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<span class="text-green-400 flex items-center gap-1">`+
			`<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/></svg>`+
			` %d</span>`, reposts)
		return
	}

	// Create repost via the social service (creates actual post with repost_of)
	if h.repostPost != nil {
		if _, err := h.repostPost(r.Context(), target); err != nil {
			slog.Error("repost failed", "error", err)
		}
	}

	// Also track as a "repost" reaction for HasReacted checks
	pubkey := kp.PublicKeyBytes()
	repostData := make([]byte, 0, 7+len(pubkey)+len(targetBytes))
	repostData = append(repostData, []byte("repost:")...)
	repostData = append(repostData, pubkey...)
	repostData = append(repostData, targetBytes...)
	cid, _ := content.ComputeCID(repostData)
	if err := h.db.InsertReaction(cid, kp.PublicKeyBytes(), targetBytes, "repost", time.Now().UnixMilli()); err != nil {
		slog.Warn("failed to insert repost reaction", "error", err)
	}

	_, _, reposts, _ := h.db.GetFullReactionCounts(targetBytes)

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<span class="text-green-400 flex items-center gap-1">`+
		`<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 16V4m0 0L3 8m4-4l4 4m6 0v12m0 0l4-4m-4 4l-4-4"/></svg>`+
		` %d</span>`, reposts)
}

// handleFollow subscribes to a user and redirects back to their profile.
func (h *Handler) handleFollow(w http.ResponseWriter, r *http.Request) {
	pubkeyHex := chi.URLParam(r, "pubkey")
	pubkeyBytes, err := hex.DecodeString(pubkeyHex)
	if err != nil {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}

	kp := h.getKeyPair(r)
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}
	if h.followUser == nil {
		http.Error(w, "follow actions not configured", http.StatusInternalServerError)
		return
	}
	if err := h.followUser(r.Context(), kp, pubkeyBytes); err != nil {
		slog.Error("failed to follow user", "error", err)
		http.Error(w, "failed to follow user", http.StatusInternalServerError)
		return
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

	kp := h.getKeyPair(r)
	if kp == nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}
	if h.unfollowUser == nil {
		http.Error(w, "follow actions not configured", http.StatusInternalServerError)
		return
	}
	if err := h.unfollowUser(r.Context(), kp, pubkeyBytes); err != nil {
		slog.Error("failed to unfollow user", "error", err)
		http.Error(w, "failed to unfollow user", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/user/"+pubkeyHex, http.StatusSeeOther)
}
