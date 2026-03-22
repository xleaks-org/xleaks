package handlers

import (
	"net/http"
	"strconv"
)

// GetFeed handles GET /api/feed?before=TIMESTAMP.
func (h *Handler) GetFeed(w http.ResponseWriter, r *http.Request) {
	var before int64
	var err error
	if b := r.URL.Query().Get("before"); b != "" {
		before, err = strconv.ParseInt(b, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid before timestamp")
			return
		}
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, err = strconv.Atoi(l)
		if err != nil || limit <= 0 {
			limit = 20
		}
	}

	entries, err := h.timeline.GetFeed(before, limit)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, timelineEntriesToJSON(entries))
}
