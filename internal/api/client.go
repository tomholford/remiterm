package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client talks to RemiliaNET /api/v1.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
	UserAgent  string
}

// New creates a client with sensible defaults.
func New(baseURL, token string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Token:     token,
		UserAgent: "remiterm/dev",
	}
}

// Author is a message or profile identity.
type Author struct {
	Handle      string `json:"handle"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// Media attached to a chat message.
type Media struct {
	Kind         string `json:"kind"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnail_url"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	IsAnimated   bool   `json:"is_animated"`
}

// Reaction summary on a message.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

// Message is a global-chat message.
type Message struct {
	ID        string     `json:"id"`
	Author    Author     `json:"author"`
	Text      string     `json:"text"`
	CreatedAt int64      `json:"created_at"`
	ReplyToID string     `json:"reply_to_id,omitempty"`
	Media     []Media    `json:"media,omitempty"`
	Reactions []Reaction `json:"reactions,omitempty"`
	EditedAt  *int64     `json:"edited_at,omitempty"`
}

// ListMessagesResponse is the data envelope for GET /global-chat/messages.
type ListMessagesResponse struct {
	Messages   []Message `json:"messages"`
	HasMore    bool      `json:"has_more"`
	NextCursor string    `json:"next_cursor"`
}

// ProfileUser is the user object inside a public/authenticated profile.
// Fields use camelCase to match GET /users/{username} and GET /me.
type ProfileUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Bio         string `json:"bio"`
	Location    string `json:"location"`
	PfpURL      string `json:"pfpUrl"`
	FriendCount int    `json:"friendCount"`
}

// ViewerContext describes the authenticated viewer relative to a profile.
type ViewerContext struct {
	AreFriends       bool `json:"areFriends"`
	CanPoke          bool `json:"canPoke"`
	PokeCooldownSecs int  `json:"pokeCooldownSeconds"`
}

// Profile is the public profile payload (no data envelope).
type Profile struct {
	User            ProfileUser   `json:"user"`
	ViewerContext   ViewerContext `json:"viewerContext"`
	IsAuthenticated bool          `json:"isAuthenticated"`
	IsOwnProfile    bool          `json:"isOwnProfile"`
}

// Me is an alias for the authenticated user's profile (GET /me).
type Me = Profile

// MeStats is the data payload for GET /me/stats (remilia:stats.read).
type MeStats struct {
	Handle          string             `json:"handle"`
	DisplayName     string             `json:"display_name"`
	Stats           MePlatformStats    `json:"stats"`
	AggregateScores map[string]float64 `json:"aggregate_scores"`
}

// MePlatformStats holds the documented /me/stats platform blobs.
// Undocumented platforms are ignored by the decoder.
type MePlatformStats struct {
	Ethereum MeEthereumStats `json:"ethereum"`
}

// MeEthereumStats is the documented ethereum subset of /me/stats.
type MeEthereumStats struct {
	CultTier   string `json:"cult_tier"`
	TotalOwned int    `json:"total_owned"`
}

// Handle returns a display handle for the profile.
func (p Profile) Handle() string {
	if p.User.Username != "" {
		return p.User.Username
	}
	return ""
}

// PokeResult is the success body for POST /users/{handle}/poke.
type PokeResult struct {
	Success bool `json:"success"`
}

// Error is the API error envelope.
type Error struct {
	Status  int
	Code    string
	Message string
	Raw     string
}

func (e *Error) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("api %d %s: %s", e.Status, e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("api %d: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("api %d: %s", e.Status, e.Raw)
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type dataEnvelope[T any] struct {
	Data T `json:"data"`
}

// Me fetches GET /me (same camelCase profile shape as GetUser).
func (c *Client) Me(ctx context.Context) (Profile, error) {
	var p Profile
	if err := c.do(ctx, http.MethodGet, "/me", nil, &p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// MeStats fetches GET /me/stats. Requires remilia:stats.read
// (user-delegated token). Success is wrapped in a data envelope.
func (c *Client) MeStats(ctx context.Context) (MeStats, error) {
	var out dataEnvelope[MeStats]
	if err := c.do(ctx, http.MethodGet, "/me/stats", nil, &out); err != nil {
		return MeStats{}, err
	}
	return out.Data, nil
}

// GetUser fetches GET /users/{username}. Public; no scope required.
// Unknown usernames return a plain-text 404 (not the JSON error envelope).
func (c *Client) GetUser(ctx context.Context, username string) (Profile, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Profile{}, &Error{Status: 400, Code: "invalid_request", Message: "username required"}
	}
	path := "/users/" + url.PathEscape(username)
	var p Profile
	if err := c.do(ctx, http.MethodGet, path, nil, &p); err != nil {
		return Profile{}, err
	}
	return p, nil
}

// Poke sends POST /users/{handle}/poke. Requires remilia:interact.poke
// (user-delegated token). Cooldown failures surface via *Error.
func (c *Client) Poke(ctx context.Context, handle string) (PokeResult, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return PokeResult{}, &Error{Status: 400, Code: "invalid_request", Message: "handle required"}
	}
	path := "/users/" + url.PathEscape(handle) + "/poke"
	var out PokeResult
	// Path is authoritative; empty JSON body keeps Content-Type application/json.
	if err := c.do(ctx, http.MethodPost, path, map[string]string{}, &out); err != nil {
		return PokeResult{}, err
	}
	// 2xx is success; body is often {"success":true}, sometimes empty.
	out.Success = true
	return out, nil
}

// ListGlobalChat fetches messages. before is an optional message id cursor.
func (c *Client) ListGlobalChat(ctx context.Context, limit int, before string) (ListMessagesResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	if before != "" {
		q.Set("before", before)
	}
	path := "/global-chat/messages?" + q.Encode()
	var out dataEnvelope[ListMessagesResponse]
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return ListMessagesResponse{}, err
	}
	return out.Data, nil
}

// PostGlobalChat posts a message. replyToID is optional.
func (c *Client) PostGlobalChat(ctx context.Context, text, replyToID string) (Message, error) {
	body := map[string]string{"text": text}
	if replyToID != "" {
		body["reply_to_id"] = replyToID
	}
	var out dataEnvelope[Message]
	if err := c.do(ctx, http.MethodPost, "/global-chat/messages", body, &out); err != nil {
		return Message{}, err
	}
	return out.Data, nil
}

func (c *Client) do(ctx context.Context, method, path string, body any, dest any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	// Prefer an explicit Token; otherwise rely on HTTPClient (e.g. oauth2 transport).
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return parseAPIError(res.StatusCode, raw)
	}

	if dest == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func parseAPIError(status int, raw []byte) error {
	var env errorEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && (env.Error.Code != "" || env.Error.Message != "") {
		return &Error{
			Status:  status,
			Code:    env.Error.Code,
			Message: env.Error.Message,
			Raw:     string(raw),
		}
	}
	// Legacy / browser-API shape used by some poke failures:
	// {"success":false,"error":"poke on cooldown (... remaining)"}
	var legacy struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil && legacy.Error != "" {
		code := ""
		if strings.Contains(strings.ToLower(legacy.Error), "cooldown") {
			code = "poke_cooldown"
		}
		return &Error{
			Status:  status,
			Code:    code,
			Message: legacy.Error,
			Raw:     string(raw),
		}
	}
	msg := strings.TrimSpace(string(raw))
	if len(msg) > 200 {
		msg = msg[:200] + "…"
	}
	code := ""
	if status == http.StatusNotFound && strings.HasPrefix(strings.ToLower(msg), "user not found") {
		code = "not_found"
	}
	return &Error{Status: status, Code: code, Message: msg, Raw: string(raw)}
}
