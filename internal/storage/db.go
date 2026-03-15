package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// Open opens (or creates) the SQLite database at path.
// Parent directory is created if it doesn't exist.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	// Single writer, multiple readers
	conn.SetMaxOpenConns(1)

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

// migrate runs schema migrations idempotently.
// Each statement is executed separately — SQLite doesn't support multi-statement Exec reliably.
func (db *DB) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS guilds (
			id   TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			icon TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id       TEXT PRIMARY KEY,
			guild_id TEXT REFERENCES guilds(id),
			name     TEXT NOT NULL,
			type     INTEGER NOT NULL DEFAULT 0,
			topic    TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id          TEXT PRIMARY KEY,
			channel_id  TEXT NOT NULL REFERENCES channels(id),
			guild_id    TEXT,
			author_id   TEXT NOT NULL,
			author_name TEXT NOT NULL,
			content     TEXT NOT NULL,
			timestamp   DATETIME NOT NULL,
			edited      INTEGER NOT NULL DEFAULT 0
		)`,
		// FTS5 virtual table — content= points at messages, keeps FTS in sync via triggers
		`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
			content,
			author_name,
			content=messages,
			content_rowid=rowid,
			tokenize="unicode61 remove_diacritics 1"
		)`,
		// Triggers to keep FTS index in sync
		`CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
			INSERT INTO messages_fts(rowid, content, author_name)
			VALUES (new.rowid, new.content, new.author_name);
		END`,
		`CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
			VALUES ('delete', old.rowid, old.content, old.author_name);
		END`,
		`CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
			INSERT INTO messages_fts(messages_fts, rowid, content, author_name)
			VALUES ('delete', old.rowid, old.content, old.author_name);
			INSERT INTO messages_fts(rowid, content, author_name)
			VALUES (new.rowid, new.content, new.author_name);
		END`,
		// Tracks how far we've synced per channel
		`CREATE TABLE IF NOT EXISTS sync_state (
			channel_id      TEXT PRIMARY KEY,
			last_message_id TEXT,
			oldest_message_id TEXT,
			synced_at       DATETIME
		)`,
		// Index for common query patterns
		`CREATE INDEX IF NOT EXISTS idx_messages_channel ON messages(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_author ON messages(author_id)`,
	}

	for _, stmt := range stmts {
		if _, err := db.conn.Exec(stmt); err != nil {
			return fmt.Errorf("migration statement failed: %w\nSQL: %s", err, stmt)
		}
	}
	return nil
}

// Stats returns counts of synced data.
type Stats struct {
	Guilds   int
	Channels int
	Messages int
}

func (db *DB) Stats() (Stats, error) {
	var s Stats
	row := db.conn.QueryRow(`
		SELECT
			(SELECT COUNT(*) FROM guilds),
			(SELECT COUNT(*) FROM channels),
			(SELECT COUNT(*) FROM messages)
	`)
	err := row.Scan(&s.Guilds, &s.Channels, &s.Messages)
	return s, err
}
