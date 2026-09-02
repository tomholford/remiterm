package tui

import (
	"sort"
	"strconv"
	"time"

	"remiterm/internal/api"
)

// editGraceMs: the public API often sets edited_at a few seconds after
// created_at for web-sent messages (server-side touch / enrichment), so a
// non-nil edited_at alone is not a user edit. Only surface "(edited)" when
// the gap is clearly intentional. Timestamps from the API are unix millis.
const editGraceMs int64 = 60_000

// msEpochFloor: values above this are treated as unix milliseconds
// (~2001-09-09 in ms). Seconds-since-epoch for real chat messages sits
// around 1e9; millis around 1e12.
const msEpochFloor int64 = 1_000_000_000_000

// isEdited reports whether msg was meaningfully edited by a user.
func isEdited(msg api.Message) bool {
	if msg.EditedAt == nil || msg.CreatedAt <= 0 {
		return false
	}
	return *msg.EditedAt > msg.CreatedAt+editGraceMs
}

// messageTime converts an API created_at / edited_at value to local time.
// Live RemiliaNET global-chat returns unix milliseconds; older fixtures and
// some endpoints may still use seconds.
func messageTime(ts int64) time.Time {
	if ts <= 0 {
		return time.Time{}
	}
	if ts >= msEpochFloor {
		return time.UnixMilli(ts)
	}
	return time.Unix(ts, 0)
}

// messageStore keeps messages ordered oldest→newest, keyed by id.
type messageStore struct {
	byID        map[string]api.Message
	order       []string // oldest first
	hasMore     bool
	cursor      string // next_cursor for older pages
	liveTrusted bool   // true after the first live (latest-page) clip
}

func newMessageStore() *messageStore {
	return &messageStore{byID: make(map[string]api.Message)}
}

// merge inserts/updates messages. Returns count of brand-new ids.
func (s *messageStore) merge(msgs []MessageLike, hasMore *bool, nextCursor string, olderPage bool) int {
	added := 0
	for _, m := range msgs {
		msg := m.asMessage()
		if msg.ID == "" {
			continue
		}
		if _, ok := s.byID[msg.ID]; !ok {
			added++
			s.order = append(s.order, msg.ID)
		}
		s.byID[msg.ID] = msg
	}
	if added > 0 {
		sort.SliceStable(s.order, func(i, j int) bool {
			return lessMessage(s.byID[s.order[i]], s.byID[s.order[j]])
		})
	}
	if olderPage {
		if hasMore != nil {
			s.hasMore = *hasMore
		}
		if nextCursor != "" {
			s.cursor = nextCursor
		}
	} else {
		// Newest page: only update hasMore/cursor if we have no history yet
		// or the server says there is more older content.
		if len(s.order) > 0 && hasMore != nil {
			// Prefer server has_more when this is the initial load.
			if s.cursor == "" {
				s.hasMore = *hasMore
				s.cursor = nextCursor
			}
		}
		if s.cursor == "" && nextCursor != "" {
			s.cursor = nextCursor
			if hasMore != nil {
				s.hasMore = *hasMore
			}
		}
	}
	return added
}

// mergeAPI is a convenience for api.ListMessagesResponse.
func (s *messageStore) mergeAPI(res api.ListMessagesResponse, olderPage bool) int {
	likes := make([]MessageLike, len(res.Messages))
	for i := range res.Messages {
		likes[i] = MessageLike{Message: res.Messages[i]}
	}
	hm := res.HasMore
	return s.merge(likes, &hm, res.NextCursor, olderPage)
}

// mergeLatest merges a newest-page poll and clips the in-memory list to that
// page's live tail the first time. A page with no shared ids is a seam
// (replace the store with the page). Overlap still drops rows older than the
// page's oldest, so a cache bag that straddles a hole cannot hide it from
// backscroll. Later latest polls only append; they must not clip backfill.
func (s *messageStore) mergeLatest(res api.ListMessagesResponse) int {
	overlap := false
	for i := range res.Messages {
		id := res.Messages[i].ID
		if id == "" {
			continue
		}
		if _, ok := s.byID[id]; ok {
			overlap = true
			break
		}
	}
	added := s.mergeAPI(res, false)
	if s.liveTrusted || len(res.Messages) == 0 {
		return added
	}
	cut := oldestMessage(res.Messages)
	if cut.ID == "" {
		return added
	}
	if !overlap {
		s.retainIDs(messageIDs(res.Messages))
	} else {
		s.dropOlderThan(cut.ID)
	}
	s.liveTrusted = true
	if res.NextCursor != "" {
		s.cursor = res.NextCursor
	} else if s.len() > 0 {
		s.cursor = s.idAt(0)
	}
	s.hasMore = res.HasMore
	return added
}

func oldestMessage(msgs []api.Message) api.Message {
	var best api.Message
	for i := range msgs {
		m := msgs[i]
		if m.ID == "" {
			continue
		}
		if best.ID == "" || lessMessage(m, best) {
			best = m
		}
	}
	return best
}

func messageIDs(msgs []api.Message) []string {
	out := make([]string, 0, len(msgs))
	for i := range msgs {
		if msgs[i].ID != "" {
			out = append(out, msgs[i].ID)
		}
	}
	return out
}

func (s *messageStore) retainIDs(ids []string) {
	if s == nil {
		return
	}
	keep := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			keep[id] = struct{}{}
		}
	}
	order := make([]string, 0, len(keep))
	for _, id := range s.order {
		if _, ok := keep[id]; ok {
			order = append(order, id)
		} else {
			delete(s.byID, id)
		}
	}
	s.order = order
}

func (s *messageStore) dropOlderThan(id string) {
	if s == nil || id == "" {
		return
	}
	idx := s.indexOf(id)
	if idx <= 0 {
		return
	}
	for _, oid := range s.order[:idx] {
		delete(s.byID, oid)
	}
	s.order = append([]string(nil), s.order[idx:]...)
}

func (s *messageStore) upsert(msg api.Message) {
	if msg.ID == "" {
		return
	}
	if _, ok := s.byID[msg.ID]; !ok {
		s.order = append(s.order, msg.ID)
	}
	s.byID[msg.ID] = msg
	sort.SliceStable(s.order, func(i, j int) bool {
		return lessMessage(s.byID[s.order[i]], s.byID[s.order[j]])
	})
}

func (s *messageStore) list() []api.Message {
	out := make([]api.Message, 0, len(s.order))
	for _, id := range s.order {
		out = append(out, s.byID[id])
	}
	return out
}

func (s *messageStore) get(id string) (api.Message, bool) {
	m, ok := s.byID[id]
	return m, ok
}

// indexOf returns the order index for id, or -1 if unknown.
func (s *messageStore) indexOf(id string) int {
	if s == nil || id == "" {
		return -1
	}
	for i, oid := range s.order {
		if oid == id {
			return i
		}
	}
	return -1
}

// idAt returns the message id at order index, or "" if out of range.
func (s *messageStore) idAt(i int) string {
	if s == nil || i < 0 || i >= len(s.order) {
		return ""
	}
	return s.order[i]
}

func (s *messageStore) len() int { return len(s.order) }

// MessageLike wraps api.Message for merge generality.
type MessageLike struct {
	api.Message
}

func (m MessageLike) asMessage() api.Message { return m.Message }

func lessMessage(a, b api.Message) bool {
	if a.CreatedAt != b.CreatedAt {
		return a.CreatedAt < b.CreatedAt
	}
	// Numeric id compare when possible.
	ai, aerr := strconv.ParseInt(a.ID, 10, 64)
	bi, berr := strconv.ParseInt(b.ID, 10, 64)
	if aerr == nil && berr == nil {
		return ai < bi
	}
	return a.ID < b.ID
}
