package handlers

import (
	"net/http"
	"strconv"
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

	if h.indexerClient == nil || !h.indexerClient.Available() {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"hashtags": []interface{}{},
			"posts":    []interface{}{},
			"message":  "No indexer nodes available",
		})
		return
	}

	resp, err := h.indexerClient.GetTrending(window, limit)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"hashtags": []interface{}{},
			"posts":    []interface{}{},
			"message":  "No indexer nodes available",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"hashtags": resp.Tags,
		"posts":    resp.Posts,
		"window":   resp.Window,
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

	if h.indexerClient == nil || !h.indexerClient.Available() {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"publishers": []interface{}{},
			"message":    "No indexer nodes available",
		})
		return
	}

	publishers, err := h.indexerClient.GetExplorePublishers(limit)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"publishers": []interface{}{},
			"message":    "No indexer nodes available",
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"publishers": publishers,
	})
}
