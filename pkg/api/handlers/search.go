package handlers

import (
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/xleaks-org/xleaks/pkg/indexer"
)

// Search handles GET /api/search?q=QUERY&type=posts|users.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respondError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}

	searchType := r.URL.Query().Get("type")
	if searchType == "" {
		searchType = "posts"
	}

	page := 0
	if p := r.URL.Query().Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n >= 0 {
			page = n
		}
	}

	pageSize := 20
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if n, err := strconv.Atoi(ps); err == nil && n > 0 {
			pageSize = n
			if pageSize > 100 {
				pageSize = 100
			}
		}
	}

	// If no indexer client is configured or no indexers are available,
	// return an empty result with a message.
	if h.indexerClient == nil || !h.indexerClient.Available() {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    searchType,
			"query":   query,
			"results": []interface{}{},
			"total":   0,
			"message": "No indexer nodes available",
		})
		return
	}

	switch searchType {
	case "posts":
		resp, err := h.indexerClient.SearchPosts(query, page, pageSize)
		if err != nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"type":    "posts",
				"query":   query,
				"results": []interface{}{},
				"total":   0,
				"message": "No indexer nodes available",
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "posts",
			"query":   query,
			"results": resp.Results,
			"total":   resp.Total,
		})

	case "users":
		resp, err := h.indexerClient.SearchUsers(query, page, pageSize)
		if err != nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"type":    "users",
				"query":   query,
				"results": []interface{}{},
				"total":   0,
				"message": "No indexer nodes available",
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "users",
			"query":   query,
			"results": resp.Results,
			"total":   resp.Total,
		})

	default:
		respondError(w, http.StatusBadRequest, "type must be 'posts' or 'users'")
	}
}

// GetTrending handles GET /api/trending.
func (h *Handler) GetTrending(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "24h"
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if h.indexerClient != nil && h.indexerClient.Available() {
		resp, err := h.indexerClient.GetTrending(window, limit)
		if err == nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"hashtags": resp.Tags,
				"posts":    resp.Posts,
				"window":   resp.Window,
			})
			return
		}
	}

	posts, tags := h.localTrending(window, limit)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"hashtags": tags,
		"posts":    posts,
		"window":   window,
		"message":  "Using local-only trending data",
	})
}

// Explore handles GET /api/explore.
func (h *Handler) Explore(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if h.indexerClient != nil && h.indexerClient.Available() {
		publishers, err := h.indexerClient.GetExplorePublishers(limit)
		if err == nil {
			respondJSON(w, http.StatusOK, map[string]interface{}{
				"publishers": publishers,
			})
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"publishers": h.localExplorePublishers(limit),
		"message":    "Using local-only publisher data",
	})
}

func (h *Handler) localTrending(window string, limit int) ([]indexer.ClientTrendingPost, []indexer.ClientTrendingTag) {
	var since int64
	switch window {
	case "1h":
		since = time.Now().Add(-1 * time.Hour).UnixMilli()
	case "6h":
		since = time.Now().Add(-6 * time.Hour).UnixMilli()
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour).UnixMilli()
	default:
		since = time.Now().Add(-24 * time.Hour).UnixMilli()
	}

	postRows, _ := h.db.GetTrendingPosts(since, limit)
	posts := make([]indexer.ClientTrendingPost, 0, len(postRows))
	for _, post := range postRows {
		likes, replies, reposts, _ := h.db.GetFullReactionCounts(post.CID)
		posts = append(posts, indexer.ClientTrendingPost{
			CID:         hex.EncodeToString(post.CID),
			Author:      hex.EncodeToString(post.Author),
			Content:     post.Content,
			LikeCount:   likes,
			ReplyCount:  replies,
			RepostCount: reposts,
			Score:       float64(likes + (replies * 2) + (reposts * 3)),
			Timestamp:   post.Timestamp,
		})
	}

	tagRows, _ := h.db.GetTrendingTagsSince(since, limit)
	tags := make([]indexer.ClientTrendingTag, 0, len(tagRows))
	for _, tag := range tagRows {
		tags = append(tags, indexer.ClientTrendingTag{
			Tag:   tag.Tag,
			Count: tag.Count,
		})
	}

	return posts, tags
}

func (h *Handler) localExplorePublishers(limit int) []indexer.ClientPublisher {
	profiles, err := h.db.GetAllProfiles()
	if err != nil {
		return nil
	}

	publishers := make([]indexer.ClientPublisher, 0, len(profiles))
	for _, profile := range profiles {
		followers, _ := h.db.GetFollowers(profile.Pubkey)
		following, _ := h.db.GetFollowing(profile.Pubkey)
		publishers = append(publishers, indexer.ClientPublisher{
			Pubkey:         hex.EncodeToString(profile.Pubkey),
			FollowerCount:  len(followers),
			FollowingCount: len(following),
			DisplayName:    profile.DisplayName,
			Bio:            profile.Bio,
		})
	}
	sort.Slice(publishers, func(i, j int) bool {
		if publishers[i].FollowerCount == publishers[j].FollowerCount {
			return publishers[i].DisplayName < publishers[j].DisplayName
		}
		return publishers[i].FollowerCount > publishers[j].FollowerCount
	})
	if len(publishers) > limit {
		publishers = publishers[:limit]
	}
	return publishers
}
