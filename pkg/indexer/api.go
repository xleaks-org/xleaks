package indexer

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// IndexerAPI exposes search, trending, and stats endpoints for regular nodes.
type IndexerAPI struct {
	search   *SearchIndex
	trending *TrendingEngine
	stats    *StatsCollector
}

// NewIndexerAPI creates a new IndexerAPI.
func NewIndexerAPI(search *SearchIndex, trending *TrendingEngine, stats *StatsCollector) *IndexerAPI {
	return &IndexerAPI{
		search:   search,
		trending: trending,
		stats:    stats,
	}
}

// Handler returns a chi router with /api/search, /api/trending,
// /api/explore/publishers, and /api/stats endpoints.
func (api *IndexerAPI) Handler() http.Handler {
	r := chi.NewRouter()

	r.Route("/api", func(r chi.Router) {
		r.Get("/search", api.handleSearch)
		r.Get("/trending", api.handleTrending)
		r.Get("/explore/publishers", api.handleExplorePublishers)
		r.Get("/stats", api.handleStats)
	})

	return r
}

func (api *IndexerAPI) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing query parameter 'q'"})
		return
	}

	searchType := r.URL.Query().Get("type")
	page := parsePageParam(r.URL.Query().Get("page"))
	pageSize := parsePageSizeParam(r.URL.Query().Get("page_size"))

	switch searchType {
	case "users":
		results, total, err := api.search.SearchUsers(query, page, pageSize)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"total":   total,
			"page":    page,
		})

	default: // "posts" or empty defaults to post search
		results, total, err := api.search.SearchPosts(query, page, pageSize)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"results": results,
			"total":   total,
			"page":    page,
		})
	}
}

func (api *IndexerAPI) handleTrending(w http.ResponseWriter, r *http.Request) {
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "24h"
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	trendingType := r.URL.Query().Get("type")

	switch trendingType {
	case "tags":
		tags, err := api.trending.GetTrendingTags(window, limit)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"tags":   tags,
			"window": window,
		})

	default: // "posts" or empty defaults to trending posts
		posts, err := api.trending.GetTrendingPosts(window, limit)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"posts":  posts,
			"window": window,
		})
	}
}

func (api *IndexerAPI) handleExplorePublishers(w http.ResponseWriter, r *http.Request) {
	// Return top publishers by follower count.
	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	// Query the follower_counts table for top publishers.
	rows, err := api.stats.db.Query(
		`SELECT fc.pubkey, fc.follower_count, fc.following_count,
		        COALESCE(p.display_name, '') AS display_name,
		        COALESCE(p.bio, '') AS bio
		 FROM follower_counts fc
		 LEFT JOIN profiles p ON fc.pubkey = p.pubkey
		 ORDER BY fc.follower_count DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	type publisher struct {
		Pubkey         string `json:"pubkey"`
		FollowerCount  int    `json:"follower_count"`
		FollowingCount int    `json:"following_count"`
		DisplayName    string `json:"display_name"`
		Bio            string `json:"bio"`
	}

	var publishers []publisher
	for rows.Next() {
		var p publisher
		var pubkeyBytes []byte
		if err := rows.Scan(&pubkeyBytes, &p.FollowerCount, &p.FollowingCount, &p.DisplayName, &p.Bio); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		p.Pubkey = fmt.Sprintf("%x", pubkeyBytes)
		publishers = append(publishers, p)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"publishers": publishers,
	})
}

func (api *IndexerAPI) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := api.stats.GetStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func parsePageSizeParam(s string) int {
	if s == "" {
		return 20
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 20
	}
	if n > 100 {
		return 100
	}
	return n
}
