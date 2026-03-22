package social

import (
	"context"
	"fmt"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ReactionService handles like creation and queries.
type ReactionService struct {
	storage  *storage.DB
	identity *identity.KeyPair
}

// NewReactionService creates a new ReactionService.
func NewReactionService(db *storage.DB, kp *identity.KeyPair) *ReactionService {
	return &ReactionService{
		storage:  db,
		identity: kp,
	}
}

func (s *ReactionService) SetIdentity(kp *identity.KeyPair) { s.identity = kp }

// CreateReaction creates a new "like" reaction on the given target post.
func (s *ReactionService) CreateReaction(ctx context.Context, targetCID []byte) (*pb.Reaction, error) {
	reaction := &pb.Reaction{
		Author:       s.identity.PublicKeyBytes(),
		Target:       targetCID,
		ReactionType: "like",
		Timestamp:    uint64(time.Now().UnixMilli()),
	}

	// Compute signing payload (id and signature zeroed).
	sigPayload, err := signingPayloadReaction(reaction)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	// Sign.
	sig, err := identity.SignProtoMessage(s.identity, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign reaction: %w", err)
	}
	reaction.Signature = sig

	// Compute CID from signing payload.
	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		return nil, fmt.Errorf("compute CID: %w", err)
	}
	reaction.Id = cid

	// Store in DB (dedup handled by DB via INSERT OR IGNORE + UNIQUE constraint).
	if err := s.storage.InsertReaction(cid, reaction.Author, reaction.Target, reaction.ReactionType, int64(reaction.Timestamp)); err != nil {
		return nil, fmt.Errorf("store reaction: %w", err)
	}

	// Update reaction count on the target post.
	if err := s.storage.UpdateReactionCount(targetCID); err != nil {
		return nil, fmt.Errorf("update reaction count: %w", err)
	}

	return reaction, nil
}

// HasReacted returns true if the author has already liked the target post.
func (s *ReactionService) HasReacted(author, target []byte) bool {
	return s.storage.HasReacted(author, target, "like")
}

// signingPayloadReaction returns the serialized Reaction with id and signature zeroed.
func signingPayloadReaction(reaction *pb.Reaction) ([]byte, error) {
	clone := proto.Clone(reaction).(*pb.Reaction)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal reaction for signing: %w", err)
	}
	return data, nil
}
