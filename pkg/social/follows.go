package social

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/feed"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// FollowService signs, stores, and broadcasts follow state changes.
type FollowService struct {
	storage   *storage.DB
	feed      *feed.Manager
	identity  *identity.KeyPair
	publisher Publisher
}

// NewFollowService creates a new FollowService.
func NewFollowService(db *storage.DB, feedMgr *feed.Manager, kp *identity.KeyPair) *FollowService {
	return &FollowService{
		storage:  db,
		feed:     feedMgr,
		identity: kp,
	}
}

// SetIdentity updates the active key pair used for signing.
func (s *FollowService) SetIdentity(kp *identity.KeyPair) { s.identity = kp }

// SetPublisher configures the optional outbound P2P publisher.
func (s *FollowService) SetPublisher(publisher Publisher) { s.publisher = publisher }

// Follow records and broadcasts a follow event.
func (s *FollowService) Follow(ctx context.Context, target []byte) (*pb.FollowEvent, error) {
	return s.changeFollowState(ctx, target, "follow")
}

// Unfollow records and broadcasts an unfollow event.
func (s *FollowService) Unfollow(ctx context.Context, target []byte) (*pb.FollowEvent, error) {
	return s.changeFollowState(ctx, target, "unfollow")
}

func (s *FollowService) changeFollowState(ctx context.Context, target []byte, action string) (*pb.FollowEvent, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	event := &pb.FollowEvent{
		Author:    kp.PublicKeyBytes(),
		Target:    target,
		Action:    action,
		Timestamp: uint64(time.Now().UnixMilli()),
	}

	sigPayload, err := signingPayloadFollowEvent(event)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	sig, err := identity.SignProtoMessage(kp, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign follow event: %w", err)
	}
	event.Signature = sig

	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidateFollowEvent(event, verifier); err != nil {
		return nil, fmt.Errorf("validate follow event: %w", err)
	}

	timestamp := int64(event.Timestamp)
	switch action {
	case "follow":
		if err := s.feed.Follow(ctx, target, timestamp); err != nil {
			return nil, err
		}
	case "unfollow":
		if err := s.feed.Unfollow(target); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported follow action %q", action)
	}

	if err := s.storage.InsertFollowEvent(event.Author, event.Target, event.Action, timestamp); err != nil {
		return nil, fmt.Errorf("store follow event: %w", err)
	}
	if err := s.storage.UpdateFollowerCount(event.Author); err != nil {
		log.Printf("update own follow counts: %v", err)
	}
	if err := s.storage.UpdateFollowerCount(event.Target); err != nil {
		log.Printf("update target follow counts: %v", err)
	}

	if err := publishFollowEvent(ctx, s.publisher, event); err != nil {
		log.Printf("publish follow event: %v", err)
	}

	return event, nil
}

func signingPayloadFollowEvent(event *pb.FollowEvent) ([]byte, error) {
	clone := proto.Clone(event).(*pb.FollowEvent)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal follow event for signing: %w", err)
	}
	return data, nil
}
