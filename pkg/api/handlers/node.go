package handlers

import (
	"encoding/json"
	"net/http"
	"time"
)

// nodeStartTime records when the node started for uptime calculation.
var nodeStartTime = time.Now()

// GetNodeStatus handles GET /api/node/status.
func (h *Handler) GetNodeStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(nodeStartTime).Seconds()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"peers":     0,            // Stub - would query P2P host
		"bandwidth": map[string]interface{}{
			"total_in":  0,
			"total_out": 0,
			"rate_in":   0,
			"rate_out":  0,
		},
		"storage": map[string]interface{}{
			"used":  0,
			"limit": 0,
		},
		"uptime": uptime,
	})
}

// GetNodePeers handles GET /api/node/peers.
func (h *Handler) GetNodePeers(w http.ResponseWriter, r *http.Request) {
	// Stub - would query the P2P host for connected peers.
	respondJSON(w, http.StatusOK, []interface{}{})
}

// GetNodeConfig handles GET /api/node/config.
func (h *Handler) GetNodeConfig(w http.ResponseWriter, r *http.Request) {
	// Stub - would return actual node configuration.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"listen_addresses": []string{},
		"max_connections":  50,
		"storage_limit":   0,
	})
}

// UpdateNodeConfig handles PUT /api/node/config.
func (h *Handler) UpdateNodeConfig(w http.ResponseWriter, r *http.Request) {
	var config map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Stub - would apply configuration changes.
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
	})
}
