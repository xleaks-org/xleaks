package social

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/storage"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// ProfileService handles profile creation, updates, and retrieval.
type ProfileService struct {
	storage   *storage.DB
	identity  *identity.KeyPair
	publisher Publisher
}

// NewProfileService creates a new ProfileService.
func NewProfileService(db *storage.DB, kp *identity.KeyPair) *ProfileService {
	return &ProfileService{
		storage:  db,
		identity: kp,
	}
}

func (s *ProfileService) SetIdentity(kp *identity.KeyPair) { s.identity = kp }

// SetPublisher configures the optional outbound P2P publisher.
func (s *ProfileService) SetPublisher(publisher Publisher) { s.publisher = publisher }

// CreateProfile creates a new profile (version 1).
func (s *ProfileService) CreateProfile(ctx context.Context, displayName, bio, website string, avatarCID, bannerCID []byte) (*pb.Profile, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	profile := &pb.Profile{
		Author:      kp.PublicKeyBytes(),
		DisplayName: displayName,
		Bio:         bio,
		Website:     website,
		AvatarCid:   avatarCID,
		BannerCid:   bannerCID,
		Version:     1,
		Timestamp:   uint64(time.Now().UnixMilli()),
	}

	// Sign the profile.
	sigPayload, err := signingPayloadProfile(profile)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	sig, err := identity.SignProtoMessage(kp, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign profile: %w", err)
	}
	profile.Signature = sig

	// Validate the profile.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidateProfile(profile, verifier); err != nil {
		return nil, fmt.Errorf("validate profile: %w", err)
	}

	// Store in DB.
	if err := s.storage.UpsertProfile(
		profile.Author,
		profile.DisplayName,
		profile.Bio,
		profile.AvatarCid,
		profile.BannerCid,
		profile.Website,
		profile.Version,
		int64(profile.Timestamp),
	); err != nil {
		return nil, fmt.Errorf("store profile: %w", err)
	}

	if err := publishProfile(ctx, s.publisher, profile); err != nil {
		log.Printf("publish profile: %v", err)
	}

	return profile, nil
}

// UpdateProfile updates the user's profile by incrementing the version number.
func (s *ProfileService) UpdateProfile(ctx context.Context, displayName, bio, website string, avatarCID, bannerCID []byte) (*pb.Profile, error) {
	kp, err := activeIdentity(s.identity)
	if err != nil {
		return nil, err
	}

	// Get current version.
	var currentVersion uint64
	existing, err := s.storage.GetProfile(kp.PublicKeyBytes())
	if err != nil {
		return nil, fmt.Errorf("get current profile: %w", err)
	}
	if existing != nil {
		currentVersion = existing.Version
	}

	profile := &pb.Profile{
		Author:      kp.PublicKeyBytes(),
		DisplayName: displayName,
		Bio:         bio,
		Website:     website,
		AvatarCid:   avatarCID,
		BannerCid:   bannerCID,
		Version:     currentVersion + 1,
		Timestamp:   uint64(time.Now().UnixMilli()),
	}

	// Sign the profile.
	sigPayload, err := signingPayloadProfile(profile)
	if err != nil {
		return nil, fmt.Errorf("compute signing payload: %w", err)
	}

	sig, err := identity.SignProtoMessage(kp, sigPayload)
	if err != nil {
		return nil, fmt.Errorf("sign profile: %w", err)
	}
	profile.Signature = sig

	// Validate the profile.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidateProfile(profile, verifier); err != nil {
		return nil, fmt.Errorf("validate profile: %w", err)
	}

	// Store in DB (UpsertProfile only updates if version is greater).
	if err := s.storage.UpsertProfile(
		profile.Author,
		profile.DisplayName,
		profile.Bio,
		profile.AvatarCid,
		profile.BannerCid,
		profile.Website,
		profile.Version,
		int64(profile.Timestamp),
	); err != nil {
		return nil, fmt.Errorf("store profile: %w", err)
	}

	if err := publishProfile(ctx, s.publisher, profile); err != nil {
		log.Printf("publish profile: %v", err)
	}

	return profile, nil
}

// GetProfile retrieves a profile from the local DB by public key.
func (s *ProfileService) GetProfile(pubkey []byte) (*pb.Profile, error) {
	row, err := s.storage.GetProfile(pubkey)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	if row == nil {
		return nil, nil
	}

	return profileRowToProto(row), nil
}

// HandleRemoteProfile validates and stores a remote profile if its version
// is newer than the one we have stored locally.
func (s *ProfileService) HandleRemoteProfile(profile *pb.Profile) error {
	if profile == nil {
		return fmt.Errorf("profile is nil")
	}

	// Validate the profile.
	verifier := func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}
	if err := content.ValidateProfile(profile, verifier); err != nil {
		return fmt.Errorf("validate remote profile: %w", err)
	}

	// Store in DB (UpsertProfile handles the version check).
	if err := s.storage.UpsertProfile(
		profile.Author,
		profile.DisplayName,
		profile.Bio,
		profile.AvatarCid,
		profile.BannerCid,
		profile.Website,
		profile.Version,
		int64(profile.Timestamp),
	); err != nil {
		return fmt.Errorf("store remote profile: %w", err)
	}

	return nil
}

// profileRowToProto converts a storage.ProfileRow to a pb.Profile.
func profileRowToProto(row *storage.ProfileRow) *pb.Profile {
	return &pb.Profile{
		Author:      row.Pubkey,
		DisplayName: row.DisplayName,
		Bio:         row.Bio,
		AvatarCid:   row.AvatarCID,
		BannerCid:   row.BannerCID,
		Website:     row.Website,
		Version:     row.Version,
		Timestamp:   uint64(row.UpdatedAt),
	}
}

// signingPayloadProfile returns the serialized Profile with signature zeroed.
func signingPayloadProfile(profile *pb.Profile) ([]byte, error) {
	clone := proto.Clone(profile).(*pb.Profile)
	clone.Signature = nil
	data, err := proto.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("marshal profile for signing: %w", err)
	}
	return data, nil
}
