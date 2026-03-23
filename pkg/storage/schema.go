package storage

// Schema contains all CREATE TABLE and CREATE INDEX statements for the XLeaks
// SQLite database. All statements use IF NOT EXISTS for idempotent migration.
const Schema = `
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;
PRAGMA foreign_keys = ON;

-- User's own identities
CREATE TABLE IF NOT EXISTS identities (
    pubkey BLOB PRIMARY KEY,
    display_name TEXT NOT NULL,
    is_active INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL
);

-- Known profiles (other users)
CREATE TABLE IF NOT EXISTS profiles (
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
CREATE TABLE IF NOT EXISTS posts (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    content TEXT,
    reply_to BLOB,
    repost_of BLOB,
    timestamp INTEGER NOT NULL,
    signature BLOB NOT NULL,
    received_at INTEGER NOT NULL,
    FOREIGN KEY (author) REFERENCES profiles(pubkey)
);

CREATE INDEX IF NOT EXISTS idx_posts_author ON posts(author, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_posts_timestamp ON posts(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_posts_reply_to ON posts(reply_to) WHERE reply_to IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_posts_repost_of ON posts(repost_of) WHERE repost_of IS NOT NULL;

-- Media references in posts
CREATE TABLE IF NOT EXISTS post_media (
    post_cid BLOB NOT NULL,
    media_cid BLOB NOT NULL,
    position INTEGER NOT NULL,
    PRIMARY KEY (post_cid, media_cid)
);

-- Media object metadata
CREATE TABLE IF NOT EXISTS media_objects (
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
    fully_fetched INTEGER DEFAULT 0
);

-- Reactions (likes)
CREATE TABLE IF NOT EXISTS reactions (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    target BLOB NOT NULL,
    reaction_type TEXT NOT NULL DEFAULT 'like',
    timestamp INTEGER NOT NULL,
    UNIQUE(author, target, reaction_type)
);

CREATE INDEX IF NOT EXISTS idx_reactions_target ON reactions(target);

-- Aggregated reaction counts (materialized for performance)
CREATE TABLE IF NOT EXISTS reaction_counts (
    post_cid BLOB PRIMARY KEY,
    like_count INTEGER DEFAULT 0,
    reply_count INTEGER DEFAULT 0,
    repost_count INTEGER DEFAULT 0
);

-- Subscriptions (who this user follows)
CREATE TABLE IF NOT EXISTS subscriptions (
    pubkey BLOB PRIMARY KEY,
    followed_at INTEGER NOT NULL,
    sync_completed INTEGER DEFAULT 0
);

-- Follow events (seen on network, for follower count display)
CREATE TABLE IF NOT EXISTS follow_events (
    author BLOB NOT NULL,
    target BLOB NOT NULL,
    action TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    PRIMARY KEY (author, target)
);

CREATE INDEX IF NOT EXISTS idx_follow_events_target ON follow_events(target);

-- Follower counts (materialized)
CREATE TABLE IF NOT EXISTS follower_counts (
    pubkey BLOB PRIMARY KEY,
    follower_count INTEGER DEFAULT 0,
    following_count INTEGER DEFAULT 0
);

-- Direct messages
CREATE TABLE IF NOT EXISTS direct_messages (
    cid BLOB PRIMARY KEY,
    author BLOB NOT NULL,
    recipient BLOB NOT NULL,
    encrypted_content BLOB NOT NULL,
    nonce BLOB NOT NULL,
    timestamp INTEGER NOT NULL,
    read INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_dm_author ON direct_messages(author, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_dm_recipient ON direct_messages(recipient, timestamp DESC);

-- Notifications
CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    actor BLOB NOT NULL,
    target_cid BLOB,
    related_cid BLOB,
    timestamp INTEGER NOT NULL,
    read INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(read, timestamp DESC);

-- Hashtags for local search/trending
CREATE TABLE IF NOT EXISTS post_tags (
    post_cid BLOB NOT NULL,
    tag TEXT NOT NULL,
    PRIMARY KEY (post_cid, tag)
);

CREATE INDEX IF NOT EXISTS idx_post_tags_tag ON post_tags(tag);

-- Peer reputation (track reliable peers for content fetching)
CREATE TABLE IF NOT EXISTS peer_stats (
    peer_id TEXT PRIMARY KEY,
    successful_fetches INTEGER DEFAULT 0,
    failed_fetches INTEGER DEFAULT 0,
    bytes_received INTEGER DEFAULT 0,
    bytes_sent INTEGER DEFAULT 0,
    last_seen INTEGER NOT NULL
);

-- Content eviction tracking
CREATE TABLE IF NOT EXISTS content_access (
    cid BLOB PRIMARY KEY,
    last_accessed INTEGER NOT NULL,
    access_count INTEGER DEFAULT 1,
    is_pinned INTEGER DEFAULT 0
);

-- Terms acceptance tracking
CREATE TABLE IF NOT EXISTS terms_acceptance (
    pubkey BLOB PRIMARY KEY,
    terms_version TEXT NOT NULL DEFAULT '1.0',
    accepted_at INTEGER NOT NULL,
    device_node_agreed INTEGER NOT NULL DEFAULT 0
);
`
