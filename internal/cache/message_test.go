package cache

import (
	"context"
	"path/filepath"
	"testing"

	"remiterm/internal/api"
)

func TestMessageRoundtripAndUpsert(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "chat.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	orig := api.Message{
		ID:        "2",
		Text:      "hello",
		CreatedAt: 200,
		Author:    api.Author{Handle: "alice"},
	}
	if err = db.SaveMessage(ctx, orig); err != nil {
		t.Fatal(err)
	}
	if err = db.SaveMessage(ctx, api.Message{ID: "", Text: "skip"}); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	got, err := db.LatestMessages(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "2" || got[0].Text != "hello" || got[0].Author.Handle != "alice" {
		t.Fatalf("%+v", got)
	}

	updated := orig
	updated.Text = "edited"
	updated.CreatedAt = 250
	if err = db.SaveMessage(ctx, updated); err != nil {
		t.Fatal(err)
	}
	got, err = db.LatestMessages(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Text != "edited" || got[0].CreatedAt != 250 {
		t.Fatalf("upsert %+v", got)
	}
}

func TestLatestMessagesOrderAndLimit(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for _, m := range []api.Message{
		{ID: "1", Text: "a", CreatedAt: 100},
		{ID: "3", Text: "c", CreatedAt: 300},
		{ID: "2", Text: "b", CreatedAt: 200},
	} {
		if err = db.SaveMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.LatestMessages(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "2" || got[1].ID != "3" {
		t.Fatalf("oldest-first newest two: %+v", got)
	}
}

func TestListMessagesBeforeID(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for _, m := range []api.Message{
		{ID: "1", Text: "a", CreatedAt: 100},
		{ID: "2", Text: "b", CreatedAt: 200},
		{ID: "3", Text: "c", CreatedAt: 300},
		{ID: "4", Text: "d", CreatedAt: 400},
	} {
		if err = db.SaveMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListMessages(ctx, "3", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "1" || got[1].ID != "2" {
		t.Fatalf("before 3: %+v", got)
	}
	got, err = db.ListMessages(ctx, "1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("before oldest: %+v", got)
	}
	got, err = db.ListMessages(ctx, "missing", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown cursor: %+v", got)
	}
}

func TestSaveAfterCloseDoesNotPanic(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	err = db.SaveMessage(context.Background(), api.Message{ID: "1", Text: "x", CreatedAt: 1})
	if err == nil {
		t.Fatal("expected error after close")
	}
}

func TestListMessagesSameCreatedAtTieBreak(t *testing.T) {
	t.Parallel()
	db, err := OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()
	for _, m := range []api.Message{
		{ID: "a", CreatedAt: 100},
		{ID: "b", CreatedAt: 100},
		{ID: "c", CreatedAt: 100},
	} {
		if err = db.SaveMessage(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListMessages(ctx, "c", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "a" || got[1].ID != "b" {
		t.Fatalf("%+v", got)
	}
}
