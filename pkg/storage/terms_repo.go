package storage

import "time"

// HasAcceptedTerms checks whether the given pubkey has accepted the terms of service.
func (db *DB) HasAcceptedTerms(pubkey []byte) bool {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM terms_acceptance WHERE pubkey = ?", pubkey).Scan(&count)
	return err == nil && count > 0
}

// AcceptTerms records that the given pubkey has accepted the specified terms version.
func (db *DB) AcceptTerms(pubkey []byte, version string) error {
	_, err := db.Exec(
		"INSERT OR REPLACE INTO terms_acceptance (pubkey, terms_version, accepted_at, device_node_agreed) VALUES (?, ?, ?, 1)",
		pubkey, version, time.Now().UnixMilli(),
	)
	return err
}
