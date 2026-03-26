package handlers

import (
	"encoding/hex"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

	switch searchType {
	case "posts":
		results, message := h.searchPostsResults(query, page, pageSize)
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "posts",
			"query":   query,
			"results": results,
			"total":   len(results),
			"message": message,
		})

	case "users":
		results, message := h.searchUsersResults(query, page, pageSize)
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "users",
			"query":   query,
			"results": results,
			"total":   len(results),
			"message": message,
		})

	default:
		respondError(w, http.StatusBadRequest, "type must be 'posts' or 'users'")
	}
}

func (h *Handler) searchPostsResults(query string, page, pageSize int) ([]indexer.ClientSearchHit, string) {
	results := make([]indexer.ClientSearchHit, 0, pageSize)
	seen := make(map[string]struct{}, pageSize)
	message := "Using local-only post search"
	indexerQuery := strings.TrimSpace(strings.TrimPrefix(query, "#"))

	if h.indexerClient != nil && h.indexerClient.Available() && indexerQuery != "" {
		resp, err := h.indexerClient.SearchPosts(indexerQuery, page, pageSize)
		if err == nil && resp != nil {
			for _, hit := range resp.Results {
				if hit.ID == "" {
					continue
				}
				results = append(results, hit)
				seen[hit.ID] = struct{}{}
				if len(results) >= pageSize {
					return results, ""
				}
			}
			message = ""
		}
	}

	if strings.HasPrefix(query, "#") {
		tag := strings.TrimSpace(strings.TrimPrefix(query, "#"))
		if posts, err := h.db.GetPostsByTag(tag, 0, pageSize); err == nil {
			for _, post := range posts {
				id := hex.EncodeToString(post.CID)
				if _, ok := seen[id]; ok {
					continue
				}
				results = append(results, indexer.ClientSearchHit{
					ID:      id,
					Type:    "post",
					Content: post.Content,
					Author:  hex.EncodeToString(post.Author),
				})
				seen[id] = struct{}{}
			}
		}
	} else if posts, err := h.db.SearchPostsByContent(query, pageSize); err == nil {
		for _, post := range posts {
			id := hex.EncodeToString(post.CID)
			if _, ok := seen[id]; ok {
				continue
			}
			results = append(results, indexer.ClientSearchHit{
				ID:      id,
				Type:    "post",
				Content: post.Content,
				Author:  hex.EncodeToString(post.Author),
			})
			seen[id] = struct{}{}
		}
	}

	return results, message
}

func (h *Handler) searchUsersResults(query string, page, pageSize int) ([]indexer.ClientSearchHit, string) {
	results := make([]indexer.ClientSearchHit, 0, pageSize)
	seen := make(map[string]struct{}, pageSize)
	normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(query, "@")))
	if normalized == "" {
		return results, "query parameter 'q' is required"
	}
	message := "Using local-only user search"

	if h.indexerClient != nil && h.indexerClient.Available() {
		resp, err := h.indexerClient.SearchUsers(normalized, page, pageSize)
		if err == nil && resp != nil {
			for _, hit := range resp.Results {
				pubkeyHex := hit.Author
				if pubkeyHex == "" {
					pubkeyHex = hit.ID
				}
				if pubkeyHex == "" {
					continue
				}
				if _, ok := seen[pubkeyHex]; ok {
					continue
				}
				hit.ID = pubkeyHex
				hit.Type = "user"
				results = append(results, hit)
				seen[pubkeyHex] = struct{}{}
				if len(results) >= pageSize {
					return results, ""
				}
			}
			message = ""
		}
	}

	if profiles, err := h.db.GetAllProfiles(); err == nil {
		for _, profile := range profiles {
			pubkeyHex := hex.EncodeToString(profile.Pubkey)
			if _, ok := seen[pubkeyHex]; ok {
				continue
			}
			if !strings.Contains(strings.ToLower(profile.DisplayName), normalized) &&
				!strings.Contains(strings.ToLower(profile.Bio), normalized) &&
				!strings.Contains(strings.ToLower(profile.Website), normalized) &&
				!strings.Contains(strings.ToLower(pubkeyHex), normalized) {
				continue
			}
			results = append(results, indexer.ClientSearchHit{
				ID:    pubkeyHex,
				Type:  "user",
				Name:  profile.DisplayName,
				Bio:   profile.Bio,
				Score: 1,
			})
			seen[pubkeyHex] = struct{}{}
			if len(results) >= pageSize {
				return results, message
			}
		}
	}

	return results, message
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
