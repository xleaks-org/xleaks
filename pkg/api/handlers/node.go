package handlers

import (
	"encoding/hex"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/xleaks-org/xleaks/pkg/config"
	"github.com/xleaks-org/xleaks/pkg/content"
	xlog "github.com/xleaks-org/xleaks/pkg/logging"
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

const (
	minThumbnailQuality = 10
	maxThumbnailQuality = 100
)

var allowedNodeConfigUpdateKeys = map[string]struct{}{
	"max_connections":      {},
	"storage_limit_gb":     {},
	"enable_relay":         {},
	"enable_mdns":          {},
	"enable_hole_punching": {},
	"bandwidth_limit_mbps": {},
	"bootstrap_peers":      {},
	"known_indexers":       {},
	"enable_websocket":     {},
	"enable_web_ui":        {},
	"allow_remote_web_ui":  {},
	"auto_fetch_media":     {},
	"max_upload_size_mb":   {},
	"thumbnail_quality":    {},
	"log_level":            {},
}

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
			"enable_web_ui":           true,
			"allow_remote_web_ui":     false,
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
		"enable_web_ui":           h.cfg.API.EnableWebUI,
		"allow_remote_web_ui":     h.cfg.API.AllowRemoteWebUI,
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
	if err := parseJSON(w, r, &updates); err != nil {
		respondBadRequestError(w, err)
		return
	}
	if err := validateAllowedUpdateKeys(updates, allowedNodeConfigUpdateKeys); err != nil {
		respondBadRequestError(w, err)
		return
	}

	next := cloneConfig(h.cfg)
	refreshIndexers := false

	if n, ok, err := optionalIntField(updates, "max_connections"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		if n < 1 {
			respondError(w, http.StatusBadRequest, "max_connections must be at least 1")
			return
		}
		next.Network.MaxPeers = n
	}
	if n, ok, err := optionalIntField(updates, "storage_limit_gb"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Node.MaxStorageGB = n
	}
	if b, ok, err := optionalBoolField(updates, "enable_relay"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Network.EnableRelay = b
	}
	if b, ok, err := optionalBoolField(updates, "enable_mdns"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Network.EnableMDNS = b
	}
	if b, ok, err := optionalBoolField(updates, "enable_hole_punching"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Network.EnableHolePunching = b
	}
	if n, ok, err := optionalIntField(updates, "bandwidth_limit_mbps"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		if n < 0 {
			respondError(w, http.StatusBadRequest, "bandwidth_limit_mbps must be 0 or greater")
			return
		}
		next.Network.BandwidthLimitMbps = n
	}
	if peers, ok, err := optionalStringSliceField(updates, "bootstrap_peers"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		peers = normalizeStringSlice(peers)
		if err := validateBootstrapPeers(peers); err != nil {
			respondBadRequestError(w, err)
			return
		}
		next.Network.BootstrapPeers = peers
	}
	if indexers, ok, err := optionalStringSliceField(updates, "known_indexers"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		indexers, err = normalizeIndexerURLs(indexers)
		if err != nil {
			respondBadRequestError(w, err)
			return
		}
		next.Indexer.KnownIndexers = indexers
		refreshIndexers = true
	}
	if b, ok, err := optionalBoolField(updates, "enable_websocket"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.API.EnableWebSocket = b
	}
	if b, ok, err := optionalBoolField(updates, "enable_web_ui"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.API.EnableWebUI = b
	}
	if b, ok, err := optionalBoolField(updates, "allow_remote_web_ui"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.API.AllowRemoteWebUI = b
	}
	if b, ok, err := optionalBoolField(updates, "auto_fetch_media"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Media.AutoFetchMedia = b
	}
	if n, ok, err := optionalIntField(updates, "max_upload_size_mb"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		if n < 1 {
			respondError(w, http.StatusBadRequest, "max_upload_size_mb must be at least 1")
			return
		}
		next.Media.MaxUploadSizeMB = n
	}
	if n, ok, err := optionalIntField(updates, "thumbnail_quality"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		if n < minThumbnailQuality || n > maxThumbnailQuality {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("thumbnail_quality must be between %d and %d", minThumbnailQuality, maxThumbnailQuality))
			return
		}
		next.Media.ThumbnailQuality = n
	}
	if level, ok, err := optionalLogLevelField(updates, "log_level"); err != nil {
		respondBadRequestError(w, err)
		return
	} else if ok {
		next.Logging.Level = level
	}
	if err := validateWebUIConfig(next.API.ListenAddress, h.apiTokenConfigured, next.API.EnableWebUI, next.API.AllowRemoteWebUI); err != nil {
		respondBadRequestError(w, err)
		return
	}
	if err := next.ValidateStorageLimit(); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Persist to disk if we have a config path.
	if h.cfgPath != "" {
		if err := next.Save(h.cfgPath); err != nil {
			respondInternalError(w, "failed to save node config", err, "failed to save config", "path", xlog.RedactPath(h.cfgPath))
			return
		}
	}
	*h.cfg = *next
	if refreshIndexers && h.indexerClient != nil {
		h.indexerClient.SetIndexers(next.Indexer.KnownIndexers)
	}
	if h.onStorageLimitChange != nil {
		h.onStorageLimitChange(next.MaxStorageBytes())
	}
	slog.Info("node config updated",
		"updated_fields", sortedUpdateKeys(updates),
		"saved_to_disk", h.cfgPath != "",
		"refreshed_indexers", refreshIndexers,
	)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
	})
}

func validateWebUIConfig(listenAddr string, apiTokenConfigured, enableWebUI, allowRemoteWebUI bool) error {
	if !enableWebUI || isLoopbackAPIAddress(listenAddr) {
		return nil
	}
	if !apiTokenConfigured {
		return newRequestError("enable_web_ui on a non-loopback api_address requires API token auth")
	}
	if !allowRemoteWebUI {
		return newRequestError("enable_web_ui on a non-loopback api_address requires allow_remote_web_ui to be true")
	}
	return nil
}

func isLoopbackAPIAddress(listenAddr string) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(listenAddr))
	if err != nil {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func cloneConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.Network.ListenAddresses = append([]string(nil), cfg.Network.ListenAddresses...)
	cloned.Network.BootstrapPeers = append([]string(nil), cfg.Network.BootstrapPeers...)
	cloned.Network.RelayAddresses = append([]string(nil), cfg.Network.RelayAddresses...)
	cloned.Indexer.TrendingWindows = append([]string(nil), cfg.Indexer.TrendingWindows...)
	cloned.Indexer.KnownIndexers = append([]string(nil), cfg.Indexer.KnownIndexers...)
	return &cloned
}

func optionalIntField(updates map[string]interface{}, key string) (int, bool, error) {
	raw, ok := updates[key]
	if !ok {
		return 0, false, nil
	}
	n, ok := raw.(float64)
	if !ok || math.Trunc(n) != n {
		return 0, false, newRequestError("%s must be an integer", key)
	}
	return int(n), true, nil
}

func optionalBoolField(updates map[string]interface{}, key string) (bool, bool, error) {
	raw, ok := updates[key]
	if !ok {
		return false, false, nil
	}
	b, ok := raw.(bool)
	if !ok {
		return false, false, newRequestError("%s must be a boolean", key)
	}
	return b, true, nil
}

func optionalStringSliceField(updates map[string]interface{}, key string) ([]string, bool, error) {
	raw, ok := updates[key]
	if !ok {
		return nil, false, nil
	}
	items, ok := toStringSlice(raw)
	if !ok {
		return nil, false, newRequestError("%s must be an array of strings", key)
	}
	return items, true, nil
}

func optionalLogLevelField(updates map[string]interface{}, key string) (string, bool, error) {
	raw, ok := updates[key]
	if !ok {
		return "", false, nil
	}
	level, ok := raw.(string)
	if !ok {
		return "", false, newRequestError("%s must be a string", key)
	}
	level = strings.ToLower(strings.TrimSpace(level))
	switch level {
	case "debug", "info", "warn", "warning", "error":
		if level == "warning" {
			level = "warn"
		}
		return level, true, nil
	default:
		return "", false, newRequestError("%s must be one of debug, info, warn, warning, or error", key)
	}
}

func normalizeStringSlice(items []string) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func validateBootstrapPeers(peers []string) error {
	for _, peer := range peers {
		if _, err := ma.NewMultiaddr(peer); err != nil {
			return newRequestError("bootstrap_peers contains an invalid multiaddr")
		}
	}
	return nil
}

func normalizeIndexerURLs(urls []string) ([]string, error) {
	normalized := make([]string, 0, len(urls))
	seen := make(map[string]struct{}, len(urls))
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		parsed, err := url.ParseRequestURI(rawURL)
		if err != nil || parsed.Host == "" {
			return nil, newRequestError("known_indexers contains an invalid URL")
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return nil, newRequestError("known_indexers must use http or https URLs")
		}
		if parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, newRequestError("known_indexers must be base URLs without query or fragment")
		}
		canonical := strings.TrimRight(parsed.String(), "/")
		if _, ok := seen[canonical]; ok {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}
	return normalized, nil
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
		respondInternalError(w, "database backup failed", err, "backup failed", "dir", xlog.RedactPath(backupDir))
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		respondInternalError(w, "database backup stat failed", err, "failed to inspect backup", "path", xlog.RedactPath(path))
		return
	}
	slog.Info("database backup created", "path", xlog.RedactPath(path), "size", info.Size())

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

func sortedUpdateKeys(updates map[string]interface{}) []string {
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validateAllowedUpdateKeys(updates map[string]interface{}, allowed map[string]struct{}) error {
	var unknown []string
	for key := range updates {
		if _, ok := allowed[key]; !ok {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) == 0 {
		return nil
	}
	sort.Strings(unknown)
	return newRequestError("unknown config fields: %s", strings.Join(unknown, ", "))
}
