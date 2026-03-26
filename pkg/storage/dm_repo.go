package storage

import (
	"bytes"
	"database/sql"
	"fmt"
	"time"
)

// DMRow represents a single row from the direct_messages table.
type DMRow struct {
	CID              []byte
	Author           []byte
	Recipient        []byte
	EncryptedContent []byte
	Nonce            []byte
	Timestamp        int64
	Read             bool
}

// ConversationSummary represents a DM conversation with the most recent
// message metadata, used for listing conversations.
type ConversationSummary struct {
	PeerPubkey       []byte
	LastTimestamp    int64
	LastAuthor       []byte
	EncryptedContent []byte
	Nonce            []byte
	UnreadCount      int
}

// InsertDM inserts a new direct message.
func (db *DB) InsertDM(cid, author, recipient, encryptedContent, nonce []byte, timestamp int64) error {
	_, err := db.Exec(
		`INSERT OR IGNORE INTO direct_messages (cid, author, recipient, encrypted_content, nonce, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		cid, author, recipient, encryptedContent, nonce, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert dm: %w", err)
	}
	return nil
}

// InsertDMTx inserts a new direct message within an existing transaction.
func (db *DB) InsertDMTx(tx *sql.Tx, cid, author, recipient, encryptedContent, nonce []byte, timestamp int64) error {
	_, err := tx.Exec(
		`INSERT OR IGNORE INTO direct_messages (cid, author, recipient, encrypted_content, nonce, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		cid, author, recipient, encryptedContent, nonce, timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert dm tx: %w", err)
	}
	return nil
}

// GetConversation retrieves messages between two parties, paginated by
// timestamp (descending). Pass 0 for before to start from the most recent.
func (db *DB) GetConversation(pubkey1, pubkey2 []byte, before int64, limit int) ([]DMRow, error) {
	if before == 0 {
		before = time.Now().UnixMilli() + 1
	}
	rows, err := db.Query(
		`SELECT cid, author, recipient, encrypted_content, nonce, timestamp, read
		 FROM direct_messages
		 WHERE ((author = ? AND recipient = ?) OR (author = ? AND recipient = ?))
		   AND timestamp < ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		pubkey1, pubkey2, pubkey2, pubkey1, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	defer rows.Close()

	var dms []DMRow
	for rows.Next() {
		var d DMRow
		var readInt int
		if err := rows.Scan(&d.CID, &d.Author, &d.Recipient, &d.EncryptedContent, &d.Nonce, &d.Timestamp, &readInt); err != nil {
			return nil, fmt.Errorf("scan dm row: %w", err)
		}
		d.Read = readInt == 1
		dms = append(dms, d)
	}
	return dms, rows.Err()
}

// GetConversations lists all conversations for the given own public key,
// returning one summary per conversation partner with the latest message info.
// It uses a single aggregation query to avoid N+1 round-trips.
func (db *DB) GetConversations(ownPubkey []byte) ([]ConversationSummary, error) {
	rows, err := db.Query(
		`WITH convo_messages AS (
		     SELECT
		         CASE WHEN author < recipient THEN author ELSE recipient END AS peer_a,
		         CASE WHEN author > recipient THEN author ELSE recipient END AS peer_b,
		         author,
		         recipient,
		         encrypted_content,
		         nonce,
		         timestamp,
		         read,
		         ROW_NUMBER() OVER (
		             PARTITION BY CASE WHEN author < recipient THEN author ELSE recipient END,
		                          CASE WHEN author > recipient THEN author ELSE recipient END
		             ORDER BY timestamp DESC
		         ) AS rn
		     FROM direct_messages
		     WHERE author = ? OR recipient = ?
		 ),
		 unread AS (
		     SELECT
		         CASE WHEN author < recipient THEN author ELSE recipient END AS peer_a,
		         CASE WHEN author > recipient THEN author ELSE recipient END AS peer_b,
		         SUM(CASE WHEN read = 0 AND recipient = ? THEN 1 ELSE 0 END) AS unread
		     FROM direct_messages
		     WHERE author = ? OR recipient = ?
		     GROUP BY peer_a, peer_b
		 )
		 SELECT cm.peer_a, cm.peer_b, cm.timestamp, cm.author, cm.encrypted_content, cm.nonce, COALESCE(u.unread, 0)
		 FROM convo_messages cm
		 LEFT JOIN unread u ON u.peer_a = cm.peer_a AND u.peer_b = cm.peer_b
		 WHERE cm.rn = 1
		 ORDER BY cm.timestamp DESC`,
		ownPubkey, ownPubkey, ownPubkey, ownPubkey, ownPubkey,
	)
	if err != nil {
		return nil, fmt.Errorf("get conversations: %w", err)
	}
	defer rows.Close()

	var summaries []ConversationSummary
	for rows.Next() {
		var peerA, peerB []byte
		var cs ConversationSummary
		if err := rows.Scan(&peerA, &peerB, &cs.LastTimestamp, &cs.LastAuthor, &cs.EncryptedContent, &cs.Nonce, &cs.UnreadCount); err != nil {
			return nil, fmt.Errorf("scan conversation summary: %w", err)
		}

		// The "other peer" is whichever of peer_a/peer_b doesn't match ownPubkey.
		if bytes.Equal(peerA, ownPubkey) {
			cs.PeerPubkey = peerB
		} else {
			cs.PeerPubkey = peerA
		}

		summaries = append(summaries, cs)
	}
	return summaries, rows.Err()
}

// MarkDMRead marks a direct message as read by its CID.
func (db *DB) MarkDMRead(cid []byte) error {
	_, err := db.Exec(`UPDATE direct_messages SET read = 1 WHERE cid = ?`, cid)
	if err != nil {
		return fmt.Errorf("mark dm read: %w", err)
	}
	return nil
}
