package content

import (
	"bytes"
	"fmt"

	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ValidateDirectMessage validates all rules for a DirectMessage.
func ValidateDirectMessage(dm *pb.DirectMessage, verify SignatureVerifier) error {
	if dm == nil {
		return fmt.Errorf("direct message is nil")
	}
	if len(dm.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(dm.Author))
	}
	if len(dm.Recipient) != Ed25519PublicKeySize {
		return fmt.Errorf("recipient must be %d bytes, got %d", Ed25519PublicKeySize, len(dm.Recipient))
	}
	if bytes.Equal(dm.Author, dm.Recipient) {
		return fmt.Errorf("author and recipient must not be the same")
	}
	if len(dm.EncryptedContent) == 0 {
		return fmt.Errorf("encrypted_content must not be empty")
	}
	if len(dm.Nonce) != NaClNonceSize {
		return fmt.Errorf("nonce must be %d bytes, got %d", NaClNonceSize, len(dm.Nonce))
	}
	if err := validateTimestamp(dm.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(dm.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(dm.Signature))
	}

	sigPayload, err := dmSigningPayload(dm)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}
	if verify != nil && !verify(dm.Author, sigPayload, dm.Signature) {
		return fmt.Errorf("invalid signature")
	}
	return verifyCID(dm.Id, sigPayload)
}

// ValidateReaction validates all rules for a Reaction message.
func ValidateReaction(reaction *pb.Reaction, verify SignatureVerifier) error {
	if reaction == nil {
		return fmt.Errorf("reaction is nil")
	}
	if len(reaction.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(reaction.Author))
	}
	if len(reaction.Target) == 0 {
		return fmt.Errorf("target must not be empty")
	}
	if reaction.ReactionType != "like" {
		return fmt.Errorf("reaction_type must be \"like\", got %q", reaction.ReactionType)
	}
	if err := validateTimestamp(reaction.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(reaction.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(reaction.Signature))
	}

	sigPayload, err := reactionSigningPayload(reaction)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}
	if verify != nil && !verify(reaction.Author, sigPayload, reaction.Signature) {
		return fmt.Errorf("invalid signature")
	}
	return verifyCID(reaction.Id, sigPayload)
}

// dmSigningPayload returns the serialized DirectMessage with id and signature zeroed.
func dmSigningPayload(dm *pb.DirectMessage) ([]byte, error) {
	clone := proto.Clone(dm).(*pb.DirectMessage)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal direct message: %w", err)
	}
	return data, nil
}

// reactionSigningPayload returns the serialized Reaction with id and signature zeroed.
func reactionSigningPayload(reaction *pb.Reaction) ([]byte, error) {
	clone := proto.Clone(reaction).(*pb.Reaction)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal reaction: %w", err)
	}
	return data, nil
}
