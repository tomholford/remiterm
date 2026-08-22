package demo

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"remiterm/internal/api"
)

func TestPageBefore(t *testing.T) {
	t.Parallel()
	all := buildCorpus(3, 10) // 30 msgs, ids 1..30

	// Newest page.
	page, hasMore, cursor := pageBefore(all, "", 10)
	if len(page) != 10 {
		t.Fatalf("len=%d", len(page))
	}
	if page[0].ID != "21" || page[9].ID != "30" {
		t.Fatalf("got %s..%s", page[0].ID, page[9].ID)
	}
	if !hasMore || cursor != "21" {
		t.Fatalf("hasMore=%v cursor=%q", hasMore, cursor)
	}

	// Older page via before.
	page2, hasMore2, cursor2 := pageBefore(all, "21", 10)
	if len(page2) != 10 || page2[0].ID != "11" || page2[9].ID != "20" {
		t.Fatalf("%+v", page2)
	}
	if !hasMore2 || cursor2 != "11" {
		t.Fatalf("hasMore=%v cursor=%q", hasMore2, cursor2)
	}

	// Last older page.
	page3, hasMore3, cursor3 := pageBefore(all, "11", 10)
	if len(page3) != 10 || page3[0].ID != "1" || page3[9].ID != "10" {
		t.Fatalf("last page %+v", page3)
	}
	if hasMore3 {
		t.Fatal("expected no more")
	}
	if cursor3 != "1" {
		t.Fatalf("cursor=%q", cursor3)
	}

	// Exhausted.
	empty, more, _ := pageBefore(all, "1", 10)
	if len(empty) != 0 || more {
		t.Fatalf("empty=%d more=%v", len(empty), more)
	}

	// Unknown before.
	empty, more, _ = pageBefore(all, "99999", 10)
	if len(empty) != 0 || more {
		t.Fatalf("unknown before should be empty")
	}
}

func TestServerListAndMe(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 4, PageSize: 5, Latency: 0})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	c := api.New(ts.URL, "")
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Handle() != "demo" {
		t.Fatalf("%+v", me)
	}

	// Walk all pages.
	var all []api.Message
	before := ""
	for {
		out, err := c.ListGlobalChat(context.Background(), 5, before)
		if err != nil {
			t.Fatal(err)
		}
		all = append(out.Messages, all...) // prepend older as we go back
		if !out.HasMore {
			break
		}
		if out.NextCursor == "" {
			t.Fatal("expected next_cursor")
		}
		before = out.NextCursor
		if len(all) > 100 {
			t.Fatal("runaway pagination")
		}
	}
	// First response was newest; we prepended older pages, so all is oldest→newest.
	// Actually: first List got msgs 16-20, then before=16 got 11-15 prepended → 11-20, etc.
	// Final should be 20 messages.
	if len(all) != 20 {
		t.Fatalf("total=%d want 20", len(all))
	}
	if all[0].ID != "1" || all[len(all)-1].ID != "20" {
		t.Fatalf("range %s..%s", all[0].ID, all[len(all)-1].ID)
	}
}

func TestServerLatency(t *testing.T) {
	t.Parallel()
	const delay = 40 * time.Millisecond
	s := NewServer(Options{Pages: 1, PageSize: 5, Latency: delay})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	c := api.New(ts.URL, "")
	start := time.Now()
	if _, err := c.ListGlobalChat(context.Background(), 5, ""); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < delay {
		t.Fatalf("elapsed %v < latency %v", elapsed, delay)
	}
}

func TestServerPost(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 1, PageSize: 5, Latency: 0})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	c := api.New(ts.URL, "")
	msg, err := c.PostGlobalChat(context.Background(), "hello demo", "")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Text != "hello demo" || msg.Author.Handle != "demo" {
		t.Fatalf("%+v", msg)
	}

	out, err := c.ListGlobalChat(context.Background(), 50, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range out.Messages {
		if m.ID == msg.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("posted message missing from list")
	}
}

func TestServerMeStats(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 1, PageSize: 5})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	c := api.New(ts.URL, "")
	out, err := c.MeStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Handle != "demo" || out.AggregateScores["beetles"] != 7 {
		t.Fatalf("%+v", out)
	}
	if out.Stats.Ethereum.CultTier != "initiate" || out.Stats.Ethereum.TotalOwned != 2 {
		t.Fatalf("%+v", out.Stats.Ethereum)
	}
}

func TestServerGetUser(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 1, PageSize: 5})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	c := api.New(ts.URL, "")
	p, err := c.GetUser(context.Background(), "milady")
	if err != nil {
		t.Fatal(err)
	}
	if p.User.Username != "milady" {
		t.Fatalf("%+v", p)
	}

	_, err = c.GetUser(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected 404")
	}
}

func TestListenURL(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 1, PageSize: 2, Latency: 0})
	s.SetLogger(nil)
	if err := s.Listen(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })

	if s.URL() == "" {
		t.Fatal("empty URL")
	}
	res, err := http.Get(s.URL() + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var p api.Profile
	if err := json.NewDecoder(res.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	if p.Handle() != "demo" {
		t.Fatalf("%+v", p)
	}
}

func TestCorpusMessage5HasImage(t *testing.T) {
	t.Parallel()
	all := buildCorpus(1, 10)
	if len(all) < 5 {
		t.Fatal("corpus too small")
	}
	msg := all[4]
	if msg.ID != "5" {
		t.Fatalf("id %q", msg.ID)
	}
	if len(msg.Media) != 1 || msg.Media[0].Kind != "image" || msg.Media[0].URL != "/img/preview.png" {
		t.Fatalf("%+v", msg.Media)
	}
}

func TestPreviewPNGRoute(t *testing.T) {
	t.Parallel()
	s := NewServer(Options{Pages: 1, PageSize: 5})
	s.SetLogger(nil)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/img/preview.png")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type %q", ct)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 8 || string(body[:8]) != "\x89PNG\r\n\x1a\n" {
		t.Fatalf("not a png (%d bytes)", len(body))
	}
}
