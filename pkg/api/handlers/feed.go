package handlers

import (
	"net/http"
)

// GetFeed handles GET /api/feed?before=TIMESTAMP.
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	before, limit, err := parsePagination(r, 20)
	if err != nil {
		respondBadRequestError(w, err)
		return
	}

	entries, err := h.timeline.GetFeed(before, limit)
	if err != nil {
		respondInternalError(w, "failed to load feed", err, "failed to load feed")
		return
	}

	respondJSON(w, http.StatusOK, timelineEntriesToJSON(h.db, entries))
}
