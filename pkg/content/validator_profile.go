package content

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ValidateProfile validates all rules for a Profile message.
func ValidateProfile(profile *pb.Profile, verify SignatureVerifier) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	if len(profile.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(profile.Author))
	}

	if profile.DisplayName == "" {
		return fmt.Errorf("display_name must not be empty")
	}

	if utf8.RuneCountInString(profile.DisplayName) > MaxDisplayNameLength {
		return fmt.Errorf("display_name exceeds %d characters", MaxDisplayNameLength)
	}

	if utf8.RuneCountInString(profile.Bio) > MaxBioLength {
		return fmt.Errorf("bio exceeds %d characters", MaxBioLength)
	}

	if len(profile.Website) > MaxWebsiteLength {
		return fmt.Errorf("website exceeds %d characters", MaxWebsiteLength)
	}

	if err := validateTimestamp(profile.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(profile.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(profile.Signature))
	}

	sigPayload, err := profileSigningPayload(profile)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(profile.Author, sigPayload, profile.Signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// ValidateFollowEvent validates all rules for a FollowEvent message.
func ValidateFollowEvent(event *pb.FollowEvent, verify SignatureVerifier) error {
	if event == nil {
		return fmt.Errorf("follow event is nil")
	}

	if len(event.Author) != Ed25519PublicKeySize {
		return fmt.Errorf("author must be %d bytes, got %d", Ed25519PublicKeySize, len(event.Author))
	}

	if len(event.Target) != Ed25519PublicKeySize {
		return fmt.Errorf("target must be %d bytes, got %d", Ed25519PublicKeySize, len(event.Target))
	}

	if bytes.Equal(event.Author, event.Target) {
		return fmt.Errorf("a user cannot follow themselves")
	}

	if event.Action != "follow" && event.Action != "unfollow" {
		return fmt.Errorf("action must be \"follow\" or \"unfollow\", got %q", event.Action)
	}

	if err := validateTimestamp(event.Timestamp); err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	if len(event.Signature) != Ed25519SignatureSize {
		return fmt.Errorf("signature must be %d bytes, got %d", Ed25519SignatureSize, len(event.Signature))
	}

	sigPayload, err := followEventSigningPayload(event)
	if err != nil {
		return fmt.Errorf("failed to compute signing payload: %w", err)
	}

	if verify != nil && !verify(event.Author, sigPayload, event.Signature) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

// profileSigningPayload returns the serialized Profile with signature zeroed.
func profileSigningPayload(profile *pb.Profile) ([]byte, error) {
	clone := proto.Clone(profile).(*pb.Profile)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile: %w", err)
	}
	return data, nil
}

// followEventSigningPayload returns the serialized FollowEvent with signature zeroed.
func followEventSigningPayload(event *pb.FollowEvent) ([]byte, error) {
	clone := proto.Clone(event).(*pb.FollowEvent)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal follow event: %w", err)
	}
	return data, nil
}
