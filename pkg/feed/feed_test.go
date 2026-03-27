package feed

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
)

// testSetup creates a fresh DB and key pair for each test.
type testSetup struct {
	db *storage.DB
	kp *identity.KeyPair
}

func setup(t *testing.T) *testSetup {
	t.Helper()
	dir := t.TempDir()

	db, err := storage.NewDB(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("NewDB: %v", err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Create a profile for FK constraints.
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	return &testSetup{db: db, kp: kp}
}

// ---------- Manager tests ----------

func TestFollowAddsSubscription(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)

	targetKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	targetPubkey := targetKP.PublicKeyBytes()

	// Create profile for target so FK constraint passes.
	if err := s.db.UpsertProfile(targetPubkey, "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}

	ts := time.Now().UnixMilli()
	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, ts); err != nil {
		t.Fatalf("Follow: %v", err)
	}

	// Verify in-memory map is updated.
	if !m.IsFollowing(targetPubkey) {
		t.Error("IsFollowing should return true after Follow")
	}

	// Verify DB has the subscription.
	subs, err := s.db.GetSubscriptions(s.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	found := false
	for _, sub := range subs {
		if hex.EncodeToString(sub.Pubkey) == hex.EncodeToString(targetPubkey) {
			found = true
			break
		}
	}
	if !found {
		t.Error("subscription not found in DB after Follow")
	}
}

func TestFollowIsIdempotent(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)
	targetKP, _ := identity.GenerateKeyPair()
	targetPubkey := targetKP.PublicKeyBytes()
	if err := s.db.UpsertProfile(targetPubkey, "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}

	ts := time.Now().UnixMilli()
	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, ts); err != nil {
		t.Fatalf("Follow first: %v", err)
	}

	// Second follow should be a no-op (already in map).
	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, ts); err != nil {
		t.Fatalf("Follow second: %v", err)
	}

	if !m.IsFollowing(targetPubkey) {
		t.Error("IsFollowing should still return true")
	}
}

func TestFollowCallsOnSubscribe(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)
	var subscribedKey string
	m.OnSubscribe = func(_ context.Context, pubkeyHex string) error {
		subscribedKey = pubkeyHex
		return nil
	}

	targetKP, _ := identity.GenerateKeyPair()
	targetPubkey := targetKP.PublicKeyBytes()
	if err := s.db.UpsertProfile(targetPubkey, "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}

	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, time.Now().UnixMilli()); err != nil {
		t.Fatalf("Follow: %v", err)
	}

	wantKey := hex.EncodeToString(targetPubkey)
	if subscribedKey != wantKey {
		t.Errorf("OnSubscribe called with %q, want %q", subscribedKey, wantKey)
	}
}

func TestUnfollowRemovesSubscription(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)
	targetKP, _ := identity.GenerateKeyPair()
	targetPubkey := targetKP.PublicKeyBytes()
	if err := s.db.UpsertProfile(targetPubkey, "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}

	// Follow then unfollow.
	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, time.Now().UnixMilli()); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := m.Unfollow(s.kp.PublicKeyBytes(), targetPubkey); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}

	if m.IsFollowing(targetPubkey) {
		t.Error("IsFollowing should return false after Unfollow")
	}

	// Verify DB subscription was removed.
	subs, err := s.db.GetSubscriptions(s.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	for _, sub := range subs {
		if hex.EncodeToString(sub.Pubkey) == hex.EncodeToString(targetPubkey) {
			t.Error("subscription still in DB after Unfollow")
		}
	}
}

func TestUnfollowIsIdempotent(t *testing.T) {
	t.Parallel()
	s := setup(t)

	m := NewManager(s.db)
	targetKP, _ := identity.GenerateKeyPair()

	// Unfollow without ever following should be a no-op.
	if err := m.Unfollow(s.kp.PublicKeyBytes(), targetKP.PublicKeyBytes()); err != nil {
		t.Fatalf("Unfollow (not following): %v", err)
	}
}

func TestUnfollowCallsOnUnsubscribe(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)
	var unsubscribedKey string
	m.OnUnsubscribe = func(pubkeyHex string) error {
		unsubscribedKey = pubkeyHex
		return nil
	}

	targetKP, _ := identity.GenerateKeyPair()
	targetPubkey := targetKP.PublicKeyBytes()
	if err := s.db.UpsertProfile(targetPubkey, "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile target: %v", err)
	}

	if err := m.Follow(ctx, s.kp.PublicKeyBytes(), targetPubkey, time.Now().UnixMilli()); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := m.Unfollow(s.kp.PublicKeyBytes(), targetPubkey); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}

	wantKey := hex.EncodeToString(targetPubkey)
	if unsubscribedKey != wantKey {
		t.Errorf("OnUnsubscribe called with %q, want %q", unsubscribedKey, wantKey)
	}
}

func TestFollowedPubkeys(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)

	// Follow two targets.
	targets := make([]*identity.KeyPair, 2)
	for i := range targets {
		kp, _ := identity.GenerateKeyPair()
		targets[i] = kp
		if err := s.db.UpsertProfile(kp.PublicKeyBytes(), "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
			t.Fatalf("UpsertProfile target %d: %v", i, err)
		}
		if err := m.Follow(ctx, s.kp.PublicKeyBytes(), kp.PublicKeyBytes(), time.Now().UnixMilli()); err != nil {
			t.Fatalf("Follow target %d: %v", i, err)
		}
	}

	keys := m.FollowedPubkeys()
	if len(keys) != 2 {
		t.Fatalf("FollowedPubkeys len = %d, want 2", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, target := range targets {
		hexKey := hex.EncodeToString(target.PublicKeyBytes())
		if !keySet[hexKey] {
			t.Errorf("FollowedPubkeys missing %s", hexKey[:16])
		}
	}
}

func TestReloadSubscriptionsReconciles(t *testing.T) {
	t.Parallel()
	s := setup(t)
	ctx := context.Background()

	m := NewManager(s.db)

	var subscribed []string
	var unsubscribed []string
	m.OnSubscribe = func(_ context.Context, pubkeyHex string) error {
		subscribed = append(subscribed, pubkeyHex)
		return nil
	}
	m.OnUnsubscribe = func(pubkeyHex string) error {
		unsubscribed = append(unsubscribed, pubkeyHex)
		return nil
	}

	// Create three targets.
	targets := make([]*identity.KeyPair, 3)
	for i := range targets {
		kp, _ := identity.GenerateKeyPair()
		targets[i] = kp
		if err := s.db.UpsertProfile(kp.PublicKeyBytes(), "Target", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
			t.Fatalf("UpsertProfile: %v", err)
		}
	}

	// Manually add subscriptions to DB for targets 0 and 1.
	for i := 0; i < 2; i++ {
		if err := s.db.AddSubscription(s.kp.PublicKeyBytes(), targets[i].PublicKeyBytes(), time.Now().UnixMilli()); err != nil {
			t.Fatalf("AddSubscription: %v", err)
		}
	}

	// Pre-populate the in-memory map with targets 1 and 2 (simulating old state).
	m.subscribers[hex.EncodeToString(targets[1].PublicKeyBytes())] = true
	m.subscribers[hex.EncodeToString(targets[2].PublicKeyBytes())] = true

	// Reload should:
	//   - Subscribe to target 0 (in DB but not in memory)
	//   - Keep target 1 (in both)
	//   - Unsubscribe from target 2 (in memory but not in DB)
	if err := m.ReloadSubscriptions(ctx, s.kp.PublicKeyBytes()); err != nil {
		t.Fatalf("ReloadSubscriptions: %v", err)
	}

	// Check target 0 was subscribed.
	target0Hex := hex.EncodeToString(targets[0].PublicKeyBytes())
	foundSub := false
	for _, s := range subscribed {
		if s == target0Hex {
			foundSub = true
			break
		}
	}
	if !foundSub {
		t.Error("target 0 should have been subscribed via OnSubscribe")
	}

	// Check target 2 was unsubscribed.
	target2Hex := hex.EncodeToString(targets[2].PublicKeyBytes())
	foundUnsub := false
	for _, u := range unsubscribed {
		if u == target2Hex {
			foundUnsub = true
			break
		}
	}
	if !foundUnsub {
		t.Error("target 2 should have been unsubscribed via OnUnsubscribe")
	}

	// Verify final in-memory state.
	if !m.IsFollowing(targets[0].PublicKeyBytes()) {
		t.Error("target 0 should be in memory after reload")
	}
	if !m.IsFollowing(targets[1].PublicKeyBytes()) {
		t.Error("target 1 should be in memory after reload")
	}
	if m.IsFollowing(targets[2].PublicKeyBytes()) {
		t.Error("target 2 should NOT be in memory after reload")
	}
}

// ---------- Timeline tests ----------

func TestGetGlobalFeedReturnsAllPosts(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	// Insert some posts directly.
	now := time.Now().UnixMilli()
	for i := 0; i < 3; i++ {
		cid := make([]byte, 32)
		cid[0] = byte(i + 1)
		if err := s.db.InsertPost(cid, s.kp.PublicKeyBytes(), "Post content", nil, nil, now-int64(i*1000), make([]byte, 64)); err != nil {
			t.Fatalf("InsertPost %d: %v", i, err)
		}
	}

	entries, err := tl.GetGlobalFeed(0, 10)
	if err != nil {
		t.Fatalf("GetGlobalFeed: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("GetGlobalFeed len = %d, want 3", len(entries))
	}
}

func TestGetGlobalFeedDefaultsLimit(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	// Request with limit 0 should default to 20.
	entries, err := tl.GetGlobalFeed(0, 0)
	if err != nil {
		t.Fatalf("GetGlobalFeed: %v", err)
	}
	// Just verify no error; no posts in DB yet.
	if entries == nil {
		t.Error("expected non-nil (empty) slice")
	}
}

func TestGetGlobalFeedCapsLimit(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	// Request with limit > 100 should be capped.
	entries, err := tl.GetGlobalFeed(0, 200)
	if err != nil {
		t.Fatalf("GetGlobalFeed: %v", err)
	}
	// No posts, just ensuring no error.
	if entries == nil {
		t.Error("expected non-nil (empty) slice")
	}
}

func TestGetUserPostsFiltersByAuthor(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	// Create a second author.
	otherKP, _ := identity.GenerateKeyPair()
	if err := s.db.UpsertProfile(otherKP.PublicKeyBytes(), "Other", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile other: %v", err)
	}

	now := time.Now().UnixMilli()

	// Insert 2 posts by the main user.
	for i := 0; i < 2; i++ {
		cid := make([]byte, 32)
		cid[0] = byte(i + 1)
		if err := s.db.InsertPost(cid, s.kp.PublicKeyBytes(), "My post", nil, nil, now-int64(i*1000), make([]byte, 64)); err != nil {
			t.Fatalf("InsertPost main %d: %v", i, err)
		}
	}

	// Insert 1 post by the other user.
	otherCID := make([]byte, 32)
	otherCID[0] = 0xaa
	if err := s.db.InsertPost(otherCID, otherKP.PublicKeyBytes(), "Other post", nil, nil, now, make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost other: %v", err)
	}

	// GetUserPosts for main user should return 2.
	entries, err := tl.GetUserPosts(s.kp.PublicKeyBytes(), 0, 10)
	if err != nil {
		t.Fatalf("GetUserPosts: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("GetUserPosts len = %d, want 2", len(entries))
	}

	// All entries should have the same author.
	for _, e := range entries {
		if hex.EncodeToString(e.Post.Author) != hex.EncodeToString(s.kp.PublicKeyBytes()) {
			t.Error("entry author does not match requested user")
		}
	}
}

func TestGetFeedIncludesOwnAndFollowed(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	// Create a followed user.
	followedKP, _ := identity.GenerateKeyPair()
	if err := s.db.UpsertProfile(followedKP.PublicKeyBytes(), "Followed", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile followed: %v", err)
	}

	// Add subscription.
	if err := s.db.AddSubscription(s.kp.PublicKeyBytes(), followedKP.PublicKeyBytes(), time.Now().UnixMilli()); err != nil {
		t.Fatalf("AddSubscription: %v", err)
	}

	now := time.Now().UnixMilli()

	// Insert own post.
	ownCID := make([]byte, 32)
	ownCID[0] = 0x01
	if err := s.db.InsertPost(ownCID, s.kp.PublicKeyBytes(), "My post", nil, nil, now, make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost own: %v", err)
	}

	// Insert followed user's post.
	followedCID := make([]byte, 32)
	followedCID[0] = 0x02
	if err := s.db.InsertPost(followedCID, followedKP.PublicKeyBytes(), "Followed post", nil, nil, now-1000, make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost followed: %v", err)
	}

	// Create an unfollowed user and their post.
	unfollowedKP, _ := identity.GenerateKeyPair()
	if err := s.db.UpsertProfile(unfollowedKP.PublicKeyBytes(), "Unfollowed", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile unfollowed: %v", err)
	}
	unfollowedCID := make([]byte, 32)
	unfollowedCID[0] = 0x03
	if err := s.db.InsertPost(unfollowedCID, unfollowedKP.PublicKeyBytes(), "Not in feed", nil, nil, now-2000, make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost unfollowed: %v", err)
	}

	entries, err := tl.GetFeed(0, 50)
	if err != nil {
		t.Fatalf("GetFeed: %v", err)
	}

	// Should include own post and followed user's post but not unfollowed.
	if len(entries) != 2 {
		t.Fatalf("GetFeed len = %d, want 2", len(entries))
	}

	authorSet := make(map[string]bool)
	for _, e := range entries {
		authorSet[hex.EncodeToString(e.Post.Author)] = true
	}
	if !authorSet[hex.EncodeToString(s.kp.PublicKeyBytes())] {
		t.Error("feed should include own posts")
	}
	if !authorSet[hex.EncodeToString(followedKP.PublicKeyBytes())] {
		t.Error("feed should include followed user's posts")
	}
	if authorSet[hex.EncodeToString(unfollowedKP.PublicKeyBytes())] {
		t.Error("feed should NOT include unfollowed user's posts")
	}
}

func TestGetGlobalFeedPagination(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	now := time.Now().UnixMilli()

	// Insert 5 posts with distinct timestamps.
	for i := 0; i < 5; i++ {
		cid := make([]byte, 32)
		cid[0] = byte(i + 1)
		ts := now - int64(i*1000) // descending order: post 0 is newest
		if err := s.db.InsertPost(cid, s.kp.PublicKeyBytes(), "Post", nil, nil, ts, make([]byte, 64)); err != nil {
			t.Fatalf("InsertPost %d: %v", i, err)
		}
	}

	// First page.
	page1, err := tl.GetGlobalFeed(0, 3)
	if err != nil {
		t.Fatalf("GetGlobalFeed page1: %v", err)
	}
	if len(page1) != 3 {
		t.Fatalf("page1 len = %d, want 3", len(page1))
	}

	// Second page: use the oldest timestamp from page 1 as cursor.
	lastTS := page1[len(page1)-1].Post.Timestamp
	page2, err := tl.GetGlobalFeed(lastTS, 3)
	if err != nil {
		t.Fatalf("GetGlobalFeed page2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len = %d, want 2", len(page2))
	}
}

func TestTimelineEnrichesAuthorName(t *testing.T) {
	t.Parallel()
	s := setup(t)

	holder := identity.NewHolder(t.TempDir())
	holder.Set(s.kp)

	tl := NewTimeline(s.db, holder)

	now := time.Now().UnixMilli()
	cid := make([]byte, 32)
	cid[0] = 0x01
	if err := s.db.InsertPost(cid, s.kp.PublicKeyBytes(), "Hello", nil, nil, now, make([]byte, 64)); err != nil {
		t.Fatalf("InsertPost: %v", err)
	}

	entries, err := tl.GetGlobalFeed(0, 10)
	if err != nil {
		t.Fatalf("GetGlobalFeed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry")
	}

	// Author name should be the profile display name, not a hex string.
	if entries[0].AuthorName != "TestUser" {
		t.Errorf("AuthorName = %q, want 'TestUser'", entries[0].AuthorName)
	}
}

// ---------- nextBackoff tests ----------

func TestNextBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current time.Duration
		max     time.Duration
		wantMin time.Duration
		wantMax time.Duration
	}{
		{
			name:    "doubles with jitter",
			current: 30 * time.Second,
			max:     10 * time.Minute,
			wantMin: 60 * time.Second,            // 2x, no jitter
			wantMax: 60*time.Second + 15*time.Second, // 2x + half of current
		},
		{
			name:    "caps at max",
			current: 8 * time.Minute,
			max:     10 * time.Minute,
			wantMin: 10 * time.Minute, // capped
			wantMax: 10 * time.Minute,
		},
		{
			name:    "zero current",
			current: 0,
			max:     10 * time.Minute,
			wantMin: 0,
			wantMax: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := nextBackoff(tt.current, tt.max)
			if result < tt.wantMin {
				t.Errorf("nextBackoff(%v, %v) = %v, want >= %v", tt.current, tt.max, result, tt.wantMin)
			}
			if result > tt.wantMax {
				t.Errorf("nextBackoff(%v, %v) = %v, want <= %v", tt.current, tt.max, result, tt.wantMax)
			}
		})
	}
}
