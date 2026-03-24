# Contributing to XLeaks

Thank you for your interest in contributing to XLeaks. This document provides
everything you need to get started: setting up the development environment,
building the project, running tests, and submitting contributions.

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Development Setup](#2-development-setup)
3. [Building from Source](#3-building-from-source)
4. [Running in Development Mode](#4-running-in-development-mode)
5. [Running Tests](#5-running-tests)
6. [Code Style Guidelines](#6-code-style-guidelines)
7. [Commit Message Format](#7-commit-message-format)
8. [Pull Request Process](#8-pull-request-process)
9. [Architecture Decision Records](#9-architecture-decision-records)
10. [Project Structure Reference](#10-project-structure-reference)

---

## 1. Prerequisites

Before you begin, ensure you have the following tools installed:

| Tool | Version | Purpose |
|---|---|---|
| **Go** | 1.25.7 | Backend, P2P node, API, and embedded web UI |
| **protoc** | 3.x | Protocol Buffer compiler |
| **protoc-gen-go** | latest | Go code generation for protobuf |
| **golangci-lint** | latest | Go linter (for `make lint`) |
| **Git** | 2.x+ | Version control |
| **Make** | any | Build automation |

### Installing Prerequisites

#### Go

Download from [go.dev/dl](https://go.dev/dl/) or use a package manager:

```bash
# macOS
brew install go

# Ubuntu/Debian
sudo snap install go --classic

# Verify
go version   # Should show go1.25.7
```

#### Protocol Buffers Compiler

```bash
# macOS
brew install protobuf

# Ubuntu/Debian
sudo apt-get install -y protobuf-compiler

# Install Go plugin
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

# Verify
protoc --version
```

Make sure `$GOPATH/bin` (typically `~/go/bin`) is in your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

#### golangci-lint

```bash
# macOS
brew install golangci-lint

# Linux / other
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Verify
golangci-lint --version
```

---

## 2. Development Setup

### Clone the Repository

```bash
git clone https://github.com/xleaks-org/xleaks.git
cd xleaks
```

### Install Go Dependencies

```bash
go mod download
```

### Generate Protobuf Code

The protobuf definitions are in `proto/messages.proto`. Generated Go code is
placed in `proto/gen/`. You must regenerate after any changes to the `.proto`
file:

```bash
make proto
```

This runs `scripts/gen-proto.sh`, which invokes:

```bash
protoc \
  --go_out=proto/gen \
  --go_opt=paths=source_relative \
  -I proto \
  proto/messages.proto
```

### Verify Setup

Run the full test suite to confirm everything is working:

```bash
make test
```

---

## 3. Building from Source

### Build for Current Platform

```bash
make build
```

This compiles the Go node and embeds the server-rendered web UI assets into
`bin/xleaks`.

### Build for All Platforms

```bash
make build-all
```

Produces binaries for all supported platforms:

| Binary | Platform |
|---|---|
| `bin/xleaks-linux-amd64` | Linux x86_64 |
| `bin/xleaks-linux-arm64` | Linux ARM64 |
| `bin/xleaks-darwin-amd64` | macOS Intel |
| `bin/xleaks-darwin-arm64` | macOS Apple Silicon |
| `bin/xleaks-windows-amd64.exe` | Windows x86_64 |

### Build Release Artifacts

Creates binaries, checksums, and compressed archives:

```bash
make release
```

Output is placed in the `release/` directory.

### Clean Build Artifacts

```bash
make clean
```

Removes local build artifacts such as `bin/`.

---

## 4. Running in Development Mode

Development mode runs the Go node directly with the embedded web UI:

```bash
make dev
```

This runs `scripts/dev.sh`, which starts the Go node in the foreground.

**The node listens on:**

| Service | Address | Purpose |
|---|---|---|
| P2P | `0.0.0.0:7460` (TCP + QUIC) | Peer-to-peer networking |
| API | `127.0.0.1:7470` | Local HTTP + WebSocket API |

### Data Directory

In development, the node uses `~/.xleaks/` for data storage. To use a separate
data directory for development, set the `XLEAKS_DATA_DIR` environment variable
or modify `config.toml`.

### Running Multiple Nodes Locally

For testing P2P features, you can run multiple nodes on the same machine using
different data directories and ports. Adjust `config.toml` for each instance:

```bash
# Node 1 (default ports)
make dev

# Node 2 (different ports, different data dir)
# Create a custom config with different ports and data_dir, then:
go run ./cmd/xleaks/ --config /path/to/node2-config.toml
```

---

## 5. Running Tests

### All Tests

```bash
make test
```

Runs all tests with race detection and coverage reporting:

```bash
go test -race -cover ./...
```

### Unit Tests Only

```bash
make test-unit
```

Runs tests in `pkg/` packages with the `-short` flag:

```bash
go test -race -cover -short ./pkg/...
```

### Integration Tests

Integration tests spin up multiple nodes in-process and verify end-to-end
behavior:

```bash
make test-integration
```

This runs:

```bash
go test -race -v -count=1 ./tests/integration/...
```

Integration tests verify:

- **Message propagation:** Node A posts, Node B receives via GossipSub.
- **Content replication:** Node B follows Node A, fetches historical content.
- **Media transfer:** Chunked media upload, download, and reassembly.
- **DM delivery:** End-to-end encrypted message delivery and decryption.
- **Reaction propagation:** Like events trigger notifications.
- **Profile updates:** Profile changes propagate to connected nodes.
- **Peer discovery:** Nodes discover each other through DHT.

### Protocol Conformance Tests

Verify that all protocol validation rules are correctly enforced:

```bash
make test-protocol
```

These tests verify:

- Invalid signatures are rejected.
- Oversized content is rejected.
- Duplicate reactions are deduplicated.
- Future-dated messages are rejected.
- Stale profile version numbers are rejected.
- Self-follow is rejected.

### Test Coverage

To generate a detailed coverage report:

```bash
go test -race -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
open coverage.html
```

**Coverage target:** minimum 80% coverage per package.

### Running a Specific Test

```bash
# Run a single test function
go test -race -v -run TestPostValidation ./pkg/content/...

# Run all tests in a specific package
go test -race -v ./pkg/identity/...
```

---

## 6. Code Style Guidelines

### Go Code

- Follow the standard [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
  and [Effective Go](https://go.dev/doc/effective_go).
- Run `golangci-lint` before submitting:

  ```bash
  make lint
  ```

- **Formatting:** All Go code must be formatted with `gofmt`. The linter
  enforces this.
- **Naming:**
  - Exported names use `PascalCase`.
  - Unexported names use `camelCase`.
  - Acronyms are all-caps when exported (`CID`, `DHT`, `HTTP`), lowercase when
    unexported (`cidBytes`, `dhtClient`).
- **Error handling:** Always handle errors. Do not use `_` to ignore errors
  unless there is a documented reason.
- **Comments:** All exported types, functions, and methods must have doc
  comments. Start the comment with the name of the thing being documented.
- **Package organization:** One package per directory. Package names are short,
  lowercase, and singular.
- **Testing:** Each package has corresponding `_test.go` files in the same
  directory. Use `testify/assert` and `testify/require` for assertions.
- **Structured logging:** Use `zap` for logging. Include context fields
  (e.g., `zap.String("peer_id", id)`). Never use `fmt.Println` for
  operational output.

### Web UI Code (Go templates / htmx)

- Keep handlers, templates, and partials in sync.
- Prefer small template helpers over embedding logic in handlers.
- Preserve server-rendered flows for onboarding, feed actions, and settings.
- Put reusable HTML in `pkg/web/templates/` and route logic in `pkg/web/`.

### Protobuf

- Follow the [Protocol Buffers Style Guide](https://protobuf.dev/programming-guides/style/).
- Message names use `PascalCase`.
- Field names use `snake_case`.
- All changes to `proto/messages.proto` require regeneration (`make proto`)
  and must maintain backward compatibility.

---

## 7. Commit Message Format

Use conventional commit messages with the following format:

```
<type>(<scope>): <short description>

<optional body>

<optional footer>
```

### Types

| Type | Description |
|---|---|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only changes |
| `style` | Formatting, missing semicolons, etc. (no code change) |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `build` | Changes to build system or external dependencies |
| `ci` | Changes to CI/CD configuration |
| `chore` | Other changes that don't modify src or test files |

### Scopes

| Scope | Package / Area |
|---|---|
| `identity` | `pkg/identity/` |
| `content` | `pkg/content/` |
| `p2p` | `pkg/p2p/` |
| `feed` | `pkg/feed/` |
| `social` | `pkg/social/` |
| `indexer` | `pkg/indexer/` |
| `storage` | `pkg/storage/` |
| `api` | `pkg/api/` |
| `proto` | `proto/` |
| `web` | `web/` |
| `config` | `configs/`, `pkg/config/` |

### Examples

```
feat(social): add encrypted DM send and receive

Implement X25519 key exchange and NaCl box encryption for
direct messages between users. Messages are encrypted before
publishing to GossipSub and decrypted on receipt.

Closes #42
```

```
fix(content): reject posts exceeding 5000 character limit

The validator was counting bytes instead of UTF-8 characters,
allowing multi-byte content to exceed the intended limit.
```

```
test(p2p): add integration test for multi-node message propagation

Spawns 3 nodes, has Node A publish a post, and verifies Nodes B
and C receive it within 5 seconds via GossipSub.
```

---

## 8. Pull Request Process

### Before You Start

1. **Check existing issues** to see if someone is already working on the same
   thing.
2. **Open an issue** first for significant features or architectural changes
   to discuss the approach before investing time in code.

### Workflow

1. **Fork** the repository (or create a branch if you have write access).

2. **Create a feature branch** from `main`:

   ```bash
   git checkout -b feat/your-feature-name
   ```

3. **Make your changes** following the code style guidelines.

4. **Write or update tests** for any changed behavior. All new code must have
   corresponding tests.

5. **Run the full test suite** and linter:

   ```bash
   make test
   make lint
   ```

6. **Commit** using the commit message format described above.

7. **Push** your branch:

   ```bash
   git push origin feat/your-feature-name
   ```

8. **Open a Pull Request** against `main`:
   - Provide a clear title and description.
   - Reference any related issues.
   - Include steps to test or verify the change.

### PR Checklist

Before requesting review, ensure your PR meets these criteria:

- [ ] Code compiles without errors.
- [ ] All existing tests pass (`make test`).
- [ ] New tests cover the changed behavior.
- [ ] Linter passes (`make lint`).
- [ ] Protobuf code is regenerated if `messages.proto` was changed (`make proto`).
- [ ] No new warnings or deprecation notices.
- [ ] Documentation is updated if behavior changed.
- [ ] Commit messages follow the conventional format.

### Review Process

- At least one maintainer review is required before merging.
- Address all review comments. If you disagree, explain your reasoning -- do
  not ignore feedback.
- PRs are merged via squash-and-merge to keep the `main` branch history clean.
- After merge, the feature branch is deleted.

### What Makes a Good PR

- **Small and focused.** One logical change per PR. If you find unrelated
  issues while working, file separate issues or PRs.
- **Well-tested.** Reviewers should be able to trust the tests.
- **Well-described.** Explain what the change does and why. Include before/after
  behavior if applicable.

---

## 9. Architecture Decision Records

Significant architectural decisions are documented as Architecture Decision
Records (ADRs) to preserve context for future contributors.

### When to Write an ADR

Write an ADR when:

- Choosing between multiple viable technical approaches.
- Making a decision that would be expensive to reverse.
- Establishing a pattern that other code should follow.
- Changing a previously established pattern.

### ADR Format

ADRs are stored in `docs/adr/` and numbered sequentially. Use the following
template:

```markdown
# ADR-NNNN: Title

**Status:** Proposed | Accepted | Deprecated | Superseded by ADR-XXXX
**Date:** YYYY-MM-DD
**Author:** @github-handle

## Context

What is the issue or problem that motivates this decision?

## Decision

What is the decision that was made?

## Consequences

What are the positive and negative consequences of this decision?

## Alternatives Considered

What other options were evaluated and why were they rejected?
```

### ADR Lifecycle

1. **Author** creates the ADR with status `Proposed` and opens a PR.
2. **Team** discusses in the PR. Modifications are made based on feedback.
3. If accepted, status changes to `Accepted` and the PR is merged.
4. If a later decision supersedes an ADR, the original is updated to
   `Superseded by ADR-XXXX`.

---

## 10. Project Structure Reference

```
xleaks/
├── cmd/xleaks/main.go              # Application entry point
├── proto/
│   ├── messages.proto              # Protobuf message definitions
│   └── gen/                        # Generated Go code (do not edit)
├── pkg/
│   ├── identity/                   # Key management, signing, encryption
│   ├── content/                    # CAS, CID, chunking, validation
│   ├── p2p/                        # libp2p networking
│   ├── feed/                       # Feed assembly and replication
│   ├── social/                     # Social features (posts, DMs, etc.)
│   ├── indexer/                    # Search, trending, indexer mode
│   ├── storage/                    # SQLite database layer
│   ├── api/                        # HTTP + WebSocket API server
│   │   ├── middleware/             # Auth, CORS, rate limiting
│   │   └── handlers/              # Endpoint handlers
│   ├── config/                     # TOML configuration loading
│   └── version/                    # Build version info
├── web/                            # Embedded static assets
│   ├── public/                     # Public files served from the Go binary
│   └── embed.go                    # go:embed declarations
├── configs/
│   ├── default.toml                # Default configuration
│   └── bootstrap_peers.toml        # Bootstrap peer list
├── scripts/
│   ├── dev.sh                      # Development mode
│   └── gen-proto.sh                # Protobuf code generation
├── tests/
│   ├── integration/                # Multi-node integration tests
│   └── protocol/                   # Protocol conformance tests
├── docs/
│   ├── PROTOCOL.md                 # Wire protocol specification
│   ├── ARCHITECTURE.md             # System architecture
│   └── CONTRIBUTING.md             # This file
├── go.mod
├── go.sum
└── Makefile
```

### Makefile Quick Reference

| Target | Command | Description |
|---|---|---|
| `make build` | Build for current platform | Outputs `bin/xleaks` |
| `make build-all` | Cross-compile all platforms | Outputs to `bin/` |
| `make test` | Run all tests | With race detection and coverage |
| `make test-unit` | Run unit tests only | `pkg/` packages, `-short` flag |
| `make test-integration` | Run integration tests | Multi-node tests |
| `make test-protocol` | Run protocol tests | Conformance tests |
| `make lint` | Run golangci-lint | Static analysis |
| `make proto` | Regenerate protobuf | Outputs to `proto/gen/` |
| `make dev` | Development mode | Runs the Go node with the embedded web UI |
| `make clean` | Clean artifacts | Removes `bin/` and related outputs |
| `make release` | Build release | Binaries + checksums + archives |

---

## Questions?

If you have questions about contributing, open a
[GitHub Discussion](https://github.com/xleaks-org/xleaks/discussions) or reach
out to the maintainers.
