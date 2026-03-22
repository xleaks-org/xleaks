package social

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"time"

	"github.com/xleaks/xleaks/pkg/content"
	"github.com/xleaks/xleaks/pkg/identity"
	"github.com/xleaks/xleaks/pkg/storage"
	pb "github.com/xleaks/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// DMService handles encrypted direct message sending and receiving.
type DMService struct {
	storage  *storage.DB
	identity *identity.KeyPair
}

// NewDMService creates a new DMService.
func NewDMService(db *storage.DB, kp *identity.KeyPair) *DMService {
	return &DMService{
		storage:  db,
		identity: kp,
	}
}

func (s *DMService) SetIdentity(kp *identity.KeyPair) { s.identity = kp }

// SendDM encrypts and sends a direct message to the given recipient.
func (s *DMService) SendDM(ctx context.Context, recipientPubKey []byte, plaintext string) (*pb.DirectMessage, error) {
	// Encrypt the message.
	ciphertext, nonce, err := identity.EncryptDM(
		s.identity.PrivateKey,
		ed25519.PublicKey(recipientPubKey),
		[]byte(plaintext),
	)
	if err != nil {
		return nil, fmt.Errorf("encrypt DM: %w", err)
	}

	dm := &pb.DirectMessage{
		Author:           s.identity.PublicKeyBytes(),
		Recipient:        recipientPubKey,
		EncryptedContent: ciphertext,
		Nonce:            nonce[:],
		Timestamp:        uint64(time.Now().UnixMilli()),
	}

	// Compute signing payload (id and signature zeroed).
	sigPayload, err := signingPayloadDM(dm)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	// Sign.
	sig, err := identity.SignProtoMessage(s.identity, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign DM: %w", err)
	}
	dm.Signature = sig

	// Compute CID from signing payload.
	cid, err := content.ComputeCID(sigPayload)
	if err != nil {
		return nil, fmt.Errorf("compute CID: %w", err)
	}
	dm.Id = cid

	// Store in DB.
	if err := s.storage.InsertDM(cid, dm.Author, dm.Recipient, dm.EncryptedContent, dm.Nonce, int64(dm.Timestamp)); err != nil {
		return nil, fmt.Errorf("store DM: %w", err)
	}

	return dm, nil
}

// DecryptDM decrypts a direct message intended for us.
func (s *DMService) DecryptDM(dm *pb.DirectMessage) (string, error) {
	if dm == nil {
		return "", fmt.Errorf("direct message is nil")
	}

	// Verify this message is for us.
	if !bytes.Equal(dm.Recipient, s.identity.PublicKeyBytes()) {
		return "", fmt.Errorf("message is not addressed to us")
	}

	// Convert the nonce slice to a fixed-size array.
	if len(dm.Nonce) != 24 {
		return "", fmt.Errorf("invalid nonce length: got %d, want 24", len(dm.Nonce))
	}
	var nonce [24]byte
	copy(nonce[:], dm.Nonce)

	// Decrypt.
	plaintext, err := identity.DecryptDM(
		s.identity.PrivateKey,
		ed25519.PublicKey(dm.Author),
		dm.EncryptedContent,
		nonce,
	)
	if err != nil {
		return "", fmt.Errorf("decrypt DM: %w", err)
	}

	return string(plaintext), nil
}

// HandleIncomingDM validates an incoming DM, stores it, and creates a notification.
func (s *DMService) HandleIncomingDM(dm *pb.DirectMessage) error {
	if dm == nil {
		return fmt.Errorf("direct message is nil")
	}

	// Validate the DM.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidateDirectMessage(dm, verifier); err != nil {
		return fmt.Errorf("validate DM: %w", err)
	}

	// Store in DB.
	if err := s.storage.InsertDM(dm.Id, dm.Author, dm.Recipient, dm.EncryptedContent, dm.Nonce, int64(dm.Timestamp)); err != nil {
		return fmt.Errorf("store incoming DM: %w", err)
	}

	// Create notification if the DM is addressed to us.
	if bytes.Equal(dm.Recipient, s.identity.PublicKeyBytes()) {
		if err := s.storage.InsertNotification("dm", dm.Author, nil, dm.Id, time.Now().UnixMilli()); err != nil {
			return fmt.Errorf("create DM notification: %w", err)
		}
	}

	return nil
}

// signingPayloadDM returns the serialized DirectMessage with id and signature zeroed.
func signingPayloadDM(dm *pb.DirectMessage) ([]byte, error) {
	clone := proto.Clone(dm).(*pb.DirectMessage)
	clone.Id = nil
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal DM for signing: %w", err)
	}
	return data, nil
}
