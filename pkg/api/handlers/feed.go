package handlers

import (
	"net/http"
)

// GetFeed handles GET /api/feed?before=TIMESTAMP.
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	before, limit := parsePagination(r, 20)

	entries, err := h.timeline.GetFeed(before, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, timelineEntriesToJSON(h.db, entries))
}
