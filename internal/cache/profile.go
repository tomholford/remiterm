package cache

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"remiterm/internal/api"
)

// SaveProfile upserts a profile snapshot keyed by handle.
func (d *DB) SaveProfile(ctx context.Context, p api.Profile) error {
	if d == nil {
		return nil
	}
	handle := strings.TrimSpace(p.Handle())
	if handle == "" {
		return nil
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode profile %s: %w", handle, err)
	}
	_, err = d.sql.ExecContext(ctx, `
		insert into profiles (handle, fetched_at, payload)
		     values (?, ?, ?)
		on conflict (handle) do update set
		    fetched_at = excluded.fetched_at,
		    payload = excluded.payload
	`, handle, time.Now().UnixMilli(), string(raw))
	if err != nil {
		return fmt.Errorf("save profile %s: %w", handle, err)
	}
	return nil
}

// GetProfile returns a cached profile snapshot.
func (d *DB) GetProfile(ctx context.Context, handle string) (api.Profile, int64, error) {
	handle = strings.TrimSpace(handle)
	if d == nil || handle == "" {
		return api.Profile{}, 0, ErrNotFound
	}
	var payload string
	var fetchedAt int64
	err := d.sql.QueryRowContext(ctx, `
		select payload, fetched_at from profiles where handle = ?
	`, handle).Scan(&payload, &fetchedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return api.Profile{}, 0, ErrNotFound
		}
		return api.Profile{}, 0, fmt.Errorf("get profile %s: %w", handle, err)
	}
	var p api.Profile
	if err = json.Unmarshal([]byte(payload), &p); err != nil {
		return api.Profile{}, 0, fmt.Errorf("decode profile %s: %w", handle, err)
	}
	return p, fetchedAt, nil
}
