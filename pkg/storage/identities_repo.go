package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// IdentityRow represents a single row from the identities table.
type IdentityRow struct {
	Pubkey      []byte
	DisplayName string
	IsActive    bool
	CreatedAt   int64
}

// InsertIdentity inserts a new identity record into the identities table.
func (db *DB) InsertIdentity(pubkey []byte, displayName string, isActive bool, createdAt int64) error {
	activeInt := 0
	if isActive {
		activeInt = 1
	}
	_, err := db.Exec(
		`INSERT OR IGNORE INTO identities (pubkey, display_name, is_active, created_at)
		 VALUES (?, ?, ?, ?)`,
		pubkey, displayName, activeInt, createdAt,
	)
	if err != nil {
		return fmt.Errorf("insert identity: %w", err)
	}
	return nil
}

// GetIdentities returns all identities from the identities table.
func (db *DB) GetIdentities() ([]IdentityRow, error) {
	rows, err := db.Query(
		`SELECT pubkey, display_name, is_active, created_at FROM identities ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get identities: %w", err)
	}
	defer rows.Close()

	var identities []IdentityRow
	for rows.Next() {
		var id IdentityRow
		var activeInt int
		if err := rows.Scan(&id.Pubkey, &id.DisplayName, &activeInt, &id.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan identity row: %w", err)
		}
		id.IsActive = activeInt == 1
		identities = append(identities, id)
	}
	return identities, rows.Err()
}

// SetActiveIdentity sets the given pubkey as the active identity and deactivates all others.
func (db *DB) SetActiveIdentity(pubkey []byte) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx for set active identity: %w", err)
	}
	defer tx.Rollback()

	// Deactivate all identities.
	if _, err := tx.Exec(`UPDATE identities SET is_active = 0`); err != nil {
		return fmt.Errorf("deactivate identities: %w", err)
	}

	// Activate the specified identity.
	if _, err := tx.Exec(`UPDATE identities SET is_active = 1 WHERE pubkey = ?`, pubkey); err != nil {
		return fmt.Errorf("activate identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set active identity: %w", err)
	}
	return nil
}

// GetActiveIdentity returns the currently active identity, or nil if none is active.
func (db *DB) GetActiveIdentity() (*IdentityRow, error) {
	var id IdentityRow
	var activeInt int
	err := db.QueryRow(
		`SELECT pubkey, display_name, is_active, created_at FROM identities WHERE is_active = 1 LIMIT 1`,
	).Scan(&id.Pubkey, &id.DisplayName, &activeInt, &id.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get active identity: %w", err)
	}
	id.IsActive = activeInt == 1
	return &id, nil
}

// CountIdentities returns the number of identities stored.
func (db *DB) CountIdentities() (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM identities`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count identities: %w", err)
	}
	return count, nil
}

// IdentityExists checks if an identity with the given pubkey exists.
func (db *DB) IdentityExists(pubkey []byte) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM identities WHERE pubkey = ?`, pubkey).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check identity exists: %w", err)
	}
	return count > 0, nil
}

// nowMillisStorage returns the current time in milliseconds.
func nowMillisStorage() int64 {
	return time.Now().UnixMilli()
}
