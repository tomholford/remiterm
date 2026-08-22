package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"remiterm/internal/api"
	"remiterm/internal/cache"
)

func TestSelectedAuthorHandle(t *testing.T) {
	m := New(nil, 0)
	if got := m.selectedAuthorHandle(); got != "" {
		t.Fatalf("empty store: %q", got)
	}

	m.store.upsert(api.Message{
		ID:     "1",
		Text:   "hi",
		Author: api.Author{Handle: "alice", DisplayName: "Alice"},
	})
	m.store.upsert(api.Message{
		ID:     "2",
		Text:   "yo",
		Author: api.Author{Handle: "bob"},
	})
	// selected < 0 → last message
	m.selected = -1
	if got := m.selectedAuthorHandle(); got != "bob" {
		t.Fatalf("last: got %q", got)
	}
	m.selected = 0
	if got := m.selectedAuthorHandle(); got != "alice" {
		t.Fatalf("idx0: got %q", got)
	}
}

func TestProfileFooterHint(t *testing.T) {
	own := profileFooterHint(api.Profile{IsOwnProfile: true})
	if strings.Contains(own, "poke") {
		t.Fatalf("own should hide poke: %q", own)
	}
	other := profileFooterHint(api.Profile{})
	if !strings.Contains(other, "P poke") {
		t.Fatalf("other should offer poke: %q", other)
	}
}

func TestPokeStatusFromError(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, ""},
		{&api.Error{Code: "poke_cooldown", Message: "poke on cooldown (1h remaining)"}, "poke on cooldown (1h remaining)"},
		{&api.Error{Code: "insufficient_scope", Message: "scope remilia:interact.poke required"}, "need remilia:interact.poke scope"},
		{&api.Error{Status: 400, Message: "user not found"}, "user not found"},
		{errors.New("network down"), "network down"},
	}
	for _, tc := range cases {
		got := pokeStatusFromError(tc.err)
		if got != tc.want {
			t.Fatalf("err=%v: got %q want %q", tc.err, got, tc.want)
		}
	}
}

func TestRenderProfileCardLoading(t *testing.T) {
	m := New(nil, 0)
	m.profile = profileModal{open: true, loading: true, handle: "milady"}
	out := m.renderProfileCard(40)
	if !strings.Contains(out, "@milady") || !strings.Contains(out, "loading") {
		t.Fatalf("%q", out)
	}
}

func TestRenderProfileCardLoaded(t *testing.T) {
	m := New(nil, 0)
	m.profile = profileModal{
		open:   true,
		handle: "milady",
		profile: api.Profile{
			User: api.ProfileUser{
				Username:    "milady",
				DisplayName: "Milady Maker",
				Bio:         "gm",
				Location:    "village",
				FriendCount: 5,
			},
			ViewerContext: api.ViewerContext{AreFriends: true},
		},
	}
	out := m.renderProfileCard(40)
	for _, want := range []string{"@milady", "Milady Maker", "gm", "village", "5 friends", "P poke"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

func TestFormatStatsLines(t *testing.T) {
	got := formatStatsLines(api.MeStats{
		AggregateScores: map[string]float64{
			"social_credit": 140,
			"beetles":       7,
			"zeroed":        0,
			"zeta":          1,
			"alpha":         2,
			"mid":           3,
			"extra":         9, // 6 non-zero → cap 5 after sort
		},
		Stats: api.MePlatformStats{
			Ethereum: api.MeEthereumStats{CultTier: "initiate", TotalOwned: 3},
		},
	})
	// zeros omitted; first 5 sorted keys among non-zero: alpha, beetles, extra, mid, social_credit
	want := []string{
		"alpha 2",
		"beetles 7",
		"extra 9",
		"mid 3",
		"social credit 140",
		"cult initiate",
		"owned 3",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %q want %q", got, want)
		}
	}
	if lines := formatStatsLines(api.MeStats{}); len(lines) != 0 {
		t.Fatalf("empty: %q", lines)
	}
}

func TestRenderProfileCardOwnStats(t *testing.T) {
	base := api.Profile{
		User:         api.ProfileUser{Username: "milady", DisplayName: "Milady Maker"},
		IsOwnProfile: true,
	}

	t.Run("loading", func(t *testing.T) {
		m := New(nil, 0)
		m.profile = profileModal{open: true, handle: "milady", profile: base, statsLoading: true}
		out := m.renderProfileCard(40)
		if !strings.Contains(out, "stats") {
			t.Fatalf("missing stats loading: %q", out)
		}
	})
	t.Run("filled", func(t *testing.T) {
		m := New(nil, 0)
		m.profile = profileModal{
			open:    true,
			handle:  "milady",
			profile: base,
			stats: api.MeStats{
				AggregateScores: map[string]float64{"beetles": 7},
				Stats:           api.MePlatformStats{Ethereum: api.MeEthereumStats{CultTier: "initiate", TotalOwned: 2}},
			},
		}
		out := m.renderProfileCard(40)
		for _, want := range []string{"beetles 7", "cult initiate", "owned 2"} {
			if !strings.Contains(out, want) {
				t.Fatalf("missing %q in %q", want, out)
			}
		}
	})
	t.Run("hidden", func(t *testing.T) {
		m := New(nil, 0)
		m.profile = profileModal{
			open:        true,
			handle:      "milady",
			profile:     base,
			statsHidden: true,
			stats:       api.MeStats{AggregateScores: map[string]float64{"beetles": 7}},
		}
		out := m.renderProfileCard(40)
		if strings.Contains(out, "beetles") || strings.Contains(out, "stats") {
			t.Fatalf("hidden should omit stats: %q", out)
		}
	})
	t.Run("other user", func(t *testing.T) {
		m := New(nil, 0)
		m.profile = profileModal{
			open:    true,
			handle:  "alice",
			profile: api.Profile{User: api.ProfileUser{Username: "alice"}},
			stats:   api.MeStats{AggregateScores: map[string]float64{"beetles": 7}},
		}
		out := m.renderProfileCard(40)
		if strings.Contains(out, "beetles") || strings.Contains(out, "stats") {
			t.Fatalf("other user should omit stats: %q", out)
		}
	})
}

func TestApplyStatsMsg(t *testing.T) {
	m := New(nil, 0)
	m.meHandle = "milady"
	m.profile = profileModal{
		open:         true,
		handle:       "milady",
		profile:      api.Profile{IsOwnProfile: true, User: api.ProfileUser{Username: "milady"}},
		statsLoading: true,
		statsGen:     1,
	}

	next, _ := m.applyStatsMsg(statsMsg{gen: 0, err: errors.New("stale")})
	got := next.(Model)
	if !got.profile.statsLoading || got.profile.statsErr != "" {
		t.Fatalf("stale gen should be ignored: %+v", got.profile)
	}

	next, _ = m.applyStatsMsg(statsMsg{
		gen: 1,
		err: &api.Error{Code: "insufficient_scope", Message: "scope remilia:stats.read required"},
	})
	got = next.(Model)
	if !got.profile.statsHidden || got.profile.statsLoading || got.profile.statsErr != "" {
		t.Fatalf("scope: %+v", got.profile)
	}

	m.profile.statsHidden = false
	m.profile.statsLoading = true
	next, _ = m.applyStatsMsg(statsMsg{gen: 1, err: errors.New("network")})
	got = next.(Model)
	if got.profile.statsErr != "stats unavailable" || got.profile.statsHidden {
		t.Fatalf("err: %+v", got.profile)
	}

	m.profile.statsErr = ""
	m.profile.statsLoading = true
	next, _ = m.applyStatsMsg(statsMsg{
		gen:   1,
		stats: api.MeStats{Handle: "milady", AggregateScores: map[string]float64{"beetles": 7}},
	})
	got = next.(Model)
	if got.profile.statsLoading || got.profile.stats.Handle != "milady" || got.profile.stats.AggregateScores["beetles"] != 7 {
		t.Fatalf("ok: %+v", got.profile)
	}
}

func TestApplyProfileMsgOwnStartsStats(t *testing.T) {
	m := New(api.New("http://127.0.0.1:1", "tok"), 0)
	m.profile = profileModal{open: true, loading: true, handle: "alice"}
	next, cmd := m.applyProfileMsg(profileMsg{
		handle: "alice",
		profile: api.Profile{
			User:         api.ProfileUser{Username: "alice"},
			IsOwnProfile: true,
		},
	})
	got := next.(Model)
	if !got.profile.statsLoading || got.profile.statsGen != 1 {
		t.Fatalf("own should start stats: %+v", got.profile)
	}
	if cmd == nil {
		t.Fatal("expected stats cmd")
	}

	m = New(api.New("http://127.0.0.1:1", "tok"), 0)
	m.profile = profileModal{open: true, loading: true, handle: "bob"}
	_, cmd = m.applyProfileMsg(profileMsg{
		handle: "bob",
		profile: api.Profile{
			User:         api.ProfileUser{Username: "bob"},
			IsOwnProfile: false,
		},
	})
	if cmd != nil {
		t.Fatal("other user should not fetch stats")
	}

	m = New(api.New("http://127.0.0.1:1", "tok"), 0)
	m.profile = profileModal{open: true, loading: true, handle: "alice", statsLoading: true, statsGen: 1}
	_, cmd = m.applyProfileMsg(profileMsg{
		handle: "alice",
		me:     true,
		profile: api.Profile{
			User:         api.ProfileUser{Username: "alice"},
			IsOwnProfile: true,
		},
	})
	if cmd != nil {
		t.Fatal("already-started stats should not refetch")
	}
}

func TestWrapPlain(t *testing.T) {
	s := wrapPlain("hello world this is long", 10)
	if !strings.Contains(s, "\n") {
		t.Fatalf("expected wrap: %q", s)
	}
}

func TestOpenProfileCacheHitPaintsImmediately(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	p := api.Profile{User: api.ProfileUser{Username: "alice", Bio: "cached"}}
	if err = db.SaveProfile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	m := New(nil, 0)
	m.AttachCache(db)
	next, cmd := m.openProfile("alice", false)
	got := next.(Model)
	if got.profile.loading {
		t.Fatal("cache hit should not wait")
	}
	if got.profile.profile.User.Bio != "cached" {
		t.Fatalf("%+v", got.profile.profile)
	}
	if cmd == nil {
		t.Fatal("should still fetch")
	}
}

func TestApplyProfileMsgPersists(t *testing.T) {
	db, err := cache.OpenMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	m := New(nil, 0)
	m.AttachCache(db)
	m.profile = profileModal{open: true, loading: true, handle: "bob"}
	_, _ = m.applyProfileMsg(profileMsg{
		handle:  "bob",
		profile: api.Profile{User: api.ProfileUser{Username: "bob", Bio: "live"}},
	})
	got, _, err := db.GetProfile(context.Background(), "bob")
	if err != nil {
		t.Fatal(err)
	}
	if got.User.Bio != "live" {
		t.Fatalf("%+v", got)
	}
}

func TestApplyProfileMsgKeepsSnapshotOnError(t *testing.T) {
	m := New(nil, 0)
	m.profile = profileModal{
		open:    true,
		loading: false,
		handle:  "alice",
		profile: api.Profile{User: api.ProfileUser{Username: "alice", Bio: "stale"}},
	}
	next, _ := m.applyProfileMsg(profileMsg{handle: "alice", err: errors.New("offline")})
	got := next.(Model)
	if got.profile.profile.User.Bio != "stale" {
		t.Fatalf("snapshot %+v", got.profile.profile)
	}
	if got.profile.err != "" {
		t.Fatalf("err %q", got.profile.err)
	}
	if got.profile.loading {
		t.Fatal("loading")
	}
}

func TestApplyProfileMsgErrorWithoutSnapshot(t *testing.T) {
	m := New(nil, 0)
	m.profile = profileModal{open: true, loading: true, handle: "alice"}
	next, _ := m.applyProfileMsg(profileMsg{handle: "alice", err: errors.New("offline")})
	got := next.(Model)
	if got.profile.err != "offline" {
		t.Fatalf("%q", got.profile.err)
	}
}
