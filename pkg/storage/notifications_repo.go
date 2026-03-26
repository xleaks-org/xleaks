package storage

import (
	"database/sql"
	"fmt"
	"time"
)

// NotificationRow represents a single row from the notifications table.
type NotificationRow struct {
	ID          int64
	OwnerPubkey []byte
	Type        string
	Actor       []byte
	TargetCID   []byte
	RelatedCID  []byte
	Timestamp   int64
	Read        bool
}

// InsertNotification inserts a new notification.
func (db *DB) InsertNotification(ownerPubkey []byte, notifType string, actor, targetCID, relatedCID []byte, timestamp int64) error {
	_, err := db.Exec(
		`INSERT INTO notifications (owner_pubkey, type, actor, target_cid, related_cid, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		nilIfEmpty(ownerPubkey), notifType, actor, nilIfEmpty(targetCID), nilIfEmpty(relatedCID), timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

// InsertNotificationTx inserts a new notification within an existing transaction.
func (db *DB) InsertNotificationTx(tx *sql.Tx, ownerPubkey []byte, notifType string, actor, targetCID, relatedCID []byte, timestamp int64) error {
	_, err := tx.Exec(
		`INSERT INTO notifications (owner_pubkey, type, actor, target_cid, related_cid, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		nilIfEmpty(ownerPubkey), notifType, actor, nilIfEmpty(targetCID), nilIfEmpty(relatedCID), timestamp,
	)
	if err != nil {
		return fmt.Errorf("insert notification tx: %w", err)
	}
	return nil
}

// GetNotifications returns notifications paginated by timestamp (descending).
// Pass 0 for before to start from the most recent.
func (db *DB) GetNotifications(ownerPubkey []byte, before int64, limit int) ([]NotificationRow, error) {
	if before == 0 {
		before = time.Now().UnixMilli() + 1
	}
	rows, err := db.Query(
		`SELECT id, owner_pubkey, type, actor, target_cid, related_cid, timestamp, read
		 FROM notifications
		 WHERE owner_pubkey = ? AND timestamp < ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		ownerPubkey, before, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get notifications: %w", err)
	}
	defer rows.Close()

	var notifs []NotificationRow
	for rows.Next() {
		var n NotificationRow
		var readInt int
		if err := rows.Scan(&n.ID, &n.OwnerPubkey, &n.Type, &n.Actor, &n.TargetCID, &n.RelatedCID, &n.Timestamp, &readInt); err != nil {
			return nil, fmt.Errorf("scan notification row: %w", err)
		}
		n.Read = readInt == 1
		notifs = append(notifs, n)
	}
	return notifs, rows.Err()
}

// MarkAllRead marks all notifications as read.
func (db *DB) MarkAllRead(ownerPubkey []byte) error {
	_, err := db.Exec(`UPDATE notifications SET read = 1 WHERE owner_pubkey = ? AND read = 0`, ownerPubkey)
	if err != nil {
		return fmt.Errorf("mark all read: %w", err)
	}
	return nil
}

// MarkRead marks a single notification as read by its ID.
func (db *DB) MarkRead(ownerPubkey []byte, id int64) error {
	_, err := db.Exec(`UPDATE notifications SET read = 1 WHERE owner_pubkey = ? AND id = ?`, ownerPubkey, id)
	if err != nil {
		return fmt.Errorf("mark read: %w", err)
	}
	return nil
}

// UnreadCount returns the number of unread notifications.
func (db *DB) UnreadCount(ownerPubkey []byte) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE owner_pubkey = ? AND read = 0`, ownerPubkey).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("unread count: %w", err)
	}
	return count, nil
}
