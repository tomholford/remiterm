package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	t.Setenv("HOME", dir)
	t.Setenv("REMITERM_CONFIG", "")
	t.Setenv("REMICHAT_CONFIG", "")
	t.Setenv("REMILIA_API_BASE", "")
	t.Setenv("REMILIA_ISSUER", "")
	t.Setenv("REMILIA_CLIENT_ID", "")
}

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	isolateConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(cfg.ConfigFile, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg.ConfigFile
}

func TestLoadDefaults(t *testing.T) {
	isolateConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != DefaultAPIBase {
		t.Fatalf("api_base = %q", cfg.APIBase)
	}
	if cfg.Issuer != DefaultIssuer {
		t.Fatalf("issuer = %q", cfg.Issuer)
	}
	if cfg.PollInterval != DefaultPoll {
		t.Fatalf("poll = %s", cfg.PollInterval)
	}
	if cfg.AuthorLabel != DefaultAuthorLabel {
		t.Fatalf("author_label = %q", cfg.AuthorLabel)
	}
	if cfg.TimestampFormat != DefaultTimestampFormat {
		t.Fatalf("timestamp_format = %q", cfg.TimestampFormat)
	}
	if cfg.Theme != DefaultTheme {
		t.Fatalf("theme = %q", cfg.Theme)
	}
	if cfg.ClientID != DefaultClientID {
		t.Fatalf("client_id = %q", cfg.ClientID)
	}
	if cfg.CallbackPort != DefaultPort {
		t.Fatalf("callback_port = %d", cfg.CallbackPort)
	}
	wantCache := filepath.Join(os.Getenv("XDG_CACHE_HOME"), AppName, "chat.sqlite")
	if cfg.CachePath() != wantCache {
		t.Fatalf("CachePath = %q want %q", cfg.CachePath(), wantCache)
	}
	if cfg.CacheDir == cfg.ConfigDir {
		t.Fatal("cache dir must not be the config dir")
	}
}

func TestCachePathXDGCacheHome(t *testing.T) {
	isolateConfig(t)
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(xdg, AppName, "chat.sqlite")
	if cfg.CachePath() != want {
		t.Fatalf("CachePath = %q want %q", cfg.CachePath(), want)
	}
}

func TestCachePathFallbackHome(t *testing.T) {
	isolateConfig(t)
	t.Setenv("XDG_CACHE_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".cache", AppName, "chat.sqlite")
	if cfg.CachePath() != want {
		t.Fatalf("CachePath = %q want %q", cfg.CachePath(), want)
	}
}

func TestLoadFile(t *testing.T) {
	writeConfigFile(t, `
api_base: https://example.test/api/v1
issuer: https://example.test/oidc
client_id: tpa-test
callback_port: 9001
poll_interval: 10s
author_label: both
timestamp_format: 15:04:05
theme: dim
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != DefaultAPIBase {
		t.Fatalf("api_base = %q", cfg.APIBase)
	}
	if cfg.Issuer != DefaultIssuer {
		t.Fatalf("issuer = %q", cfg.Issuer)
	}
	if cfg.ClientID != DefaultClientID {
		t.Fatalf("client_id = %q", cfg.ClientID)
	}
	if cfg.CallbackPort != DefaultPort {
		t.Fatalf("callback_port = %d", cfg.CallbackPort)
	}
	if cfg.PollInterval != 10*time.Second {
		t.Fatalf("poll = %s", cfg.PollInterval)
	}
	if cfg.AuthorLabel != "both" {
		t.Fatalf("author_label = %q", cfg.AuthorLabel)
	}
	if cfg.TimestampFormat != "15:04:05" {
		t.Fatalf("timestamp_format = %q", cfg.TimestampFormat)
	}
	if cfg.Theme != "dim" {
		t.Fatalf("theme = %q", cfg.Theme)
	}
}

func TestLoadNamedThemes(t *testing.T) {
	for _, name := range AllowedThemes {
		t.Run(name, func(t *testing.T) {
			writeConfigFile(t, "theme: "+name+"\n")
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Theme != name {
				t.Fatalf("loaded %q", cfg.Theme)
			}
		})
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	writeConfigFile(t, `
api_base: https://file.test/api/v1
issuer: https://file.test/oidc
client_id: from-file
author_label: both
`)
	t.Setenv("REMILIA_API_BASE", "https://env.test/api/v1")
	t.Setenv("REMILIA_ISSUER", "https://env.test/oidc")
	t.Setenv("REMILIA_CLIENT_ID", "from-env")
	t.Setenv("REMILIA_CALLBACK_PORT", "9001")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != "https://env.test/api/v1" {
		t.Fatalf("api_base = %q", cfg.APIBase)
	}
	if cfg.Issuer != "https://env.test/oidc" {
		t.Fatalf("issuer = %q", cfg.Issuer)
	}
	if cfg.ClientID != "from-env" {
		t.Fatalf("client_id = %q", cfg.ClientID)
	}
	if cfg.CallbackPort != 9001 {
		t.Fatalf("callback_port = %d", cfg.CallbackPort)
	}
	if cfg.AuthorLabel != "both" {
		t.Fatalf("author_label should stay from file, got %q", cfg.AuthorLabel)
	}
}

func TestLoadInvalidFallsBack(t *testing.T) {
	writeConfigFile(t, `
author_label: nope
theme: rainbow
timestamp_format: unix
poll_interval: -3s
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthorLabel != DefaultAuthorLabel {
		t.Fatalf("author_label = %q", cfg.AuthorLabel)
	}
	if cfg.Theme != DefaultTheme {
		t.Fatalf("theme = %q", cfg.Theme)
	}
	if cfg.TimestampFormat != DefaultTimestampFormat {
		t.Fatalf("timestamp_format = %q", cfg.TimestampFormat)
	}
	if cfg.PollInterval != DefaultPoll {
		t.Fatalf("poll = %s want default", cfg.PollInterval)
	}

	writeConfigFile(t, "poll_interval: 500ms\n")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollInterval != MinPoll {
		t.Fatalf("poll = %s want clamp %s", cfg.PollInterval, MinPoll)
	}

	writeConfigFile(t, "poll_interval: 5m\n")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PollInterval != MaxPoll {
		t.Fatalf("poll = %s want clamp %s", cfg.PollInterval, MaxPoll)
	}
}

func TestSaveSettingsCreatesFile(t *testing.T) {
	isolateConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(cfg.ConfigFile); !os.IsNotExist(err) {
		t.Fatalf("expected missing file, err=%v", err)
	}

	s := Settings{
		AuthorLabel:     "display_name",
		TimestampFormat: "3:04pm",
		Theme:           "high-contrast",
		PollInterval:    2 * time.Second,
	}
	if err = cfg.SaveSettings(s); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileDoc
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.APIBase != "" || doc.Issuer != "" || doc.ClientID != "" || doc.CallbackPort != 0 {
		t.Fatalf("new file should only have setting keys, got %+v", doc)
	}
	if doc.AuthorLabel != "display_name" || doc.TimestampFormat != "3:04pm" || doc.Theme != "high-contrast" || doc.PollInterval != "2s" {
		t.Fatalf("settings mismatch: %+v", doc)
	}

	info, err := os.Stat(cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o want 0600", info.Mode().Perm())
	}
}

func TestSaveSettingsPreservesOperatorKeys(t *testing.T) {
	path := writeConfigFile(t, `
api_base: https://file.test/api/v1
issuer: https://file.test/oidc
client_id: tpa-keep
callback_port: 9001
poll_interval: 5s
`)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.SaveSettings(Settings{
		AuthorLabel:     "both",
		TimestampFormat: "relative",
		Theme:           "dim",
		PollInterval:    15 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileDoc
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.APIBase != "https://file.test/api/v1" {
		t.Fatalf("api_base = %q", doc.APIBase)
	}
	if doc.Issuer != "https://file.test/oidc" {
		t.Fatalf("issuer = %q", doc.Issuer)
	}
	if doc.ClientID != "tpa-keep" {
		t.Fatalf("client_id = %q", doc.ClientID)
	}
	if doc.CallbackPort != 9001 {
		t.Fatalf("callback_port = %d", doc.CallbackPort)
	}
	if doc.AuthorLabel != "both" || doc.Theme != "dim" || doc.TimestampFormat != "relative" || doc.PollInterval != "15s" {
		t.Fatalf("settings mismatch: %+v", doc)
	}
}

func TestSaveSettingsDoesNotPersistEnv(t *testing.T) {
	writeConfigFile(t, "client_id: tpa-keep\n")
	t.Setenv("REMILIA_API_BASE", "https://env.test/api/v1")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIBase != "https://env.test/api/v1" {
		t.Fatalf("resolved api_base = %q", cfg.APIBase)
	}

	if err = cfg.SaveSettings(cfg.Settings()); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(cfg.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc fileDoc
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.APIBase != "" {
		t.Fatalf("env api_base leaked into file: %q", doc.APIBase)
	}
	if doc.ClientID != "tpa-keep" {
		t.Fatalf("client_id = %q", doc.ClientID)
	}
}

func TestSaveSettingsNormalizes(t *testing.T) {
	isolateConfig(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if err = cfg.SaveSettings(Settings{
		AuthorLabel:     "NOPE",
		TimestampFormat: "",
		Theme:           "rainbow",
		PollInterval:    5 * time.Minute,
	}); err != nil {
		t.Fatal(err)
	}
	if cfg.AuthorLabel != DefaultAuthorLabel || cfg.Theme != DefaultTheme || cfg.TimestampFormat != DefaultTimestampFormat {
		t.Fatalf("got %+v", cfg.Settings())
	}
	if cfg.PollInterval != MaxPoll {
		t.Fatalf("poll = %s", cfg.PollInterval)
	}
}

func TestSaveSettingsCustomPath(t *testing.T) {
	isolateConfig(t)
	custom := filepath.Join(t.TempDir(), "nested", "custom.yaml")
	t.Setenv("REMITERM_CONFIG", custom)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != custom {
		t.Fatalf("ConfigFile = %q", cfg.ConfigFile)
	}
	if err = cfg.SaveSettings(Settings{Theme: "dim", PollInterval: DefaultPoll}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(custom); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRemichatConfigAlias(t *testing.T) {
	isolateConfig(t)
	custom := filepath.Join(t.TempDir(), "legacy.yaml")
	t.Setenv("REMICHAT_CONFIG", custom)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != custom {
		t.Fatalf("ConfigFile = %q", cfg.ConfigFile)
	}
}

func TestLoadRemitermConfigPrefersNewName(t *testing.T) {
	isolateConfig(t)
	newer := filepath.Join(t.TempDir(), "new.yaml")
	older := filepath.Join(t.TempDir(), "old.yaml")
	t.Setenv("REMITERM_CONFIG", newer)
	t.Setenv("REMICHAT_CONFIG", older)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConfigFile != newer {
		t.Fatalf("ConfigFile = %q", cfg.ConfigFile)
	}
}
