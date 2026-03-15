package storage

import (
	"fmt"
	"time"
)

// UpsertMessage inserts or replaces a message (idempotent for sync).
func (db *DB) UpsertMessage(id, channelID, guildID, authorID, authorName, content string, timestamp time.Time, edited bool) error {
	editedInt := 0
	if edited {
		editedInt = 1
	}
	_, err := db.conn.Exec(`
		INSERT OR REPLACE INTO messages (id, channel_id, guild_id, author_id, author_name, content, timestamp, edited)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, id, channelID, guildID, authorID, authorName, content, timestamp.UTC().Format(time.RFC3339), editedInt)
	if err != nil {
		return fmt.Errorf("upsert message %s: %w", id, err)
	}
	return nil
}

// SyncState holds per-channel sync progress.
type SyncState struct {
	ChannelID       string
	LastMessageID   string // newest message we've seen (for forward sync)
	OldestMessageID string // oldest message we've seen (for backward pagination)
	SyncedAt        time.Time
}

// GetSyncState returns the sync state for a channel, or nil if never synced.
func (db *DB) GetSyncState(channelID string) (*SyncState, error) {
	var s SyncState
	var syncedAt string
	err := db.conn.QueryRow(`
		SELECT channel_id, COALESCE(last_message_id,''), COALESCE(oldest_message_id,''), COALESCE(synced_at,'')
		FROM sync_state WHERE channel_id = ?
	`, channelID).Scan(&s.ChannelID, &s.LastMessageID, &s.OldestMessageID, &syncedAt)
	if err != nil {
		return nil, nil // no sync state yet — not an error
	}
	s.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt)
	return &s, nil
}

// GetLastMessageID is a convenience wrapper used by the gateway handler.
func (db *DB) GetLastMessageID(channelID string) (string, error) {
	s, err := db.GetSyncState(channelID)
	if err != nil || s == nil {
		return "", err
	}
	return s.LastMessageID, nil
}

// UpdateSyncState records sync progress for a channel.
// newestID is the most recent message (Discord snowflake — higher = newer).
// oldestID is the oldest message fetched so far (for resumable backward pagination).
func (db *DB) UpdateSyncState(channelID, newestID, oldestID string) error {
	_, err := db.conn.Exec(`
		INSERT INTO sync_state (channel_id, last_message_id, oldest_message_id, synced_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(channel_id) DO UPDATE SET
			last_message_id   = CASE WHEN excluded.last_message_id > last_message_id
			                         THEN excluded.last_message_id ELSE last_message_id END,
			oldest_message_id = CASE WHEN oldest_message_id = '' OR excluded.oldest_message_id < oldest_message_id
			                         THEN excluded.oldest_message_id ELSE oldest_message_id END,
			synced_at         = excluded.synced_at
	`, channelID, newestID, oldestID, time.Now().UTC().Format(time.RFC3339))
	return err
}

// MessageCount returns the number of messages stored for a channel.
func (db *DB) MessageCount(channelID string) (int, error) {
	var count int
	err := db.conn.QueryRow(`SELECT COUNT(*) FROM messages WHERE channel_id = ?`, channelID).Scan(&count)
	return count, err
}

// ListSyncedChannels returns all channels that have sync state.
type SyncedChannel struct {
	ChannelID   string
	ChannelName string
	GuildName   string
	MessageCount int
	SyncedAt    time.Time
}

func (db *DB) ListSyncedChannels() ([]SyncedChannel, error) {
	rows, err := db.conn.Query(`
		SELECT
			ss.channel_id,
			COALESCE(c.name, ss.channel_id) AS channel_name,
			COALESCE(g.name, 'DM') AS guild_name,
			(SELECT COUNT(*) FROM messages m WHERE m.channel_id = ss.channel_id) AS msg_count,
			COALESCE(ss.synced_at, '') AS synced_at
		FROM sync_state ss
		LEFT JOIN channels c ON ss.channel_id = c.id
		LEFT JOIN guilds g ON c.guild_id = g.id
		ORDER BY ss.synced_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SyncedChannel
	for rows.Next() {
		var sc SyncedChannel
		var syncedAt string
		if err := rows.Scan(&sc.ChannelID, &sc.ChannelName, &sc.GuildName, &sc.MessageCount, &syncedAt); err != nil {
			return nil, err
		}
		sc.SyncedAt, _ = time.Parse(time.RFC3339, syncedAt)
		result = append(result, sc)
	}
	return result, rows.Err()
}
