package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if got != Version {
		t.Fatalf("got %q want %q", got, Version)
	}
}

func TestVersionCommandSkipsConfig(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(bad, []byte(":\n  not: yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REMITERM_CONFIG", bad)

	root := NewRoot()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(out.String())
	if got != Version {
		t.Fatalf("got %q want %q", got, Version)
	}
}

func TestNewAPIClientUserAgent(t *testing.T) {
	c := newAPIClient("https://example.com", "tok")
	want := "remiterm/" + Version
	if c.UserAgent != want {
		t.Fatalf("UserAgent = %q want %q", c.UserAgent, want)
	}
}
