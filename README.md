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

If you want automatic wide-area peer discovery, set `network.bootstrap_peers`
to full libp2p multiaddrs that include `/p2p/<peer-id>`.
