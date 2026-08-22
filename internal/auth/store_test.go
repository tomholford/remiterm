package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tokens.json")
	s := Store{FilePath: path}
	t.Setenv("REMILIA_ACCESS_TOKEN", "")

	in := Tokens{
		AccessToken:  "at_test",
		RefreshToken: "rt_test",
		Scope:        "openid remilia:chat.read",
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.saveFile(raw); err != nil {
		t.Fatal(err)
	}

	got, err := s.loadFile()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != in.AccessToken || got.RefreshToken != in.RefreshToken {
		t.Fatalf("got %+v want %+v", got, in)
	}

	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = s.loadFile(); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	s := Store{FilePath: filepath.Join(t.TempDir(), "tokens.json")}
	t.Setenv("REMILIA_ACCESS_TOKEN", "from-env")
	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "from-env" {
		t.Fatalf("got %q", got.AccessToken)
	}
	if s.Backend() != "env:REMILIA_ACCESS_TOKEN" {
		t.Fatalf("backend %q", s.Backend())
	}
}

func TestValid(t *testing.T) {
	if (Tokens{}).Valid() {
		t.Fatal("empty should be invalid")
	}
	if !(Tokens{AccessToken: "x"}).Valid() {
		t.Fatal("token should be valid")
	}
}

func TestSaveRejectsEmpty(t *testing.T) {
	s := Store{FilePath: filepath.Join(t.TempDir(), "tokens.json")}
	if err := s.Save(Tokens{}); err == nil {
		t.Fatal("expected error")
	}
}
