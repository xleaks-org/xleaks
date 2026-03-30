package handlers

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/version"
)

// nodeStartTime records when the node started for uptime calculation.
var nodeStartTime = time.Now()

// Cached storage size to avoid walking the data directory on every status request.
var (
	cachedStorageSize int64
	storageSizeTime   time.Time
	storageSizeMu     sync.Mutex
)

// GetNodeStatus handles GET /api/node/status.
func (h *Handler) GetNodeStatus(w http.ResponseWriter, r *http.Request) {
	uptime := time.Since(nodeStartTime).Seconds()

	// Peer count from P2P host.
	peerCount := 0
	if h.p2pHost != nil {
		peerCount = h.p2pHost.PeerCount()
	}

	// Bandwidth stats from P2P host.
	bw := map[string]interface{}{
		"total_in":  int64(0),
		"total_out": int64(0),
		"rate_in":   int64(0),
		"rate_out":  int64(0),
	}
	if h.p2pHost != nil {
		stats := h.p2pHost.BandwidthStats()
		bw["total_in"] = stats.TotalIn
		bw["total_out"] = stats.TotalOut
		bw["rate_in"] = stats.RateIn
		bw["rate_out"] = stats.RateOut
	}

	// Storage: count content_access rows and compute data dir size (cached for 60s).
	storageUsed := int64(0)
	storageLimit := int64(0)
	if h.cfg != nil {
		storageLimit = h.cfg.MaxStorageBytes()
		storageSizeMu.Lock()
		if time.Since(storageSizeTime) > 60*time.Second {
			dataDir := h.cfg.DataDir()
			if s, err := content.DirSize(filepath.Join(dataDir, "data")); err == nil {
				cachedStorageSize = s
				storageSizeTime = time.Now()
			}
		}
		storageUsed = cachedStorageSize
		storageSizeMu.Unlock()
	}

	// Subscription count.
	subscriptionCount := 0
	if h.db != nil {
		var ownerPubkey []byte
		if kp := h.currentKeyPair(); kp != nil {
			ownerPubkey = kp.PublicKeyBytes()
		}
		count, err := h.db.CountSubscriptions(ownerPubkey)
		if err == nil {
			subscriptionCount = count
		}
	}

	// Identity address (public key hex).
	identityAddr := ""
	if kp := h.currentKeyPair(); kp != nil {
		identityAddr = hex.EncodeToString(kp.PublicKeyBytes())
	}

	// Node ID from P2P host.
	nodeID := ""
	if h.p2pHost != nil {
		nodeID = h.p2pHost.ID().String()
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"peers":     peerCount,
		"bandwidth": bw,
		"storage": map[string]interface{}{
			"used":  storageUsed,
			"limit": storageLimit,
		},
		"uptime":        uptime,
		"subscriptions": subscriptionCount,
		"identity":      identityAddr,
		"node_id":       nodeID,
		"version":       version.Version,
	})
}

// GetNodePeers handles GET /api/node/peers.
func (h *Handler) GetNodePeers(w http.ResponseWriter, r *http.Request) {
	if h.p2pHost == nil {
		respondJSON(w, http.StatusOK, []interface{}{})
		return
	}

	libHost := h.p2pHost.LibP2PHost()
	network := libHost.Network()
	peerIDs := network.Peers()

	type peerInfo struct {
		ID        string   `json:"id"`
		Addresses []string `json:"addresses"`
		Direction string   `json:"direction"`
		Latency   string   `json:"latency,omitempty"`
	}

	peers := make([]peerInfo, 0, len(peerIDs))
	for _, pid := range peerIDs {
		conns := network.ConnsToPeer(pid)
		if len(conns) == 0 {
			continue
		}

		addrs := make([]string, 0, len(conns))
		direction := "unknown"
		for _, conn := range conns {
			addrs = append(addrs, conn.RemoteMultiaddr().String())
			switch conn.Stat().Direction {
			case 1: // network.DirInbound
				direction = "inbound"
			case 2: // network.DirOutbound
				direction = "outbound"
			}
		}

		latency := libHost.Peerstore().LatencyEWMA(pid)
		latencyStr := ""
		if latency > 0 {
			latencyStr = latency.String()
		}

		peers = append(peers, peerInfo{
			ID:        pid.String(),
			Addresses: addrs,
			Direction: direction,
			Latency:   latencyStr,
		})
	}

	respondJSON(w, http.StatusOK, peers)
}

// GetNodeConfig handles GET /api/node/config.
func (h *Handler) GetNodeConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfg == nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"listen_addresses":        []string{},
			"bootstrap_peers":         []string{},
			"default_bootstrap_peers": config.DefaultBootstrapPeers(),
			"known_indexers":          config.DefaultKnownIndexers(),
			"default_known_indexers":  config.DefaultKnownIndexers(),
			"max_connections":         50,
			"storage_limit_gb":        0,
			"bandwidth_limit_mbps":    0,
			"enable_relay":            true,
			"enable_mdns":             true,
			"enable_hole_punching":    true,
			"enable_websocket":        true,
			"auto_fetch_media":        false,
			"max_upload_size_mb":      config.DefaultConfig().Media.MaxUploadSizeMB,
			"thumbnail_quality":       config.DefaultConfig().Media.ThumbnailQuality,
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"listen_addresses":        h.cfg.Network.ListenAddresses,
		"bootstrap_peers":         h.cfg.Network.BootstrapPeers,
		"default_bootstrap_peers": config.DefaultBootstrapPeers(),
		"known_indexers":          h.cfg.Indexer.KnownIndexers,
		"default_known_indexers":  config.DefaultKnownIndexers(),
		"max_connections":         h.cfg.Network.MaxPeers,
		"enable_relay":            h.cfg.Network.EnableRelay,
		"enable_mdns":             h.cfg.Network.EnableMDNS,
		"enable_hole_punching":    h.cfg.Network.EnableHolePunching,
		"bandwidth_limit_mbps":    h.cfg.Network.BandwidthLimitMbps,
		"storage_limit_gb":        h.cfg.Node.MaxStorageGB,
		"enable_websocket":        h.cfg.API.EnableWebSocket,
		"auto_fetch_media":        h.cfg.Media.AutoFetchMedia,
		"max_upload_size_mb":      h.cfg.Media.MaxUploadSizeMB,
		"thumbnail_quality":       h.cfg.Media.ThumbnailQuality,
		"data_dir":                h.cfg.Node.DataDir,
		"mode":                    h.cfg.Node.Mode,
		"api_address":             h.cfg.API.ListenAddress,
		"log_level":               h.cfg.Logging.Level,
	})
}

// UpdateNodeConfig handles PUT /api/node/config.
func (h *Handler) UpdateNodeConfig(w http.ResponseWriter, r *http.Request) {
	if h.cfg == nil {
		respondError(w, http.StatusInternalServerError, "configuration not available")
		return
	}

	var updates map[string]interface{}
	if err := parseJSON(r, &updates); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Apply supported config updates.
	if v, ok := updates["max_connections"]; ok {
		if n, ok := v.(float64); ok {
			h.cfg.Network.MaxPeers = int(n)
		}
	}
	if v, ok := updates["storage_limit_gb"]; ok {
		if n, ok := v.(float64); ok {
			h.cfg.Node.MaxStorageGB = int(n)
		}
	}
	if v, ok := updates["enable_relay"]; ok {
		if b, ok := v.(bool); ok {
			h.cfg.Network.EnableRelay = b
		}
	}
	if v, ok := updates["enable_mdns"]; ok {
		if b, ok := v.(bool); ok {
			h.cfg.Network.EnableMDNS = b
		}
	}
	if v, ok := updates["enable_hole_punching"]; ok {
		if b, ok := v.(bool); ok {
			h.cfg.Network.EnableHolePunching = b
		}
	}
	if v, ok := updates["bandwidth_limit_mbps"]; ok {
		if n, ok := v.(float64); ok {
			h.cfg.Network.BandwidthLimitMbps = int(n)
		}
	}
	if v, ok := updates["bootstrap_peers"]; ok {
		if peers, ok := toStringSlice(v); ok {
			h.cfg.Network.BootstrapPeers = peers
		}
	}
	if v, ok := updates["known_indexers"]; ok {
		if indexers, ok := toStringSlice(v); ok {
			h.cfg.Indexer.KnownIndexers = indexers
		}
	}
	if v, ok := updates["enable_websocket"]; ok {
		if b, ok := v.(bool); ok {
			h.cfg.API.EnableWebSocket = b
		}
	}
	if v, ok := updates["auto_fetch_media"]; ok {
		if b, ok := v.(bool); ok {
			h.cfg.Media.AutoFetchMedia = b
		}
	}
	if v, ok := updates["max_upload_size_mb"]; ok {
		if n, ok := v.(float64); ok && int(n) > 0 {
			h.cfg.Media.MaxUploadSizeMB = int(n)
		}
	}
	if v, ok := updates["thumbnail_quality"]; ok {
		if n, ok := v.(float64); ok {
			h.cfg.Media.ThumbnailQuality = int(n)
		}
	}
	if v, ok := updates["log_level"]; ok {
		if s, ok := v.(string); ok {
			h.cfg.Logging.Level = s
		}
	}

	// Persist to disk if we have a config path.
	if h.cfgPath != "" {
		if err := h.cfg.Save(h.cfgPath); err != nil {
			respondError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save config: %v", err))
			return
		}
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
	})
}

// CreateBackup handles POST /api/node/backup.
func (h *Handler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	if h.db == nil {
		respondError(w, http.StatusInternalServerError, "database not available")
		return
	}

	backupDir := filepath.Dir(h.db.Path())
	path, err := h.db.Backup(backupDir)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "backup failed: "+err.Error())
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to stat backup: "+err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"path":      path,
		"size":      info.Size(),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func toStringSlice(v interface{}) ([]string, bool) {
	items, ok := v.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}
