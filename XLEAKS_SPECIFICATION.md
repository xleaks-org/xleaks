# XLeaks — Full Production Specification

**Domain:** xleaks.org
**GitHub:** github.com/xleaks-org/xleaks
**Go Module:** github.com/xleaks-org/xleaks
**License:** AGPL-3.0
**Tagline:** Speak. Store. Stay.
**Version:** 1.0
**Date:** 2026-03-22

---

## 1. Project Overview

XLeaks is a decentralized, peer-to-peer social platform designed for journalists, whistleblowers, and anyone who needs to publish content that cannot be censored, deleted, edited, or taken down by any entity — including the platform creator.

Every user who joins the network becomes a node. There is no central server. Content is cryptographically signed by the author and replicated across subscriber nodes. The network is self-sustaining: the more users, the more resilient the data.

**This is NOT an MVP.** This specification describes a complete, production-ready, enterprise-grade application. Every feature described must be fully implemented, tested, and polished before release.

### 1.1 Core Principles

1. **Immutability** — No post, comment, like, or any piece of content can ever be edited or deleted by anyone, including the original author. This is enforced at the protocol level.
2. **Equality** — Every user is equal. There are no admin roles, no moderators, no special privileges. The project creator has the same capabilities as any other user.
3. **Censorship Resistance** — Content is distributed across all subscriber nodes. Taking down any single node, server, or even the project's website does not affect content availability.
4. **Privacy** — Users authenticate with cryptographic key pairs. No email, phone number, or personal information required. Direct messages are end-to-end encrypted.
5. **Open Protocol** — The wire protocol is open and documented. Anyone can build a compatible client. The official client is the reference implementation.

### 1.2 Analogies

- **Like Twitter:** Post, comment, like, repost, follow, feed, trending, search, DMs, notifications, profiles.
- **Like WikiLeaks:** Content cannot be taken down. Designed for whistleblowers and journalists. Anonymity is first-class.
- **Like BitTorrent:** Every user seeds content they subscribe to. Popular content is replicated widely. The network is unkillable.
- **Like Linux:** The protocol is open. The official app is owned by the project creator (brand, UX, roadmap), but no one controls the network itself.

---

## 2. Technology Stack

| Component | Technology | Version | Purpose |
|---|---|---|---|
| Language | Go | 1.22+ | All backend, node, P2P logic |
| P2P Networking | go-libp2p | latest stable | Peer connections, NAT traversal, transport |
| Pub/Sub | go-libp2p-pubsub (GossipSub) | latest stable | Real-time message propagation |
| DHT | go-libp2p-kad-dht | latest stable | Peer and content discovery |
| Content Exchange | Bitswap (go-bitswap) | latest stable | Fetch content chunks from peers |
| Serialization | Protocol Buffers (protobuf) | proto3 | Wire protocol message encoding |
| Local Database | SQLite (go-sqlite3 via CGo, or modernc.org/sqlite for pure Go) | 3.x | Per-node local persistence |
| Full-Text Search | Bleve | latest stable | Search index on indexer nodes |
| Cryptography — Signing | ed25519 (Go stdlib crypto/ed25519) | stdlib | Identity, message signing, verification |
| Cryptography — Encryption | x25519 + NaCl secretbox (golang.org/x/crypto/nacl/box) | latest | DM end-to-end encryption |
| Content Hashing | SHA-256 based CIDs (multihash) | — | Content addressing |
| Frontend | Next.js (React) | 14+ | Web UI |
| Desktop Wrapper | Wails v2 | latest stable | Native desktop app wrapping Go + Web UI |
| HTTP API | Go net/http + gorilla/mux or chi | latest | Local API between UI and node |
| WebSocket | gorilla/websocket | latest | Real-time UI updates (feed, notifications) |
| Configuration | TOML | — | Node configuration files |
| Testing | Go testing + testify | stdlib + latest | Unit and integration tests |
| Build | Go build + Makefile | — | Single binary output |

### 2.1 Build Targets

The application MUST compile to a single binary for each platform:

- `xleaks-linux-amd64`
- `xleaks-linux-arm64`
- `xleaks-darwin-amd64` (macOS Intel)
- `xleaks-darwin-arm64` (macOS Apple Silicon)
- `xleaks-windows-amd64.exe`

The Next.js frontend MUST be embedded into the Go binary using Go's `embed` package, so no external files or runtime dependencies are required.

---

## 3. Identity System

### 3.1 Key Pair Generation

- Every user identity is an **ed25519 key pair**.
- The **public key** is the user's unique identifier across the network (their "address").
- The **private key** is stored locally on the user's device, encrypted with a user-chosen passphrase using Argon2id key derivation + AES-256-GCM.
- No email, phone number, or personal data is ever required.
- Display format for public keys: `xleaks1` + bech32-encoded public key (similar to Nostr's npub format). Example: `xleaks1qyv3s8x...`

### 3.2 Key Management

- **Key generation:** On first launch, the app generates a new key pair and prompts the user to set a passphrase.
- **Key backup:** The app MUST provide a 24-word BIP39 mnemonic seed phrase that can regenerate the key pair. The user MUST be shown this seed phrase during onboarding and warned that losing it means losing their identity permanently.
- **Key import:** Users can import an existing identity from a seed phrase or a raw private key file.
- **Key export:** Users can export their encrypted private key for backup.
- **Multiple identities:** A user can create and switch between multiple identities from the same app instance.

### 3.3 Authentication Flow

- There is no "login" in the traditional sense.
- The app loads the user's encrypted private key from disk, prompts for the passphrase to decrypt it, and the user is authenticated.
- All actions (posting, liking, following) are signed with the private key. The signature IS the authentication.

---

## 4. Protocol Specification

### 4.1 Message Types

All messages are serialized using Protocol Buffers (proto3). Every message includes the author's public key, a Unix timestamp (milliseconds), and an ed25519 signature over the serialized payload (excluding the signature field itself).

#### 4.1.1 Post

```protobuf
message Post {
  bytes id = 1;                    // CID (content hash of this message)
  bytes author = 2;                // 32-byte ed25519 public key
  uint64 timestamp = 3;            // Unix timestamp in milliseconds
  string content = 4;              // Text content (max 5000 UTF-8 characters)
  repeated bytes media_cids = 5;   // CIDs of attached media objects
  bytes reply_to = 6;              // CID of parent post (empty if top-level)
  bytes repost_of = 7;             // CID of original post (empty if original)
  repeated string tags = 8;        // Hashtags extracted from content
  bytes signature = 9;             // ed25519 signature
}
```

**Validation rules:**
- `content` MUST NOT exceed 5000 UTF-8 characters.
- `content` MUST NOT be empty unless `media_cids` or `repost_of` is non-empty.
- `media_cids` MUST NOT exceed 10 items.
- `reply_to` and `repost_of` are mutually exclusive (a post cannot be both a reply and a repost).
- `id` MUST equal the SHA-256 multihash of the serialized message (with `id` and `signature` fields zeroed).
- `signature` MUST be a valid ed25519 signature by `author` over the serialized message (with `signature` field zeroed).
- `timestamp` MUST be within ±5 minutes of the receiving node's clock (to prevent backdating attacks while allowing clock drift).

#### 4.1.2 Reaction

```protobuf
message Reaction {
  bytes id = 1;                    // CID
  bytes author = 2;                // 32-byte ed25519 public key
  bytes target = 3;                // CID of the post being reacted to
  string reaction_type = 4;        // "like" (extensible for future types)
  uint64 timestamp = 5;
  bytes signature = 6;
}
```

**Validation rules:**
- One reaction per author per target per reaction_type. Duplicate reactions from the same author MUST be ignored.
- `reaction_type` MUST be "like" in v1.0.

#### 4.1.3 Profile

```protobuf
message Profile {
  bytes author = 1;                // 32-byte ed25519 public key
  string display_name = 2;        // Max 50 UTF-8 characters
  string bio = 3;                 // Max 500 UTF-8 characters
  bytes avatar_cid = 4;           // CID of avatar image (optional)
  bytes banner_cid = 5;           // CID of banner image (optional)
  string website = 6;             // URL (optional, max 200 chars)
  uint64 version = 7;             // Monotonically increasing version number
  uint64 timestamp = 8;
  bytes signature = 9;
}
```

**Validation rules:**
- Nodes MUST only accept a profile update if `version` is greater than the currently known version for that `author`.
- `display_name` MUST NOT be empty.
- `avatar_cid` and `banner_cid`, if provided, MUST reference valid media objects.

#### 4.1.4 Follow

```protobuf
message FollowEvent {
  bytes author = 1;                // 32-byte ed25519 public key (follower)
  bytes target = 2;                // 32-byte ed25519 public key (being followed)
  string action = 3;               // "follow" or "unfollow"
  uint64 timestamp = 4;
  bytes signature = 5;
}
```

**Validation rules:**
- `action` MUST be "follow" or "unfollow".
- A user MUST NOT follow themselves.

#### 4.1.5 DirectMessage

```protobuf
message DirectMessage {
  bytes id = 1;                    // CID
  bytes author = 2;                // 32-byte ed25519 public key (sender)
  bytes recipient = 3;             // 32-byte ed25519 public key (recipient)
  bytes encrypted_content = 4;    // NaCl box encrypted payload
  bytes nonce = 5;                // 24-byte NaCl nonce
  uint64 timestamp = 6;
  bytes signature = 7;
}
```

**Encryption:**
- Sender derives a shared secret using X25519 Diffie-Hellman (sender's private key + recipient's public key).
- Content is encrypted using NaCl secretbox (XSalsa20-Poly1305) with a random 24-byte nonce.
- Only sender and recipient can decrypt.

#### 4.1.6 MediaObject

```protobuf
message MediaObject {
  bytes cid = 1;                   // CID (hash of the complete media file)
  bytes author = 2;                // 32-byte ed25519 public key
  string mime_type = 3;            // MIME type (image/jpeg, image/png, video/mp4, etc.)
  uint64 size = 4;                 // Total size in bytes
  uint32 chunk_count = 5;          // Number of 256KB chunks
  repeated bytes chunk_cids = 6;   // Ordered list of chunk CIDs
  uint32 width = 7;               // Width in pixels (images/video)
  uint32 height = 8;              // Height in pixels (images/video)
  uint32 duration = 9;            // Duration in seconds (video/audio, 0 for images)
  bytes thumbnail_cid = 10;       // CID of thumbnail (auto-generated, max 100KB JPEG)
  uint64 timestamp = 11;
  bytes signature = 12;
}
```

#### 4.1.7 MediaChunk

```protobuf
message MediaChunk {
  bytes cid = 1;                   // CID (hash of this chunk's data)
  bytes parent_cid = 2;            // CID of the parent MediaObject
  uint32 index = 3;               // Chunk sequence number (0-indexed)
  bytes data = 4;                 // Raw bytes (max 262144 bytes = 256KB)
}
```

**Media constraints:**
- Maximum file size: 100MB per media object.
- Supported image formats: JPEG, PNG, WebP, GIF.
- Supported video formats: MP4 (H.264), WebM (VP9).
- Supported audio formats: MP3, OGG, WAV.
- Chunk size: 256KB (262,144 bytes). Last chunk may be smaller.
- A thumbnail MUST be auto-generated for all images and videos (max 100KB, JPEG, 320px wide).

### 4.2 GossipSub Topics

Content propagates through GossipSub topics. Topic naming convention:

- `/xleaks/posts/<author-pubkey-hex>` — All posts (including replies and reposts) by a specific author.
- `/xleaks/reactions/<post-cid-hex>` — All reactions to a specific post.
- `/xleaks/profiles` — All profile updates (global topic, low volume).
- `/xleaks/follows/<author-pubkey-hex>` — Follow/unfollow events by a specific user.
- `/xleaks/dm/<recipient-pubkey-hex>` — Direct messages for a specific recipient.
- `/xleaks/global` — Global announcement channel (used by indexer nodes for trending, network announcements).

**Subscription behavior:**
- When a user follows a publisher, their node subscribes to `/xleaks/posts/<publisher-pubkey-hex>`.
- When a user opens a post's detail view, their node subscribes to `/xleaks/reactions/<post-cid-hex>` to receive live reaction updates.
- Every node subscribes to `/xleaks/dm/<own-pubkey-hex>` to receive direct messages.
- Every node subscribes to `/xleaks/global` for network-wide announcements.

### 4.3 Content-Addressed Storage (CAS)

Every node maintains a local content-addressed store on disk.

**Directory structure:**
```
~/.xleaks/
├── config.toml                    # Node configuration
├── identity/
│   ├── primary.key               # Encrypted primary key
│   └── identities/               # Additional identities
├── data/
│   ├── objects/                   # Content-addressed objects (CID → serialized protobuf)
│   │   ├── ab/                   # First 2 hex chars of CID (sharding)
│   │   │   └── ab3f...full-cid   # Object file
│   │   └── ...
│   ├── media/                    # Media chunks (same sharding scheme)
│   │   ├── cd/
│   │   │   └── cd8a...full-cid   # Chunk file
│   │   └── ...
│   └── index.db                  # SQLite database (indexes, feed, metadata)
├── logs/
│   └── xleaks.log
└── cache/
    └── thumbnails/               # Generated thumbnail cache
```

### 4.4 Peer Discovery

1. **Bootstrap nodes:** On first launch, the node connects to a hardcoded list of bootstrap peers (operated by the XLeaks project and community volunteers). These bootstrap nodes are normal nodes — they have no special authority.
2. **Kademlia DHT:** After connecting to bootstrap nodes, the node participates in the Kademlia distributed hash table for ongoing peer discovery.
3. **mDNS:** For local network discovery (find other XLeaks nodes on the same LAN without internet).
4. **Relay circuits:** For nodes behind strict NATs/firewalls that cannot accept incoming connections, libp2p relay circuits route traffic through a relay node.
5. **Hole punching:** libp2p's DCUtR protocol attempts direct connections between NAT-ed peers.

### 4.5 Content Replication Strategy

When a user follows a publisher:

1. Node subscribes to the publisher's GossipSub topic.
2. Node queries the DHT for peers that have content from this publisher.
3. Node fetches the publisher's historical posts via Bitswap (content IDs discovered via the DHT provider records).
4. Going forward, new posts arrive in real-time via GossipSub.
5. All fetched content is stored in the local CAS and indexed in SQLite.

**Replication guarantees:**
- Every subscriber node stores a full copy of every post from publishers they follow.
- Media chunks are fetched on-demand by default (lazy loading), but can be pre-fetched based on user preference.
- Users can configure how much disk space to dedicate to the network (default: 5GB, minimum: 1GB).
- When disk space is full, the node evicts content using LRU (least recently used) — BUT content from followed publishers is NEVER evicted.

---

## 5. Node Architecture

### 5.1 Module Breakdown

```
xleaks/
├── cmd/
│   └── xleaks/
│       └── main.go                     # Entry point — starts node + API + UI
│
├── proto/
│   ├── messages.proto                  # All protobuf message definitions
│   └── gen/                            # Generated Go code (protoc output)
│
├── pkg/
│   ├── identity/
│   │   ├── keypair.go                  # Ed25519 key generation
│   │   ├── keystore.go                 # Encrypted key storage (Argon2id + AES-256-GCM)
│   │   ├── mnemonic.go                 # BIP39 seed phrase generation/recovery
│   │   ├── signing.go                  # Sign and verify messages
│   │   ├── encryption.go              # X25519 DH + NaCl box for DMs
│   │   └── address.go                 # xleaks1... bech32 address encoding
│   │
│   ├── content/
│   │   ├── store.go                    # Content-addressed storage (disk operations)
│   │   ├── cid.go                      # CID generation (SHA-256 multihash)
│   │   ├── chunker.go                  # Split media into 256KB chunks
│   │   ├── assembler.go               # Reassemble chunks into media files
│   │   ├── validator.go               # Validate message signatures, CIDs, constraints
│   │   └── thumbnail.go              # Auto-generate thumbnails for images/video
│   │
│   ├── p2p/
│   │   ├── host.go                     # libp2p host initialization and lifecycle
│   │   ├── config.go                   # P2P configuration (listen addresses, limits)
│   │   ├── discovery.go               # DHT bootstrap, mDNS, peer discovery
│   │   ├── pubsub.go                  # GossipSub setup, topic management, message handlers
│   │   ├── bitswap.go                 # Content exchange protocol (fetch/serve chunks)
│   │   ├── relay.go                   # Circuit relay client configuration
│   │   ├── nat.go                     # NAT traversal and hole punching
│   │   ├── bandwidth.go              # Bandwidth tracking and rate limiting
│   │   └── metrics.go                # P2P network metrics (connected peers, bandwidth)
│   │
│   ├── feed/
│   │   ├── manager.go                  # Subscription management (follow/unfollow logic)
│   │   ├── assembler.go               # Build feed from local database
│   │   ├── replicator.go             # Pin/fetch content from followed publishers
│   │   ├── sync.go                    # Historical content sync for new follows
│   │   └── timeline.go               # Chronological timeline assembly with pagination
│   │
│   ├── social/
│   │   ├── posts.go                    # Post creation, validation, publishing
│   │   ├── reactions.go               # Like handling, deduplication, count aggregation
│   │   ├── reposts.go                 # Repost creation and display logic
│   │   ├── threads.go                 # Comment thread tree assembly
│   │   ├── profile.go                 # Profile creation, update, versioning
│   │   ├── dm.go                      # Encrypted DM send/receive/decrypt
│   │   └── notifications.go          # Notification generation (likes, replies, reposts, follows, DMs)
│   │
│   ├── indexer/
│   │   ├── indexer.go                  # Indexer node mode (subscribe broadly)
│   │   ├── search.go                  # Full-text search index (Bleve)
│   │   ├── trending.go               # Trending content algorithm
│   │   ├── stats.go                   # Network-wide statistics aggregation
│   │   └── api.go                     # Public HTTP API for search/trending/discovery
│   │
│   ├── storage/
│   │   ├── db.go                       # SQLite connection pool, migrations, lifecycle
│   │   ├── schema.go                  # Database schema (auto-migration)
│   │   ├── posts_repo.go             # Post CRUD, feed queries
│   │   ├── reactions_repo.go         # Reaction storage, count queries
│   │   ├── profiles_repo.go          # Profile storage
│   │   ├── subscriptions_repo.go     # Follow list storage
│   │   ├── notifications_repo.go     # Notification queue
│   │   ├── dm_repo.go                # DM conversation storage
│   │   └── media_repo.go             # Media object metadata
│   │
│   └── api/
│       ├── server.go                   # HTTP + WebSocket server setup
│       ├── router.go                  # Route definitions
│       ├── ws.go                      # WebSocket hub for real-time UI updates
│       ├── middleware/
│       │   ├── auth.go                # Verify requests are from local UI (localhost only)
│       │   ├── cors.go                # CORS for development
│       │   └── ratelimit.go          # Rate limiting
│       └── handlers/
│           ├── posts.go               # POST /api/posts, GET /api/posts/:id
│           ├── feed.go                # GET /api/feed (paginated)
│           ├── profile.go             # GET/PUT /api/profile, GET /api/users/:pubkey
│           ├── reactions.go           # POST /api/reactions
│           ├── follow.go              # POST /api/follow, DELETE /api/follow
│           ├── search.go              # GET /api/search?q=
│           ├── trending.go            # GET /api/trending
│           ├── notifications.go       # GET /api/notifications
│           ├── dm.go                  # GET/POST /api/dm/:pubkey
│           ├── media.go               # POST /api/media (upload), GET /api/media/:cid
│           ├── node.go                # GET /api/node/status (peers, bandwidth, storage)
│           └── identity.go            # POST /api/identity/create, /import, /export
│
├── web/                                # Next.js frontend (embedded via go:embed)
│   ├── src/
│   │   ├── app/                        # Next.js App Router pages
│   │   │   ├── page.tsx               # Home feed
│   │   │   ├── post/[id]/page.tsx     # Post detail + thread
│   │   │   ├── user/[pubkey]/page.tsx # User profile
│   │   │   ├── search/page.tsx        # Search results
│   │   │   ├── trending/page.tsx      # Trending content
│   │   │   ├── messages/page.tsx      # DM inbox
│   │   │   ├── messages/[pubkey]/page.tsx # DM conversation
│   │   │   ├── notifications/page.tsx # Notification center
│   │   │   ├── settings/page.tsx      # Node settings, identity management
│   │   │   ├── onboarding/page.tsx    # First-run key generation + seed phrase
│   │   │   └── layout.tsx             # Root layout with sidebar navigation
│   │   ├── components/
│   │   │   ├── PostCard.tsx           # Single post display (content, media, actions)
│   │   │   ├── PostComposer.tsx       # New post input (text + media upload)
│   │   │   ├── ThreadView.tsx         # Nested comment thread
│   │   │   ├── UserCard.tsx           # User profile summary (avatar, name, bio)
│   │   │   ├── Feed.tsx               # Scrollable feed with infinite pagination
│   │   │   ├── MediaViewer.tsx        # Image gallery + video player
│   │   │   ├── MediaUploader.tsx      # Drag-and-drop media upload
│   │   │   ├── NotificationItem.tsx   # Single notification display
│   │   │   ├── DMConversation.tsx     # Chat-style DM view
│   │   │   ├── SearchBar.tsx          # Global search input
│   │   │   ├── TrendingList.tsx       # Trending topics/posts sidebar
│   │   │   ├── Sidebar.tsx            # Main navigation sidebar
│   │   │   ├── NodeStatus.tsx         # P2P node status indicator (peers, sync)
│   │   │   ├── SeedPhraseBackup.tsx   # Seed phrase display and confirmation
│   │   │   └── PassphrasePrompt.tsx   # Unlock identity dialog
│   │   ├── hooks/
│   │   │   ├── useWebSocket.ts        # WebSocket connection to local node
│   │   │   ├── useFeed.ts             # Feed data + infinite scroll
│   │   │   ├── useProfile.ts          # User profile data
│   │   │   ├── useNotifications.ts    # Live notification updates
│   │   │   └── useNodeStatus.ts       # Node health/peer count
│   │   ├── lib/
│   │   │   ├── api.ts                 # HTTP client for local node API
│   │   │   ├── ws.ts                  # WebSocket client
│   │   │   ├── formatters.ts          # Date, pubkey, count formatting
│   │   │   └── types.ts              # TypeScript type definitions matching protobuf
│   │   └── styles/
│   │       └── globals.css            # Tailwind CSS + custom theme (dark mode default)
│   ├── public/
│   │   └── icons/                     # App icons
│   ├── next.config.js
│   ├── tailwind.config.js
│   ├── tsconfig.json
│   └── package.json
│
├── configs/
│   ├── default.toml                   # Default node configuration
│   └── bootstrap_peers.toml           # Hardcoded bootstrap peer list
│
├── scripts/
│   ├── build.sh                       # Build all platforms
│   ├── gen-proto.sh                   # Regenerate protobuf Go code
│   └── dev.sh                         # Development mode (hot reload frontend + Go node)
│
├── tests/
│   ├── unit/                          # Unit tests (per package)
│   ├── integration/                   # Multi-node integration tests
│   │   ├── network_test.go            # Spin up N nodes, verify message propagation
│   │   ├── replication_test.go        # Verify content replication on follow
│   │   ├── dm_test.go                 # Verify E2E encrypted DM delivery
│   │   └── media_test.go             # Verify chunked media upload/download
│   └── protocol/                      # Protocol conformance tests
│       └── validation_test.go         # Verify all message validation rules
│
├── docs/
│   ├── PROTOCOL.md                    # Full wire protocol specification
│   ├── ARCHITECTURE.md                # System architecture documentation
│   └── CONTRIBUTING.md                # How to contribute
│
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 5.2 Node Lifecycle

1. **Startup:**
   - Load configuration from `~/.xleaks/config.toml`.
   - Initialize SQLite database, run migrations if needed.
   - If no identity exists, redirect UI to onboarding flow.
   - If identity exists, prompt for passphrase to unlock.
   - Initialize content-addressed store.
   - Initialize libp2p host (listen on configured addresses, start DHT, connect to bootstrap peers).
   - Subscribe to GossipSub topics for all followed publishers + own DM topic + global topic.
   - Start content replication engine (sync any missed content from followed publishers).
   - Start local HTTP + WebSocket API server.
   - Open UI (web browser or desktop app window).

2. **Running:**
   - Accept and validate incoming GossipSub messages, store valid ones.
   - Serve content via Bitswap to requesting peers.
   - Respond to local API requests from the UI.
   - Push real-time updates to UI via WebSocket (new posts, notifications, DMs).
   - Periodically refresh DHT presence and peer connections.
   - Monitor disk usage, evict LRU non-subscribed content if over limit.

3. **Shutdown:**
   - Gracefully close all peer connections.
   - Flush any pending database writes.
   - Close SQLite database.
   - Exit.

---

## 6. SQLite Database Schema

```sql
-- User's own identities
CREATE TABLE identities (
    pubkey BLOB PRIMARY KEY,          -- 32-byte ed25519 public key
    display_name TEXT NOT NULL,
    is_active INTEGER DEFAULT 0,      -- 1 if this is the currently active identity
    created_at INTEGER NOT NULL        -- Unix timestamp ms
);

-- Known profiles (other users)
CREATE TABLE profiles (
    pubkey BLOB PRIMARY KEY,
    display_name TEXT NOT NULL,
    bio TEXT DEFAULT '',
    avatar_cid BLOB,
    banner_cid BLOB,
    website TEXT DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    updated_at INTEGER NOT NULL
);

-- Posts (own + from followed publishers + discovered)
CREATE TABLE posts (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    content TEXT,
    reply_to BLOB,                    -- NULL for top-level posts
    repost_of BLOB,                   -- NULL for original posts
    timestamp INTEGER NOT NULL,
    signature BLOB NOT NULL,
    received_at INTEGER NOT NULL,     -- When this node received the post
    FOREIGN KEY (author) REFERENCES profiles(pubkey)
);

CREATE INDEX idx_posts_author ON posts(author, timestamp DESC);
CREATE INDEX idx_posts_timestamp ON posts(timestamp DESC);
CREATE INDEX idx_posts_reply_to ON posts(reply_to) WHERE reply_to IS NOT NULL;
CREATE INDEX idx_posts_repost_of ON posts(repost_of) WHERE repost_of IS NOT NULL;

-- Media references in posts
CREATE TABLE post_media (
    post_cid BLOB NOT NULL,
    media_cid BLOB NOT NULL,
    position INTEGER NOT NULL,        -- Order of media in post (0-indexed)
    PRIMARY KEY (post_cid, media_cid)
);

-- Media object metadata
CREATE TABLE media_objects (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    mime_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    chunk_count INTEGER NOT NULL,
    width INTEGER DEFAULT 0,
    height INTEGER DEFAULT 0,
    duration INTEGER DEFAULT 0,
    thumbnail_cid BLOB,
    timestamp INTEGER NOT NULL,
    fully_fetched INTEGER DEFAULT 0   -- 1 if all chunks are stored locally
);

-- Reactions (likes)
CREATE TABLE reactions (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    target BLOB NOT NULL,
    reaction_type TEXT NOT NULL DEFAULT 'like',
    timestamp INTEGER NOT NULL,
    UNIQUE(author, target, reaction_type)
);

CREATE INDEX idx_reactions_target ON reactions(target);

-- Aggregated reaction counts (materialized for performance)
CREATE TABLE reaction_counts (
    post_cid BLOB PRIMARY KEY,
    like_count INTEGER DEFAULT 0,
    reply_count INTEGER DEFAULT 0,
    repost_count INTEGER DEFAULT 0
);

-- Subscriptions (who this user follows)
CREATE TABLE subscriptions (
    pubkey BLOB PRIMARY KEY,          -- The publisher being followed
    followed_at INTEGER NOT NULL,
    sync_completed INTEGER DEFAULT 0  -- 1 if historical sync is done
);

-- Follow events (seen on network, for follower count display)
CREATE TABLE follow_events (
    author BLOB NOT NULL,
    target BLOB NOT NULL,
    action TEXT NOT NULL,             -- 'follow' or 'unfollow'
    timestamp INTEGER NOT NULL,
    PRIMARY KEY (author, target)
);

CREATE INDEX idx_follow_events_target ON follow_events(target);

-- Follower counts (materialized)
CREATE TABLE follower_counts (
    pubkey BLOB PRIMARY KEY,
    follower_count INTEGER DEFAULT 0,
    following_count INTEGER DEFAULT 0
);

-- Direct messages
CREATE TABLE direct_messages (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    recipient BLOB NOT NULL,
    encrypted_content BLOB NOT NULL,
    nonce BLOB NOT NULL,
    timestamp INTEGER NOT NULL,
    read INTEGER DEFAULT 0
);

CREATE INDEX idx_dm_conversation ON direct_messages(
    MIN(author, recipient), MAX(author, recipient), timestamp DESC
);

-- Notifications
CREATE TABLE notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,                -- 'like', 'reply', 'repost', 'follow', 'dm'
    actor BLOB NOT NULL,              -- Who triggered the notification
    target_cid BLOB,                  -- The post being liked/replied to (NULL for follow/dm)
    related_cid BLOB,                 -- The reaction/reply/repost CID
    timestamp INTEGER NOT NULL,
    read INTEGER DEFAULT 0
);

CREATE INDEX idx_notifications_unread ON notifications(read, timestamp DESC);

-- Hashtags for local search/trending
CREATE TABLE post_tags (
    post_cid BLOB NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (post_cid, tag)
);

CREATE INDEX idx_post_tags_tag ON post_tags(tag);

-- Peer reputation (track reliable peers for content fetching)
CREATE TABLE peer_stats (
    peer_id TEXT PRIMARY KEY,
    successful_fetches INTEGER DEFAULT 0,
    failed_fetches INTEGER DEFAULT 0,
    bytes_received INTEGER DEFAULT 0,
    bytes_sent INTEGER DEFAULT 0,
    last_seen INTEGER NOT NULL
);

-- Content eviction tracking
CREATE TABLE content_access (
    cid BLOB PRIMARY KEY,
    last_accessed INTEGER NOT NULL,
    access_count INTEGER DEFAULT 1,
    is_pinned INTEGER DEFAULT 0       -- 1 if from a followed publisher (never evict)
);
```

---

## 7. API Endpoints

All endpoints are served on `localhost:7470` (default port) and are only accessible from the local machine.

### 7.1 Identity

| Method | Path | Description |
|---|---|---|
| POST | /api/identity/create | Generate new key pair, returns pubkey + seed phrase |
| POST | /api/identity/import | Import identity from seed phrase or key file |
| POST | /api/identity/unlock | Decrypt active identity with passphrase |
| GET | /api/identity/active | Get active identity pubkey and profile |
| POST | /api/identity/lock | Lock (re-encrypt) identity |
| GET | /api/identity/list | List all local identities |
| PUT | /api/identity/switch/:pubkey | Switch active identity |
| GET | /api/identity/export | Export encrypted key file |

### 7.2 Posts

| Method | Path | Description |
|---|---|---|
| POST | /api/posts | Create new post (text + optional media CIDs) |
| GET | /api/posts/:cid | Get single post by CID |
| GET | /api/posts/:cid/thread | Get full thread (parent chain + replies) |
| GET | /api/posts/:cid/reactions | Get reactions for a post |
| GET | /api/users/:pubkey/posts | Get all posts by a user (paginated) |

### 7.3 Feed

| Method | Path | Description |
|---|---|---|
| GET | /api/feed | Get chronological feed from followed publishers (paginated) |
| GET | /api/feed?before=TIMESTAMP | Pagination cursor |

### 7.4 Social Actions

| Method | Path | Description |
|---|---|---|
| POST | /api/reactions | Create a reaction (like) on a post |
| POST | /api/repost | Repost a post |
| POST | /api/follow/:pubkey | Follow a user |
| DELETE | /api/follow/:pubkey | Unfollow a user |
| GET | /api/following | List followed users |
| GET | /api/users/:pubkey/followers | List followers of a user (from local knowledge) |

### 7.5 Profiles

| Method | Path | Description |
|---|---|---|
| GET | /api/profile | Get own profile |
| PUT | /api/profile | Update own profile |
| GET | /api/users/:pubkey | Get another user's profile |

### 7.6 Search & Discovery

| Method | Path | Description |
|---|---|---|
| GET | /api/search?q=QUERY&type=posts|users | Search posts or users |
| GET | /api/trending | Get trending posts/tags |
| GET | /api/explore | Suggested users to follow |

### 7.7 Direct Messages

| Method | Path | Description |
|---|---|---|
| GET | /api/dm | List DM conversations |
| GET | /api/dm/:pubkey | Get messages with a specific user (paginated) |
| POST | /api/dm/:pubkey | Send encrypted DM to a user |

### 7.8 Notifications

| Method | Path | Description |
|---|---|---|
| GET | /api/notifications | Get notifications (paginated) |
| PUT | /api/notifications/read | Mark all as read |
| PUT | /api/notifications/:id/read | Mark one as read |
| GET | /api/notifications/unread-count | Get unread count |

### 7.9 Media

| Method | Path | Description |
|---|---|---|
| POST | /api/media | Upload media file (returns MediaObject with CID) |
| GET | /api/media/:cid | Get media file (assembled from chunks) |
| GET | /api/media/:cid/thumbnail | Get media thumbnail |
| GET | /api/media/:cid/status | Get fetch status (% of chunks downloaded) |

### 7.10 Node Status

| Method | Path | Description |
|---|---|---|
| GET | /api/node/status | Node health (peers, bandwidth, storage, uptime) |
| GET | /api/node/peers | List connected peers |
| GET | /api/node/config | Get node configuration |
| PUT | /api/node/config | Update node configuration |

### 7.11 WebSocket

| Path | Description |
|---|---|
| /ws | WebSocket connection for real-time updates |

**WebSocket event types:**
```json
{"type": "new_post", "data": {Post object}}
{"type": "new_reaction", "data": {Reaction object}}
{"type": "new_notification", "data": {Notification object}}
{"type": "new_dm", "data": {DirectMessage object}}
{"type": "node_status", "data": {peers: N, bandwidth: {...}}}
{"type": "sync_progress", "data": {pubkey: "...", progress: 0.75}}
```

---

## 8. Frontend Specification

### 8.1 Design System

- **Color scheme:** Dark mode by default (dark gray backgrounds, white text), with light mode toggle.
- **Font:** Inter (system font stack fallback).
- **CSS:** Tailwind CSS.
- **Layout:** Twitter-like three-column layout on desktop:
  - Left sidebar: navigation (Home, Search, Trending, Notifications, Messages, Profile, Settings).
  - Center: main content feed.
  - Right sidebar: trending topics, suggested follows, node status widget.
- **Mobile:** Single column with bottom tab navigation.
- **Responsive breakpoints:** Mobile (<768px), Tablet (768-1024px), Desktop (>1024px).

### 8.2 Pages

#### 8.2.1 Onboarding (First Run)
1. Welcome screen explaining XLeaks.
2. "Create Identity" button → generates key pair → displays seed phrase.
3. Seed phrase confirmation step (user must re-enter select words).
4. Set passphrase for local encryption.
5. Set display name (optional bio and avatar).
6. Suggest initial publishers to follow (if indexer nodes are available).
7. Done → redirect to home feed.

#### 8.2.2 Home Feed
- Chronological feed of posts from followed publishers.
- Post composer at top (text input + media upload button).
- Each post card shows: author avatar + name + pubkey snippet + relative timestamp, post content, media (images/video inline), action bar (reply count, repost count, like count, share).
- Infinite scroll pagination.
- Pull-to-refresh on mobile.
- "New posts available" indicator when new content arrives while scrolled down.

#### 8.2.3 Post Detail / Thread
- Full post with all media displayed.
- Parent post chain shown above (if it's a reply).
- Replies shown below in threaded view (nested, collapsible).
- Real-time reaction count updates via WebSocket.

#### 8.2.4 User Profile
- Banner image, avatar, display name, bio, website link.
- Public key displayed (truncated with copy button).
- Follower count, following count, post count (approximate).
- Follow/unfollow button.
- Tabs: Posts, Replies, Media, Likes.
- Timeline of user's content (paginated).

#### 8.2.5 Search
- Search bar at top.
- Tabs: Posts, Users.
- Results from local index + indexer nodes.
- Hashtag search support (#tag).

#### 8.2.6 Trending
- Trending hashtags with post counts.
- Trending posts (most reposted/liked in recent time window).
- Data sourced from indexer nodes.

#### 8.2.7 Notifications
- Chronological list of notifications.
- Types: someone liked your post, replied to your post, reposted your post, followed you, sent you a DM.
- Unread indicator badge on sidebar.
- Mark all as read button.

#### 8.2.8 Direct Messages
- Conversation list (sorted by most recent message).
- Conversation view: chat-style bubbles, newest at bottom.
- Encrypt/send message input.
- Unread indicator badge.
- "Messages are end-to-end encrypted" notice.

#### 8.2.9 Settings
- **Identity:** View public key, export key, backup seed phrase, create/import/switch identities.
- **Node:** Storage usage bar, max storage setting, connected peers count, bandwidth stats.
- **Display:** Dark/light mode toggle, font size.
- **Network:** Bootstrap peers list, relay configuration.
- **About:** Version info, protocol version, open source links.

### 8.3 Real-time Behavior

The frontend maintains a persistent WebSocket connection to the local node. All of the following update in real-time without page refresh:

- New posts appearing in feed.
- Reaction counts updating on visible posts.
- New notifications (with browser notification permission).
- New DMs arriving.
- Node status (peer count, sync progress).

---

## 9. Indexer Node Mode

Any node can run in indexer mode by setting `mode = "indexer"` in `config.toml`. Indexer nodes:

1. Subscribe to a large number of publishers (can subscribe to the global network via DHT crawling).
2. Build a full-text search index using Bleve.
3. Compute trending content (most liked/reposted in rolling time windows: 1h, 6h, 24h, 7d).
4. Expose a public HTTP API (configurable port, default 7471) that regular nodes can query for search and discovery.
5. Publish periodic "trending" digests to the `/xleaks/global` GossipSub topic.

**Indexer API endpoints:**

| Method | Path | Description |
|---|---|---|
| GET | /api/search?q=QUERY&type=posts|users&page=N | Full-text search |
| GET | /api/trending?window=1h|6h|24h|7d | Trending posts and tags |
| GET | /api/explore/publishers | Suggested publishers (most followed) |
| GET | /api/stats | Network statistics (total posts, users, etc.) |

Regular nodes discover indexer nodes via the DHT (indexer nodes advertise themselves under a well-known DHT key `/xleaks/indexers`).

---

## 10. Configuration

Default `config.toml`:

```toml
[node]
# Unique node ID is derived from the libp2p host key (auto-generated)
data_dir = "~/.xleaks"
mode = "user"                       # "user" or "indexer"
max_storage_gb = 5                  # Max disk space for content store

[network]
listen_addresses = ["/ip4/0.0.0.0/tcp/7460", "/ip4/0.0.0.0/udp/7460/quic-v1"]
enable_relay = true                 # Use relay circuits if direct connection fails
enable_mdns = true                  # Local network discovery
enable_hole_punching = true         # NAT traversal
max_peers = 100                     # Maximum concurrent peer connections
bandwidth_limit_mbps = 0            # 0 = unlimited

[api]
listen_address = "127.0.0.1:7470"  # Local API (localhost only!)
enable_websocket = true

[indexer]
# Only used when mode = "indexer"
public_api_address = "0.0.0.0:7471"
max_indexed_publishers = 100000
trending_windows = ["1h", "6h", "24h", "7d"]

[media]
max_upload_size_mb = 100
auto_fetch_media = false            # If true, pre-fetch all media. If false, fetch on demand.
thumbnail_quality = 80              # JPEG quality for auto-generated thumbnails

[identity]
passphrase_min_length = 8

[logging]
level = "info"                      # debug, info, warn, error
file = "~/.xleaks/logs/xleaks.log"
max_size_mb = 50
max_backups = 3
```

---

## 11. Security Requirements

1. **All peer connections** MUST be encrypted using libp2p Noise or TLS 1.3.
2. **All messages** MUST have valid ed25519 signatures. Messages with invalid signatures MUST be dropped silently.
3. **Private keys** MUST never leave the user's device in plaintext. They are always encrypted at rest with Argon2id + AES-256-GCM.
4. **The local API** MUST only listen on 127.0.0.1 (localhost). It MUST NOT be accessible from the network.
5. **Direct messages** MUST be end-to-end encrypted. The node MUST NOT store plaintext DM content from other users.
6. **Rate limiting** — nodes MUST implement rate limiting on incoming GossipSub messages to prevent spam floods (max 10 posts/minute per author, max 100 reactions/minute per author).
7. **Content validation** — all incoming content MUST be validated against the rules in Section 4.1 before storage.
8. **Replay protection** — duplicate messages (same CID) MUST be ignored.
9. **Clock drift tolerance** — messages with timestamps more than 5 minutes in the future or more than 30 days in the past MUST be rejected (except during historical sync, where the 30-day limit does not apply).

---

## 12. Performance Requirements

1. **Feed loading** — home feed MUST render first page (20 posts) within 200ms of API request.
2. **Post creation** — creating a text post MUST complete (signed + stored + published to GossipSub) within 500ms.
3. **Peer connections** — node MUST establish connections to at least 5 peers within 30 seconds of startup.
4. **Media upload** — chunking and local storage of a 10MB image MUST complete within 2 seconds.
5. **Search** — local search MUST return results within 500ms. Indexer search within 2 seconds.
6. **Memory usage** — the node process MUST use less than 256MB RAM during normal operation (excluding media cache).
7. **Binary size** — the final binary (with embedded frontend) MUST be under 50MB.
8. **Database** — SQLite MUST use WAL mode for concurrent read/write performance.

---

## 13. Testing Requirements

### 13.1 Unit Tests

Every package MUST have comprehensive unit tests. Minimum coverage: 80%.

Key areas:
- Identity: key generation, signing, verification, encryption/decryption, mnemonic encode/decode.
- Content: CID generation, chunking, assembly, all validation rules.
- Storage: all database operations, migration testing.
- Protocol: serialization/deserialization roundtrip for all message types.

### 13.2 Integration Tests

Multi-node tests that spin up N nodes in-process and verify:

- **Message propagation:** Node A posts → Node B (subscribed) receives within 5 seconds.
- **Content replication:** Node B follows Node A → Node B fetches all historical content.
- **Media transfer:** Node A uploads image → Node B fetches all chunks and reassembles correctly.
- **DM delivery:** Node A sends encrypted DM to Node B → Node B decrypts successfully → Node C cannot decrypt.
- **Reaction propagation:** Node A likes Node B's post → Node B receives notification.
- **Profile updates:** Node A updates profile → all connected nodes see the update.
- **Peer discovery:** Node A and Node C both know Node B → Node A discovers Node C through DHT.

### 13.3 Protocol Conformance Tests

Verify that:
- Invalid signatures are rejected.
- Oversized content is rejected.
- Duplicate reactions are deduplicated.
- Future-dated messages are rejected.
- Profile updates with stale version numbers are rejected.

---

## 14. Build and Distribution

### 14.1 Makefile Targets

```makefile
build:          # Build for current platform
build-all:      # Build for all platforms (linux/mac/windows, amd64/arm64)
test:           # Run all tests
test-unit:      # Run unit tests only
test-integration: # Run integration tests (spawns multiple nodes)
lint:           # Run golangci-lint
proto:          # Regenerate protobuf code
frontend:       # Build Next.js frontend
dev:            # Development mode (hot reload)
clean:          # Clean build artifacts
release:        # Build all platforms + create checksums + create release archives
```

### 14.2 Release Artifacts

For each platform:
- `xleaks-{os}-{arch}(.exe)` — single binary
- `xleaks-{os}-{arch}.sha256` — checksum file
- `xleaks-{os}-{arch}.tar.gz` / `.zip` — compressed archive

### 14.3 Development Workflow

```bash
# First time setup
make proto           # Generate protobuf code
cd web && npm install  # Install frontend dependencies

# Development
make dev             # Starts Go node + Next.js dev server with hot reload

# Testing
make test            # Full test suite

# Production build
make build-all       # Cross-compile for all platforms
```

---

## 15. Future Considerations (Post-1.0)

These are NOT part of the 1.0 specification but should be considered in architectural decisions:

1. **Tor integration** — optional routing of all P2P traffic through Tor for enhanced anonymity.
2. **Anonymous posting credits** — prepaid codes that allow posting without an identity trail.
3. **Verified journalist badges** — a verification program (out-of-band) that adds a trust signal to profiles.
4. **Mobile native apps** — iOS and Android apps using the Go node as a library (gomobile).
5. **Browser extension** — lightweight client that connects to a remote node (for users who can't run a full node).
6. **Plugin system** — allow third-party extensions to the client.
7. **Content warnings** — optional, user-applied content warnings (not moderation — the author tags their own content).
8. **Bookmarks** — local-only saved posts.
9. **Lists** — curated lists of publishers.
10. **Polls** — poll message type with cryptographic vote privacy.

---

## 16. Summary of Key Constraints

- **Language:** Go 1.22+
- **Frontend:** Next.js 14+ with Tailwind CSS, embedded in Go binary
- **Database:** SQLite (WAL mode, pure Go or CGo)
- **P2P:** go-libp2p (GossipSub, Kademlia DHT, Bitswap, Noise, Relay)
- **Cryptography:** ed25519 (signing), X25519 + NaCl (DMs), Argon2id + AES-256-GCM (key storage), BIP39 (mnemonics)
- **Serialization:** Protocol Buffers (proto3)
- **Single binary:** All platforms, frontend embedded via go:embed
- **No central server:** Every feature must work in a fully peer-to-peer manner
- **No deletion:** Immutability is a protocol-level guarantee, not a policy
- **No admin roles:** All users are equal, including the project creator
- **Production quality:** Full test coverage, comprehensive error handling, structured logging, graceful shutdown, migration support
