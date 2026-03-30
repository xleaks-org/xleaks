package content

import (
	"errors"
	"fmt"
)

const (
	MaxImageWidth  = 12000
	MaxImageHeight = 12000
	MaxImagePixels = 40_000_000
)

var (
	ErrInvalidImageDimensions  = errors.New("image dimensions must be greater than zero")
	ErrImageDimensionsTooLarge = errors.New("image dimensions exceed supported limits")
)

// ValidateImageDimensions rejects images that would require unreasonable decode
// allocations or exceed the supported dimension envelope.
func ValidateImageDimensions(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidImageDimensions, width, height)
	}
	if width > MaxImageWidth || height > MaxImageHeight {
		return fmt.Errorf("%w: %dx%d exceeds %dx%d", ErrImageDimensionsTooLarge, width, height, MaxImageWidth, MaxImageHeight)
	}

	pixels := int64(width) * int64(height)
	if pixels > MaxImagePixels {
		return fmt.Errorf("%w: %dx%d exceeds %d pixels", ErrImageDimensionsTooLarge, width, height, MaxImagePixels)
	}
	return nil
}

// ValidateImageMetadataFields validates width/height values carried in media metadata.
func ValidateImageMetadataFields(width, height uint32) error {
	return ValidateImageDimensions(int(width), int(height))
}
