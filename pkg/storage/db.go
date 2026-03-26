package storage

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps a *sql.DB connection to the SQLite database.
type DB struct {
	*sql.DB
}

// NewDB opens a SQLite database at dbPath with WAL mode, a 5-second busy
// timeout, and foreign keys enabled. The caller must call Close when done.
func NewDB(dbPath string) (*DB, error) {
	dsn := fmt.Sprintf("%s?_pragma=journal_mode%%3DWAL&_pragma=busy_timeout%%3D5000&_pragma=foreign_keys%%3DON", dbPath)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// Verify connection works.
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Migrate runs the schema migration, creating all tables and indexes if they
// do not already exist.
func (db *DB) Migrate() error {
	_, err := db.Exec(Schema)
	if err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := db.migrateSubscriptionsTable(); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_subscriptions_owner ON subscriptions(owner_pubkey, followed_at DESC)`); err != nil {
		return fmt.Errorf("create subscriptions owner index: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE notifications ADD COLUMN owner_pubkey BLOB`); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		return fmt.Errorf("migrate notifications.owner_pubkey: %w", err)
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_notifications_owner_unread ON notifications(owner_pubkey, read, timestamp DESC)`); err != nil {
		return fmt.Errorf("create notifications owner index: %w", err)
	}
	return nil
}

func (db *DB) migrateSubscriptionsTable() error {
	if hasOwner, err := db.tableHasColumn("subscriptions", "owner_pubkey"); err != nil {
		return fmt.Errorf("inspect subscriptions schema: %w", err)
	} else if hasOwner {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin subscriptions migration: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		CREATE TABLE subscriptions_new (
			owner_pubkey BLOB NOT NULL,
			pubkey BLOB NOT NULL,
			followed_at INTEGER NOT NULL,
			sync_completed INTEGER DEFAULT 0,
			PRIMARY KEY (owner_pubkey, pubkey)
		)
	`); err != nil {
		return fmt.Errorf("create subscriptions_new: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO subscriptions_new (owner_pubkey, pubkey, followed_at, sync_completed)
		SELECT
			COALESCE(
				(SELECT pubkey FROM identities WHERE is_active = 1 LIMIT 1),
				(SELECT pubkey FROM identities ORDER BY created_at ASC LIMIT 1),
				x''
			),
			pubkey,
			followed_at,
			sync_completed
		FROM subscriptions
	`); err != nil {
		return fmt.Errorf("copy subscriptions: %w", err)
	}

	if _, err := tx.Exec(`DROP TABLE subscriptions`); err != nil {
		return fmt.Errorf("drop old subscriptions: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE subscriptions_new RENAME TO subscriptions`); err != nil {
		return fmt.Errorf("rename subscriptions_new: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscriptions migration: %w", err)
	}
	return nil
}

func (db *DB) tableHasColumn(tableName, columnName string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + tableName + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	return false, rows.Err()
}

// WithTransaction executes fn within a database transaction. If fn returns
// an error the transaction is rolled back; otherwise it is committed.
func (db *DB) WithTransaction(fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
