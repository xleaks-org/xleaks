package storage

import (
	"database/sql"
	"fmt"
)

// ProfileRow represents a single row from the profiles table.
type ProfileRow struct {
	Pubkey      []byte
	DisplayName string
	Bio         string
	AvatarCID   []byte
	BannerCID   []byte
	Website     string
	Version     uint64
	UpdatedAt   int64
}

// UpsertProfile inserts a new profile or updates an existing one only if the
// incoming version is strictly greater than the stored version.
func (db *DB) UpsertProfile(pubkey []byte, displayName, bio string, avatarCID, bannerCID []byte, website string, version uint64, updatedAt int64) error {
	_, err := db.Exec(
		`INSERT INTO profiles (pubkey, display_name, bio, avatar_cid, banner_cid, website, version, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(pubkey) DO UPDATE SET
		     display_name = excluded.display_name,
		     bio = excluded.bio,
		     avatar_cid = excluded.avatar_cid,
		     banner_cid = excluded.banner_cid,
		     website = excluded.website,
		     version = excluded.version,
		     updated_at = excluded.updated_at
		 WHERE excluded.version > profiles.version`,
		pubkey, displayName, bio, nilIfEmpty(avatarCID), nilIfEmpty(bannerCID), website, version, updatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert profile: %w", err)
	}
	return nil
}

// GetProfileVersion returns the version number of a stored profile.
// If no profile exists for the given pubkey, found is false.
func (db *DB) GetProfileVersion(pubkey []byte) (version uint64, found bool, err error) {
	err = db.QueryRow(`SELECT version FROM profiles WHERE pubkey = ?`, pubkey).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("get profile version: %w", err)
	}
	return version, true, nil
}

// GetProfile retrieves a profile by public key. Returns nil if not found.
func (db *DB) GetProfile(pubkey []byte) (*ProfileRow, error) {
	var p ProfileRow
	err := db.QueryRow(
		`SELECT pubkey, display_name, bio, avatar_cid, banner_cid, website, version, updated_at
		 FROM profiles WHERE pubkey = ?`,
		pubkey,
	).Scan(&p.Pubkey, &p.DisplayName, &p.Bio, &p.AvatarCID, &p.BannerCID, &p.Website, &p.Version, &p.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}
