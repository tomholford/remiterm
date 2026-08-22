package cache

import (
	"context"
	"errors"
	"testing"

	"remiterm/internal/api"
)

func TestProfileRoundtripAndCase(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	p := api.Profile{
		User: api.ProfileUser{
			Username:    "Alice",
			DisplayName: "A",
			Bio:         "hi",
		},
		IsAuthenticated: true,
	}
	if err = db.SaveProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, fetchedAt, err := db.GetProfile(ctx, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if fetchedAt <= 0 {
		t.Fatalf("fetched_at %d", fetchedAt)
	}
	if got.User.Username != "Alice" || got.User.Bio != "hi" || !got.IsAuthenticated {
		t.Fatalf("%+v", got)
	}

	p.User.Bio = "updated"
	if err = db.SaveProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, _, err = db.GetProfile(ctx, "ALICE")
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Bio != "updated" {
		t.Fatalf("upsert %+v", got)
	}
}

func TestGetProfileNotFound(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	_, _, err = db.GetProfile(context.Background(), "nobody")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err %v", err)
	}
	if err = db.SaveProfile(context.Background(), api.Profile{}); err != nil {
		t.Fatal(err)
	}
}
