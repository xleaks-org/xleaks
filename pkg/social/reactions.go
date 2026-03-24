package social

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ReactionService handles like creation and queries.
type ReactionService struct {
	storage   *storage.DB
	identity  *identity.KeyPair
	publisher Publisher
}

// NewReactionService creates a new ReactionService.
func NewReactionService(db *storage.DB, kp *identity.KeyPair) *ReactionService {
	return &ReactionService{
		storage:  db,
		identity: kp,
	}
}

func (s *ReactionService) SetIdentity(kp *identity.KeyPair) { s.identity = kp }

// SetPublisher configures the optional outbound P2P publisher.
func (s *ReactionService) SetPublisher(publisher Publisher) { s.publisher = publisher }

// CreateReaction creates a new "like" reaction on the given target post.
func (s *ReactionService) CreateReaction(ctx context.Context, targetCID []byte) (*pb.Reaction, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	reaction := &pb.Reaction{
		Author:       kp.PublicKeyBytes(),
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
	sig, err := identity.SignProtoMessage(kp, sigPayload)
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

	// Store in DB and update reaction count in a single transaction.
	err = s.storage.WithTransaction(func(tx *sql.Tx) error {
		if err := s.storage.InsertReactionTx(tx, cid, reaction.Author, reaction.Target, reaction.ReactionType, int64(reaction.Timestamp)); err != nil {
			return fmt.Errorf("store reaction: %w", err)
		}
		if err := s.storage.UpdateReactionCountTx(tx, targetCID); err != nil {
			return fmt.Errorf("update reaction count: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := publishReaction(ctx, s.publisher, reaction); err != nil {
		log.Printf("publish reaction: %v", err)
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
