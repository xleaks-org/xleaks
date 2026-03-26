package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/xleaks-org/xleaks/pkg/config"
)

var (
	defaultBootstrapManifestURLs = []string{
		"https://raw.githubusercontent.com/xleaks-org/xleaks/main/configs/bootstrap_peers.toml",
	}
	defaultBootstrapNodeAPIBases = []string{
		"https://xleaks.org",
	}
)

type bootstrapManifestResponse struct {
	Peers []struct {
		Address string `toml:"address"`
	} `toml:"peers"`
}

type bootstrapNodeConfigResponse struct {
	BootstrapPeers        []string `json:"bootstrap_peers"`
	DefaultBootstrapPeers []string `json:"default_bootstrap_peers"`
	ListenAddresses       []string `json:"listen_addresses"`
}

type bootstrapNodeStatusResponse struct {
	NodeID string `json:"node_id"`
}

type bootstrapNodePeerResponse struct {
	ID        string   `json:"id"`
	Addresses []string `json:"addresses"`
}

func discoverBootstrapFallbackPeers(ctx context.Context, cfg *config.Config) []string {
	client := &http.Client{Timeout: 5 * time.Second}

	peers := make([]string, 0, 16)
	for _, rawURL := range defaultBootstrapManifestURLs {
		found, err := fetchBootstrapPeersFromManifest(ctx, client, rawURL)
		if err == nil {
			peers = append(peers, found...)
		}
	}

	for _, baseURL := range bootstrapNodeAPIBases(cfg) {
		found, err := fetchBootstrapPeersFromNodeAPI(ctx, client, baseURL)
		if err == nil {
			peers = append(peers, found...)
		}
	}

	return dedupeStrings(peers)
}

func bootstrapNodeAPIBases(cfg *config.Config) []string {
	bases := append([]string(nil), defaultBootstrapNodeAPIBases...)
	if cfg == nil {
		return dedupeStrings(bases)
	}
	for _, indexerURL := range cfg.Indexer.KnownIndexers {
		bases = append(bases, rootAPIBaseCandidates(indexerURL)...)
	}
	return dedupeStrings(bases)
}

func fetchBootstrapPeersFromManifest(
	ctx context.Context,
	client *http.Client,
	rawURL string,
) ([]string, error) {
	body, err := fetchURL(ctx, client, rawURL)
	if err != nil {
		return nil, err
	}

	var manifest bootstrapManifestResponse
	if err := toml.Unmarshal(body, &manifest); err != nil {
		return nil, fmt.Errorf("parse bootstrap manifest %s: %w", rawURL, err)
	}

	peers := make([]string, 0, len(manifest.Peers))
	for _, peer := range manifest.Peers {
		if strings.TrimSpace(peer.Address) == "" {
			continue
		}
		peers = append(peers, strings.TrimSpace(peer.Address))
	}
	return dedupeStrings(peers), nil
}

func fetchBootstrapPeersFromNodeAPI(
	ctx context.Context,
	client *http.Client,
	baseURL string,
) ([]string, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	configBody, configErr := fetchURL(ctx, client, baseURL+"/api/node/config")
	statusBody, statusErr := fetchURL(ctx, client, baseURL+"/api/node/status")
	peersBody, peersErr := fetchURL(ctx, client, baseURL+"/api/node/peers")
	if configErr != nil && statusErr != nil && peersErr != nil {
		return nil, fmt.Errorf("no bootstrap node API available at %s", baseURL)
	}

	var nodeCfg bootstrapNodeConfigResponse
	if configErr == nil {
		if err := json.Unmarshal(configBody, &nodeCfg); err != nil {
			return nil, fmt.Errorf("parse node config %s: %w", baseURL, err)
		}
	}

	var nodeStatus bootstrapNodeStatusResponse
	if statusErr == nil {
		if err := json.Unmarshal(statusBody, &nodeStatus); err != nil {
			return nil, fmt.Errorf("parse node status %s: %w", baseURL, err)
		}
	}

	peers := append([]string(nil), nodeCfg.BootstrapPeers...)
	peers = append(peers, nodeCfg.DefaultBootstrapPeers...)
	peers = append(peers, advertisedPeersForNode(baseURL, nodeCfg.ListenAddresses, nodeStatus.NodeID)...)

	if peersErr == nil {
		var nodePeers []bootstrapNodePeerResponse
		if err := json.Unmarshal(peersBody, &nodePeers); err == nil {
			peers = append(peers, bootstrapPeersFromConnectedPeers(nodePeers)...)
		}
	}

	return dedupeStrings(peers), nil
}

func fetchURL(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, rawURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}
	return body, nil
}

func rootAPIBaseCandidates(rawURL string) []string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" {
		return nil
	}

	host := parsed.Hostname()
	if host == "" {
		return nil
	}

	candidates := []string{
		fmt.Sprintf("%s://%s", parsed.Scheme, host),
	}
	if parsed.Scheme == "http" {
		candidates = append(candidates, "https://"+host)
	}
	if parsed.Scheme == "https" {
		candidates = append(candidates, "http://"+host)
	}
	return dedupeStrings(candidates)
}

func advertisedPeersForNode(baseURL string, listenAddrs []string, peerID string) []string {
	if peerID == "" {
		return nil
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return nil
	}

	hostComponent := hostMultiaddrComponent(parsed.Hostname())
	if hostComponent == "" {
		return nil
	}

	peers := make([]string, 0, len(listenAddrs))
	for _, addr := range listenAddrs {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		var builder strings.Builder
		builder.WriteString(hostComponent)

		if strings.Contains(addr, "/udp/") {
			if port := multiaddrPort(addr, "udp"); port != "" {
				builder.WriteString("/udp/")
				builder.WriteString(port)
				if strings.Contains(addr, "/quic-v1") {
					builder.WriteString("/quic-v1")
				}
			}
		} else if port := multiaddrPort(addr, "tcp"); port != "" {
			builder.WriteString("/tcp/")
			builder.WriteString(port)
		}

		if !strings.Contains(builder.String(), "/tcp/") && !strings.Contains(builder.String(), "/udp/") {
			continue
		}

		builder.WriteString("/p2p/")
		builder.WriteString(peerID)
		peers = append(peers, builder.String())
	}
	return dedupeStrings(peers)
}

func bootstrapPeersFromConnectedPeers(peers []bootstrapNodePeerResponse) []string {
	results := make([]string, 0, len(peers))
	for _, peer := range peers {
		if peer.ID == "" {
			continue
		}
		for _, addr := range peer.Addresses {
			if strings.TrimSpace(addr) == "" {
				continue
			}
			if strings.Contains(addr, "/p2p/") {
				results = append(results, strings.TrimSpace(addr))
				continue
			}
			results = append(results, strings.TrimRight(strings.TrimSpace(addr), "/")+"/p2p/"+peer.ID)
		}
	}
	return dedupeStrings(results)
}

func hostMultiaddrComponent(host string) string {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() != nil {
			return "/ip4/" + host
		}
		return "/ip6/" + host
	}
	return "/dns/" + host
}

func multiaddrPort(addr string, proto string) string {
	parts := strings.Split(strings.Trim(addr, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == proto {
			return parts[i+1]
		}
	}
	return ""
}

func dedupeStrings(values []string) []string {
	results := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || slices.Contains(results, value) {
			continue
		}
		results = append(results, value)
	}
	return results
}
