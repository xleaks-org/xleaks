package content

import (
	"fmt"

	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ValidateMediaObject validates a media metadata descriptor.
func ValidateMediaObject(obj *pb.MediaObject, verify SignatureVerifier) error {
	if obj == nil {
		return fmt.Errorf("media object is nil")
	}
	if len(obj.Cid) == 0 {
		return fmt.Errorf("cid must not be empty")
	}
	if len(obj.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(obj.Author))
	}
	if err := ValidateMediaType(obj.MimeType); err != nil {
		return err
	}
	if obj.Size == 0 {
		return fmt.Errorf("size must be greater than zero")
	}
	if obj.Size > MaxMediaSize {
		return fmt.Errorf("size exceeds maximum of %d bytes", MaxMediaSize)
	}
	if obj.ChunkCount == 0 {
		return fmt.Errorf("chunk_count must be greater than zero")
	}
	if len(obj.ChunkCids) != int(obj.ChunkCount) {
		return fmt.Errorf("chunk_cids length must equal chunk_count")
	}
	if err := validateTimestamp(obj.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}
	if len(obj.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(obj.Signature))
	}

	sigPayload, err := mediaObjectSigningPayload(obj)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}
	if verify != nil && !verify(obj.Author, sigPayload, obj.Signature) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

// ValidateMediaChunk validates a media chunk payload.
func ValidateMediaChunk(chunk *pb.MediaChunk) error {
	if chunk == nil {
		return fmt.Errorf("media chunk is nil")
	}
	if len(chunk.Cid) == 0 {
		return fmt.Errorf("cid must not be empty")
	}
	if len(chunk.ParentCid) == 0 {
		return fmt.Errorf("parent_cid must not be empty")
	}
	if len(chunk.Data) == 0 {
		return fmt.Errorf("data must not be empty")
	}
	if len(chunk.Data) > ChunkSize {
		return fmt.Errorf("data exceeds chunk size of %d bytes", ChunkSize)
	}
	return verifyCID(chunk.Cid, chunk.Data)
}

func mediaObjectSigningPayload(obj *pb.MediaObject) ([]byte, error) {
	clone := proto.Clone(obj).(*pb.MediaObject)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal media object: %w", err)
	}
	return data, nil
}
