package social_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/social"
	"github.com/xleaks/xleaks/pkg/storage"
)

// testSetup creates a fresh DB, CAS, and key pair for each test.
type testSetup struct {
	db  *storage.DB
	cas *content.ContentStore
	kp  *identity.KeyPair
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

	cas, err := content.NewContentStore(filepath.Join(dir, "cas"))
	if err != nil {
		t.Fatalf("NewContentStore: %v", err)
	}

	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}

	// Create a profile for FK constraints.
	if err := db.UpsertProfile(kp.PublicKeyBytes(), "TestUser", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile: %v", err)
	}

	return &testSetup{db: db, cas: cas, kp: kp}
}

func TestCreatePostExtractsHashtags(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	postSvc := social.NewPostService(s.db, s.cas, s.kp)

	post, err := postSvc.CreatePost(ctx, "Hello #xleaks! Check out #decentralized #xleaks", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Tags should be deduplicated and lowercased.
	if len(post.Tags) != 2 {
		t.Fatalf("expected 2 unique tags, got %d: %v", len(post.Tags), post.Tags)
	}

	tagSet := make(map[string]bool)
	for _, tag := range post.Tags {
		tagSet[tag] = true
	}
	if !tagSet["xleaks"] {
		t.Error("missing tag 'xleaks'")
	}
	if !tagSet["decentralized"] {
		t.Error("missing tag 'decentralized'")
	}
}

func TestCreatePostSignsCorrectly(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	postSvc := social.NewPostService(s.db, s.cas, s.kp)

	post, err := postSvc.CreatePost(ctx, "Signed post", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Verify the signature is 64 bytes (ed25519).
	if len(post.Signature) != 64 {
		t.Errorf("signature length = %d, want 64", len(post.Signature))
	}

	// Verify the CID is not empty.
	if len(post.Id) == 0 {
		t.Fatal("post ID should not be empty")
	}

	// Verify the author matches.
	if !bytes.Equal(post.Author, s.kp.PublicKeyBytes()) {
		t.Error("post author does not match key pair public key")
	}

	// Validate through the validator with a real verifier.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidatePost(post, verifier); err != nil {
		t.Fatalf("ValidatePost failed on a properly signed post: %v", err)
	}
}

func TestCreateRepostSetsRepostOf(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	postSvc := social.NewPostService(s.db, s.cas, s.kp)

	// Create an original post first.
	original, err := postSvc.CreatePost(ctx, "Original content", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost (original): %v", err)
	}

	// Create a repost.
	repost, err := postSvc.CreateRepost(ctx, original.Id)
	if err != nil {
		t.Fatalf("CreateRepost: %v", err)
	}

	// Verify repost_of is set.
	if !bytes.Equal(repost.RepostOf, original.Id) {
		t.Error("repost.RepostOf should equal original post ID")
	}

	// Verify content is empty (reposts have no content).
	if repost.Content != "" {
		t.Errorf("repost Content = %q, want empty", repost.Content)
	}

	// Verify reply_to is not set.
	if len(repost.ReplyTo) != 0 {
		t.Error("repost should not have reply_to set")
	}
}

func TestCreateReactionDedup(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	postSvc := social.NewPostService(s.db, s.cas, s.kp)
	reactionSvc := social.NewReactionService(s.db, s.kp)

	// Create a post.
	post, err := postSvc.CreatePost(ctx, "Like me!", nil, nil)
	if err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	// Like it.
	r1, err := reactionSvc.CreateReaction(ctx, post.Id)
	if err != nil {
		t.Fatalf("first CreateReaction: %v", err)
	}
	if r1.ReactionType != "like" {
		t.Errorf("ReactionType = %q, want 'like'", r1.ReactionType)
	}

	// Like again -- should succeed (DB ignores duplicate).
	_, err = reactionSvc.CreateReaction(ctx, post.Id)
	if err != nil {
		t.Fatalf("second CreateReaction: %v", err)
	}

	// Verify only one reaction.
	reactions, err := s.db.GetReactions(post.Id)
	if err != nil {
		t.Fatalf("GetReactions: %v", err)
	}
	if len(reactions) != 1 {
		t.Errorf("expected 1 reaction, got %d", len(reactions))
	}

	// HasReacted should return true.
	if !reactionSvc.HasReacted(s.kp.PublicKeyBytes(), post.Id) {
		t.Error("HasReacted should return true after creating reaction")
	}
}

func TestProfileVersionIncrement(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	profileSvc := social.NewProfileService(s.db, s.kp)

	// Create initial profile.
	p1, err := profileSvc.CreateProfile(ctx, "TestUser", "bio1", "", nil, nil)
	if err != nil {
		t.Fatalf("CreateProfile: %v", err)
	}
	if p1.Version != 1 {
		t.Errorf("initial version = %d, want 1", p1.Version)
	}

	// Update profile -- should increment version.
	p2, err := profileSvc.UpdateProfile(ctx, "TestUserUpdated", "bio2", "https://test.com", nil, nil)
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if p2.Version != 2 {
		t.Errorf("updated version = %d, want 2", p2.Version)
	}

	// Update again.
	p3, err := profileSvc.UpdateProfile(ctx, "TestUserV3", "bio3", "", nil, nil)
	if err != nil {
		t.Fatalf("UpdateProfile v3: %v", err)
	}
	if p3.Version != 3 {
		t.Errorf("v3 version = %d, want 3", p3.Version)
	}

	// Verify DB has latest version.
	row, err := s.db.GetProfile(s.kp.PublicKeyBytes())
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if row.Version != 3 {
		t.Errorf("DB version = %d, want 3", row.Version)
	}
	if row.DisplayName != "TestUserV3" {
		t.Errorf("DB DisplayName = %q, want 'TestUserV3'", row.DisplayName)
	}
}

func TestDMEncryptDecrypt(t *testing.T) {
	s := setup(t)
	ctx := context.Background()

	// Create a recipient key pair with a profile.
	recipientKP, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair recipient: %v", err)
	}
	if err := s.db.UpsertProfile(recipientKP.PublicKeyBytes(), "Recipient", "", nil, nil, "", 1, time.Now().UnixMilli()); err != nil {
		t.Fatalf("UpsertProfile recipient: %v", err)
	}

	// Sender sends a DM.
	senderDM := social.NewDMService(s.db, s.kp)
	plaintext := "Top secret XLeaks message!"
	dm, err := senderDM.SendDM(ctx, recipientKP.PublicKeyBytes(), plaintext)
	if err != nil {
		t.Fatalf("SendDM: %v", err)
	}

	// Verify it's encrypted.
	if bytes.Equal(dm.EncryptedContent, []byte(plaintext)) {
		t.Fatal("encrypted content should not equal plaintext")
	}

	// Recipient decrypts.
	recipientDM := social.NewDMService(s.db, recipientKP)
	decrypted, err := recipientDM.DecryptDM(dm)
	if err != nil {
		t.Fatalf("DecryptDM: %v", err)
	}
	if decrypted != plaintext {
		t.Errorf("decrypted = %q, want %q", decrypted, plaintext)
	}

	// Wrong recipient should fail.
	wrongKP, _ := identity.GenerateKeyPair()
	wrongDM := social.NewDMService(s.db, wrongKP)
	_, err = wrongDM.DecryptDM(dm)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}
