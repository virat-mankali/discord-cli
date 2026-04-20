package storage

import (
	"fmt"
	"strings"
	"time"
)

type SearchOptions struct {
	Channel string
	Guild   string
	Limit   int
}

type SearchResult struct {
	ID          string
	ChannelID   string
	ChannelName string
	GuildName   string
	AuthorName  string
	Content     string
	Timestamp   time.Time
	Rank        float64
}

func (db *DB) SearchMessages(query string, opts SearchOptions) ([]SearchResult, error) {
	if opts.Limit == 0 {
		opts.Limit = 20
	}

	var conditions []string
	var args []interface{}

	args = append(args, query)

	if opts.Channel != "" {
		conditions = append(conditions, "c.name = ?")
		args = append(args, opts.Channel)
	}
	if opts.Guild != "" {
		conditions = append(conditions, "COALESCE(g.name, gc.name) = ?")
		args = append(args, opts.Guild)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "AND " + strings.Join(conditions, " AND ")
	}

	args = append(args, opts.Limit)

	q := fmt.Sprintf(`
		SELECT
			m.id, m.channel_id, c.name AS channel_name,
			COALESCE(g.name, gc.name, 'DM') AS guild_name,
			m.author_name, m.content, m.timestamp,
			rank
		FROM messages_fts
		JOIN messages m ON messages_fts.rowid = m.rowid
		JOIN channels c ON m.channel_id = c.id
		LEFT JOIN guilds g ON m.guild_id = g.id
		LEFT JOIN guilds gc ON c.guild_id = gc.id
		WHERE messages_fts MATCH ?
		%s
		ORDER BY rank
		LIMIT ?
	`, whereClause)

	rows, err := db.conn.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var ts string
		if err := rows.Scan(
			&r.ID, &r.ChannelID, &r.ChannelName, &r.GuildName,
			&r.AuthorName, &r.Content, &ts, &r.Rank,
		); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		results = append(results, r)
	}
	return results, rows.Err()
}
