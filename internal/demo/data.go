package demo

import (
	"fmt"
	"strconv"
	"time"

	"remiterm/internal/api"
)

// Options configures the synthetic chat corpus and request delay.
type Options struct {
	Pages    int
	PageSize int
	Latency  time.Duration
}

func (o *Options) normalize() {
	if o.Pages <= 0 {
		// Plenty of older pages for backscroll drills (page size × this).
		o.Pages = 100
	}
	if o.PageSize <= 0 {
		o.PageSize = 100
	}
	if o.PageSize > 100 {
		o.PageSize = 100
	}
}

// TotalMessages is the full corpus size.
func (o Options) TotalMessages() int {
	o.normalize()
	return o.Pages * o.PageSize
}

var demoHandles = []struct {
	handle, display string
}{
	{"demo", "Demo User"},
	{"milady", "Milady"},
	{"remilia", "Remilia"},
	{"alice", "Alice"},
	{"bob", "Bob"},
	{"carol", "Carol"},
}

// buildCorpus returns messages oldest→newest with ids "1"…"N".
// Text is multi-line so a tall terminal still overflows on the first page
// (TUI loads ~50 msgs at a time) and backscroll can be exercised.
// Every 7th message (i%7==0, i>=7) replies to the previous id so the TUI
// reply-snippet path is easy to exercise in `remiterm demo`. One orphan
// reply (id 3 → "0") keeps the missing-parent ↳ path visible.
// Message 5 carries a still so `i` in `remiterm demo` can exercise preview.
// Message 10 is a hub with two children (11, 12) for message-detail chain UI;
// a few messages carry sample reactions for detail rendering.
func buildCorpus(pages, pageSize int) []api.Message {
	total := pages * pageSize
	out := make([]api.Message, 0, total)
	// Base near "now" so timestamps look live; step back per message.
	base := time.Now().UnixMilli()
	for i := 1; i <= total; i++ {
		h := demoHandles[(i-1)%len(demoHandles)]
		// Older messages have smaller ids and earlier times.
		created := base - int64(total-i)*45_000 // 45s apart
		msg := api.Message{
			ID: strconv.Itoa(i),
			Author: api.Author{
				Handle:      h.handle,
				DisplayName: h.display,
			},
			Text:      demoText(i, h.handle),
			CreatedAt: created,
		}
		switch {
		case i == 3:
			msg.ReplyToID = "0" // never loaded
		case i == 11 || i == 12:
			// Fan-out under #10 for detail children list.
			msg.ReplyToID = "10"
		case i >= 7 && i%7 == 0:
			msg.ReplyToID = strconv.Itoa(i - 1)
		}
		if i == 5 {
			msg.Media = []api.Media{{
				Kind: "image",
				URL:  "/img/preview.png",
			}}
		}
		if i == 10 {
			msg.Reactions = []api.Reaction{
				{Emoji: "🔥", Count: 2},
				{Emoji: "🤍", Count: 1},
			}
		}
		out = append(out, msg)
	}
	return out
}

// demoText is intentionally multi-line. Short one-liners fit a tall screen in
// a single page and never arm older-history loads.
func demoText(i int, handle string) string {
	// Vary height a bit so the viewport doesn't look perfectly uniform.
	switch i % 5 {
	case 0:
		return fmt.Sprintf(
			"demo msg #%d from @%s — scroll up for history\n"+
				"extra padding so tall terminals still need to scroll\n"+
				"third line · id=%d · keep going up for older pages",
			i, handle, i,
		)
	case 1:
		return fmt.Sprintf(
			"demo msg #%d from @%s — scroll up for history\n"+
				"two-line body so rows eat vertical space",
			i, handle,
		)
	default:
		return fmt.Sprintf(
			"demo msg #%d from @%s — scroll up for history\n"+
				"filler line · page material for backscroll UX",
			i, handle,
		)
	}
}

// DemoMe is the fixed profile for GET /me.
func DemoMe() api.Profile {
	return api.Profile{
		User: api.ProfileUser{
			Username:    "demo",
			DisplayName: "Demo User",
			Bio:         "local remiterm demo — no network",
			Location:    "localhost",
			FriendCount: 0,
		},
		ViewerContext: api.ViewerContext{
			AreFriends: false,
			CanPoke:    false,
		},
		IsAuthenticated: true,
		IsOwnProfile:    true,
	}
}

// DemoMeStats is the fixed payload for GET /me/stats (inside data).
func DemoMeStats() api.MeStats {
	return api.MeStats{
		Handle:      "demo",
		DisplayName: "Demo User",
		Stats: api.MePlatformStats{
			Ethereum: api.MeEthereumStats{
				CultTier:   "initiate",
				TotalOwned: 2,
			},
		},
		AggregateScores: map[string]float64{
			"beetles":       7,
			"social_credit": 140,
		},
	}
}

// profileFor returns a stub profile for GET /users/{username}.
func profileFor(username string) api.Profile {
	if username == "demo" {
		return DemoMe()
	}
	for _, h := range demoHandles {
		if h.handle == username {
			return api.Profile{
				User: api.ProfileUser{
					Username:    h.handle,
					DisplayName: h.display,
					Bio:         "demo profile",
				},
				ViewerContext:   api.ViewerContext{CanPoke: true},
				IsAuthenticated: true,
				IsOwnProfile:    false,
			}
		}
	}
	return api.Profile{}
}
