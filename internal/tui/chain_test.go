package tui

import (
	"testing"

	"remiterm/internal/api"
)

func seedStore(msgs ...api.Message) *messageStore {
	s := newMessageStore()
	for _, m := range msgs {
		s.upsert(m)
	}
	return s
}

func TestAncestors(t *testing.T) {
	t.Parallel()
	// A ← B ← C (C replies to B replies to A)
	s := seedStore(
		api.Message{ID: "A", Text: "root", CreatedAt: 100},
		api.Message{ID: "B", Text: "mid", CreatedAt: 200, ReplyToID: "A"},
		api.Message{ID: "C", Text: "leaf", CreatedAt: 300, ReplyToID: "B"},
	)

	got := s.ancestors("C")
	if len(got) != 2 || got[0].ID != "A" || got[1].ID != "B" {
		t.Fatalf("deep chain: %+v", idsOf(got))
	}
	if len(s.ancestors("A")) != 0 {
		t.Fatalf("root should have no ancestors")
	}
	if s.ancestors("missing") != nil {
		t.Fatalf("unknown focus")
	}
	if s.ancestors("") != nil {
		t.Fatalf("empty id")
	}
}

func TestAncestorsStopsAtMissingParent(t *testing.T) {
	t.Parallel()
	// B replies to unloaded "0"; C replies to B.
	s := seedStore(
		api.Message{ID: "B", Text: "mid", CreatedAt: 200, ReplyToID: "0"},
		api.Message{ID: "C", Text: "leaf", CreatedAt: 300, ReplyToID: "B"},
	)
	got := s.ancestors("C")
	if len(got) != 1 || got[0].ID != "B" {
		t.Fatalf("got %+v", idsOf(got))
	}
}

func TestAncestorsCycleGuard(t *testing.T) {
	t.Parallel()
	s := seedStore(
		api.Message{ID: "A", Text: "a", CreatedAt: 100, ReplyToID: "B"},
		api.Message{ID: "B", Text: "b", CreatedAt: 200, ReplyToID: "A"},
	)
	got := s.ancestors("A")
	// Should include B once and stop (not loop forever).
	if len(got) != 1 || got[0].ID != "B" {
		t.Fatalf("got %+v", idsOf(got))
	}
}

func TestChildren(t *testing.T) {
	t.Parallel()
	s := seedStore(
		api.Message{ID: "A", Text: "root", CreatedAt: 100},
		api.Message{ID: "C", Text: "later child", CreatedAt: 300, ReplyToID: "A"},
		api.Message{ID: "B", Text: "earlier child", CreatedAt: 200, ReplyToID: "A"},
		api.Message{ID: "D", Text: "unrelated", CreatedAt: 250, ReplyToID: "X"},
	)
	got := s.children("A")
	if len(got) != 2 || got[0].ID != "B" || got[1].ID != "C" {
		t.Fatalf("time order: %+v", idsOf(got))
	}
	if len(s.children("B")) != 0 {
		t.Fatalf("no children expected")
	}
	if s.children("") != nil {
		t.Fatalf("empty id")
	}
}

func TestReplyChain(t *testing.T) {
	t.Parallel()
	// A ← B ← C, plus sibling D also replies to B
	s := seedStore(
		api.Message{ID: "A", Text: "root", CreatedAt: 100},
		api.Message{ID: "B", Text: "mid", CreatedAt: 200, ReplyToID: "A"},
		api.Message{ID: "C", Text: "leaf", CreatedAt: 300, ReplyToID: "B"},
		api.Message{ID: "D", Text: "sib", CreatedAt: 250, ReplyToID: "B"},
	)

	chain := s.replyChain("B")
	// A (ancestor), B (focus), D then C (children by time)
	want := []struct {
		kind chainKind
		id   string
	}{
		{chainAncestor, "A"},
		{chainFocus, "B"},
		{chainChild, "D"},
		{chainChild, "C"},
	}
	if len(chain) != len(want) {
		t.Fatalf("len %d want %d: %+v", len(chain), len(want), chainKinds(chain))
	}
	for i, w := range want {
		if chain[i].Kind != w.kind || chain[i].Msg.ID != w.id {
			t.Fatalf("[%d] kind=%v id=%q want kind=%v id=%q", i, chain[i].Kind, chain[i].Msg.ID, w.kind, w.id)
		}
	}
	if fi := focusIndex(chain); fi != 1 {
		t.Fatalf("focusIndex = %d", fi)
	}
}

func TestReplyChainMissingParent(t *testing.T) {
	t.Parallel()
	s := seedStore(
		api.Message{ID: "B", Text: "orphan", CreatedAt: 200, ReplyToID: "0"},
		api.Message{ID: "C", Text: "child", CreatedAt: 300, ReplyToID: "B"},
	)
	chain := s.replyChain("B")
	if len(chain) < 2 {
		t.Fatalf("short chain: %+v", chainKinds(chain))
	}
	if chain[0].Kind != chainMissingParent || chain[0].MissingReplyToID != "0" {
		t.Fatalf("want missing parent first: %+v", chain[0])
	}
	if chain[1].Kind != chainFocus || chain[1].Msg.ID != "B" {
		t.Fatalf("want focus B: %+v", chain[1])
	}
	if chain[2].Kind != chainChild || chain[2].Msg.ID != "C" {
		t.Fatalf("want child C: %+v", chain[2])
	}

	// Unknown focus
	if s.replyChain("nope") != nil {
		t.Fatal("unknown focus should be nil")
	}
}

func TestReplyChainMissingParentOnDeepEdge(t *testing.T) {
	t.Parallel()
	// Loaded: B←C; B's parent "0" not loaded.
	s := seedStore(
		api.Message{ID: "B", Text: "mid", CreatedAt: 200, ReplyToID: "0"},
		api.Message{ID: "C", Text: "leaf", CreatedAt: 300, ReplyToID: "B"},
	)
	chain := s.replyChain("C")
	// missing "0", B ancestor, C focus
	if len(chain) != 3 {
		t.Fatalf("len %d: %+v", len(chain), chainKinds(chain))
	}
	if chain[0].Kind != chainMissingParent || chain[0].MissingReplyToID != "0" {
		t.Fatalf("edge missing: %+v", chain[0])
	}
	if chain[1].Kind != chainAncestor || chain[1].Msg.ID != "B" {
		t.Fatalf("ancestor: %+v", chain[1])
	}
	if chain[2].Kind != chainFocus || chain[2].Msg.ID != "C" {
		t.Fatalf("focus: %+v", chain[2])
	}
}

func idsOf(msgs []api.Message) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.ID
	}
	return out
}

func chainKinds(items []chainItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		switch it.Kind {
		case chainMissingParent:
			out[i] = "missing:" + it.MissingReplyToID
		case chainAncestor:
			out[i] = "anc:" + it.Msg.ID
		case chainFocus:
			out[i] = "focus:" + it.Msg.ID
		case chainChild:
			out[i] = "child:" + it.Msg.ID
		default:
			out[i] = "?"
		}
	}
	return out
}
