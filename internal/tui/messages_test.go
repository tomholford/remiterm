package tui

import (
	"strconv"
	"testing"
	"time"

	"remiterm/internal/api"
)

func TestMessageTime(t *testing.T) {
	t.Parallel()
	// Live API sample: 2026-08-11 14:19:15.022 UTC
	const liveMS int64 = 1786457955022
	got := messageTime(liveMS).UTC()
	want := time.Date(2026, 8, 11, 14, 19, 15, 22*1e6, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("millis: got %v want %v", got, want)
	}

	// Seconds form of the same instant.
	const liveSec int64 = 1786457955
	gotSec := messageTime(liveSec).UTC()
	wantSec := time.Unix(liveSec, 0).UTC()
	if !gotSec.Equal(wantSec) {
		t.Fatalf("seconds: got %v want %v", gotSec, wantSec)
	}

	if !messageTime(0).IsZero() {
		t.Fatal("zero ts should be zero time")
	}
}

func TestIsEdited(t *testing.T) {
	t.Parallel()
	created := int64(1_000_000)
	near := created + 7_000   // typical web-path touch (~7s)
	real := created + 120_000 // clear user edit (2m later)
	before := created - 1

	cases := []struct {
		name string
		msg  api.Message
		want bool
	}{
		{name: "nil edited_at", msg: api.Message{CreatedAt: created}, want: false},
		{name: "near create (API noise)", msg: api.Message{CreatedAt: created, EditedAt: &near}, want: false},
		{name: "real edit", msg: api.Message{CreatedAt: created, EditedAt: &real}, want: true},
		{name: "edited_at before create", msg: api.Message{CreatedAt: created, EditedAt: &before}, want: false},
		{name: "zero created_at", msg: api.Message{CreatedAt: 0, EditedAt: &real}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEdited(tc.msg); got != tc.want {
				t.Fatalf("isEdited() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMessageStoreMergeAndOrder(t *testing.T) {
	s := newMessageStore()
	res := api.ListMessagesResponse{
		Messages: []api.Message{
			{ID: "2", Text: "b", CreatedAt: 200},
			{ID: "1", Text: "a", CreatedAt: 100},
		},
		HasMore:    true,
		NextCursor: "1",
	}
	added := s.mergeAPI(res, false)
	if added != 2 {
		t.Fatalf("added %d", added)
	}
	list := s.list()
	if len(list) != 2 || list[0].ID != "1" || list[1].ID != "2" {
		t.Fatalf("%+v", list)
	}

	// Poll with overlapping + new.
	res2 := api.ListMessagesResponse{
		Messages: []api.Message{
			{ID: "2", Text: "b", CreatedAt: 200},
			{ID: "3", Text: "c", CreatedAt: 300},
		},
	}
	added = s.mergeAPI(res2, false)
	if added != 1 {
		t.Fatalf("added %d", added)
	}
	if s.len() != 3 {
		t.Fatalf("len %d", s.len())
	}

	// Older page.
	older := api.ListMessagesResponse{
		Messages: []api.Message{
			{ID: "0", Text: "z", CreatedAt: 50},
		},
		HasMore:    false,
		NextCursor: "",
	}
	s.mergeAPI(older, true)
	list = s.list()
	if list[0].ID != "0" {
		t.Fatalf("%+v", list)
	}
	if s.hasMore {
		t.Fatal("expected hasMore false")
	}
}

func TestMessageStoreMergeLatestClipsTail(t *testing.T) {
	ids := func(s *messageStore) []string {
		out := make([]string, s.len())
		for i := 0; i < s.len(); i++ {
			out[i] = s.idAt(i)
		}
		return out
	}

	t.Run("empty store takes the latest page", func(t *testing.T) {
		s := newMessageStore()
		var page []api.Message
		for i := 100; i <= 110; i++ {
			page = append(page, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeLatest(api.ListMessagesResponse{Messages: page, HasMore: true, NextCursor: "100"})
		if !s.liveTrusted {
			t.Fatal("liveTrusted")
		}
		if s.idAt(0) != "100" || s.idAt(s.len()-1) != "110" || s.len() != 11 {
			t.Fatalf("%+v", ids(s))
		}
		if s.cursor != "100" {
			t.Fatalf("cursor %q", s.cursor)
		}
		if !s.hasMore {
			t.Fatal("hasMore")
		}
	})

	t.Run("overlap clips seed older than the page", func(t *testing.T) {
		s := newMessageStore()
		var seed []api.Message
		for i := 1; i <= 10; i++ {
			seed = append(seed, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		for i := 100; i <= 110; i++ {
			seed = append(seed, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeAPI(api.ListMessagesResponse{Messages: seed, HasMore: true}, false)
		var page []api.Message
		for i := 100; i <= 110; i++ {
			page = append(page, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeLatest(api.ListMessagesResponse{Messages: page, HasMore: true, NextCursor: "100"})
		if s.len() != 11 || s.idAt(0) != "100" {
			t.Fatalf("%+v", ids(s))
		}
		if s.cursor != "100" {
			t.Fatalf("cursor %q", s.cursor)
		}
	})

	t.Run("overlapping latest page drops older seed", func(t *testing.T) {
		s := newMessageStore()
		var seed []api.Message
		for i := 1; i <= 10; i++ {
			seed = append(seed, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeAPI(api.ListMessagesResponse{Messages: seed, HasMore: true}, false)
		var page []api.Message
		for i := 5; i <= 10; i++ {
			page = append(page, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeLatest(api.ListMessagesResponse{Messages: page, HasMore: true, NextCursor: "5"})
		if s.len() != 6 || s.idAt(0) != "5" {
			t.Fatalf("%+v", ids(s))
		}
	})

	t.Run("second latest poll does not clip backfill", func(t *testing.T) {
		s := newMessageStore()
		var seed []api.Message
		for i := 1; i <= 10; i++ {
			seed = append(seed, api.Message{ID: itoa(i), CreatedAt: int64(i)})
		}
		s.mergeAPI(api.ListMessagesResponse{Messages: seed, HasMore: true}, false)
		s.mergeLatest(api.ListMessagesResponse{
			Messages: []api.Message{
				{ID: "5", CreatedAt: 5},
				{ID: "6", CreatedAt: 6},
				{ID: "7", CreatedAt: 7},
				{ID: "8", CreatedAt: 8},
				{ID: "9", CreatedAt: 9},
				{ID: "10", CreatedAt: 10},
			},
			HasMore:    true,
			NextCursor: "5",
		})
		s.mergeLatest(api.ListMessagesResponse{
			Messages: []api.Message{
				{ID: "6", CreatedAt: 6},
				{ID: "7", CreatedAt: 7},
				{ID: "8", CreatedAt: 8},
				{ID: "9", CreatedAt: 9},
				{ID: "10", CreatedAt: 10},
				{ID: "11", CreatedAt: 11},
			},
			HasMore:    true,
			NextCursor: "6",
		})
		if s.idAt(0) != "5" {
			t.Fatalf("kept backfill oldest %q want 5: %+v", s.idAt(0), ids(s))
		}
		if s.idAt(s.len()-1) != "11" {
			t.Fatalf("newest %q: %+v", s.idAt(s.len()-1), ids(s))
		}
	})

	t.Run("non-numeric ids overlap without panic", func(t *testing.T) {
		s := newMessageStore()
		s.mergeAPI(api.ListMessagesResponse{
			Messages: []api.Message{
				{ID: "old-a", CreatedAt: 1},
				{ID: "old-b", CreatedAt: 2},
			},
			HasMore: true,
		}, false)
		s.mergeLatest(api.ListMessagesResponse{
			Messages: []api.Message{{ID: "old-b", CreatedAt: 2}},
			HasMore:  true,
		})
		if s.len() != 1 || s.idAt(0) != "old-b" {
			t.Fatalf("%+v", ids(s))
		}
	})
}

func itoa(n int) string {
	return strconv.Itoa(n)
}

func TestUpsertOwnPost(t *testing.T) {
	s := newMessageStore()
	s.upsert(api.Message{ID: "9", Text: "hi", CreatedAt: 1})
	s.upsert(api.Message{ID: "9", Text: "hi", CreatedAt: 1, Author: api.Author{Handle: "me"}})
	if s.len() != 1 {
		t.Fatal(s.len())
	}
	m, _ := s.get("9")
	if m.Author.Handle != "me" {
		t.Fatalf("%+v", m)
	}
}
