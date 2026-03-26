# XLeaks Wire Protocol Specification

**Protocol Version:** 1.0
**Date:** 2026-03-22
**Serialization:** Protocol Buffers (proto3)
**Signing Algorithm:** Ed25519
**Content Addressing:** SHA-256 Multihash (CID)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Message Envelope](#2-message-envelope)
3. [Message Types](#3-message-types)
4. [Signing and CID Computation](#4-signing-and-cid-computation)
5. [GossipSub Topics](#5-gossipsub-topics)
6. [Content-Addressed Storage Format](#6-content-addressed-storage-format)
7. [Media Chunking Protocol](#7-media-chunking-protocol)
8. [DM Encryption Protocol](#8-dm-encryption-protocol)
9. [Timestamp Validation Rules](#9-timestamp-validation-rules)
10. [Rate Limiting Rules](#10-rate-limiting-rules)
11. [Replay Protection](#11-replay-protection)
12. [Clock Drift Tolerance](#12-clock-drift-tolerance)

---

## 1. Overview

XLeaks uses a content-addressed, signature-authenticated messaging protocol
transported over libp2p GossipSub. Every message on the network is:

- **Serialized** as a Protocol Buffer (proto3) byte sequence.
- **Content-addressed** via a CID derived from the SHA-256 hash of the
  serialized payload (with `id` and `signature` fields zeroed).
- **Signed** with the author's Ed25519 private key.
- **Immutable** -- once published, messages cannot be edited or deleted by
  anyone, including the original author.

All peers independently validate every message before storing or forwarding it.
Invalid messages are silently dropped.

---

## 2. Message Envelope

All messages are wrapped in an `Envelope` for network transport. The envelope
uses a protobuf `oneof` to carry exactly one message type per transmission.

```protobuf
message Envelope {
  oneof payload {
    Post post = 1;
    Reaction reaction = 2;
    Profile profile = 3;
    FollowEvent follow_event = 4;
    DirectMessage direct_message = 5;
    MediaObject media_object = 6;
    MediaChunk media_chunk = 7;
  }
}
```

When a node receives an envelope via GossipSub, it:

1. Deserializes the envelope.
2. Extracts the inner message.
3. Validates the message according to its type-specific rules (see Section 3).
4. If valid, stores the message and forwards it to GossipSub peers.
5. If invalid, silently drops the message.

---

## 3. Message Types

### 3.1 Post

A post is the primary content unit: a text message optionally accompanied by
media attachments.

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

#### Validation Rules

| Field | Rule |
|---|---|
| `id` | MUST equal SHA-256 multihash of the serialized message with `id` and `signature` fields zeroed. |
| `author` | MUST be exactly 32 bytes (a valid Ed25519 public key). |
| `timestamp` | MUST pass timestamp validation (see [Section 9](#9-timestamp-validation-rules)). |
| `content` | MUST NOT exceed 5000 UTF-8 characters. MUST NOT be empty unless `media_cids` is non-empty OR `repost_of` is non-empty. |
| `media_cids` | MUST NOT exceed 10 items. Each item MUST be a valid CID. |
| `reply_to` | Mutually exclusive with `repost_of`. If set, MUST be a valid CID. |
| `repost_of` | Mutually exclusive with `reply_to`. If set, MUST be a valid CID. |
| `tags` | Informational. Extracted from `#hashtag` patterns in `content`. |
| `signature` | MUST be a valid Ed25519 signature by `author` over the serialized message with the `signature` field zeroed. |

### 3.2 Reaction

A reaction represents a user's response to a post (e.g., a "like").

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

#### Validation Rules

| Field | Rule |
|---|---|
| `id` | MUST equal SHA-256 multihash of the serialized message with `id` and `signature` fields zeroed. |
| `author` | MUST be exactly 32 bytes. |
| `target` | MUST be a valid CID referencing an existing or future post. |
| `reaction_type` | MUST be `"like"` in protocol version 1.0. |
| `timestamp` | MUST pass timestamp validation. |
| `signature` | MUST be a valid Ed25519 signature by `author`. |

#### Deduplication

One reaction per `(author, target, reaction_type)` tuple. If a node receives a
duplicate reaction from the same author for the same target and type, it MUST
silently ignore the duplicate.

### 3.3 Profile

A profile contains a user's public identity metadata.

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

#### Validation Rules

| Field | Rule |
|---|---|
| `author` | MUST be exactly 32 bytes. |
| `display_name` | MUST NOT be empty. MUST NOT exceed 50 UTF-8 characters. |
| `bio` | MUST NOT exceed 500 UTF-8 characters. |
| `avatar_cid` | If provided, MUST reference a valid media object. |
| `banner_cid` | If provided, MUST reference a valid media object. |
| `website` | MUST NOT exceed 200 characters. |
| `version` | MUST be strictly greater than the currently known version for this `author`. Stale versions MUST be rejected. |
| `timestamp` | MUST pass timestamp validation. |
| `signature` | MUST be a valid Ed25519 signature by `author`. |

### 3.4 FollowEvent

A follow event records a subscription relationship between two users.

```protobuf
message FollowEvent {
  bytes author = 1;                // 32-byte ed25519 public key (follower)
  bytes target = 2;                // 32-byte ed25519 public key (being followed)
  string action = 3;               // "follow" or "unfollow"
  uint64 timestamp = 4;
  bytes signature = 5;
}
```

#### Validation Rules

| Field | Rule |
|---|---|
| `author` | MUST be exactly 32 bytes. |
| `target` | MUST be exactly 32 bytes. MUST NOT equal `author` (self-follow is forbidden). |
| `action` | MUST be exactly `"follow"` or `"unfollow"`. |
| `timestamp` | MUST pass timestamp validation. |
| `signature` | MUST be a valid Ed25519 signature by `author`. |

### 3.5 DirectMessage

An end-to-end encrypted message between two users.

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

#### Validation Rules

| Field | Rule |
|---|---|
| `id` | MUST equal SHA-256 multihash of the serialized message with `id` and `signature` fields zeroed. |
| `author` | MUST be exactly 32 bytes. |
| `recipient` | MUST be exactly 32 bytes. MUST NOT equal `author`. |
| `encrypted_content` | MUST NOT be empty. |
| `nonce` | MUST be exactly 24 bytes. |
| `timestamp` | MUST pass timestamp validation. |
| `signature` | MUST be a valid Ed25519 signature by `author`. |

See [Section 8](#8-dm-encryption-protocol) for full encryption details.

### 3.6 MediaObject

Metadata descriptor for a media file split into fixed-size chunks.

```protobuf
message MediaObject {
  bytes cid = 1;                   // CID (hash of the complete media file)
  bytes author = 2;                // 32-byte ed25519 public key
  string mime_type = 3;            // MIME type
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

#### Validation Rules

| Field | Rule |
|---|---|
| `cid` | MUST be a valid SHA-256 multihash of the complete media file. |
| `author` | MUST be exactly 32 bytes. |
| `mime_type` | MUST be one of the supported types (see table below). |
| `size` | MUST NOT exceed 104,857,600 bytes (100 MB). |
| `chunk_count` | MUST equal `ceil(size / 262144)`. |
| `chunk_cids` | Length MUST equal `chunk_count`. Each MUST be a valid CID. |
| `thumbnail_cid` | If provided, MUST reference a valid thumbnail (max 100 KB JPEG, 320px wide). |
| `timestamp` | MUST pass timestamp validation. |
| `signature` | MUST be a valid Ed25519 signature by `author`. |

#### Supported MIME Types

| Category | MIME Types |
|---|---|
| Image | `image/jpeg`, `image/png`, `image/webp`, `image/gif` |
| Video | `video/mp4` (H.264), `video/webm` (VP9) |
| Audio | `audio/mpeg` (MP3), `audio/ogg`, `audio/wav` |

### 3.7 MediaChunk

A single chunk of a media file. Chunks are not signed individually -- integrity
is verified via CID matching against the parent MediaObject's `chunk_cids` list.

```protobuf
message MediaChunk {
  bytes cid = 1;                   // CID (hash of this chunk's data)
  bytes parent_cid = 2;            // CID of the parent MediaObject
  uint32 index = 3;               // Chunk sequence number (0-indexed)
  bytes data = 4;                 // Raw bytes (max 262144 bytes = 256KB)
}
```

#### Validation Rules

| Field | Rule |
|---|---|
| `cid` | MUST equal SHA-256 multihash of the `data` field. |
| `parent_cid` | MUST reference a known MediaObject. |
| `index` | MUST be less than the parent MediaObject's `chunk_count`. |
| `data` | MUST NOT exceed 262,144 bytes (256 KB). Last chunk MAY be smaller. |
| Cross-check | `cid` MUST match `chunk_cids[index]` in the parent MediaObject. |

---

## 4. Signing and CID Computation

### 4.1 CID Computation

Content Identifiers (CIDs) are computed using SHA-256 multihash encoding. The
process is deterministic and reproducible by any node.

**Algorithm:**

```
1. Take the protobuf message.
2. Zero out the `id` field (set to empty bytes).
3. Zero out the `signature` field (set to empty bytes).
4. Serialize the message to protobuf wire format.
5. Compute SHA-256 hash of the serialized bytes.
6. Encode as a multihash: [hash_function_code | digest_length | digest]
     - SHA-256 function code: 0x12
     - Digest length: 0x20 (32 bytes)
7. The resulting multihash is the CID.
```

**Pseudocode:**

```go
func ComputeCID(msg proto.Message) []byte {
    // Clone to avoid mutating the original
    clone := proto.Clone(msg)

    // Zero id and signature fields
    zeroIDAndSignature(clone)

    // Serialize
    data, _ := proto.Marshal(clone)

    // SHA-256 hash
    hash := sha256.Sum256(data)

    // Encode as multihash: 0x12 (SHA-256) | 0x20 (32 bytes) | hash
    mh := append([]byte{0x12, 0x20}, hash[:]...)
    return mh
}
```

### 4.2 Signing Process

Messages are signed using Ed25519 with the author's private key. The signature
covers the same canonical payload used for CID computation: the serialized
message with both `id` and `signature` fields zeroed.

**Algorithm:**

```
1. Take the protobuf message.
2. Zero out the `id` field (set to empty bytes).
3. Zero out the `signature` field (set to empty bytes).
4. Serialize the message to protobuf wire format.
5. Sign the serialized bytes using Ed25519 with the author's private key.
6. Set the `signature` field to the resulting 64-byte signature.
```

**Verification (performed by every receiving node):**

```
1. Extract the `signature` field and save it.
2. Zero out the `id` field.
3. Zero out the `signature` field.
4. Serialize the message to protobuf wire format.
5. Verify the saved signature against the serialized bytes using the
   `author` public key and Ed25519 verify.
6. If verification fails, drop the message silently.
```

### 4.3 Message Construction Order

When creating a new message, the fields MUST be populated in this order:

```
1. Populate all content fields (author, timestamp, content, etc.).
2. Clone the message, zero `id` and `signature`, and serialize it.
3. Compute the CID from that canonical byte sequence and set the `id` field.
4. Sign the same canonical byte sequence.
5. Set the `signature` field.
```

### 4.4 Public Key Encoding

Public keys are 32-byte Ed25519 keys. For human-readable display, they are
encoded using Bech32 with the `xleaks1` prefix:

```
Format:  xleaks1<bech32-encoded-32-byte-pubkey>
Example: xleaks1qyv3s8x...
```

This is analogous to Nostr's `npub1` format but uses the `xleaks1` human-readable
part (HRP).

---

## 5. GossipSub Topics

Content propagates across the network through GossipSub topics. Each topic is a
named channel that nodes subscribe to and publish on.

### 5.1 Topic Naming Convention

| Topic Pattern | Description | Volume |
|---|---|---|
| `/xleaks/posts/<author-pubkey-hex>` | All posts (including replies and reposts) by a specific author. | Per-author |
| `/xleaks/reactions/<post-cid-hex>` | All reactions to a specific post. | Per-post |
| `/xleaks/profiles` | All profile updates across the network. | Low (global) |
| `/xleaks/follows/<author-pubkey-hex>` | Follow/unfollow events by a specific user. | Per-author |
| `/xleaks/dm/<recipient-pubkey-hex>` | Direct messages destined for a specific recipient. | Per-recipient |
| `/xleaks/global` | Network-wide announcements (trending digests, indexer broadcasts). | Low (global) |

Key identifiers in topic names use lowercase hexadecimal encoding of the raw
bytes (pubkeys are 32 bytes = 64 hex characters; CIDs are 34 bytes = 68 hex
characters).

### 5.2 Subscription Behavior

| Event | Subscription Action |
|---|---|
| User follows a publisher | Subscribe to `/xleaks/posts/<publisher-pubkey-hex>` |
| User unfollows a publisher | Unsubscribe from `/xleaks/posts/<publisher-pubkey-hex>` |
| User opens a post detail view | Subscribe to `/xleaks/reactions/<post-cid-hex>` |
| User closes a post detail view | Unsubscribe from `/xleaks/reactions/<post-cid-hex>` |
| Node starts | Subscribe to `/xleaks/dm/<own-pubkey-hex>` |
| Node starts | Subscribe to `/xleaks/global` |
| Node starts | Subscribe to `/xleaks/profiles` |
| Node starts | Subscribe to `/xleaks/posts/<pubkey-hex>` for every followed publisher |
| Node sees a profile or post author | Subscribe to `/xleaks/follows/<author-pubkey-hex>` |
| Node sees a post for the first time | Subscribe to `/xleaks/reactions/<post-cid-hex>` |

### 5.3 Publishing Behavior

| Action | Topic |
|---|---|
| User creates a post | Publish to `/xleaks/posts/<own-pubkey-hex>` |
| User likes a post | Publish to `/xleaks/reactions/<post-cid-hex>` |
| User updates profile | Publish to `/xleaks/profiles` |
| User follows/unfollows | Publish to `/xleaks/follows/<own-pubkey-hex>` |
| User sends a DM | Publish to `/xleaks/dm/<recipient-pubkey-hex>` |
| User uploads media metadata | Publish `MediaObject` to `/xleaks/global` |

---

## 6. Content-Addressed Storage Format

Every node maintains a local content-addressed store (CAS) on disk. Objects are
stored as serialized protobuf bytes, keyed by their CID.

### 6.1 Directory Layout

```
~/.xleaks/
├── config.toml                    # Node configuration
├── identity/
│   ├── primary.key               # Encrypted primary key (Argon2id + AES-256-GCM)
│   └── identities/               # Additional identity key files
├── data/
│   ├── objects/                   # Content-addressed objects (CID -> serialized protobuf)
│   │   ├── ab/                   # First 2 hex chars of CID (sharding)
│   │   │   └── ab3f...full-cid   # Object file
│   │   └── ...
│   ├── media/                    # Media chunks (same sharding scheme)
│   │   ├── cd/
│   │   │   └── cd8a...full-cid   # Chunk data file
│   │   └── ...
│   └── index.db                  # SQLite database (indexes, feed, metadata)
├── logs/
│   └── xleaks.log
└── cache/
    └── thumbnails/               # Generated thumbnail cache
```

### 6.2 Object Storage

- Objects are stored as raw protobuf-serialized bytes.
- The filename is the full CID in hexadecimal encoding.
- A two-character directory prefix (first two hex chars of the CID) is used for
  filesystem sharding to avoid excessively large directories.
- To retrieve an object: read the file at
  `objects/<cid[0:2]>/<cid-full-hex>`, then deserialize the protobuf bytes.

### 6.3 Storage Limits and Eviction

| Parameter | Default |
|---|---|
| Maximum storage | 5 GB (configurable, minimum 1 GB) |
| Eviction policy | LRU (least recently used) |
| Pinned content | Content from followed publishers is NEVER evicted |
| Media fetch mode | On-demand by default (configurable to pre-fetch) |

When disk usage exceeds the configured maximum, the node evicts unpinned
content in LRU order. Access timestamps are tracked in the `content_access`
SQLite table.

---

## 7. Media Chunking Protocol

Media files are split into fixed-size chunks for efficient transfer and storage.
The signed `MediaObject` metadata is announced over GossipSub; the raw file,
thumbnail, and chunk bytes are then fetched on demand through the content
exchange layer.

### 7.1 Chunking Process

```
Input:  Raw media file bytes
Output: MediaObject descriptor + ordered chunk CID list

1. Validate file size <= 100 MB.
2. Validate MIME type is in the supported set.
3. Split file into 256 KB (262,144 byte) chunks.
   - Last chunk may be smaller than 256 KB.
4. For each chunk:
   a. Compute chunk CID = SHA-256 multihash of the chunk data.
   b. Store the raw chunk bytes in local content-addressed storage.
5. Compute overall file CID = SHA-256 multihash of the entire original file.
6. Generate thumbnail:
   - Images: resize to 320px wide, maintain aspect ratio, JPEG quality 80.
   - Videos: extract first frame, resize to 320px wide, JPEG quality 80.
   - Audio: no thumbnail.
   - Thumbnail MUST NOT exceed 100 KB.
7. Store thumbnail as a separate chunk with its own CID.
8. Create MediaObject with all metadata and ordered chunk CID list.
9. Sign the MediaObject.
10. Publish the MediaObject envelope on `/xleaks/global`.
11. Announce the file CID, thumbnail CID, and chunk CIDs to the content exchange.
```

### 7.2 Chunk Parameters

| Parameter | Value |
|---|---|
| Chunk size | 262,144 bytes (256 KB) |
| Maximum file size | 104,857,600 bytes (100 MB) |
| Maximum chunks per file | 400 (100 MB / 256 KB) |
| Thumbnail max size | 102,400 bytes (100 KB) |
| Thumbnail format | JPEG |
| Thumbnail width | 320 pixels |
| Thumbnail quality | 80 (configurable) |

### 7.3 Chunk Transfer

Chunks are exchanged between peers using the node's content exchange service:

```
1. Node A wants media CID X.
2. Node A loads the MediaObject metadata for CID X to get the chunk list.
3. Node A first checks local CAS for the assembled file or thumbnail.
4. If the bytes are missing locally, Node A asks peers advertising that CID via
   the content exchange.
5. On receipt, Node A verifies:
   a. the advertised CID matches SHA-256 of the received bytes
   b. chunk CIDs match the expected `chunk_cids[index]`
6. Node A stores the fetched bytes locally.
7. Once all chunks are fetched, the media object may be marked `fully_fetched`.
```

### 7.4 Reassembly

```
1. Load MediaObject to get ordered chunk CID list.
2. For each chunk CID in order (index 0 to chunk_count-1):
   a. Read chunk data from local storage.
   b. Verify CID matches SHA-256 of chunk data.
3. Concatenate all chunk data in order.
4. Verify overall file CID matches SHA-256 of concatenated data.
5. Return assembled file with the MIME type from MediaObject.
```

---

## 8. DM Encryption Protocol

Direct messages are end-to-end encrypted using X25519 Diffie-Hellman key
agreement and NaCl secretbox (XSalsa20-Poly1305) authenticated encryption.

### 8.1 Key Derivation

Ed25519 signing keys are converted to X25519 Diffie-Hellman keys for encryption:

```
1. Sender's Ed25519 private key -> X25519 private key (via clamping/conversion).
2. Recipient's Ed25519 public key -> X25519 public key (via birational map).
3. Shared secret = X25519(sender_private, recipient_public).
   - This produces a 32-byte shared secret.
   - The same shared secret is computed by: X25519(recipient_private, sender_public).
```

### 8.2 Encryption (Sending)

```
1. Convert sender's Ed25519 private key to X25519 private key.
2. Convert recipient's Ed25519 public key to X25519 public key.
3. Generate a random 24-byte nonce.
4. Compute shared secret via X25519 Diffie-Hellman.
5. Encrypt plaintext using NaCl box.Seal:
     encrypted = box.Seal(nil, plaintext, &nonce, &recipientX25519Pub, &senderX25519Priv)
   This produces: encrypted_content = ciphertext || 16-byte Poly1305 MAC.
6. Construct DirectMessage:
   - Set encrypted_content to the encrypted bytes.
   - Set nonce to the 24-byte nonce.
   - Compute CID and signature as usual.
```

### 8.3 Decryption (Receiving)

```
1. Convert recipient's Ed25519 private key to X25519 private key.
2. Convert sender's Ed25519 public key (from the `author` field) to X25519 public key.
3. Extract the 24-byte nonce from the message.
4. Decrypt using NaCl box.Open:
     plaintext, ok = box.Open(nil, encrypted_content, &nonce, &senderX25519Pub, &recipientX25519Priv)
5. If ok is false, decryption failed (message corrupted or not for this recipient).
6. Return plaintext.
```

### 8.4 Security Properties

| Property | Guarantee |
|---|---|
| Confidentiality | Only sender and recipient can decrypt the message. |
| Authenticity | The Poly1305 MAC verifies the message was created by someone with the shared secret. The Ed25519 signature on the outer message independently proves authorship. |
| Integrity | Any tampering with the ciphertext, nonce, or sender/recipient fields causes decryption failure or signature verification failure. |
| Forward secrecy | Not provided in v1.0 (static DH keys). Listed as a post-1.0 consideration. |
| Replay protection | CID-based deduplication prevents replays (see [Section 11](#11-replay-protection)). |

### 8.5 Important Constraints

- Nodes MUST NOT store plaintext DM content from other users. Only the sender
  and recipient can decrypt.
- Intermediate nodes forward encrypted DMs without being able to read them.
- The `author` and `recipient` fields are visible to all nodes (metadata is not
  hidden). Only the `encrypted_content` is confidential.

---

## 9. Timestamp Validation Rules

Timestamps are Unix timestamps in milliseconds. Validation depends on the
context in which a message is received.

### 9.1 Real-Time Messages (via GossipSub)

Messages received in real-time through GossipSub subscriptions are subject to
strict timestamp validation:

| Check | Rule | Rationale |
|---|---|---|
| Future limit | Timestamp MUST NOT be more than **5 minutes** ahead of the receiving node's clock. | Prevents pre-dating attacks. |
| Past limit | Timestamp MUST NOT be more than **30 days** behind the receiving node's clock. | Prevents injection of old content as "new." |

### 9.2 Historical Sync Messages

When a node performs historical content sync (e.g., fetching a publisher's back
catalog after following them), the **30-day past limit does not apply**. The
5-minute future limit still applies.

### 9.3 Timestamp Format

- All timestamps are **uint64** values representing **milliseconds since Unix
  epoch** (January 1, 1970, 00:00:00 UTC).
- Timestamps MUST be in UTC.
- Nodes SHOULD use NTP or system clock synchronization to minimize drift.

---

## 10. Rate Limiting Rules

Nodes MUST enforce rate limits on incoming GossipSub messages to prevent spam
and denial-of-service attacks.

### 10.1 Per-Author Limits

| Message Type | Limit | Window |
|---|---|---|
| Posts (including replies and reposts) | 10 messages | 1 minute |
| Reactions | 100 messages | 1 minute |
| Profile updates | 1 update | 1 minute |
| Follow events | 20 events | 1 minute |
| Direct messages | 30 messages | 1 minute |

### 10.2 Enforcement Behavior

- When a rate limit is exceeded, additional messages from that author are
  **silently dropped** for the remainder of the rate-limit window.
- Rate limits are tracked per-author (by public key), not per-peer.
- Rate limit state is ephemeral (in-memory) and does not persist across restarts.
- Rate limits apply to incoming messages only. A node does not rate-limit its
  own user's outbound messages at the protocol level (the API layer may
  impose its own limits for UX purposes).

### 10.3 API Rate Limiting

The local HTTP API server (localhost only) enforces separate rate limits to
prevent runaway UI clients:

| Endpoint Category | Limit |
|---|---|
| Write operations (POST, PUT, DELETE) | 60 requests/minute |
| Read operations (GET) | 300 requests/minute |
| Media uploads | 10 uploads/minute |

---

## 11. Replay Protection

Replay protection ensures that the same message cannot be injected into the
network multiple times.

### 11.1 CID-Based Deduplication

Every message has a unique CID derived from its content hash. When a node
receives a message:

```
1. Compute or extract the message CID.
2. Check if a message with this CID already exists in local storage.
3. If it exists, silently ignore the duplicate.
4. If it does not exist, proceed with validation and storage.
```

### 11.2 Reaction Deduplication

Reactions have an additional deduplication rule beyond CID uniqueness:

```
One reaction per (author, target, reaction_type) tuple.
```

Even if two Reaction messages have different CIDs (e.g., different timestamps),
they are considered duplicates if they share the same author, target, and
reaction_type. The first-seen reaction is kept; subsequent duplicates are dropped.

### 11.3 Profile Version Gating

Profile updates use a monotonically increasing `version` number. A profile
update is rejected if its `version` is less than or equal to the currently
known version for that author. This prevents replay of old profile data.

### 11.4 Follow Event Deduplication

Follow events are deduplicated by `(author, target)` pair. The latest event
(by timestamp) takes precedence, allowing toggle between follow and unfollow.

---

## 12. Clock Drift Tolerance

The protocol accounts for clock differences between nodes while preventing
timestamp manipulation.

### 12.1 Tolerance Parameters

| Parameter | Value |
|---|---|
| Maximum future drift | 5 minutes |
| Maximum past age (real-time) | 30 days |
| Maximum past age (historical sync) | Unlimited |

### 12.2 Rationale

- **5-minute future tolerance:** Accommodates normal clock drift between nodes
  without NTP. Messages from the "future" by more than 5 minutes are likely
  malicious attempts to place content ahead in timeline ordering.
- **30-day past tolerance:** Allows nodes that have been offline for extended
  periods to catch up, while preventing injection of ancient content as
  current. This limit is relaxed during explicit historical sync operations.

### 12.3 Recommendations for Node Operators

- Nodes SHOULD use NTP to keep system clocks synchronized.
- Nodes SHOULD log warnings when received messages have timestamps close to the
  tolerance boundaries, as this may indicate clock drift on the local node.
- Nodes MUST NOT modify timestamps of messages they relay. Timestamps are
  covered by the author's signature and any modification would invalidate it.

---

## Appendix A: Cryptographic Primitives Summary

| Purpose | Algorithm | Key Size | Output Size |
|---|---|---|---|
| Identity / Signing | Ed25519 | 32-byte private, 32-byte public | 64-byte signature |
| Content Addressing | SHA-256 (multihash) | N/A | 34 bytes (2-byte prefix + 32-byte hash) |
| DM Key Agreement | X25519 (Curve25519 ECDH) | 32 bytes | 32-byte shared secret |
| DM Encryption | NaCl secretbox (XSalsa20-Poly1305) | 32-byte key | ciphertext + 16-byte MAC |
| DM Nonce | Random | N/A | 24 bytes |
| Key Storage Encryption | AES-256-GCM | 32 bytes (derived) | ciphertext + 16-byte tag |
| Key Derivation (passphrase) | Argon2id | User passphrase | 32-byte derived key |
| Seed Phrases | BIP39 | 256 bits entropy | 24 words |
| Address Encoding | Bech32 | 32-byte pubkey | `xleaks1...` string |

## Appendix B: Protobuf Package

All protobuf definitions are in the `xleaks` package:

```protobuf
syntax = "proto3";
package xleaks;
option go_package = "github.com/xleaks/xleaks/proto/gen";
```

Generated Go code is placed in `proto/gen/` and regenerated via:

```bash
./scripts/gen-proto.sh
```

## Appendix C: Port Assignments

| Port | Protocol | Purpose |
|---|---|---|
| 7460 | TCP + QUIC (UDP) | libp2p peer-to-peer communication |
| 7470 | HTTP + WebSocket | Local API server (localhost only) |
| 7471 | HTTP | Indexer public API (indexer mode only) |
