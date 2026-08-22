package tui

import (
	"sort"

	"remiterm/internal/api"
)

// chainKind labels a row in the flat reply chain shown in message detail.
type chainKind int

const (
	chainAncestor chainKind = iota
	chainFocus
	chainChild
	chainMissingParent // placeholder when ReplyToID is set but parent is not loaded
)

// chainItem is one row in the detail reply chain (ancestors → focus → children).
type chainItem struct {
	Kind chainKind
	Msg  api.Message // empty when Kind is chainMissingParent
	// MissingReplyToID is set on chainMissingParent (the id we could not resolve).
	MissingReplyToID string
}

// ancestors returns loaded parents of focusID, root-first (oldest ancestor first).
// Stops when ReplyToID is empty or the parent is not in the store.
func (s *messageStore) ancestors(focusID string) []api.Message {
	if s == nil || focusID == "" {
		return nil
	}
	focus, ok := s.get(focusID)
	if !ok {
		return nil
	}
	var stack []api.Message
	seen := map[string]bool{focusID: true}
	id := focus.ReplyToID
	for id != "" {
		if seen[id] {
			break // cycle guard
		}
		parent, ok := s.get(id)
		if !ok {
			break
		}
		seen[id] = true
		stack = append(stack, parent)
		id = parent.ReplyToID
	}
	// stack is immediate-parent first; reverse to root-first.
	for i, j := 0, len(stack)-1; i < j; i, j = i+1, j-1 {
		stack[i], stack[j] = stack[j], stack[i]
	}
	return stack
}

// children returns messages in the store with ReplyToID == focusID, time-ordered.
func (s *messageStore) children(focusID string) []api.Message {
	if s == nil || focusID == "" {
		return nil
	}
	var out []api.Message
	for _, id := range s.order {
		msg := s.byID[id]
		if msg.ReplyToID == focusID {
			out = append(out, msg)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return lessMessage(out[i], out[j])
	})
	return out
}

// replyChain builds the flat detail list: optional missing-parent placeholder,
// loaded ancestors, focus, then children. Returns nil if focusID is unknown.
func (s *messageStore) replyChain(focusID string) []chainItem {
	if s == nil || focusID == "" {
		return nil
	}
	focus, ok := s.get(focusID)
	if !ok {
		return nil
	}

	var items []chainItem
	anc := s.ancestors(focusID)

	// Missing parent at the top of the loaded ancestor stack (or of focus).
	edgeReplyTo := focus.ReplyToID
	if len(anc) > 0 {
		edgeReplyTo = anc[0].ReplyToID
	}
	if edgeReplyTo != "" {
		if _, ok := s.get(edgeReplyTo); !ok {
			items = append(items, chainItem{
				Kind:             chainMissingParent,
				MissingReplyToID: edgeReplyTo,
			})
		}
	}

	for _, a := range anc {
		items = append(items, chainItem{Kind: chainAncestor, Msg: a})
	}
	items = append(items, chainItem{Kind: chainFocus, Msg: focus})
	for _, c := range s.children(focusID) {
		items = append(items, chainItem{Kind: chainChild, Msg: c})
	}
	return items
}

// focusIndex returns the index of the chainFocus item, or -1.
func focusIndex(chain []chainItem) int {
	for i, it := range chain {
		if it.Kind == chainFocus {
			return i
		}
	}
	return -1
}
