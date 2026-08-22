package demo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"remiterm/internal/api"
)

// Server is a local fake RemiliaNET-shaped API for TUI development.
type Server struct {
	opts    Options
	mu      sync.Mutex
	msgs    []api.Message // oldest → newest
	nextID  int
	calls   atomic.Int64
	logf    func(format string, args ...any)
	httpSrv *http.Server
	ln      net.Listener
}

// NewServer builds a demo server. Call Listen or Start; Close when done.
func NewServer(opts Options) *Server {
	opts.normalize()
	s := &Server{
		opts:   opts,
		msgs:   buildCorpus(opts.Pages, opts.PageSize),
		nextID: opts.TotalMessages() + 1,
		logf:   log.Printf,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/me", s.handleMe)
	mux.HandleFunc("/me/stats", s.handleMeStats)
	mux.HandleFunc("/global-chat/messages", s.handleMessages)
	mux.HandleFunc("/users/", s.handleUsers)
	mux.HandleFunc("/img/preview.png", s.handlePreviewPNG)
	s.httpSrv = &http.Server{Handler: mux}
	return s
}

// SetLogger overrides request logging (default log.Printf). Pass nil to silence.
func (s *Server) SetLogger(fn func(format string, args ...any)) {
	s.logf = fn
}

// CallCount is the number of list/post/me/user requests handled.
func (s *Server) CallCount() int64 {
	return s.calls.Load()
}

// URL is the base URL after Listen/Start (no trailing slash).
func (s *Server) URL() string {
	if s.ln == nil {
		return ""
	}
	return "http://" + s.ln.Addr().String()
}

// Listen binds 127.0.0.1:0 and starts serving in a goroutine.
func (s *Server) Listen() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s.ln = ln
	go func() {
		_ = s.httpSrv.Serve(ln)
	}()
	return nil
}

// Close shuts down the HTTP server.
func (s *Server) Close() error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Close()
}

// Handler exposes the mux for httptest tests without Listen.
func (s *Server) Handler() http.Handler {
	return s.httpSrv.Handler
}

func (s *Server) delay() {
	if s.opts.Latency > 0 {
		time.Sleep(s.opts.Latency)
	}
}

func (s *Server) logreq(format string, args ...any) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

func (s *Server) handlePreviewPNG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(demoPreviewPNG())
}

func demoPreviewPNG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := range 32 {
		for x := range 32 {
			img.Set(x, y, color.RGBA{
				R: uint8(x * 8),
				G: uint8(y * 8),
				B: 180,
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	s.delay()
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, DemoMe())
}

func (s *Server) handleMeStats(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	s.delay()
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"data": DemoMeStats()})
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	s.delay()
	path := strings.TrimPrefix(r.URL.Path, "/users/")
	path = strings.Trim(path, "/")
	if path == "" {
		http.Error(w, "username required", http.StatusBadRequest)
		return
	}
	// POST /users/{handle}/poke
	if strings.HasSuffix(path, "/poke") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, api.PokeResult{Success: true})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := profileFor(path)
	if p.User.Username == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, p)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	s.calls.Add(1)
	s.delay()
	switch r.Method {
	case http.MethodGet:
		s.listMessages(w, r)
	case http.MethodPost:
		s.postMessage(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listMessages(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	before := r.URL.Query().Get("before")

	s.mu.Lock()
	defer s.mu.Unlock()

	page, hasMore, nextCursor := pageBefore(s.msgs, before, limit)
	s.logreq("demo ListGlobalChat before=%q limit=%d → %d msgs has_more=%v cursor=%q calls=%d",
		before, limit, len(page), hasMore, nextCursor, s.calls.Load())

	writeJSON(w, map[string]any{
		"data": api.ListMessagesResponse{
			Messages:   page,
			HasMore:    hasMore,
			NextCursor: nextCursor,
		},
	})
}

func (s *Server) postMessage(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text      string `json:"text"`
		ReplyToID string `json:"reply_to_id"`
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	msg := api.Message{
		ID: strconv.Itoa(s.nextID),
		Author: api.Author{
			Handle:      "demo",
			DisplayName: "Demo User",
		},
		Text:      text,
		CreatedAt: time.Now().UnixMilli(),
		ReplyToID: body.ReplyToID,
	}
	s.nextID++
	s.msgs = append(s.msgs, msg)
	s.mu.Unlock()

	s.logreq("demo PostGlobalChat id=%s calls=%d", msg.ID, s.calls.Load())
	writeJSON(w, map[string]any{"data": msg})
}

// pageBefore returns up to limit messages strictly older than before
// (or the newest page when before is empty). Messages in the page are
// oldest→newest. next_cursor is the oldest id in the returned page.
func pageBefore(all []api.Message, before string, limit int) (page []api.Message, hasMore bool, nextCursor string) {
	n := len(all)
	if n == 0 || limit <= 0 {
		return nil, false, ""
	}

	end := n // exclusive end index into all
	if before != "" {
		idx := -1
		for i, m := range all {
			if m.ID == before {
				idx = i
				break
			}
		}
		if idx <= 0 {
			// Unknown cursor or nothing older.
			return nil, false, ""
		}
		end = idx
	}

	start := end - limit
	if start < 0 {
		start = 0
	}
	page = make([]api.Message, end-start)
	copy(page, all[start:end])
	hasMore = start > 0
	if len(page) > 0 {
		nextCursor = page[0].ID
	}
	return page, hasMore, nextCursor
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Best-effort; connection may already be half-closed.
		_, _ = fmt.Fprintf(w, `{"error":{"message":%q}}`, err.Error())
	}
}
