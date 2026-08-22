package auth

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestRedirectURI(t *testing.T) {
	c := OAuthConfig{CallbackPort: 8765}
	if got := c.RedirectURI(); got != "http://127.0.0.1:8765/callback" {
		t.Fatalf("got %s", got)
	}
	c.CallbackPort = 0
	if got := c.RedirectURI(); got != "http://127.0.0.1:8765/callback" {
		t.Fatalf("default port got %s", got)
	}
}

func TestFromToOAuth2RoundTrip(t *testing.T) {
	exp := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	in := &oauth2.Token{
		AccessToken:  "at",
		RefreshToken: "rt",
		TokenType:    "Bearer",
		Expiry:       exp,
	}
	in = in.WithExtra(map[string]any{
		"scope":    "openid remilia:chat.read",
		"id_token": "idt",
	})
	tok := FromOAuth2(in)
	if tok.AccessToken != "at" || tok.RefreshToken != "rt" || tok.Scope != "openid remilia:chat.read" || tok.IDToken != "idt" {
		t.Fatalf("%+v", tok)
	}
	if !tok.ExpiresAt.Equal(exp) {
		t.Fatalf("expiry %v want %v", tok.ExpiresAt, exp)
	}
	back := tok.ToOAuth2()
	if back.AccessToken != "at" || back.RefreshToken != "rt" {
		t.Fatalf("%+v", back)
	}
}

func TestDiscover(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": "https://example.test/auth",
			"token_endpoint":         "https://example.test/token",
		})
	}))
	t.Cleanup(srv.Close)

	ep, err := discover(context.Background(), srv.URL, srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	if ep.AuthURL != "https://example.test/auth" || ep.TokenURL != "https://example.test/token" {
		t.Fatalf("%+v", ep)
	}
}

func TestLoginPKCEFlow(t *testing.T) {
	// Mock token endpoint; discovery points auth elsewhere (we don't hit it).
	var sawCode, sawVerifier string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_ = json.NewEncoder(w).Encode(map[string]string{
			"authorization_endpoint": base + "/auth",
			"token_endpoint":         base + "/token",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		vals, _ := url.ParseQuery(string(body))
		sawCode = vals.Get("code")
		sawVerifier = vals.Get("code_verifier")
		if vals.Get("grant_type") != "authorization_code" {
			t.Errorf("grant_type %q", vals.Get("grant_type"))
		}
		if vals.Get("client_id") != "tpa-remiterm" {
			t.Errorf("client_id %q", vals.Get("client_id"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "access-xyz",
			"refresh_token": "refresh-xyz",
			"token_type":    "Bearer",
			"expires_in":    300,
			"scope":         "openid remilia:chat.read remilia:chat.write",
			"id_token":      "id-xyz",
		})
	})
	as := httptest.NewServer(mux)
	t.Cleanup(as.Close)

	// Pick a free port for the loopback callback.
	lnPort := freePort(t)

	cfg := OAuthConfig{
		ClientID:     "tpa-remiterm",
		Issuer:       as.URL,
		CallbackPort: lnPort,
		HTTPClient:   as.Client(),
		OpenBrowser: func(authURL string) error {
			// Simulate browser redirect to our loopback with the right state/code.
			u, err := url.Parse(authURL)
			if err != nil {
				t.Fatal(err)
			}
			state := u.Query().Get("state")
			if state == "" || u.Query().Get("code_challenge") == "" {
				t.Fatalf("missing pkce/state in %s", authURL)
			}
			if u.Query().Get("code_challenge_method") != "S256" {
				t.Fatalf("challenge method %q", u.Query().Get("code_challenge_method"))
			}
			redir := u.Query().Get("redirect_uri")
			go func() {
				time.Sleep(50 * time.Millisecond)
				cb := redir + "?code=authcode-1&state=" + url.QueryEscape(state)
				res, err := http.Get(cb) //nolint:gosec // loopback test
				if err != nil {
					t.Errorf("callback: %v", err)
					return
				}
				res.Body.Close()
			}()
			return nil
		},
	}

	tok, err := Login(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "access-xyz" || tok.RefreshToken != "refresh-xyz" {
		t.Fatalf("%+v", tok)
	}
	if tok.Scope == "" || !strings.Contains(tok.Scope, "chat.read") {
		t.Fatalf("scope %q", tok.Scope)
	}
	if sawCode != "authcode-1" || sawVerifier == "" {
		t.Fatalf("token endpoint saw code=%q verifier empty=%v", sawCode, sawVerifier == "")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	// Bind :0 then close to get an ephemeral free port for the login listener.
	// Racey in theory; fine for unit tests.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
