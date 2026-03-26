package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/xleaks-org/xleaks/pkg/config"
)

func TestFetchBootstrapPeersFromManifest(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
[[peers]]
address = "/dns/bootstrap-1.example.org/tcp/7460/p2p/12D3KooW111111111111111111111111111111111111111111"

[[peers]]
address = "/dns/bootstrap-2.example.org/udp/7460/quic-v1/p2p/12D3KooW222222222222222222222222222222222222222222"
`))
	}))
	defer server.Close()

	peers, err := fetchBootstrapPeersFromManifest(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchBootstrapPeersFromManifest() error = %v", err)
	}

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d (%v)", len(peers), peers)
	}
}

func TestFetchBootstrapPeersFromNodeAPI(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/node/config":
			_, _ = w.Write([]byte(`{
				"bootstrap_peers": [],
				"default_bootstrap_peers": ["/dns/bootstrap.example.org/tcp/7460/p2p/12D3KooWbootstrap"],
				"listen_addresses": ["/ip4/0.0.0.0/tcp/7460", "/ip4/0.0.0.0/udp/7460/quic-v1"]
			}`))
		case "/api/node/status":
			_, _ = w.Write([]byte(`{"node_id":"12D3KooWlocalnode"}`))
		case "/api/node/peers":
			_, _ = w.Write([]byte(`[{
				"id": "12D3KooWremotepeer",
				"addresses": ["/dns/peer-cache.example.org/tcp/7460"]
			}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	peers, err := fetchBootstrapPeersFromNodeAPI(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetchBootstrapPeersFromNodeAPI() error = %v", err)
	}

	if !slices.Contains(peers, "/dns/bootstrap.example.org/tcp/7460/p2p/12D3KooWbootstrap") {
		t.Fatalf("expected default bootstrap peer in %v", peers)
	}
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("url.Parse(server.URL) error = %v", err)
	}
	localHost := hostMultiaddrComponent(parsed.Hostname())
	if !slices.Contains(peers, localHost+"/tcp/7460/p2p/12D3KooWlocalnode") {
		t.Fatalf("expected derived advertised TCP peer in %v", peers)
	}
	if !slices.Contains(peers, localHost+"/udp/7460/quic-v1/p2p/12D3KooWlocalnode") {
		t.Fatalf("expected derived advertised QUIC peer in %v", peers)
	}
	if !slices.Contains(peers, "/dns/peer-cache.example.org/tcp/7460/p2p/12D3KooWremotepeer") {
		t.Fatalf("expected connected peer bootstrap address in %v", peers)
	}
}

func TestBootstrapNodeAPIBasesIncludesKnownIndexerHosts(t *testing.T) {
	t.Parallel()

	cfg := config.DefaultConfig()
	cfg.Indexer.KnownIndexers = []string{"http://indexer.example.org:7471"}

	bases := bootstrapNodeAPIBases(cfg)
	if !slices.Contains(bases, "https://xleaks.org") {
		t.Fatalf("expected built-in node API base in %v", bases)
	}
	if !slices.Contains(bases, "http://indexer.example.org") {
		t.Fatalf("expected HTTP base derived from known indexer in %v", bases)
	}
	if !slices.Contains(bases, "https://indexer.example.org") {
		t.Fatalf("expected HTTPS base derived from known indexer in %v", bases)
	}
}
