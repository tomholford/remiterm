package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"remiterm/internal/api"
)

const (
	defaultPage = 50
	maxPage     = 500
)

// SaveMessage upserts a message by API id.
func (d *DB) SaveMessage(ctx context.Context, msg api.Message) error {
	if d == nil || msg.ID == "" {
		return nil
	}
	raw, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode message %s: %w", msg.ID, err)
	}
	_, err = d.sql.ExecContext(ctx, `
		insert into messages (id, created_at, payload)
		     values (?, ?, ?)
		on conflict (id) do update set
		    created_at = excluded.created_at,
		    payload = excluded.payload
	`, msg.ID, msg.CreatedAt, string(raw))
	if err != nil {
		return fmt.Errorf("save message %s: %w", msg.ID, err)
	}
	return nil
}

// LatestMessages returns the newest messages, oldest-first, capped at limit.
func (d *DB) LatestMessages(ctx context.Context, limit int) ([]api.Message, error) {
	limit = clampLimit(limit)
	rows, err := d.sql.QueryContext(ctx, `
		select payload from messages
		 order by created_at desc, id desc
		 limit ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("latest messages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanMessagesOldestFirst(rows)
}

// ListMessages returns messages older than beforeID, oldest-first.
// Unknown beforeID yields an empty page (caller should hit the API).
func (d *DB) ListMessages(ctx context.Context, beforeID string, limit int) ([]api.Message, error) {
	if beforeID == "" {
		return d.LatestMessages(ctx, limit)
	}
	limit = clampLimit(limit)
	var createdAt int64
	var id string
	err := d.sql.QueryRowContext(ctx, `
		select created_at, id from messages where id = ?
	`, beforeID).Scan(&createdAt, &id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup cursor %s: %w", beforeID, err)
	}
	rows, err := d.sql.QueryContext(ctx, `
		select payload from messages
		 where created_at < ?
		    or (created_at = ? and id < ?)
		 order by created_at desc, id desc
		 limit ?
	`, createdAt, createdAt, id, limit)
	if err != nil {
		return nil, fmt.Errorf("list messages before %s: %w", beforeID, err)
	}
	defer func() { _ = rows.Close() }()
	return scanMessagesOldestFirst(rows)
}

func scanMessagesOldestFirst(rows *sql.Rows) ([]api.Message, error) {
	var newestFirst []api.Message
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		var msg api.Message
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			continue
		}
		if msg.ID == "" {
			continue
		}
		newestFirst = append(newestFirst, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	out := make([]api.Message, 0, len(newestFirst))
	for i := len(newestFirst) - 1; i >= 0; i-- {
		out = append(out, newestFirst[i])
	}
	return out, nil
}

func clampLimit(n int) int {
	if n <= 0 {
		return defaultPage
	}
	if n > maxPage {
		return maxPage
	}
	return n
}
