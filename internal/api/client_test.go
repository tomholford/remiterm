package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListGlobalChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/global-chat/messages" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("auth %q", got)
		}
		if r.URL.Query().Get("limit") != "50" {
			t.Fatalf("limit %q", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("before") != "998" {
			t.Fatalf("before %q", r.URL.Query().Get("before"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"messages": []map[string]any{
					{
						"id":   "1042",
						"text": "gm",
						"author": map[string]any{
							"handle":       "milady",
							"display_name": "Milady",
						},
						"created_at": 1754870400,
						"media": []map[string]any{
							{"kind": "image", "url": "https://example.com/a.png"},
						},
						"reply_to_id": "1000",
					},
				},
				"has_more":    true,
				"next_cursor": "998",
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	out, err := c.ListGlobalChat(context.Background(), 50, "998")
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 || out.Messages[0].Text != "gm" {
		t.Fatalf("%+v", out)
	}
	if !out.HasMore || out.NextCursor != "998" {
		t.Fatalf("%+v", out)
	}
	if out.Messages[0].ReplyToID != "1000" || len(out.Messages[0].Media) != 1 {
		t.Fatalf("%+v", out.Messages[0])
	}
}

func TestPostGlobalChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/global-chat/messages" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["text"] != "gm" || body["reply_to_id"] != "1042" {
			t.Fatalf("%v", body)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":          "2000",
				"text":        "gm",
				"reply_to_id": "1042",
				"author":      map[string]any{"handle": "me", "display_name": "Me"},
				"created_at":  1754870401,
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	msg, err := c.PostGlobalChat(context.Background(), "gm", "1042")
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != "2000" || msg.ReplyToID != "1042" {
		t.Fatalf("%+v", msg)
	}
}

func TestAPIErrorEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "rate_limited",
				"message": "slow down",
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	_, err := c.PostGlobalChat(context.Background(), "x", "")
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.Code != "rate_limited" || ae.Status != 429 {
		t.Fatalf("%+v", ae)
	}
}

func TestMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/me" {
			t.Fatalf("%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{
				"username":    "remilia",
				"displayName": "Remilia",
				"bio":         "hi",
				"location":    "milady village",
				"friendCount": 3,
			},
			"isOwnProfile": true,
			"viewerContext": map[string]any{
				"areFriends": false,
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if me.Handle() != "remilia" || !me.IsOwnProfile || me.User.FriendCount != 3 {
		t.Fatalf("%+v", me)
	}
}

func TestMeStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/me/stats" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("auth %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"handle":       "remilia",
				"display_name": "Remilia",
				"stats": map[string]any{
					"ethereum": map[string]any{
						"cult_tier":   "initiate",
						"total_owned": 3,
					},
					"miladychan": map[string]any{"posts": 12},
				},
				"aggregate_scores": map[string]any{
					"social_credit": 140,
					"beetles":       7,
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	out, err := c.MeStats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Handle != "remilia" || out.DisplayName != "Remilia" {
		t.Fatalf("%+v", out)
	}
	if out.Stats.Ethereum.CultTier != "initiate" || out.Stats.Ethereum.TotalOwned != 3 {
		t.Fatalf("%+v", out.Stats.Ethereum)
	}
	if out.AggregateScores["social_credit"] != 140 || out.AggregateScores["beetles"] != 7 {
		t.Fatalf("%+v", out.AggregateScores)
	}
}

func TestMeStatsInsufficientScope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":    "insufficient_scope",
				"message": "scope remilia:stats.read required",
			},
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	_, err := c.MeStats(context.Background())
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.Status != 403 || ae.Code != "insufficient_scope" {
		t.Fatalf("%+v", ae)
	}
}

func TestGetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/users/milady" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"user": map[string]any{
				"username":    "milady",
				"displayName": "Milady",
				"bio":         "gm",
				"location":    "village",
				"pfpUrl":      "https://example.com/p.png",
				"friendCount": 20,
			},
			"viewerContext": map[string]any{
				"areFriends":          true,
				"canPoke":             true,
				"pokeCooldownSeconds": 0,
			},
			"isAuthenticated": true,
			"isOwnProfile":    false,
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	p, err := c.GetUser(context.Background(), "milady")
	if err != nil {
		t.Fatal(err)
	}
	if p.Handle() != "milady" || p.User.DisplayName != "Milady" || !p.ViewerContext.AreFriends {
		t.Fatalf("%+v", p)
	}
	if p.IsOwnProfile || !p.IsAuthenticated || p.User.FriendCount != 20 {
		t.Fatalf("%+v", p)
	}
}

func TestGetUserNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "user not found", http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	_, err := c.GetUser(context.Background(), "nope")
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.Status != 404 || ae.Code != "not_found" {
		t.Fatalf("%+v", ae)
	}
}

func TestPoke(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/milady/poke" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Fatalf("auth %q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	out, err := c.Poke(context.Background(), "milady")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Success {
		t.Fatalf("%+v", out)
	}
}

func TestPokeCooldown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "poke on cooldown (23h59m30s remaining)",
		})
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, "tok")
	_, err := c.Poke(context.Background(), "milady")
	var ae *Error
	if !errors.As(err, &ae) {
		t.Fatalf("want *Error, got %T %v", err, err)
	}
	if ae.Code != "poke_cooldown" || ae.Status != 400 {
		t.Fatalf("%+v", ae)
	}
	if !strings.Contains(ae.Message, "cooldown") {
		t.Fatalf("%+v", ae)
	}
}
