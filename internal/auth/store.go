package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "remiterm"
	keyringAccount = "default"
)

// ErrNotFound means no token is stored.
var ErrNotFound = errors.New("no token stored")

// Tokens are OAuth (or pasted) credentials.
type Tokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
}

// Valid reports whether an access token is present.
func (t Tokens) Valid() bool {
	return strings.TrimSpace(t.AccessToken) != ""
}

// Store persists tokens in the OS keyring with a file fallback.
type Store struct {
	FilePath string
}

// Save writes tokens to keyring, falling back to file on keyring errors.
func (s Store) Save(t Tokens) error {
	if !t.Valid() {
		return errors.New("access_token is empty")
	}
	if t.TokenType == "" {
		t.TokenType = "Bearer"
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := keyring.Set(keyringService, keyringAccount, string(raw)); err == nil {
		// Best-effort: remove stale file if keyring works.
		_ = os.Remove(s.FilePath)
		return nil
	}
	return s.saveFile(raw)
}

// Load returns stored tokens. REMILIA_ACCESS_TOKEN env wins when set.
func (s Store) Load() (Tokens, error) {
	if env := strings.TrimSpace(os.Getenv("REMILIA_ACCESS_TOKEN")); env != "" {
		return Tokens{AccessToken: env, TokenType: "Bearer"}, nil
	}

	if secret, err := keyring.Get(keyringService, keyringAccount); err == nil {
		var t Tokens
		if err := json.Unmarshal([]byte(secret), &t); err != nil {
			return Tokens{}, fmt.Errorf("decode keyring token: %w", err)
		}
		if !t.Valid() {
			return Tokens{}, ErrNotFound
		}
		return t, nil
	}

	return s.loadFile()
}

// Clear removes keyring and file entries.
func (s Store) Clear() error {
	_ = keyring.Delete(keyringService, keyringAccount)
	if s.FilePath == "" {
		return nil
	}
	err := os.Remove(s.FilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Backend describes where credentials would be read from (for status UX).
func (s Store) Backend() string {
	if strings.TrimSpace(os.Getenv("REMILIA_ACCESS_TOKEN")) != "" {
		return "env:REMILIA_ACCESS_TOKEN"
	}
	if _, err := keyring.Get(keyringService, keyringAccount); err == nil {
		return "keyring"
	}
	if _, err := os.Stat(s.FilePath); err == nil {
		return "file:" + s.FilePath
	}
	return "none"
}

func (s Store) saveFile(raw []byte) error {
	if s.FilePath == "" {
		return errors.New("keyring unavailable and no file path configured")
	}
	if err := os.WriteFile(s.FilePath, raw, 0o600); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

func (s Store) loadFile() (Tokens, error) {
	if s.FilePath == "" {
		return Tokens{}, ErrNotFound
	}
	raw, err := os.ReadFile(s.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return Tokens{}, ErrNotFound
		}
		return Tokens{}, err
	}
	var t Tokens
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tokens{}, fmt.Errorf("decode token file: %w", err)
	}
	if !t.Valid() {
		return Tokens{}, ErrNotFound
	}
	return t, nil
}
