# XLeaks

XLeaks is a Go-based peer-to-peer social node. The repository contains the libp2p networking layer, local SQLite/CAS storage, HTTP/WebSocket API, and the embedded server-rendered web UI.

## Requirements

- Go `1.25.7`
- `protoc` plus `protoc-gen-go` if you need to regenerate `proto/gen/messages.pb.go`
- `golangci-lint` if you want to run `make lint`

## Quick Start

```bash
make build
./bin/xleaks
```

For local development:

```bash
./scripts/dev.sh
```

Useful commands:

```bash
go test ./...
go vet ./...
make proto
```

The node listens on `127.0.0.1:7470` for the local API/web UI and `7460` for P2P networking by default. Configuration is loaded from `~/.xleaks/config.toml`.

Remote API exposure now requires API token auth, and remote exposure of the embedded web UI is a separate explicit opt-in via `api.allow_remote_web_ui = true`. The default listener is still loopback-only, which keeps the API and embedded web UI local by default.

Default configs now ship with a working public bootstrap peer on `xleaks.org`
plus a built-in public indexer at `http://xleaks.org:7471`, so clean installs
can discover WAN peers and immediately use search/explore/trending without
manual edits. You can override or clear `network.bootstrap_peers` and
`indexer.known_indexers` in your config or from the settings page if you want
to use a different discovery set. When `configs/bootstrap_peers.toml` is
present in the packaged app or working tree, its entries are merged into the
default bootstrap set as well. If the static peer list is stale, startup also
tries the public node API and the repository bootstrap manifest as fallback
sources before giving up on WAN bootstrap.
