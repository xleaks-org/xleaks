package handlers

import (
	"net/http"
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

	// Stub - actual implementation would query an indexer or local DB.
	switch searchType {
	case "posts":
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "posts",
			"query":   query,
			"results": []interface{}{},
		})
	case "users":
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"type":    "users",
			"query":   query,
			"results": []interface{}{},
		})
	default:
		respondError(w, http.StatusBadRequest, "type must be 'posts' or 'users'")
	}
}

// GetTrending handles GET /api/trending.
func (h *Handler) GetTrending(w http.ResponseWriter, r *http.Request) {
	// Stub - would return trending hashtags or posts based on reaction counts.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"hashtags": []interface{}{},
		"posts":    []interface{}{},
	})
}

// Explore handles GET /api/explore.
func (h *Handler) Explore(w http.ResponseWriter, r *http.Request) {
	// Stub - would return popular content from the network.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"posts": []interface{}{},
		"users": []interface{}{},
	})
}
