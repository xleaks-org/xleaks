package content

import (
	"errors"
	"testing"

	pb "github.com/xleaks-org/xleaks/proto/gen"
)

func TestValidateMediaObjectRejectsOversizedImageDimensions(t *testing.T) {
	t.Parallel()

	obj := &pb.MediaObject{
		Cid:        []byte("media-cid"),
		Author:     make([]byte, Ed25519PublicKeySize),
		MimeType:   "image/png",
		Size:       1024,
		ChunkCount: 1,
		ChunkCids:  [][]byte{[]byte("chunk-cid")},
		Width:      MaxImageWidth,
		Height:     MaxImageHeight,
		Timestamp:  1,
		Signature:  make([]byte, Ed25519SignatureSize),
	}
	obj.Author[0] = 1

	err := ValidateMediaObject(obj, nil)
	if !errors.Is(err, ErrImageDimensionsTooLarge) {
		t.Fatalf("ValidateMediaObject error = %v, want ErrImageDimensionsTooLarge", err)
	}
}
