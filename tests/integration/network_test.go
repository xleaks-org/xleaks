package integration_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/social"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// testEnv bundles all the infrastructure needed for integration tests.
type testEnv struct {
	db  *storage.DB
	cas *content.ContentStore
	kp  *identity.KeyPair
}

// newTestEnv creates a fresh test environment with a temp DB and content store.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "test.db")
	db, err := storage.NewDB(dbPath)
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	casDir := filepath.Join(dir, "cas")
	cas, err := content.NewContentStore(casDir)
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Create a profile for this key pair so FK constraints are satisfied.
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	return &testEnv{db: db, cas: cas, kp: kp}
}

func TestPostLifecycle(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	postSvc := social.NewPostService(env.db, env.cas, env.kp)

	// Create a post.
	post, err := postSvc.CreatePost(ctx, "Hello XLeaks integration test!", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	if len(post.Id) == 0 {
		t.Fatal("post ID should not be empty")
	}
	if post.Content != "Hello XLeaks integration test!" {
		t.Errorf("Content = %q", post.Content)
	}

	// Verify the post exists in DB.
	if !env.db.PostExists(post.Id) {
		t.Fatal("post should exist in DB after creation")
	}

	// Retrieve it.
	row, err := env.db.GetPost(post.Id)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if row.Content != post.Content {
		t.Errorf("retrieved content = %q, want %q", row.Content, post.Content)
	}

	// Verify the post is stored in CAS.
	if !env.cas.Has(post.Id) {
		t.Fatal("post should exist in CAS after creation")
	}
}

func TestFollowUnfollow(t *testing.T) {
	env := newTestEnv(t)

	// Create a second user to follow.
	otherKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if err := env.db.UpsertProfile(otherKP.PublicKeyBytes(), "OtherUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile other: %v", err)
	}

	// Follow.
	if err := env.db.InsertFollowEvent(env.kp.PublicKeyBytes(), otherKP.PublicKeyBytes(), "follow", time.Now().UnixMilli()); err != nil {
		t.Fatalf("InsertFollowEvent follow: %v", err)
	}

	// Verify following.
	following, err := env.db.GetFollowing(env.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetFollowing: %v", err)
	}
	if len(following) != 1 {
		t.Fatalf("expected 1 following, got %d", len(following))
	}
	if !bytes.Equal(following[0], otherKP.PublicKeyBytes()) {
		t.Error("following wrong pubkey")
	}

	// Verify follower on the other side.
	followers, err := env.db.GetFollowers(otherKP.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetFollowers: %v", err)
	}
	if len(followers) != 1 {
		t.Fatalf("expected 1 follower, got %d", len(followers))
	}

	// Create a post from the followed user and verify it shows in feed.
	ctx := context.Background()
	otherCAS, err := content.NewContentStore(filepath.Join(t.TempDir(), "other_cas"))
	if err != nil {
		t.Fatalf("NewContentStore other: %v", err)
	}
	otherPostSvc := social.NewPostService(env.db, otherCAS, otherKP)
	otherPost, err := otherPostSvc.CreatePost(ctx, "Post from followed user", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost other: %v", err)
	}

	feed, err := env.db.GetFeed([][]byte{otherKP.PublicKeyBytes()}, 0, 10)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}
	if len(feed) != 1 {
		t.Fatalf("expected 1 feed post, got %d", len(feed))
	}
	if !bytes.Equal(feed[0].CID, otherPost.Id) {
		t.Error("feed contains wrong post")
	}

	// Unfollow.
	if err := env.db.InsertFollowEvent(env.kp.PublicKeyBytes(), otherKP.PublicKeyBytes(), "unfollow", time.Now().UnixMilli()); err != nil {
		t.Fatalf("InsertFollowEvent unfollow: %v", err)
	}

	// Verify no longer following.
	following, err = env.db.GetFollowing(env.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetFollowing after unfollow: %v", err)
	}
	if len(following) != 0 {
		t.Errorf("expected 0 following after unfollow, got %d", len(following))
	}
}

func TestEncryptedDMRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	// Create a recipient.
	recipientKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair recipient: %v", err)
	}
	if err := env.db.UpsertProfile(recipientKP.PublicKeyBytes(), "Recipient", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile recipient: %v", err)
	}

	// Sender sends a DM.
	senderDM := social.NewDMService(env.db, env.kp)
	plaintext := "This is a secret message!"
	dm, err := senderDM.SendDM(ctx, recipientKP.PublicKeyBytes(), plaintext)
	if err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	if len(dm.Id) == 0 {
		t.Fatal("DM ID should not be empty")
	}
	if len(dm.EncryptedContent) == 0 {
		t.Fatal("encrypted content should not be empty")
	}

	// Verify the encrypted content is not the plaintext.
	if bytes.Equal(dm.EncryptedContent, []byte(plaintext)) {
		t.Fatal("encrypted content should not equal plaintext")
	}

	// Recipient decrypts the DM.
	recipientDM := social.NewDMService(env.db, recipientKP)
	decrypted, err := recipientDM.DecryptDM(dm)
	if err != nil {
		t.Fatalf("DecryptDM: %v", err)
	}

	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}

	// Verify DM is stored in DB.
	conv, err := env.db.GetConversation(env.kp.PublicKeyBytes(), recipientKP.PublicKeyBytes(), 0, 10)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if len(conv) != 1 {
		t.Fatalf("expected 1 DM in conversation, got %d", len(conv))
	}
}

func TestProfileVersioning(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	profileSvc := social.NewProfileService(env.db, env.kp)

	// Create initial profile (version 1).
	p1, err := profileSvc.CreateProfile(ctx, "Alice", "First bio", "", nil, nil)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p1.Version != 1 {
		t.Errorf("Version = %d, want 1", p1.Version)
	}

	// Update profile (should be version 2).
	p2, err := profileSvc.UpdateProfile(ctx, "Alice Updated", "Second bio", "https://alice.com", nil, nil)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if p2.Version != 2 {
		t.Errorf("Version = %d, want 2", p2.Version)
	}

	// Verify the DB has the updated version.
	row, err := env.db.GetProfile(env.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if row.DisplayName != "Alice Updated" {
		t.Errorf("DisplayName = %q, want 'Alice Updated'", row.DisplayName)
	}
	if row.Version != 2 {
		t.Errorf("DB Version = %d, want 2", row.Version)
	}

	// Try to insert a stale profile (version 1) -- should not overwrite.
	if err := env.db.UpsertProfile(env.kp.PublicKeyBytes(), "Stale Alice", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile stale: %v", err)
	}
	row, _ = env.db.GetProfile(env.kp.PublicKeyBytes())
	if row.DisplayName != "Alice Updated" {
		t.Errorf("stale version overwrote profile: DisplayName = %q", row.DisplayName)
	}
}

func TestReactionDedup(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	postSvc := social.NewPostService(env.db, env.cas, env.kp)
	reactionSvc := social.NewReactionService(env.db, env.kp)

	// Create a post.
	post, err := postSvc.CreatePost(ctx, "Post to like twice", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Like the post.
	_, err = reactionSvc.CreateReaction(ctx, post.Id)
	if err != nil {
		t.Fatalf("first CreateReaction: %v", err)
	}

	// Like the same post again -- should be silently ignored by DB.
	_, err = reactionSvc.CreateReaction(ctx, post.Id)
	if err != nil {
		t.Fatalf("second CreateReaction: %v", err)
	}

	// Verify only one reaction exists.
	reactions, err := env.db.GetReactions(post.Id)
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(reactions) != 1 {
		t.Errorf("expected 1 reaction after dedup, got %d", len(reactions))
	}
}

func TestNotificationGeneration(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	notifSvc := social.NewNotificationService(env.db)

	// Create a second user.
	otherKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if err := env.db.UpsertProfile(otherKP.PublicKeyBytes(), "Liker", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	postSvc := social.NewPostService(env.db, env.cas, env.kp)

	// Create a post.
	post, err := postSvc.CreatePost(ctx, "Post for notifications", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Generate a like notification.
	if err := notifSvc.NotifyLike(otherKP.PublicKeyBytes(), post.Id, []byte("reaction_cid_xxxxxxxxxxxxxxxxxx")); err != nil {
		t.Fatalf("NotifyLike: %v", err)
	}

	// Generate a reply notification.
	if err := notifSvc.NotifyReply(otherKP.PublicKeyBytes(), post.Id, []byte("reply_cid_xxxxxxxxxxxxxxxxxxxxx")); err != nil {
		t.Fatalf("NotifyReply: %v", err)
	}

	// Generate a follow notification.
	if err := notifSvc.NotifyFollow(otherKP.PublicKeyBytes()); err != nil {
		t.Fatalf("NotifyFollow: %v", err)
	}

	// Verify notifications.
	notifs, err := notifSvc.GetNotifications(0, 10)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	if len(notifs) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(notifs))
	}

	// Check notification types.
	typeMap := make(map[string]int)
	for _, n := range notifs {
		typeMap[n.Type]++
	}
	if typeMap["like"] != 1 {
		t.Errorf("expected 1 'like' notification, got %d", typeMap["like"])
	}
	if typeMap["reply"] != 1 {
		t.Errorf("expected 1 'reply' notification, got %d", typeMap["reply"])
	}
	if typeMap["follow"] != 1 {
		t.Errorf("expected 1 'follow' notification, got %d", typeMap["follow"])
	}

	// Verify unread count.
	unread, err := env.db.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if unread != 3 {
		t.Errorf("unread = %d, want 3", unread)
	}

	// Mark all read and verify.
	if err := env.db.MarkAllRead(); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	unread, _ = env.db.UnreadCount()
	if unread != 0 {
		t.Errorf("unread after MarkAllRead = %d, want 0", unread)
	}
}
