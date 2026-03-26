package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	// mdnsServiceTag is the mDNS service tag used for LAN peer discovery.
	mdnsServiceTag = "_xleaks-discovery._udp"

	// indexerDHTKey is the well-known DHT key under which indexer nodes
	// advertise themselves.
	indexerDHTKey = "/xleaks/indexers"
)

// IndexerInfo describes a discovered indexer node and the HTTP API endpoints
// that regular nodes can query for search/explore/trending data.
type IndexerInfo struct {
	PeerID      peer.ID
	Addrs       []ma.Multiaddr
	APIBaseURLs []string
}

type indexerAdvertisement struct {
	PeerID      string   `json:"peer_id"`
	Addrs       []string `json:"addrs,omitempty"`
	APIBaseURLs []string `json:"api_base_urls,omitempty"`
}

// Bootstrap connects to the provided bootstrap peers and bootstraps the DHT
// routing table. If bootstrapPeers is empty, the peers from the host's
// configuration are used.
func (h *Host) Bootstrap(ctx context.Context, bootstrapPeers []string) error {
	if len(bootstrapPeers) == 0 {
		bootstrapPeers = h.cfg.BootstrapPeers
	}

	// Parse and connect to each bootstrap peer concurrently.
	var wg sync.WaitGroup
	errCh := make(chan error, len(bootstrapPeers))
	validPeers := 0

	for _, addrStr := range bootstrapPeers {
		maddr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			log.Printf("warning: skipping invalid bootstrap peer %q: %v", addrStr, err)
			continue
		}

		info, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Printf("warning: skipping invalid bootstrap peer %q: %v", addrStr, err)
			continue
		}

		validPeers++
		wg.Add(1)
		go func(pi peer.AddrInfo) {
			defer wg.Done()
			if err := h.host.Connect(ctx, pi); err != nil {
				errCh <- fmt.Errorf("connecting to bootstrap peer %s: %w", pi.ID, err)
			}
		}(*info)
	}

	wg.Wait()
	close(errCh)

	// Collect errors but don't fail if at least some peers are unreachable;
	// the DHT can still bootstrap with partial connectivity.
	var connectErrors []error
	for err := range errCh {
		connectErrors = append(connectErrors, err)
	}

	if len(connectErrors) == validPeers && validPeers > 0 {
		return fmt.Errorf("failed to connect to any bootstrap peer; last error: %w",
			connectErrors[len(connectErrors)-1])
	}

	// Bootstrap the DHT routing table.
	if err := h.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("bootstrapping DHT: %w", err)
	}

	return nil
}

// SetupMDNS enables mDNS discovery for finding peers on the local network.
// Discovered peers are automatically connected to.
func (h *Host) SetupMDNS(ctx context.Context) error {
	notifee := &mdnsNotifee{
		host: h,
		ctx:  ctx,
	}

	svc := mdns.NewMdnsService(h.host, mdnsServiceTag, notifee)
	if err := svc.Start(); err != nil {
		return fmt.Errorf("starting mDNS service: %w", err)
	}

	return nil
}

// mdnsNotifee implements the mdns.Notifee interface and connects to
// discovered peers.
type mdnsNotifee struct {
	host *Host
	ctx  context.Context
}

// HandlePeerFound is called when a new peer is discovered via mDNS.
func (n *mdnsNotifee) HandlePeerFound(info peer.AddrInfo) {
	// Don't connect to ourselves.
	if info.ID == n.host.ID() {
		return
	}
	// Best-effort connection; ignore errors for LAN discovery.
	_ = n.host.host.Connect(n.ctx, info)
}

// AdvertiseAsIndexer publishes this node's peer info and public indexer API
// endpoints under the well-known
// DHT key so that other nodes can discover indexer nodes.
func (h *Host) AdvertiseAsIndexer(ctx context.Context, publicAPIAddress string) error {
	info := indexerAdvertisement{
		PeerID:      h.host.ID().String(),
		APIBaseURLs: indexerAPIBaseURLs(h.host.Addrs(), publicAPIAddress),
	}
	for _, addr := range h.host.Addrs() {
		info.Addrs = append(info.Addrs, addr.String())
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshaling peer info: %w", err)
	}

	if err := h.dht.PutValue(ctx, indexerDHTKey, data); err != nil {
		return fmt.Errorf("advertising as indexer via DHT: %w", err)
	}

	return nil
}

// FindIndexers looks up indexer nodes by querying the DHT for the well-known
// indexer key. It collects up to 10 results from the DHT query channel.
func (h *Host) FindIndexers(ctx context.Context) ([]IndexerInfo, error) {
	const maxResults = 10

	ch, err := h.dht.SearchValue(ctx, indexerDHTKey)
	if err != nil {
		return nil, fmt.Errorf("querying DHT for indexers: %w", err)
	}

	seen := make(map[peer.ID]bool)
	var results []IndexerInfo

	for val := range ch {
		info, err := parseIndexerInfo(val)
		if err != nil {
			log.Printf("warning: failed to unmarshal indexer info: %v", err)
			continue
		}
		if seen[info.PeerID] {
			continue
		}
		seen[info.PeerID] = true
		results = append(results, info)
		if len(results) >= maxResults {
			break
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no indexers found via DHT")
	}

	return results, nil
}

func parseIndexerInfo(data []byte) (IndexerInfo, error) {
	var advert indexerAdvertisement
	if err := json.Unmarshal(data, &advert); err == nil && advert.PeerID != "" {
		pid, err := peer.Decode(advert.PeerID)
		if err != nil {
			return IndexerInfo{}, err
		}
		return IndexerInfo{
			PeerID:      pid,
			Addrs:       parseMultiaddrs(advert.Addrs),
			APIBaseURLs: dedupeStrings(advert.APIBaseURLs),
		}, nil
	}

	var legacy peer.AddrInfo
	if err := json.Unmarshal(data, &legacy); err != nil {
		return IndexerInfo{}, err
	}
	return IndexerInfo{
		PeerID:      legacy.ID,
		Addrs:       legacy.Addrs,
		APIBaseURLs: indexerAPIBaseURLs(legacy.Addrs, ":7471"),
	}, nil
}

func parseMultiaddrs(values []string) []ma.Multiaddr {
	addrs := make([]ma.Multiaddr, 0, len(values))
	for _, value := range values {
		addr, err := ma.NewMultiaddr(value)
		if err != nil {
			continue
		}
		addrs = append(addrs, addr)
	}
	return addrs
}

func indexerAPIBaseURLs(addrs []ma.Multiaddr, publicAPIAddress string) []string {
	port := apiPort(publicAPIAddress)
	if port == "" {
		port = "7471"
	}

	results := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		host := multiaddrHost(addr)
		if host == "" || !isPublicIndexerHost(host) {
			continue
		}
		results = append(results, "http://"+net.JoinHostPort(host, port))
	}
	return dedupeStrings(results)
}

func apiPort(publicAPIAddress string) string {
	if publicAPIAddress == "" {
		return ""
	}
	if _, port, err := net.SplitHostPort(publicAPIAddress); err == nil {
		return port
	}
	if parsed, err := url.Parse(publicAPIAddress); err == nil {
		if port := parsed.Port(); port != "" {
			return port
		}
	}
	if strings.HasPrefix(publicAPIAddress, ":") {
		return strings.TrimPrefix(publicAPIAddress, ":")
	}
	if _, err := strconv.Atoi(publicAPIAddress); err == nil {
		return publicAPIAddress
	}
	return ""
}

func multiaddrHost(addr ma.Multiaddr) string {
	for _, code := range []int{ma.P_DNS, ma.P_DNS4, ma.P_DNS6, ma.P_IP4, ma.P_IP6} {
		if value, err := addr.ValueForProtocol(code); err == nil && value != "" {
			return value
		}
	}
	return ""
}

func isPublicIndexerHost(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return host != "" && host != "localhost"
	}
	if !ip.IsGlobalUnicast() {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
