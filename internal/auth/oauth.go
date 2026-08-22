package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const (
	// DefaultIssuer is the RemiliaNET OIDC issuer.
	DefaultIssuer = "https://www.remilia.net/oidc/realms/remilia"
	// DefaultScope is what we request; portal-granted remilia:* scopes
	// are attached to the issued token for the registered client.
	DefaultScope = "openid"
	// loginTimeout is how long we wait for the browser callback.
	loginTimeout = 3 * time.Minute
)

// OAuthConfig drives Authorization Code + PKCE login.
type OAuthConfig struct {
	ClientID     string
	Issuer       string // default DefaultIssuer
	CallbackPort int    // listen on 127.0.0.1:port/callback
	Scopes       []string
	// OpenBrowser opens the authorize URL; nil uses the OS default opener.
	OpenBrowser func(url string) error
	// HTTPClient for discovery + token exchange; nil uses http.DefaultClient.
	HTTPClient *http.Client
}

// RedirectURI is the loopback callback registered in the developer portal.
func (c OAuthConfig) RedirectURI() string {
	port := c.CallbackPort
	if port <= 0 {
		port = 8765
	}
	return fmt.Sprintf("http://127.0.0.1:%d/callback", port)
}

func (c OAuthConfig) issuer() string {
	if c.Issuer == "" {
		return DefaultIssuer
	}
	return strings.TrimRight(c.Issuer, "/")
}

func (c OAuthConfig) scopes() []string {
	if len(c.Scopes) > 0 {
		return c.Scopes
	}
	return []string{DefaultScope}
}

func (c OAuthConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// oauth2Config builds an x/oauth2 config from discovery (or static Keycloak paths).
func (c OAuthConfig) oauth2Config(ctx context.Context) (*oauth2.Config, error) {
	if strings.TrimSpace(c.ClientID) == "" {
		return nil, errors.New("client_id is required")
	}
	ep, err := discover(ctx, c.issuer(), c.httpClient())
	if err != nil {
		// Fall back to documented Keycloak paths under the issuer.
		ep = oauth2.Endpoint{
			AuthURL:  c.issuer() + "/protocol/openid-connect/auth",
			TokenURL: c.issuer() + "/protocol/openid-connect/token",
		}
	}
	// Public login clients have no secret — send client_id in the form body.
	ep.AuthStyle = oauth2.AuthStyleInParams
	return &oauth2.Config{
		ClientID:    c.ClientID,
		RedirectURL: c.RedirectURI(),
		Scopes:      c.scopes(),
		Endpoint:    ep,
	}, nil
}

type discoveryDoc struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

func discover(ctx context.Context, issuer string, hc *http.Client) (oauth2.Endpoint, error) {
	u := issuer + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return oauth2.Endpoint{}, err
	}
	res, err := hc.Do(req)
	if err != nil {
		return oauth2.Endpoint{}, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return oauth2.Endpoint{}, fmt.Errorf("discovery %s: HTTP %d", u, res.StatusCode)
	}
	var doc discoveryDoc
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil {
		return oauth2.Endpoint{}, err
	}
	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" {
		return oauth2.Endpoint{}, errors.New("discovery missing endpoints")
	}
	return oauth2.Endpoint{
		AuthURL:   doc.AuthorizationEndpoint,
		TokenURL:  doc.TokenEndpoint,
		AuthStyle: oauth2.AuthStyleInParams,
	}, nil
}

// Login runs browser PKCE login and returns tokens (does not persist them).
func Login(ctx context.Context, cfg OAuthConfig) (Tokens, error) {
	o2, err := cfg.oauth2Config(ctx)
	if err != nil {
		return Tokens{}, err
	}

	state, err := randomURLString(16)
	if err != nil {
		return Tokens{}, err
	}
	verifier := oauth2.GenerateVerifier()

	port := cfg.CallbackPort
	if port <= 0 {
		port = 8765
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			desc := q.Get("error_description")
			msg := e
			if desc != "" {
				msg = e + ": " + desc
			}
			http.Error(w, "login failed: "+msg, http.StatusBadRequest)
			select {
			case errCh <- fmt.Errorf("authorization error: %s", msg):
			default:
			}
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			select {
			case errCh <- errors.New("state mismatch — possible CSRF, aborting"):
			default:
			}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "missing code", http.StatusBadRequest)
			select {
			case errCh <- errors.New("callback missing code"):
			default:
			}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<!doctype html><meta charset="utf-8"><title>remiterm</title>
<style>body{font-family:system-ui,sans-serif;max-width:32rem;margin:3rem auto;padding:0 1rem}
h1{font-size:1.25rem}</style>
<h1>Signed in to remiterm</h1>
<p>You can close this tab and return to the terminal.</p>`)
		select {
		case codeCh <- code:
		default:
		}
	})

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return Tokens{}, fmt.Errorf("listen on 127.0.0.1:%d: %w (is another remiterm login running?)", port, err)
	}
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	authURL := o2.AuthCodeURL(state,
		oauth2.S256ChallengeOption(verifier),
	)

	open := cfg.OpenBrowser
	if open == nil {
		open = openBrowser
	}
	if openErr := open(authURL); openErr != nil {
		// Still print the URL so the user can open it manually.
		fmt.Printf("Open this URL in a browser to sign in:\n\n  %s\n\n", authURL)
	} else {
		fmt.Printf("Opening browser for RemiliaNET login…\nIf nothing opens, visit:\n\n  %s\n\n", authURL)
	}
	fmt.Printf("Waiting for callback on %s (timeout %s)…\n", cfg.RedirectURI(), loginTimeout)

	waitCtx, cancel := context.WithTimeout(ctx, loginTimeout)
	defer cancel()

	var code string
	select {
	case <-waitCtx.Done():
		return Tokens{}, fmt.Errorf("login timed out after %s", loginTimeout)
	case loginErr := <-errCh:
		return Tokens{}, loginErr
	case code = <-codeCh:
	}

	tokCtx := context.WithValue(ctx, oauth2.HTTPClient, cfg.httpClient())
	tok, err := o2.Exchange(tokCtx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Tokens{}, fmt.Errorf("token exchange: %w", err)
	}
	return FromOAuth2(tok), nil
}

// FromOAuth2 converts an oauth2.Token to Tokens.
func FromOAuth2(t *oauth2.Token) Tokens {
	if t == nil {
		return Tokens{}
	}
	out := Tokens{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		ExpiresAt:    t.Expiry,
	}
	if s, ok := t.Extra("scope").(string); ok {
		out.Scope = s
	}
	if id, ok := t.Extra("id_token").(string); ok {
		out.IDToken = id
	}
	if out.TokenType == "" {
		out.TokenType = "Bearer"
	}
	return out
}

// ToOAuth2 converts Tokens to oauth2.Token for refresh.
func (t Tokens) ToOAuth2() *oauth2.Token {
	tok := &oauth2.Token{
		AccessToken:  t.AccessToken,
		RefreshToken: t.RefreshToken,
		TokenType:    t.TokenType,
		Expiry:       t.ExpiresAt,
	}
	extra := map[string]any{}
	if t.Scope != "" {
		extra["scope"] = t.Scope
	}
	if t.IDToken != "" {
		extra["id_token"] = t.IDToken
	}
	if len(extra) > 0 {
		tok = tok.WithExtra(extra)
	}
	return tok
}

// randomURLString returns n random bytes as unpadded base64url.
func randomURLString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func openBrowser(rawURL string) error {
	// Validate absolute http(s) URL before handing to the OS.
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("invalid url")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
