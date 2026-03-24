# XLeaks System Architecture

**Version:** 1.0
**Date:** 2026-03-22

---

## Table of Contents

1. [High-Level Architecture](#1-high-level-architecture)
2. [Module Breakdown and Dependencies](#2-module-breakdown-and-dependencies)
3. [Node Lifecycle](#3-node-lifecycle)
4. [Content Replication Strategy](#4-content-replication-strategy)
5. [Peer Discovery Mechanisms](#5-peer-discovery-mechanisms)
6. [Data Flow Diagrams](#6-data-flow-diagrams)
7. [SQLite Schema Overview](#7-sqlite-schema-overview)
8. [API Server Architecture](#8-api-server-architecture)
9. [WebSocket Real-Time Updates](#9-websocket-real-time-updates)

---

## 1. High-Level Architecture

XLeaks is a fully decentralized, peer-to-peer social platform. Every user runs
a node that embeds both the backend (Go) and the frontend (server-rendered Go
templates plus embedded static assets). There is no central server.

```
┌─────────────────────────────────────────────────────────────────────┐
│                         XLeaks Node                                 │
│                                                                     │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │                 Web UI (Go templates + htmx)                 │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │  │
│  │  │   Feed   │ │ Profile  │ │   DMs    │ │  Notifications │  │  │
│  │  └────┬─────┘ └────┬─────┘ └────┬─────┘ └───────┬────────┘  │  │
│  │       │             │            │               │            │  │
│  │       └─────────────┴────────────┴───────────────┘            │  │
│  │                         │  HTTP + WebSocket                   │  │
│  └─────────────────────────┼─────────────────────────────────────┘  │
│                            │                                        │
│  ┌─────────────────────────┼─────────────────────────────────────┐  │
│  │                    API Server (localhost:7470)                 │  │
│  │  ┌──────────┐ ┌────────┴───────┐ ┌──────────┐ ┌──────────┐  │  │
│  │  │  Router  │ │  WS Hub        │ │   Auth   │ │  Rate    │  │  │
│  │  │          │ │  (real-time)   │ │ (local)  │ │  Limit   │  │  │
│  │  └────┬─────┘ └────────────────┘ └──────────┘ └──────────┘  │  │
│  └───────┼───────────────────────────────────────────────────────┘  │
│          │                                                          │
│  ┌───────┴───────────────────────────────────────────────────────┐  │
│  │                     Core Services                             │  │
│  │                                                               │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │  │
│  │  │ Identity │ │  Social  │ │   Feed   │ │    Content     │  │  │
│  │  │ (keys,   │ │ (posts,  │ │ (timeline│ │    (CAS,       │  │  │
│  │  │  signing)│ │  DMs,    │ │  sync,   │ │     chunking,  │  │  │
│  │  │          │ │  likes)  │ │  replc.) │ │     validation)│  │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └────────────────┘  │  │
│  │                                                               │  │
│  │  ┌──────────┐ ┌──────────────────────────────────────────┐   │  │
│  │  │ Indexer  │ │              Storage (SQLite)             │   │  │
│  │  │ (search, │ │  ┌────────┐ ┌────────┐ ┌──────────────┐ │   │  │
│  │  │  trending│ │  │ Posts  │ │Profiles│ │ Reactions    │ │   │  │
│  │  │  stats)  │ │  │  Repo  │ │  Repo  │ │  Repo        │ │   │  │
│  │  └──────────┘ │  └────────┘ └────────┘ └──────────────┘ │   │  │
│  │               │  ┌────────┐ ┌────────┐ ┌──────────────┐ │   │  │
│  │               │  │  DM    │ │ Follow │ │ Notification │ │   │  │
│  │               │  │  Repo  │ │  Repo  │ │  Repo        │ │   │  │
│  │               │  └────────┘ └────────┘ └──────────────┘ │   │  │
│  │               └──────────────────────────────────────────┘   │  │
│  └───────────────────────────────────────────────────────────────┘  │
│          │                                                          │
│  ┌───────┴───────────────────────────────────────────────────────┐  │
│  │                    P2P Networking (libp2p)                     │  │
│  │                                                               │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌────────────────┐  │  │
│  │  │GossipSub │ │   DHT    │ │ Bitswap  │ │  NAT / Relay   │  │  │
│  │  │(pub/sub) │ │(Kademlia)│ │(content  │ │  (hole punch,  │  │  │
│  │  │          │ │          │ │ exchange)│ │   circuit relay)│  │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └────────────────┘  │  │
│  │                                                               │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────────────────────────┐  │  │
│  │  │  mDNS    │ │Bandwidth │ │       Metrics                │  │  │
│  │  │  (LAN)   │ │ Tracking │ │  (peers, bandwidth, uptime)  │  │  │
│  │  └──────────┘ └──────────┘ └──────────────────────────────┘  │  │
│  └───────────────────────────────────────────────────────────────┘  │
│          │                                                          │
│  ┌───────┴───────────────────────────────────────────────────────┐  │
│  │                     Local Disk Storage                        │  │
│  │                                                               │  │
│  │  ~/.xleaks/                                                   │  │
│  │  ├── config.toml        (configuration)                       │  │
│  │  ├── identity/          (encrypted keys)                      │  │
│  │  ├── data/objects/      (content-addressed protobuf objects)  │  │
│  │  ├── data/media/        (media chunks)                        │  │
│  │  ├── data/index.db      (SQLite database)                     │  │
│  │  ├── logs/              (application logs)                    │  │
│  │  └── cache/             (thumbnails, temp files)              │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
          │                             │
          │    TCP/QUIC (port 7460)     │
          ▼                             ▼
   ┌─────────────┐              ┌─────────────┐
   │  Peer Node  │ ◄──────────► │  Peer Node  │
   └─────────────┘              └─────────────┘
```

### Key Architectural Decisions

- **Single binary:** The Go backend and embedded web UI assets are compiled
  into one executable via Go's `embed` package.
- **No central server:** All features work in a fully peer-to-peer manner.
  Indexer nodes provide optional convenience services (search, trending) but
  the network functions without them.
- **Content immutability:** Posts, reactions, and all content are permanently
  immutable. The protocol has no edit or delete operations.
- **Local-first:** All data the user interacts with is stored locally. The
  network is used for replication, not as a primary data source.

---

## 2. Module Breakdown and Dependencies

### 2.1 Module Map

```
cmd/xleaks/main.go          Entry point: initializes and wires all modules
        │
        ├── pkg/identity/    Cryptographic identity management
        ├── pkg/content/     Content-addressed storage, validation, chunking
        ├── pkg/p2p/         libp2p networking layer
        ├── pkg/feed/        Feed assembly, subscriptions, replication
        ├── pkg/social/      Social actions (posts, reactions, DMs, profiles)
        ├── pkg/indexer/     Indexer mode (search, trending, stats)
        ├── pkg/storage/     SQLite database layer
        ├── pkg/api/         HTTP + WebSocket API server
        ├── pkg/config/      TOML configuration loading
        └── pkg/version/     Build version information
```

### 2.2 Package Details and Dependencies

#### `pkg/identity` -- Cryptographic Identity

Manages Ed25519 key pairs, signing, verification, encryption (for DMs), and
human-readable address encoding.

| File | Responsibility |
|---|---|
| `keypair.go` | Ed25519 key generation |
| `keystore.go` | Encrypted key storage (Argon2id + AES-256-GCM) |
| `mnemonic.go` | BIP39 seed phrase generation and recovery |
| `signing.go` | Sign and verify protobuf messages |
| `encryption.go` | X25519 DH + NaCl box for DM encryption |
| `address.go` | `xleaks1...` Bech32 address encoding/decoding |

**Dependencies:** Go stdlib `crypto/ed25519`, `golang.org/x/crypto/nacl/box`,
`go-bip39`, `go-multihash`

#### `pkg/content` -- Content Management

Handles content-addressed storage on disk, CID computation, media chunking and
reassembly, message validation, and thumbnail generation.

| File | Responsibility |
|---|---|
| `store.go` | Disk read/write with CID-based sharding |
| `cid.go` | SHA-256 multihash CID computation |
| `chunker.go` | Split media files into 256 KB chunks |
| `assembler.go` | Reassemble chunks into complete media files |
| `validator.go` | Validate signatures, CIDs, field constraints |
| `thumbnail.go` | Auto-generate image/video thumbnails |

**Dependencies:** `pkg/identity` (for signature verification), `go-multihash`,
`golang.org/x/image`

#### `pkg/p2p` -- Networking Layer

Manages all peer-to-peer communication using libp2p.

| File | Responsibility |
|---|---|
| `host.go` | libp2p host initialization, listen addresses, lifecycle |
| `config.go` | P2P configuration (ports, limits, transport) |
| `discovery.go` | DHT bootstrap, mDNS, peer discovery |
| `pubsub.go` | GossipSub setup, topic management, message routing |
| `bitswap.go` | Content exchange (serve and fetch chunks/objects) |
| `relay.go` | Circuit relay client for NAT-restricted nodes |
| `nat.go` | NAT traversal and DCUtR hole punching |
| `bandwidth.go` | Bandwidth tracking and optional rate limiting |
| `metrics.go` | Network metrics (peer count, bandwidth, latency) |

**Dependencies:** `go-libp2p`, `go-libp2p-pubsub`, `go-libp2p-kad-dht`,
`go-bitswap`, `pkg/content` (for validation)

#### `pkg/feed` -- Feed Management

Manages subscriptions, feed assembly, content replication from followed
publishers, and timeline construction.

| File | Responsibility |
|---|---|
| `manager.go` | Follow/unfollow logic, subscription lifecycle |
| `assembler.go` | Build feed from local database |
| `replicator.go` | Pin and fetch content from followed publishers |
| `sync.go` | Historical content sync for new follows |
| `timeline.go` | Chronological timeline with cursor-based pagination |

**Dependencies:** `pkg/p2p` (for GossipSub subscriptions and Bitswap),
`pkg/storage`, `pkg/content`

#### `pkg/social` -- Social Features

Implements the social application logic that sits between the API layer and the
storage/networking layers.

| File | Responsibility |
|---|---|
| `posts.go` | Create, validate, and publish posts |
| `reactions.go` | Like handling, deduplication, count aggregation |
| `reposts.go` | Repost creation and display logic |
| `threads.go` | Comment thread tree assembly |
| `profile.go` | Profile creation, versioned updates |
| `dm.go` | Encrypted DM send/receive/decrypt |
| `notifications.go` | Notification generation for likes, replies, follows, DMs |

**Dependencies:** `pkg/identity`, `pkg/content`, `pkg/p2p`, `pkg/storage`

#### `pkg/indexer` -- Indexer Mode

Optional module activated when node runs in indexer mode. Provides search,
trending, and discovery services to the network.

| File | Responsibility |
|---|---|
| `indexer.go` | Broad subscription, content ingestion |
| `search.go` | Full-text search index (Bleve) |
| `trending.go` | Trending algorithm (rolling time windows) |
| `stats.go` | Network-wide statistics |
| `api.go` | Public HTTP API for search/trending/discovery |

**Dependencies:** `bleve`, `pkg/storage`, `pkg/p2p`

#### `pkg/storage` -- Database Layer

All SQLite database operations, connection management, migrations, and
repository pattern implementations.

| File | Responsibility |
|---|---|
| `db.go` | SQLite connection pool, WAL mode, migrations, lifecycle |
| `schema.go` | Database schema definitions, auto-migration |
| `posts_repo.go` | Post CRUD, feed queries, thread queries |
| `reactions_repo.go` | Reaction storage, count queries, deduplication |
| `profiles_repo.go` | Profile storage, version-gated updates |
| `subscriptions_repo.go` | Follow list storage |
| `notifications_repo.go` | Notification queue, unread counts |
| `dm_repo.go` | DM conversation storage |
| `media_repo.go` | Media object metadata, fetch status |

**Dependencies:** `modernc.org/sqlite` (pure Go SQLite)

#### `pkg/api` -- HTTP and WebSocket Server

Local API server serving the web UI. Listens on localhost only.

| File | Responsibility |
|---|---|
| `server.go` | HTTP + WebSocket server setup and lifecycle |
| `router.go` | Route definitions, handler registration |
| `ws.go` | WebSocket hub: manages connections, broadcasts events |
| `middleware/auth.go` | Verify requests originate from localhost |
| `middleware/cors.go` | CORS headers for development |
| `middleware/ratelimit.go` | Per-endpoint rate limiting |
| `handlers/*.go` | Individual endpoint handlers |

**Dependencies:** `go-chi`, `gorilla/websocket`, all `pkg/*` service packages

### 2.3 Dependency Graph

```
                    cmd/xleaks/main.go
                           │
           ┌───────────────┼───────────────┐
           │               │               │
        pkg/api        pkg/config     pkg/version
           │
     ┌─────┼─────────┬─────────┬──────────┐
     │     │         │         │          │
 pkg/social  pkg/feed  pkg/indexer  pkg/identity
     │         │         │
     ├─────────┤         │
     │         │         │
 pkg/content   │     pkg/storage
     │         │         │
     └────┬────┘         │
          │              │
       pkg/p2p           │
          │              │
          └──────────────┘
                 │
          (libp2p, SQLite)
```

---

## 3. Node Lifecycle

### 3.1 Startup Sequence

```
┌─────────────────────────────────────────────────────────────┐
│                       STARTUP                                │
│                                                             │
│  1. Load config.toml                                        │
│     └─ Parse TOML, apply defaults, validate                 │
│                                                             │
│  2. Initialize SQLite database                              │
│     ├─ Open/create ~/.xleaks/data/index.db                  │
│     ├─ Enable WAL mode                                      │
│     └─ Run schema migrations                                │
│                                                             │
│  3. Check identity                                          │
│     ├─ If no identity exists -> redirect to onboarding      │
│     └─ If identity exists -> prompt passphrase -> decrypt   │
│                                                             │
│  4. Initialize content-addressed store                      │
│     └─ Ensure directory structure under ~/.xleaks/data/     │
│                                                             │
│  5. Initialize libp2p host                                  │
│     ├─ Generate or load libp2p host key                     │
│     ├─ Listen on configured addresses (TCP + QUIC)          │
│     ├─ Start Kademlia DHT                                   │
│     ├─ Connect to bootstrap peers                           │
│     ├─ Start mDNS discovery (if enabled)                    │
│     ├─ Configure relay circuits (if enabled)                │
│     └─ Configure hole punching (if enabled)                 │
│                                                             │
│  6. Subscribe to GossipSub topics                           │
│     ├─ /xleaks/dm/<own-pubkey-hex>                          │
│     ├─ /xleaks/global                                       │
│     └─ /xleaks/posts/<pubkey-hex> for each followed user    │
│                                                             │
│  7. Start content replication engine                        │
│     └─ Sync missed content from all followed publishers     │
│                                                             │
│  8. Start API server (localhost:7470)                       │
│     ├─ HTTP endpoints                                       │
│     └─ WebSocket hub                                        │
│                                                             │
│  9. Open UI                                                 │
│     ├─ Desktop: open Wails window                           │
│     └─ CLI mode: print URL for browser                      │
│                                                             │
│  ► Node is now RUNNING                                      │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Running State

While the node is running, it performs these concurrent operations:

```
┌─────────────────────────────────────────────────────────────┐
│                       RUNNING                                │
│                                                             │
│  Goroutine: GossipSub Message Handler                       │
│  ├─ Receive messages from subscribed topics                 │
│  ├─ Validate signature, CID, field constraints              │
│  ├─ Check rate limits                                       │
│  ├─ Check replay protection (CID dedup)                     │
│  ├─ Store valid messages in CAS + SQLite                    │
│  └─ Push events to WebSocket hub                            │
│                                                             │
│  Goroutine: Bitswap Server                                  │
│  └─ Serve content chunks to requesting peers                │
│                                                             │
│  Goroutine: API Server                                      │
│  ├─ Handle HTTP requests from local UI                      │
│  └─ Manage WebSocket connections                            │
│                                                             │
│  Goroutine: DHT Maintenance                                 │
│  ├─ Periodically refresh routing table                      │
│  ├─ Re-announce provider records                            │
│  └─ Discover new peers                                      │
│                                                             │
│  Goroutine: Storage Manager                                 │
│  ├─ Monitor disk usage                                      │
│  └─ Evict LRU unpinned content when over limit              │
│                                                             │
│  Goroutine: Replication Engine                               │
│  └─ Background sync of content from followed publishers     │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### 3.3 Shutdown Sequence

```
┌─────────────────────────────────────────────────────────────┐
│                       SHUTDOWN                               │
│                                                             │
│  1. Signal received (SIGINT, SIGTERM, or UI close)          │
│                                                             │
│  2. Stop accepting new API requests                         │
│     └─ Drain in-flight requests (5s timeout)                │
│                                                             │
│  3. Close WebSocket connections                             │
│     └─ Send close frame to all connected clients            │
│                                                             │
│  4. Unsubscribe from all GossipSub topics                   │
│                                                             │
│  5. Close all peer connections gracefully                    │
│     └─ libp2p host.Close()                                  │
│                                                             │
│  6. Flush pending database writes                           │
│                                                             │
│  7. Close SQLite database                                   │
│     └─ Checkpoint WAL                                       │
│                                                             │
│  8. Exit process                                            │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 4. Content Replication Strategy

### 4.1 Overview

Content is replicated across the network through a combination of real-time
push (GossipSub) and pull-based sync (Bitswap + DHT).

```
                    Publisher Node
                         │
                   ┌─────┴─────┐
                   │  Publishes │
                   │  to topic  │
                   └─────┬─────┘
                         │
              GossipSub propagation
                    ┌────┼────┐
                    │    │    │
                    ▼    ▼    ▼
             ┌──────┐┌──────┐┌──────┐
             │Sub A ││Sub B ││Sub C │   (Subscriber nodes)
             │      ││      ││      │
             │Store ││Store ││Store │   (Each stores full copy)
             └──────┘└──────┘└──────┘
```

### 4.2 Following a New Publisher

When a user follows a new publisher, the following sequence occurs:

```
Step 1: Subscribe to GossipSub topic
  └── /xleaks/posts/<publisher-pubkey-hex>
       → Future posts arrive in real-time

Step 2: Query DHT for historical content
  └── Look up provider records for publisher's content CIDs
       → Discover which peers have this publisher's posts

Step 3: Fetch historical content via Bitswap
  └── Request all known post CIDs from available peers
       → Download and validate each post
       → Store in local CAS + index in SQLite

Step 4: Mark sync as complete
  └── Set sync_completed = 1 in subscriptions table
```

### 4.3 Storage Guarantees

| Content Type | Eviction Policy |
|---|---|
| Posts from followed publishers | NEVER evicted (pinned) |
| Media from followed publishers | Metadata pinned; chunks fetched on demand by default |
| Discovered content (not followed) | LRU eviction when storage limit is reached |
| Own content | NEVER evicted |

### 4.4 Media Fetch Modes

| Mode | Behavior |
|---|---|
| `auto_fetch_media = false` (default) | Only fetch media chunks when the user opens the post in the UI. Metadata (MediaObject) is stored immediately. |
| `auto_fetch_media = true` | Pre-fetch all media chunks for followed publishers as soon as the MediaObject is received. Uses more bandwidth and storage. |

---

## 5. Peer Discovery Mechanisms

XLeaks uses a layered approach to peer discovery, ensuring connectivity across
diverse network environments.

### 5.1 Discovery Layers

```
┌────────────────────────────────────────────────────────┐
│                  Peer Discovery Stack                   │
│                                                        │
│  Layer 4: Relay Circuits (fallback)                    │
│  ├── For nodes behind strict NATs/firewalls            │
│  └── Route traffic through public relay nodes          │
│                                                        │
│  Layer 3: Hole Punching (DCUtR)                        │
│  ├── Attempt direct connection between NAT-ed peers    │
│  └── Coordinate via relay, then connect directly       │
│                                                        │
│  Layer 2: Kademlia DHT (primary)                       │
│  ├── Distributed peer discovery                        │
│  ├── Content provider records                          │
│  └── Indexer node advertisement                        │
│                                                        │
│  Layer 1: mDNS (local network)                         │
│  ├── Zero-configuration LAN discovery                  │
│  └── Works without internet connectivity               │
│                                                        │
│  Layer 0: Bootstrap Peers (initial)                    │
│  ├── Hardcoded list in bootstrap_peers.toml            │
│  ├── Used only on first connection                     │
│  └── No special authority (regular nodes)              │
│                                                        │
└────────────────────────────────────────────────────────┘
```

### 5.2 Bootstrap Process

```
1. Node starts with no known peers.
2. Read bootstrap peer list from configs/bootstrap_peers.toml.
3. Connect to bootstrap peers via TCP or QUIC.
4. Once connected, join the Kademlia DHT.
5. DHT routing table populates with nearby peers.
6. Discover additional peers through DHT walks.
7. mDNS broadcasts on local network for LAN peers.
8. Peer connections stabilize (target: max_peers from config, default 100).
```

### 5.3 Indexer Discovery

Regular nodes discover indexer nodes through a well-known DHT key:

```
DHT key: /xleaks/indexers

Indexer nodes advertise themselves as providers of this key.
Regular nodes query this key to find indexer API endpoints.
```

---

## 6. Data Flow Diagrams

### 6.1 Creating and Publishing a Post

```
User types post in UI
         │
         ▼
┌─────────────────┐
│  POST /api/posts│  (HTTP request from frontend)
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│              API Handler                 │
│  1. Parse request body (text + media)   │
│  2. Validate content length <= 5000     │
│  3. Extract hashtags from content       │
└────────┬────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│            Social Service                │
│  1. Build Post protobuf message         │
│  2. Set author = active identity pubkey │
│  3. Set timestamp = now (ms)            │
│  4. Compute CID (SHA-256 multihash)     │
│  5. Sign with Ed25519 private key       │
└────────┬────────────────────────────────┘
         │
         ├──────────────────┐
         ▼                  ▼
┌──────────────┐   ┌──────────────────┐
│ Content Store│   │    SQLite DB     │
│ Write object │   │ Insert into      │
│ to disk CAS  │   │ posts table +    │
│              │   │ update indexes   │
└──────────────┘   └──────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│           P2P / GossipSub                │
│  Publish to /xleaks/posts/<own-pubkey>  │
│  → Message propagates to all            │
│    subscribers across the network       │
└────────┬────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│           WebSocket Hub                  │
│  Broadcast {type: "new_post", data: {}} │
│  → UI updates feed in real-time         │
└─────────────────────────────────────────┘
```

### 6.2 Following a User

```
User clicks "Follow" on a profile
         │
         ▼
┌──────────────────────┐
│ POST /api/follow/:pk │
└──────────┬───────────┘
           │
           ▼
┌──────────────────────────────────────────┐
│            Feed Manager                   │
│  1. Create FollowEvent message           │
│  2. Sign with Ed25519                    │
│  3. Store in subscriptions table         │
│  4. Publish to /xleaks/follows/<own-pk>  │
└──────────┬───────────────────────────────┘
           │
     ┌─────┴─────┐
     │           │
     ▼           ▼
┌─────────┐ ┌────────────────────────────────┐
│Subscribe│ │     Replication Engine           │
│to topic │ │  1. Query DHT for publisher's   │
│/xleaks/ │ │     content providers           │
│posts/   │ │  2. Fetch historical posts      │
│<target> │ │     via Bitswap                 │
│         │ │  3. Validate and store each     │
│(future  │ │  4. Index in SQLite             │
│ posts)  │ │  5. Mark sync_completed = 1     │
└─────────┘ └────────────────────────────────┘
```

### 6.3 Sending a Direct Message

```
User types message in DM conversation
         │
         ▼
┌───────────────────────┐
│ POST /api/dm/:pubkey  │
└──────────┬────────────┘
           │
           ▼
┌──────────────────────────────────────────┐
│           DM Service                      │
│  1. Convert sender Ed25519 key → X25519  │
│  2. Convert recipient pubkey → X25519    │
│  3. Generate random 24-byte nonce        │
│  4. NaCl box.Seal(plaintext)             │
│     → encrypted_content                  │
│  5. Build DirectMessage protobuf         │
│  6. Compute CID, sign with Ed25519      │
└──────────┬───────────────────────────────┘
           │
     ┌─────┴──────────────┐
     │                    │
     ▼                    ▼
┌──────────┐   ┌────────────────────────────┐
│  SQLite  │   │        GossipSub           │
│  Store   │   │  Publish to                │
│  DM in   │   │  /xleaks/dm/<recipient-pk> │
│  local   │   │  → Encrypted message       │
│  DB      │   │    routed to recipient     │
└──────────┘   └───────────┬────────────────┘
                           │
                           ▼
                ┌────────────────────┐
                │  Recipient Node    │
                │  1. Receive msg    │
                │  2. Validate sig   │
                │  3. Store encrypted│
                │  4. Decrypt with   │
                │     own X25519 key │
                │  5. Display in UI  │
                └────────────────────┘
```

### 6.4 Media Upload

```
User drags image into post composer
         │
         ▼
┌────────────────────┐
│  POST /api/media   │  (multipart upload)
└─────────┬──────────┘
          │
          ▼
┌─────────────────────────────────────────────┐
│              Content Service                 │
│  1. Validate file size <= 100 MB            │
│  2. Validate MIME type                       │
│  3. Generate thumbnail (320px wide JPEG)     │
│  4. Split file into 256 KB chunks           │
│  5. Compute CID for each chunk              │
│  6. Compute CID for complete file           │
│  7. Store thumbnail chunk                    │
│  8. Build MediaObject protobuf              │
│  9. Sign MediaObject                         │
└─────────┬───────────────────────────────────┘
          │
    ┌─────┴──────────┐
    │                │
    ▼                ▼
┌─────────┐  ┌──────────────┐
│ Disk    │  │   SQLite     │
│ Store   │  │ Insert into  │
│ all     │  │ media_objects │
│ chunks  │  │ table        │
│ in CAS  │  │              │
└─────────┘  └──────────────┘
    │
    ▼
┌──────────────────────────────────────┐
│  Return MediaObject CID to caller    │
│  → Used in Post.media_cids field    │
│  → Chunks served via Bitswap to     │
│    peers who request them            │
└──────────────────────────────────────┘
```

---

## 7. SQLite Schema Overview

The SQLite database (`~/.xleaks/data/index.db`) serves as the index and
metadata layer. It operates in WAL (Write-Ahead Logging) mode for concurrent
read/write performance.

### 7.1 Table Summary

| Table | Purpose | Primary Key |
|---|---|---|
| `identities` | Local user identities | `pubkey` (BLOB) |
| `profiles` | Known user profiles from the network | `pubkey` (BLOB) |
| `posts` | All posts (own + followed + discovered) | `cid` (BLOB) |
| `post_media` | Media attachments linked to posts | `(post_cid, media_cid)` |
| `media_objects` | Media file metadata (size, chunks, dimensions) | `cid` (BLOB) |
| `reactions` | Likes (one per author per target per type) | `cid` (BLOB) |
| `reaction_counts` | Materialized reaction/reply/repost counts | `post_cid` (BLOB) |
| `subscriptions` | Who the local user follows | `pubkey` (BLOB) |
| `follow_events` | Follow/unfollow events observed on network | `(author, target)` |
| `follower_counts` | Materialized follower/following counts | `pubkey` (BLOB) |
| `direct_messages` | Encrypted DM storage | `cid` (BLOB) |
| `notifications` | Notification queue | `id` (INTEGER AUTOINCREMENT) |
| `post_tags` | Hashtag index for search/trending | `(post_cid, tag)` |
| `peer_stats` | Peer reputation tracking | `peer_id` (TEXT) |
| `content_access` | LRU eviction tracking | `cid` (BLOB) |

### 7.2 Key Indexes

```sql
-- Fast feed queries (posts by author, sorted by time)
CREATE INDEX idx_posts_author ON posts(author, timestamp DESC);

-- Chronological feed assembly
CREATE INDEX idx_posts_timestamp ON posts(timestamp DESC);

-- Thread queries (find replies to a post)
CREATE INDEX idx_posts_reply_to ON posts(reply_to) WHERE reply_to IS NOT NULL;

-- Repost discovery
CREATE INDEX idx_posts_repost_of ON posts(repost_of) WHERE repost_of IS NOT NULL;

-- Reaction lookups by target post
CREATE INDEX idx_reactions_target ON reactions(target);

-- Follower lookups
CREATE INDEX idx_follow_events_target ON follow_events(target);

-- Unread notifications
CREATE INDEX idx_notifications_unread ON notifications(read, timestamp DESC);

-- DM conversation ordering
CREATE INDEX idx_dm_conversation ON direct_messages(
    MIN(author, recipient), MAX(author, recipient), timestamp DESC
);

-- Hashtag search
CREATE INDEX idx_post_tags_tag ON post_tags(tag);
```

### 7.3 Materialized Views

Two tables act as materialized aggregates, updated incrementally:

- **`reaction_counts`** -- stores `like_count`, `reply_count`, and
  `repost_count` per post. Updated on each new reaction, reply, or repost
  to avoid expensive COUNT queries at read time.

- **`follower_counts`** -- stores `follower_count` and `following_count` per
  user. Updated on each follow/unfollow event.

### 7.4 Database Configuration

```
Mode:           WAL (Write-Ahead Logging)
Journal:        WAL
Synchronous:    NORMAL
Cache size:     Default (2000 pages)
Max connections: Connection pool managed by pkg/storage/db.go
```

---

## 8. API Server Architecture

### 8.1 Overview

The API server is an HTTP + WebSocket server that runs on localhost only. It
provides the interface between the embedded web UI and the Go node backend.

```
┌──────────────────────────────────────────────────┐
│           API Server (127.0.0.1:7470)             │
│                                                  │
│  ┌──────────────────────────────────────────┐    │
│  │              Middleware Chain              │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ │    │
│  │  │ LocalOnly│→│ RateLimit│→│   CORS   │ │    │
│  │  │          │ │          │ │          │ │    │
│  │  └──────────┘ └──────────┘ └──────────┘ │    │
│  └──────────────────┬───────────────────────┘    │
│                     │                            │
│  ┌──────────────────┴───────────────────────┐    │
│  │                 Router (chi)              │    │
│  │                                          │    │
│  │  /api/identity/*  → identity handlers    │    │
│  │  /api/posts/*     → posts handlers       │    │
│  │  /api/feed        → feed handler         │    │
│  │  /api/reactions   → reactions handler     │    │
│  │  /api/follow/*    → follow handlers      │    │
│  │  /api/profile     → profile handlers     │    │
│  │  /api/dm/*        → DM handlers          │    │
│  │  /api/search      → search handler       │    │
│  │  /api/trending    → trending handler     │    │
│  │  /api/media/*     → media handlers       │    │
│  │  /api/node/*      → node status handlers │    │
│  │  /api/notifications/* → notif handlers   │    │
│  │  /ws              → WebSocket upgrade    │    │
│  │  /*               → Embedded web routes  │    │
│  └──────────────────────────────────────────┘    │
│                                                  │
└──────────────────────────────────────────────────┘
```

### 8.2 Endpoint Groups

#### Identity Management

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/identity/create` | Generate new key pair; returns pubkey + seed phrase |
| `POST` | `/api/identity/import` | Import identity from seed phrase or key file |
| `POST` | `/api/identity/unlock` | Decrypt active identity with passphrase |
| `GET` | `/api/identity/active` | Get active identity pubkey and profile |
| `POST` | `/api/identity/lock` | Lock (re-encrypt) identity |
| `GET` | `/api/identity/list` | List all local identities |
| `PUT` | `/api/identity/switch/:pubkey` | Switch active identity |
| `GET` | `/api/identity/export` | Export encrypted key file |

#### Posts

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/posts` | Create new post (text + optional media CIDs) |
| `GET` | `/api/posts/:cid` | Get single post by CID |
| `GET` | `/api/posts/:cid/thread` | Get full thread (parent chain + replies) |
| `GET` | `/api/posts/:cid/reactions` | Get reactions for a post |
| `GET` | `/api/users/:pubkey/posts` | Get all posts by a user (paginated) |

#### Feed

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/feed` | Chronological feed from followed publishers |
| `GET` | `/api/feed?before=TIMESTAMP` | Cursor-based pagination |

#### Social Actions

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/reactions` | Create a reaction (like) on a post |
| `POST` | `/api/repost` | Repost a post |
| `POST` | `/api/follow/:pubkey` | Follow a user |
| `DELETE` | `/api/follow/:pubkey` | Unfollow a user |
| `GET` | `/api/following` | List followed users |
| `GET` | `/api/users/:pubkey/followers` | List followers (from local knowledge) |

#### Profiles

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/profile` | Get own profile |
| `PUT` | `/api/profile` | Update own profile |
| `GET` | `/api/users/:pubkey` | Get another user's profile |

#### Search and Discovery

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/search?q=QUERY&type=posts\|users` | Search posts or users |
| `GET` | `/api/trending` | Get trending posts and tags |
| `GET` | `/api/explore` | Suggested users to follow |

#### Direct Messages

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/dm` | List DM conversations |
| `GET` | `/api/dm/:pubkey` | Get messages with a user (paginated) |
| `POST` | `/api/dm/:pubkey` | Send encrypted DM to a user |

#### Notifications

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/notifications` | Get notifications (paginated) |
| `PUT` | `/api/notifications/read` | Mark all as read |
| `PUT` | `/api/notifications/:id/read` | Mark one as read |
| `GET` | `/api/notifications/unread-count` | Get unread count |

#### Media

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/media` | Upload media file (returns CID) |
| `GET` | `/api/media/:cid` | Get assembled media file |
| `GET` | `/api/media/:cid/thumbnail` | Get thumbnail |
| `GET` | `/api/media/:cid/status` | Get fetch progress (% of chunks) |

#### Node Status

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/node/status` | Health info (peers, bandwidth, storage, uptime) |
| `GET` | `/api/node/peers` | List connected peers |
| `GET` | `/api/node/config` | Get node configuration |
| `PUT` | `/api/node/config` | Update node configuration |

### 8.3 Security Model

- The API server binds to `127.0.0.1` by default and is wrapped in localhost
  enforcement middleware.
- The default node relies on the passphrase-unlocked identity in memory as the
  primary authentication context for state-changing actions.
- Optional bearer-token support exists in the API server package, but the
  default node startup path does not configure it.
- CORS is enabled on the local API surface because the server is constrained to
  localhost by middleware.

### 8.4 Indexer Public API

When running in indexer mode, a separate API server starts on port 7471
(configurable) and is publicly accessible:

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/search?q=QUERY&type=posts\|users&page=N` | Full-text search |
| `GET` | `/api/trending?window=1h\|6h\|24h\|7d` | Trending posts and tags |
| `GET` | `/api/explore/publishers` | Most followed publishers |
| `GET` | `/api/stats` | Network statistics |

---

## 9. WebSocket Real-Time Updates

### 9.1 Connection

The web UI maintains a persistent WebSocket connection to the local node at
`ws://127.0.0.1:7470/ws`. This connection enables server-pushed updates without
polling.

```
┌──────────────┐         WebSocket          ┌──────────────┐
│    Web UI    │ ◄═══════════════════════► │   WS Hub     │
│  (embedded)  │    Persistent connection   │   (Go)       │
└──────────────┘                            └──────┬───────┘
                                                   │
                              ┌─────────────────────┤
                              │                     │
                              ▼                     ▼
                     ┌──────────────┐      ┌──────────────┐
                     │  GossipSub   │      │   Social     │
                     │  messages    │      │   events     │
                     └──────────────┘      └──────────────┘
```

### 9.2 Event Types

All WebSocket messages are JSON-encoded with a `type` field and a `data` field.

```json
{"type": "<event_type>", "data": <event_data>}
```

| Event Type | Data | Trigger |
|---|---|---|
| `new_post` | Post object | A new post arrives from a followed publisher |
| `new_reaction` | Reaction object | A reaction is received for a visible post |
| `new_notification` | Notification object | A like, reply, repost, follow, or DM arrives |
| `new_dm` | DirectMessage object (encrypted) | A new DM is received |
| `node_status` | `{peers: N, bandwidth: {...}}` | Periodic status update (every 30 seconds) |
| `sync_progress` | `{pubkey: "...", progress: 0.75}` | Historical sync progress for a followed publisher |

### 9.3 Hub Architecture

The WebSocket hub manages multiple concurrent connections (e.g., multiple
browser tabs) and broadcasts events to all connected clients:

```
┌───────────────────────────────────┐
│           WebSocket Hub            │
│                                   │
│  Channels:                        │
│  ├── register   ← new connections │
│  ├── unregister ← disconnections  │
│  └── broadcast  ← events to send  │
│                                   │
│  Clients map:                     │
│  ├── client1 → send channel       │
│  ├── client2 → send channel       │
│  └── client3 → send channel       │
│                                   │
│  Broadcast loop:                  │
│  1. Receive event on broadcast ch │
│  2. For each registered client:   │
│     a. Try send to client's ch    │
│     b. If blocked, disconnect     │
│  3. Repeat                        │
│                                   │
└───────────────────────────────────┘
```

### 9.4 Reconnection

The frontend client handles WebSocket disconnections by:

1. Detecting the close event.
2. Waiting with exponential backoff (1s, 2s, 4s, 8s, ... max 30s).
3. Attempting to reconnect.
4. On reconnection, fetching any missed updates via HTTP API.

---

## Appendix: Configuration Reference

The node is configured via `~/.xleaks/config.toml`. See `configs/default.toml`
for the complete default configuration.

| Section | Key | Default | Description |
|---|---|---|---|
| `[node]` | `data_dir` | `~/.xleaks` | Data storage directory |
| `[node]` | `mode` | `"user"` | Node mode: `"user"` or `"indexer"` |
| `[node]` | `max_storage_gb` | `5` | Max disk space for content (min 1 GB) |
| `[network]` | `listen_addresses` | TCP + QUIC on `:7460` | libp2p listen addresses |
| `[network]` | `enable_relay` | `true` | Use circuit relay for NAT traversal |
| `[network]` | `enable_mdns` | `true` | Enable LAN peer discovery |
| `[network]` | `enable_hole_punching` | `true` | Enable DCUtR NAT traversal |
| `[network]` | `max_peers` | `100` | Maximum concurrent peer connections |
| `[network]` | `bandwidth_limit_mbps` | `0` | Bandwidth limit (0 = unlimited) |
| `[api]` | `listen_address` | `127.0.0.1:7470` | Local API server address |
| `[api]` | `enable_websocket` | `true` | Enable WebSocket endpoint |
| `[indexer]` | `public_api_address` | `0.0.0.0:7471` | Indexer public API address |
| `[indexer]` | `max_indexed_publishers` | `100000` | Max publishers to index |
| `[indexer]` | `trending_windows` | `["1h","6h","24h","7d"]` | Trending time windows |
| `[media]` | `max_upload_size_mb` | `100` | Max upload size per file |
| `[media]` | `auto_fetch_media` | `false` | Pre-fetch media for followed publishers |
| `[media]` | `thumbnail_quality` | `80` | JPEG quality for thumbnails |
| `[identity]` | `passphrase_min_length` | `8` | Minimum passphrase length |
| `[logging]` | `level` | `"info"` | Log level: debug, info, warn, error |
| `[logging]` | `file` | `~/.xleaks/logs/xleaks.log` | Log file path |
| `[logging]` | `max_size_mb` | `50` | Max log file size |
| `[logging]` | `max_backups` | `3` | Number of rotated log files to keep |
