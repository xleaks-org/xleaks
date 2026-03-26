package p2p

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

func TestIndexerAPIBaseURLs(t *testing.T) {
	t.Parallel()

	ipAddr := ma.StringCast("/ip4/203.0.113.10/tcp/7460")
	dnsAddr := ma.StringCast("/dns4/indexer.example.org/tcp/7460")
	loopbackAddr := ma.StringCast("/ip4/127.0.0.1/tcp/7460")

	got := indexerAPIBaseURLs([]ma.Multiaddr{ipAddr, dnsAddr, loopbackAddr}, "0.0.0.0:7471")
	if !slices.Contains(got, "http://203.0.113.10:7471") {
		t.Fatalf("expected IP-derived indexer URL in %v", got)
	}
	if !slices.Contains(got, "http://indexer.example.org:7471") {
		t.Fatalf("expected DNS-derived indexer URL in %v", got)
	}
	if slices.Contains(got, "http://127.0.0.1:7471") {
		t.Fatalf("did not expect loopback indexer URL in %v", got)
	}
}

func TestParseIndexerInfoLegacyAdvertisement(t *testing.T) {
	t.Parallel()

	pid, err := peer.Decode("12D3KooWSy7rrdGY2AbGPHgHMgJkuuDxiZp88TSsiFGNnpSgiSto")
	if err != nil {
		t.Fatalf("peer.Decode() error = %v", err)
	}

	legacy := peer.AddrInfo{
		ID:    pid,
		Addrs: []ma.Multiaddr{ma.StringCast("/dns4/indexer.example.org/tcp/7460")},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	info, err := parseIndexerInfo(data)
	if err != nil {
		t.Fatalf("parseIndexerInfo() error = %v", err)
	}
	if info.PeerID != pid {
		t.Fatalf("expected peer ID %s, got %s", pid, info.PeerID)
	}
	if !slices.Contains(info.APIBaseURLs, "http://indexer.example.org:7471") {
		t.Fatalf("expected derived API base URL in %v", info.APIBaseURLs)
	}
}
