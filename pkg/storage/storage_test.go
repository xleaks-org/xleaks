package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestNewDB_InvalidPath(t *testing.T) {
	_, err := NewDB("/nonexistent/dir/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	// Run migrate again; should not error.
	if err := db.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestMigrate_LegacySubscriptionsAndNotifications(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`
		CREATE TABLE subscriptions (
			pubkey BLOB NOT NULL PRIMARY KEY,
			followed_at INTEGER NOT NULL,
			sync_completed INTEGER DEFAULT 0
		);
		CREATE TABLE notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type TEXT NOT NULL,
			actor BLOB NOT NULL,
			target_cid BLOB,
			related_cid BLOB,
			timestamp INTEGER NOT NULL,
			read INTEGER DEFAULT 0
		);
	`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate legacy schema: %v", err)
	}

	hasSubOwner, err := db.tableHasColumn("subscriptions", "owner_pubkey")
	if err != nil {
		t.Fatalf("tableHasColumn subscriptions.owner_pubkey: %v", err)
	}
	if !hasSubOwner {
		t.Fatal("expected subscriptions.owner_pubkey after migration")
	}

	hasNotifOwner, err := db.tableHasColumn("notifications", "owner_pubkey")
	if err != nil {
		t.Fatalf("tableHasColumn notifications.owner_pubkey: %v", err)
	}
	if !hasNotifOwner {
		t.Fatal("expected notifications.owner_pubkey after migration")
	}
}

func TestNewDB_WALMode(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "wal_test.db")
	db, err := NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	// Verify WAL file exists.
	if _, err := os.Stat(dbPath + "-wal"); err != nil {
		t.Logf("WAL file check: %v (may not exist yet if no writes)", err)
	}
}

func TestTrackContentAccess_PreservesPinState(t *testing.T) {
	db := setupTestDB(t)
	cid := []byte("content-access-cid")

	if err := db.TrackContentAccess(cid, true); err != nil {
		t.Fatalf("TrackContentAccess pin: %v", err)
	}
	if err := db.TrackContentAccess(cid, false); err != nil {
		t.Fatalf("TrackContentAccess unpinned access: %v", err)
	}

	var pinned int
	if err := db.QueryRow(`SELECT is_pinned FROM content_access WHERE cid = ?`, cid).Scan(&pinned); err != nil {
		t.Fatalf("QueryRow pinned: %v", err)
	}
	if pinned != 1 {
		t.Fatalf("expected content to remain pinned, got %d", pinned)
	}
}

func TestSetPinnedForAuthor_BackfillsStoredContent(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("owner-local-identity-xxxxxxxxxxxxx")
	author := []byte("followed-author-xxxxxxxxxxxxxxxx")
	postCID := []byte("post-cid-followed-author-xxxxxxx")
	reactionCID := []byte("reaction-cid-followed-author")
	mediaCID := []byte("media-cid-followed-author-xxxxxx")
	thumbCID := []byte("thumb-cid-followed-author-xxxxxx")

	if err := db.InsertIdentity(owner, "Owner", true, 1000); err != nil {
		t.Fatalf("InsertIdentity: %v", err)
	}
	if err := db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000); err != nil {
		t.Fatalf("UpsertProfile author: %v", err)
	}
	if err := db.InsertPost(postCID, author, "hello", nil, nil, 2000, []byte("sig")); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}
	if err := db.InsertReaction(reactionCID, author, postCID, "like", 3000); err != nil {
		t.Fatalf("InsertReaction: %v", err)
	}
	if err := db.InsertMediaObject(mediaCID, author, "image/png", 128, 1, 0, 0, 0, thumbCID, 4000); err != nil {
		t.Fatalf("InsertMediaObject: %v", err)
	}

	if err := db.SetPinnedForAuthor(author, true); err != nil {
		t.Fatalf("SetPinnedForAuthor true: %v", err)
	}
	for _, cid := range [][]byte{author, postCID, reactionCID, mediaCID, thumbCID} {
		assertPinnedState(t, db, cid, 1)
	}

	if err := db.SetPinnedForAuthor(author, false); err != nil {
		t.Fatalf("SetPinnedForAuthor false: %v", err)
	}
	for _, cid := range [][]byte{author, postCID, reactionCID, mediaCID, thumbCID} {
		assertPinnedState(t, db, cid, 0)
	}
}

func TestTrackReactionContent_PinsWhenTargetAuthorIsFollowed(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("owner-local-identity-2-xxxxxxxx")
	targetAuthor := []byte("target-author-followed-xxxxxxxx")
	reactionAuthor := []byte("reaction-author-remote-xxxxxxx")
	postCID := []byte("target-post-cid-followed-author")
	reactionCID := []byte("reaction-cid-target-followed")

	if err := db.InsertIdentity(owner, "Owner", true, 1000); err != nil {
		t.Fatalf("InsertIdentity: %v", err)
	}
	if err := db.AddSubscription(owner, targetAuthor, 2000); err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}
	if err := db.UpsertProfile(targetAuthor, "Target", "", nil, nil, "", 1, 1000); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}
	if err := db.InsertPost(postCID, targetAuthor, "post", nil, nil, 3000, []byte("sig")); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	if err := db.TrackReactionContent(reactionCID, reactionAuthor, postCID); err != nil {
		t.Fatalf("TrackReactionContent: %v", err)
	}
	assertPinnedState(t, db, reactionCID, 1)
}

func assertPinnedState(t *testing.T, db *DB, cid []byte, want int) {
	t.Helper()
	var pinned int
	if err := db.QueryRow(`SELECT is_pinned FROM content_access WHERE cid = ?`, cid).Scan(&pinned); err != nil {
		t.Fatalf("QueryRow pinned %q: %v", string(cid), err)
	}
	if pinned != want {
		t.Fatalf("pinned state for %q = %d, want %d", string(cid), pinned, want)
	}
}

// --- Profiles ---

func TestUpsertAndGetProfile(t *testing.T) {
	db := setupTestDB(t)
	pubkey := []byte("pubkey_profile_1_xxxxxxxxxxxxxxxxx")

	err := db.UpsertProfile(pubkey, "Alice", "Hello world", []byte("avatar_cid"), nil, "https://alice.com", 1, 1000)
	if err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	p, err := db.GetProfile(pubkey)
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p == nil {
		t.Fatal("expected profile, got nil")
	}
	if p.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice", p.DisplayName)
	}
	if p.Bio != "Hello world" {
		t.Errorf("Bio = %q, want Hello world", p.Bio)
	}
	if p.Website != "https://alice.com" {
		t.Errorf("Website = %q", p.Website)
	}
	if p.Version != 1 {
		t.Errorf("Version = %d, want 1", p.Version)
	}
}

func TestUpsertProfile_VersionGuard(t *testing.T) {
	db := setupTestDB(t)
	pubkey := []byte("pubkey_profile_2_xxxxxxxxxxxxxxxxx")

	_ = db.UpsertProfile(pubkey, "Alice", "", nil, nil, "", 5, 1000)
	// Lower version should not update.
	_ = db.UpsertProfile(pubkey, "Bob", "", nil, nil, "", 3, 2000)

	p, _ := db.GetProfile(pubkey)
	if p.DisplayName != "Alice" {
		t.Errorf("DisplayName = %q, want Alice (version guard failed)", p.DisplayName)
	}

	// Higher version should update.
	_ = db.UpsertProfile(pubkey, "Charlie", "", nil, nil, "", 10, 3000)
	p, _ = db.GetProfile(pubkey)
	if p.DisplayName != "Charlie" {
		t.Errorf("DisplayName = %q, want Charlie", p.DisplayName)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	db := setupTestDB(t)
	p, err := db.GetProfile([]byte("nonexistent"))
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p != nil {
		t.Fatal("expected nil for nonexistent profile")
	}
}

// --- Posts ---

func TestInsertAndGetPost(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_post_1_xxxxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)

	cid := []byte("post_cid_1_xxxxxxxxxxxxxxxxxxxxxxx")
	sig := []byte("signature_xxxxxxxxxxxxxxxxxxxxxxxxx")
	err := db.InsertPost(cid, author, "Hello XLeaks!", nil, nil, 1000, sig)
	if err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	p, err := db.GetPost(cid)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if p.Content != "Hello XLeaks!" {
		t.Errorf("Content = %q", p.Content)
	}
	if p.ReplyTo != nil {
		t.Errorf("ReplyTo should be nil, got %v", p.ReplyTo)
	}
}

func TestPostExists(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_postex_1_xxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)

	cid := []byte("post_exists_cid_xxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(cid, author, "exists", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	if !db.PostExists(cid) {
		t.Error("PostExists returned false for existing post")
	}
	if db.PostExists([]byte("nonexistent")) {
		t.Error("PostExists returned true for nonexistent post")
	}
}

func TestGetPostsByAuthor(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_bypages_1_xxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)

	for i := int64(1); i <= 5; i++ {
		cid := []byte("post_by_author_cid_" + string(rune('a'+i-1)) + "xxxxxxxxx")
		_ = db.InsertPost(cid, author, "post", nil, nil, i*1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	}

	posts, err := db.GetPostsByAuthor(author, 0, 3)
	if err != nil {
		t.Fatalf("GetPostsByAuthor: %v", err)
	}
	if len(posts) != 3 {
		t.Fatalf("expected 3 posts, got %d", len(posts))
	}
	// Should be descending by timestamp.
	if posts[0].Timestamp < posts[1].Timestamp {
		t.Error("posts not in descending timestamp order")
	}
}

func TestGetFeed(t *testing.T) {
	db := setupTestDB(t)
	a1 := []byte("author_feed_1_xxxxxxxxxxxxxxxxxxxx")
	a2 := []byte("author_feed_2_xxxxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(a1, "A1", "", nil, nil, "", 1, 1000)
	_ = db.UpsertProfile(a2, "A2", "", nil, nil, "", 1, 1000)

	_ = db.InsertPost([]byte("feed_cid_1_xxxxxxxxxxxxxxxxxxxxxxx"), a1, "a1 post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	_ = db.InsertPost([]byte("feed_cid_2_xxxxxxxxxxxxxxxxxxxxxxx"), a2, "a2 post", nil, nil, 2000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	posts, err := db.GetFeed([][]byte{a1, a2}, 0, 10)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(posts) != 2 {
		t.Fatalf("expected 2 feed posts, got %d", len(posts))
	}
}

func TestGetFeed_Empty(t *testing.T) {
	db := setupTestDB(t)
	posts, err := db.GetFeed(nil, 0, 10)
	if err != nil {
		t.Fatalf("GetFeed nil: %v", err)
	}
	if posts != nil {
		t.Fatalf("expected nil for empty feed, got %d posts", len(posts))
	}
}

func TestGetThread(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_thread_1_xxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)

	parent := []byte("thread_parent_cid_xxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(parent, author, "parent", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	reply1 := []byte("thread_reply_cid_1_xxxxxxxxxxxxxxxx")
	reply2 := []byte("thread_reply_cid_2_xxxxxxxxxxxxxxxx")
	_ = db.InsertPost(reply1, author, "reply1", parent, nil, 2000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	_ = db.InsertPost(reply2, author, "reply2", parent, nil, 3000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	thread, err := db.GetThread(parent)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(thread) != 2 {
		t.Fatalf("expected 2 replies, got %d", len(thread))
	}
	// Should be ascending by timestamp.
	if thread[0].Timestamp > thread[1].Timestamp {
		t.Error("thread not in ascending timestamp order")
	}
}

// --- Reactions ---

func TestInsertAndGetReactions(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_react_1_xxxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)
	postCID := []byte("react_post_cid_xxxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(postCID, author, "post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	reactionCID := []byte("reaction_cid_1_xxxxxxxxxxxxxxxxxx")
	liker := []byte("liker_1_xxxxxxxxxxxxxxxxxxxxxxxxxxx")
	err := db.InsertReaction(reactionCID, liker, postCID, "like", 2000)
	if err != nil {
		t.Fatalf("InsertReaction: %v", err)
	}

	reactions, err := db.GetReactions(postCID)
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(reactions) != 1 {
		t.Fatalf("expected 1 reaction, got %d", len(reactions))
	}
	if reactions[0].ReactionType != "like" {
		t.Errorf("ReactionType = %q", reactions[0].ReactionType)
	}
}

func TestInsertReaction_Dedup(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_react_dd_xxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)
	postCID := []byte("react_dd_post_cid_xxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(postCID, author, "post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	liker := []byte("liker_dd_xxxxxxxxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertReaction([]byte("rcid_dd_1_xxxxxxxxxxxxxxxxxxxxxxxxx"), liker, postCID, "like", 2000)
	_ = db.InsertReaction([]byte("rcid_dd_2_xxxxxxxxxxxxxxxxxxxxxxxxx"), liker, postCID, "like", 3000) // duplicate author+target+type

	reactions, _ := db.GetReactions(postCID)
	if len(reactions) != 1 {
		t.Errorf("expected 1 reaction after dedup, got %d", len(reactions))
	}
}

func TestHasReacted(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_hasreact_xxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)
	postCID := []byte("hasreact_post_cid_xxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(postCID, author, "post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	liker := []byte("liker_hasreact_xxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertReaction([]byte("hasreact_rcid_xxxxxxxxxxxxxxxxxxxxx"), liker, postCID, "like", 2000)

	if !db.HasReacted(liker, postCID, "like") {
		t.Error("HasReacted returned false for existing reaction")
	}
	if db.HasReacted(liker, postCID, "dislike") {
		t.Error("HasReacted returned true for non-existing reaction type")
	}
}

func TestUpdateReactionCount(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("author_rcount_1_xxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)
	postCID := []byte("rcount_post_cid_xxxxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(postCID, author, "post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	_ = db.InsertReaction([]byte("rcount_rcid_1_xxxxxxxxxxxxxxxxxxxxx"), []byte("liker_rcount_1_xxxxxxxxxxxxxxxxxxxx"), postCID, "like", 2000)
	_ = db.InsertReaction([]byte("rcount_rcid_2_xxxxxxxxxxxxxxxxxxxxx"), []byte("liker_rcount_2_xxxxxxxxxxxxxxxxxxxx"), postCID, "like", 3000)

	// Add a reply to the post.
	_ = db.InsertPost([]byte("rcount_reply_cid_xxxxxxxxxxxxxxxxxx"), author, "reply", postCID, nil, 4000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))

	err := db.UpdateReactionCount(postCID)
	if err != nil {
		t.Fatalf("UpdateReactionCount: %v", err)
	}

	likes, err := db.GetReactionCount(postCID)
	if err != nil {
		t.Fatalf("GetReactionCount: %v", err)
	}
	if likes != 2 {
		t.Errorf("likes = %d, want 2", likes)
	}
}

// --- Subscriptions ---

func TestAddAndGetSubscriptions(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("sub_owner_1_xxxxxxxxxxxxxxxxxxxxxxx")
	pubkey := []byte("sub_pubkey_1_xxxxxxxxxxxxxxxxxxxxxx")
	err := db.AddSubscription(owner, pubkey, 1000)
	if err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}

	subs, err := db.GetSubscriptions(owner)
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}

	if !db.IsSubscribed(owner, pubkey) {
		t.Error("IsSubscribed returned false")
	}
}

func TestRemoveSubscription(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("sub_rm_owner_1_xxxxxxxxxxxxxxxxx")
	pubkey := []byte("sub_rm_pubkey_1_xxxxxxxxxxxxxxxxxxx")
	_ = db.AddSubscription(owner, pubkey, 1000)
	_ = db.RemoveSubscription(owner, pubkey)

	if db.IsSubscribed(owner, pubkey) {
		t.Error("IsSubscribed returned true after removal")
	}
}

func TestFollowEvents(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("fe_author_1_xxxxxxxxxxxxxxxxxxxxxxx")
	target := []byte("fe_target_1_xxxxxxxxxxxxxxxxxxxxxxx")

	err := db.InsertFollowEvent(author, target, "follow", 1000)
	if err != nil {
		t.Fatalf("InsertFollowEvent: %v", err)
	}

	followers, err := db.GetFollowers(target)
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if len(followers) != 1 {
		t.Fatalf("expected 1 follower, got %d", len(followers))
	}

	following, err := db.GetFollowing(author)
	if err != nil {
		t.Fatalf("GetFollowing: %v", err)
	}
	if len(following) != 1 {
		t.Fatalf("expected 1 following, got %d", len(following))
	}
}

func TestFollowEvent_Unfollow(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("fe_unf_author_1_xxxxxxxxxxxxxxxxxxx")
	target := []byte("fe_unf_target_1_xxxxxxxxxxxxxxxxxxx")

	_ = db.InsertFollowEvent(author, target, "follow", 1000)
	_ = db.InsertFollowEvent(author, target, "unfollow", 2000) // overwrites

	followers, _ := db.GetFollowers(target)
	if len(followers) != 0 {
		t.Errorf("expected 0 followers after unfollow, got %d", len(followers))
	}
}

func TestUpdateFollowerCount(t *testing.T) {
	db := setupTestDB(t)
	user := []byte("fc_user_1_xxxxxxxxxxxxxxxxxxxxxxxxx")
	f1 := []byte("fc_follower_1_xxxxxxxxxxxxxxxxxxxxx")
	f2 := []byte("fc_follower_2_xxxxxxxxxxxxxxxxxxxxx")
	t1 := []byte("fc_following_1_xxxxxxxxxxxxxxxxxxxx")

	_ = db.InsertFollowEvent(f1, user, "follow", 1000)
	_ = db.InsertFollowEvent(f2, user, "follow", 2000)
	_ = db.InsertFollowEvent(user, t1, "follow", 3000)

	err := db.UpdateFollowerCount(user)
	if err != nil {
		t.Fatalf("UpdateFollowerCount: %v", err)
	}

	// Verify via a direct query.
	var fc, fgc int
	_ = db.QueryRow(`SELECT follower_count, following_count FROM follower_counts WHERE pubkey = ?`, user).Scan(&fc, &fgc)
	if fc != 2 {
		t.Errorf("follower_count = %d, want 2", fc)
	}
	if fgc != 1 {
		t.Errorf("following_count = %d, want 1", fgc)
	}
}

// --- Notifications ---

func TestInsertAndGetNotifications(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("notif_owner_1_xxxxxxxxxxxxxxxxxxxxx")
	actor := []byte("notif_actor_1_xxxxxxxxxxxxxxxxxxxxx")
	targetCID := []byte("notif_target_cid_xxxxxxxxxxxxxxxxxx")

	err := db.InsertNotification(owner, "like", actor, targetCID, nil, 1000)
	if err != nil {
		t.Fatalf("InsertNotification: %v", err)
	}

	notifs, err := db.GetNotifications(owner, 0, 10)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(notifs) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifs))
	}
	if notifs[0].Type != "like" {
		t.Errorf("Type = %q", notifs[0].Type)
	}
	if notifs[0].Read {
		t.Error("notification should be unread")
	}
}

func TestMarkRead(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("mr_owner_1_xxxxxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertNotification(owner, "like", []byte("mr_actor_1_xxxxxxxxxxxxxxxxxxxxxxx"), nil, nil, 1000)

	notifs, _ := db.GetNotifications(owner, 0, 10)
	err := db.MarkRead(owner, notifs[0].ID)
	if err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	notifs, _ = db.GetNotifications(owner, 0, 10)
	if !notifs[0].Read {
		t.Error("notification should be read after MarkRead")
	}
}

func TestMarkAllRead(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("mar_owner_1_xxxxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertNotification(owner, "like", []byte("mar_actor_1_xxxxxxxxxxxxxxxxxxxxxx"), nil, nil, 1000)
	_ = db.InsertNotification(owner, "reply", []byte("mar_actor_2_xxxxxxxxxxxxxxxxxxxxxx"), nil, nil, 2000)

	err := db.MarkAllRead(owner)
	if err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}

	count, _ := db.UnreadCount(owner)
	if count != 0 {
		t.Errorf("unread count = %d after MarkAllRead", count)
	}
}

func TestUnreadCount(t *testing.T) {
	db := setupTestDB(t)
	owner := []byte("uc_owner_1_xxxxxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertNotification(owner, "like", []byte("uc_actor_1_xxxxxxxxxxxxxxxxxxxxxxx"), nil, nil, 1000)
	_ = db.InsertNotification(owner, "reply", []byte("uc_actor_2_xxxxxxxxxxxxxxxxxxxxxxx"), nil, nil, 2000)

	count, err := db.UnreadCount(owner)
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if count != 2 {
		t.Errorf("unread count = %d, want 2", count)
	}
}

// --- Direct Messages ---

func TestInsertAndGetConversation(t *testing.T) {
	db := setupTestDB(t)
	alice := []byte("dm_alice_1_xxxxxxxxxxxxxxxxxxxxxxxx")
	bob := []byte("dm_bob_1_xxxxxxxxxxxxxxxxxxxxxxxxxx")

	err := db.InsertDM([]byte("dm_cid_1_xxxxxxxxxxxxxxxxxxxxxxxxxx"), alice, bob, []byte("enc1"), []byte("nonce1_xxxxxxxxxxxxxxxxx"), 1000)
	if err != nil {
		t.Fatalf("InsertDM: %v", err)
	}
	_ = db.InsertDM([]byte("dm_cid_2_xxxxxxxxxxxxxxxxxxxxxxxxxx"), bob, alice, []byte("enc2"), []byte("nonce2_xxxxxxxxxxxxxxxxx"), 2000)

	conv, err := db.GetConversation(alice, bob, 0, 10)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(conv) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(conv))
	}
	// Should be descending by timestamp.
	if conv[0].Timestamp < conv[1].Timestamp {
		t.Error("conversation not in descending timestamp order")
	}
}

func TestGetConversations(t *testing.T) {
	db := setupTestDB(t)
	me := []byte("dm_me_xxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	peer1 := []byte("dm_peer1_xxxxxxxxxxxxxxxxxxxxxxxxxx")
	peer2 := []byte("dm_peer2_xxxxxxxxxxxxxxxxxxxxxxxxxx")

	_ = db.InsertDM([]byte("dm_conv_cid_1_xxxxxxxxxxxxxxxxxxxxx"), me, peer1, []byte("enc1"), []byte("nonce_conv_1_xxxxxxxxxxxx"), 1000)
	_ = db.InsertDM([]byte("dm_conv_cid_2_xxxxxxxxxxxxxxxxxxxxx"), peer2, me, []byte("enc2"), []byte("nonce_conv_2_xxxxxxxxxxxx"), 2000)

	convs, err := db.GetConversations(me)
	if err != nil {
		t.Fatalf("GetConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(convs))
	}
	// peer2's conversation should be first (more recent).
	if convs[0].LastTimestamp < convs[1].LastTimestamp {
		t.Error("conversations not ordered by last timestamp desc")
	}
}

func TestMarkDMRead(t *testing.T) {
	db := setupTestDB(t)
	cid := []byte("dm_markread_cid_xxxxxxxxxxxxxxxxxxx")
	_ = db.InsertDM(cid, []byte("dm_mr_author_xxxxxxxxxxxxxxxxxxxxx"), []byte("dm_mr_recip_xxxxxxxxxxxxxxxxxxxxxx"), []byte("enc"), []byte("nonce_mr_xxxxxxxxxxxxxxxx"), 1000)

	err := db.MarkDMRead(cid)
	if err != nil {
		t.Fatalf("MarkDMRead: %v", err)
	}

	conv, _ := db.GetConversation([]byte("dm_mr_author_xxxxxxxxxxxxxxxxxxxxx"), []byte("dm_mr_recip_xxxxxxxxxxxxxxxxxxxxxx"), 0, 10)
	if len(conv) != 1 || !conv[0].Read {
		t.Error("DM should be marked as read")
	}
}

// --- Media ---

func TestInsertAndGetMediaObject(t *testing.T) {
	db := setupTestDB(t)
	cid := []byte("media_cid_1_xxxxxxxxxxxxxxxxxxxxxxx")
	author := []byte("media_author_1_xxxxxxxxxxxxxxxxxxxx")

	err := db.InsertMediaObject(cid, author, "image/jpeg", 1024000, 4, 1920, 1080, 0, []byte("thumb_cid_xxxxxxxxxxxxxxxxxxxxx"), 1000)
	if err != nil {
		t.Fatalf("InsertMediaObject: %v", err)
	}

	m, err := db.GetMediaObject(cid)
	if err != nil {
		t.Fatalf("GetMediaObject: %v", err)
	}
	if m == nil {
		t.Fatal("expected media object, got nil")
	}
	if m.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q", m.MimeType)
	}
	if m.Size != 1024000 {
		t.Errorf("Size = %d", m.Size)
	}
	if m.ChunkCount != 4 {
		t.Errorf("ChunkCount = %d", m.ChunkCount)
	}
	if m.Width != 1920 || m.Height != 1080 {
		t.Errorf("Dimensions = %dx%d", m.Width, m.Height)
	}
	if m.FullyFetched {
		t.Error("should not be fully fetched initially")
	}
}

func TestSetMediaFetched(t *testing.T) {
	db := setupTestDB(t)
	cid := []byte("media_fetch_cid_xxxxxxxxxxxxxxxxxxx")
	_ = db.InsertMediaObject(cid, []byte("media_fetch_author_xxxxxxxxxxxxxxxx"), "video/mp4", 50000000, 200, 1280, 720, 120, nil, 1000)

	err := db.SetMediaFetched(cid)
	if err != nil {
		t.Fatalf("SetMediaFetched: %v", err)
	}

	m, _ := db.GetMediaObject(cid)
	if !m.FullyFetched {
		t.Error("should be fully fetched after SetMediaFetched")
	}
}

func TestGetMediaObject_NotFound(t *testing.T) {
	db := setupTestDB(t)
	m, err := db.GetMediaObject([]byte("nonexistent"))
	if err != nil {
		t.Fatalf("GetMediaObject: %v", err)
	}
	if m != nil {
		t.Fatal("expected nil for nonexistent media object")
	}
}

func TestInsertPostMedia(t *testing.T) {
	db := setupTestDB(t)
	author := []byte("pm_author_1_xxxxxxxxxxxxxxxxxxxxxxx")
	_ = db.UpsertProfile(author, "Author", "", nil, nil, "", 1, 1000)
	postCID := []byte("pm_post_cid_1_xxxxxxxxxxxxxxxxxxxxx")
	mediaCID := []byte("pm_media_cid_1_xxxxxxxxxxxxxxxxxxxx")
	_ = db.InsertPost(postCID, author, "media post", nil, nil, 1000, []byte("sig_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"))
	_ = db.InsertMediaObject(mediaCID, author, "image/png", 500, 1, 100, 100, 0, nil, 1000)

	err := db.InsertPostMedia(postCID, mediaCID, 0)
	if err != nil {
		t.Fatalf("InsertPostMedia: %v", err)
	}

	// Verify via direct query.
	var pos int
	_ = db.QueryRow(`SELECT position FROM post_media WHERE post_cid = ? AND media_cid = ?`, postCID, mediaCID).Scan(&pos)
	if pos != 0 {
		t.Errorf("position = %d, want 0", pos)
	}
}
