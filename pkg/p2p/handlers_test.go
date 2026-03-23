package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// Mock implementations
// ---------------------------------------------------------------------------

type mockStorage struct {
	posts       map[string]bool
	reactions   map[string]bool
	follows     []followRecord
	dms         []dmRecord
	profiles    map[string]profileRecord
	insertErr   error
	profilesMap map[string]profileRecord
}

type followRecord struct {
	Author, Target []byte
	Action         string
	Timestamp      int64
}

type dmRecord struct {
	CID, Author, Recipient, EncryptedContent, Nonce []byte
	Timestamp                                       int64
}

type profileRecord struct {
	Version uint64
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		posts:       make(map[string]bool),
		reactions:   make(map[string]bool),
		profiles:    make(map[string]profileRecord),
		profilesMap: make(map[string]profileRecord),
	}
}

func (m *mockStorage) InsertPost(cid, author []byte, content string, replyTo, repostOf []byte, timestamp int64, signature []byte) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.posts[string(cid)] = true
	return nil
}

func (m *mockStorage) UpsertProfile(pubkey []byte, displayName, bio string, avatarCID, bannerCID []byte, website string, version uint64, updatedAt int64) error {
	m.profiles[string(pubkey)] = profileRecord{Version: version}
	return nil
}

func (m *mockStorage) InsertReaction(cid, author, target []byte, reactionType string, timestamp int64) error {
	m.reactions[string(cid)] = true
	return nil
}

func (m *mockStorage) InsertFollowEvent(author, target []byte, action string, timestamp int64) error {
	m.follows = append(m.follows, followRecord{Author: author, Target: target, Action: action, Timestamp: timestamp})
	return nil
}

func (m *mockStorage) InsertDM(cid, author, recipient, encryptedContent, nonce []byte, timestamp int64) error {
	m.dms = append(m.dms, dmRecord{CID: cid, Author: author, Recipient: recipient, EncryptedContent: encryptedContent, Nonce: nonce, Timestamp: timestamp})
	return nil
}

func (m *mockStorage) PostExists(cid []byte) bool {
	return m.posts[string(cid)]
}

func (m *mockStorage) GetProfileVersion(pubkey []byte) (uint64, bool, error) {
	p, ok := m.profiles[string(pubkey)]
	if !ok {
		return 0, false, nil
	}
	return p.Version, true, nil
}

func (m *mockStorage) UpdateReactionCount(_ []byte) error {
	return nil
}

type mockCAS struct {
	data map[string][]byte
}

func newMockCAS() *mockCAS {
	return &mockCAS{data: make(map[string][]byte)}
}

func (m *mockCAS) Put(cid []byte, data []byte) error {
	m.data[string(cid)] = data
	return nil
}

func (m *mockCAS) Has(cid []byte) bool {
	_, ok := m.data[string(cid)]
	return ok
}

type mockNotifier struct {
	likes   []notifRecord
	replies []notifRecord
	follows [][]byte
	dmNotif [][]byte
}

type notifRecord struct {
	Actor, Target, Related []byte
}

func (m *mockNotifier) NotifyLike(actor, targetCID, reactionCID []byte) error {
	m.likes = append(m.likes, notifRecord{Actor: actor, Target: targetCID, Related: reactionCID})
	return nil
}

func (m *mockNotifier) NotifyReply(actor, targetCID, replyCID []byte) error {
	m.replies = append(m.replies, notifRecord{Actor: actor, Target: targetCID, Related: replyCID})
	return nil
}

func (m *mockNotifier) NotifyFollow(actor []byte) error {
	m.follows = append(m.follows, actor)
	return nil
}

func (m *mockNotifier) NotifyDM(actor []byte) error {
	m.dmNotif = append(m.dmNotif, actor)
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeKeyPair(t *testing.T) *identity.KeyPair {
	t.Helper()
	kp, err := identity.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return kp
}

func nowMillis() uint64 {
	return uint64(time.Now().UnixMilli())
}

// signedPost creates a valid, signed Post and returns it along with the
// serialized Envelope bytes ready for HandleMessage.
func signedPost(t *testing.T, kp *identity.KeyPair, text string, replyTo []byte) ([]byte, *pb.Post) {
	t.Helper()
	post := &pb.Post{
		Author:    kp.PublicKeyBytes(),
		Content:   text,
		Timestamp: nowMillis(),
		ReplyTo:   replyTo,
	}

	// Compute signing payload (id and signature zeroed).
	payload, err := proto.Marshal(&pb.Post{
		Author:    post.Author,
		Content:   post.Content,
		Timestamp: post.Timestamp,
		ReplyTo:   post.ReplyTo,
	})
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}

	sig, err := identity.SignProtoMessage(kp, payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	post.Signature = sig

	cid, err := content.ComputeCID(payload)
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	post.Id = cid

	env := &pb.Envelope{Payload: &pb.Envelope_Post{Post: post}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return data, post
}

// signedReaction creates a valid, signed Reaction envelope.
func signedReaction(t *testing.T, kp *identity.KeyPair, targetCID []byte) ([]byte, *pb.Reaction) {
	t.Helper()
	reaction := &pb.Reaction{
		Author:       kp.PublicKeyBytes(),
		Target:       targetCID,
		ReactionType: "like",
		Timestamp:    nowMillis(),
	}

	payload, err := proto.Marshal(&pb.Reaction{
		Author:       reaction.Author,
		Target:       reaction.Target,
		ReactionType: reaction.ReactionType,
		Timestamp:    reaction.Timestamp,
	})
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}

	sig, err := identity.SignProtoMessage(kp, payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	reaction.Signature = sig

	cid, err := content.ComputeCID(payload)
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	reaction.Id = cid

	env := &pb.Envelope{Payload: &pb.Envelope_Reaction{Reaction: reaction}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return data, reaction
}

// signedProfile creates a valid, signed Profile envelope.
func signedProfile(t *testing.T, kp *identity.KeyPair, displayName string, version uint64) ([]byte, *pb.Profile) {
	t.Helper()
	profile := &pb.Profile{
		Author:      kp.PublicKeyBytes(),
		DisplayName: displayName,
		Bio:         "test bio",
		Version:     version,
		Timestamp:   nowMillis(),
	}

	payload, err := proto.Marshal(&pb.Profile{
		Author:      profile.Author,
		DisplayName: profile.DisplayName,
		Bio:         profile.Bio,
		Version:     profile.Version,
		Timestamp:   profile.Timestamp,
	})
	if err != nil {
		t.Fatalf("marshal signing payload: %v", err)
	}

	sig, err := identity.SignProtoMessage(kp, payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	profile.Signature = sig

	env := &pb.Envelope{Payload: &pb.Envelope_Profile{Profile: profile}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return data, profile
}

func newProcessor() (*MessageProcessor, *mockStorage, *mockCAS, *mockNotifier) {
	db := newMockStorage()
	cas := newMockCAS()
	notif := &mockNotifier{}
	mp := NewMessageProcessor(db, cas, notif)
	return mp, db, cas, notif
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestHandleMessage_ValidPost_IsStored(t *testing.T) {
	mp, db, cas, _ := newProcessor()
	kp := makeKeyPair(t)

	data, post := signedPost(t, kp, "Hello world", nil)

	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if !db.PostExists(post.Id) {
		t.Error("expected post to be stored in DB")
	}
	if !cas.Has(post.Id) {
		t.Error("expected post to be stored in CAS")
	}
}

func TestHandleMessage_InvalidSignature_Rejected(t *testing.T) {
	mp, db, _, _ := newProcessor()
	kp := makeKeyPair(t)

	_, post := signedPost(t, kp, "Hello world", nil)

	// Corrupt the signature.
	post.Signature[0] ^= 0xFF

	env := &pb.Envelope{Payload: &pb.Envelope_Post{Post: post}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := mp.HandleMessage(context.Background(), data); err == nil {
		t.Fatal("expected error for invalid signature")
	}

	if db.PostExists(post.Id) {
		t.Error("invalid post should not be stored")
	}
}

func TestHandleMessage_DuplicatePost_Ignored(t *testing.T) {
	mp, _, cas, _ := newProcessor()
	kp := makeKeyPair(t)

	data, post := signedPost(t, kp, "Hello world", nil)

	// First time: should succeed.
	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("first HandleMessage: %v", err)
	}

	casBefore := len(cas.data)

	// Second time: should be silently ignored (dedup).
	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("second HandleMessage: %v", err)
	}

	// CAS should not have grown (no new entry).
	if len(cas.data) != casBefore {
		t.Error("duplicate post should not create new CAS entry")
	}
	_ = post
}

func TestHandleMessage_ValidReaction_CreatesNotification(t *testing.T) {
	mp, _, _, notif := newProcessor()
	kp := makeKeyPair(t)

	// Create a fake target CID.
	targetCID := []byte("fake-target-cid-for-test-32bytes")

	data, _ := signedReaction(t, kp, targetCID)

	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(notif.likes) != 1 {
		t.Fatalf("expected 1 like notification, got %d", len(notif.likes))
	}
}

func TestHandleMessage_InvalidReactionType_Rejected(t *testing.T) {
	mp, _, _, _ := newProcessor()
	kp := makeKeyPair(t)

	// Build a reaction with an invalid type.
	reaction := &pb.Reaction{
		Author:       kp.PublicKeyBytes(),
		Target:       []byte("fake-target-cid-for-test-32bytes"),
		ReactionType: "dislike", // Invalid: only "like" is allowed.
		Timestamp:    nowMillis(),
	}

	payload, err := proto.Marshal(&pb.Reaction{
		Author:       reaction.Author,
		Target:       reaction.Target,
		ReactionType: reaction.ReactionType,
		Timestamp:    reaction.Timestamp,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	sig, err := identity.SignProtoMessage(kp, payload)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	reaction.Signature = sig

	cid, err := content.ComputeCID(payload)
	if err != nil {
		t.Fatalf("ComputeCID: %v", err)
	}
	reaction.Id = cid

	env := &pb.Envelope{Payload: &pb.Envelope_Reaction{Reaction: reaction}}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	if err := mp.HandleMessage(context.Background(), data); err == nil {
		t.Fatal("expected error for invalid reaction type")
	}
}

func TestHandleMessage_ProfileUpdate_HigherVersionAccepted(t *testing.T) {
	mp, db, _, _ := newProcessor()
	kp := makeKeyPair(t)

	// Insert version 1.
	data1, _ := signedProfile(t, kp, "Alice v1", 1)
	if err := mp.HandleMessage(context.Background(), data1); err != nil {
		t.Fatalf("HandleMessage v1: %v", err)
	}

	p, ok := db.profiles[string(kp.PublicKeyBytes())]
	if !ok {
		t.Fatal("profile not found after v1 insert")
	}
	if p.Version != 1 {
		t.Fatalf("expected version 1, got %d", p.Version)
	}

	// Insert version 2: should be accepted.
	data2, _ := signedProfile(t, kp, "Alice v2", 2)
	if err := mp.HandleMessage(context.Background(), data2); err != nil {
		t.Fatalf("HandleMessage v2: %v", err)
	}

	p2 := db.profiles[string(kp.PublicKeyBytes())]
	if p2.Version != 2 {
		t.Fatalf("expected version 2 after update, got %d", p2.Version)
	}
}

func TestHandleMessage_ProfileUpdate_LowerVersionIgnored(t *testing.T) {
	mp, db, _, _ := newProcessor()
	kp := makeKeyPair(t)

	// Insert version 5.
	data5, _ := signedProfile(t, kp, "Alice v5", 5)
	if err := mp.HandleMessage(context.Background(), data5); err != nil {
		t.Fatalf("HandleMessage v5: %v", err)
	}

	// Try to insert version 3: should be silently ignored.
	data3, _ := signedProfile(t, kp, "Alice v3", 3)
	if err := mp.HandleMessage(context.Background(), data3); err != nil {
		t.Fatalf("HandleMessage v3: %v", err)
	}

	p := db.profiles[string(kp.PublicKeyBytes())]
	if p.Version != 5 {
		t.Fatalf("expected version to remain 5, got %d", p.Version)
	}
}

func TestHandleMessage_Reply_CreatesNotification(t *testing.T) {
	mp, _, _, notif := newProcessor()
	kp := makeKeyPair(t)

	parentCID := []byte("parent-post-cid-for-test")
	data, _ := signedPost(t, kp, "This is a reply", parentCID)

	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	if len(notif.replies) != 1 {
		t.Fatalf("expected 1 reply notification, got %d", len(notif.replies))
	}
}

func TestHandleMessage_InvalidEnvelope_ReturnsError(t *testing.T) {
	mp, _, _, _ := newProcessor()

	if err := mp.HandleMessage(context.Background(), []byte("not a valid protobuf")); err == nil {
		t.Fatal("expected error for invalid protobuf data")
	}
}

func TestHandleMessage_UnknownPayload_Ignored(t *testing.T) {
	mp, _, _, _ := newProcessor()

	// An empty envelope has no payload set.
	env := &pb.Envelope{}
	data, err := proto.Marshal(env)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if err := mp.HandleMessage(context.Background(), data); err != nil {
		t.Fatalf("expected nil for unknown payload, got: %v", err)
	}
}
