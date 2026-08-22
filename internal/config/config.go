package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

const (
	AppName         = "remiterm"
	DefaultAPIBase  = "https://www.remilia.net/api/v1"
	DefaultIssuer   = "https://www.remilia.net/oidc/realms/remilia"
	DefaultClientID = "tpa-remiterm"
	DefaultPoll     = 5 * time.Second
	DefaultPort     = 8765

	DefaultAuthorLabel     = "handle"
	DefaultTimestampFormat = "15:04"
	DefaultTheme           = "default"

	MinPoll = time.Second
	MaxPoll = 60 * time.Second
)

var (
	AllowedAuthorLabels     = []string{"handle", "display_name", "both"}
	AllowedTimestampFormats = []string{"15:04", "15:04:05", "3:04pm", "relative"}
	AllowedThemes           = []string{"default", "dim", "high-contrast", "monokai", "tomorrow-night", "dracula"}
)

// Config holds runtime settings.
type Config struct {
	APIBase      string
	Issuer       string
	PollInterval time.Duration
	ClientID     string
	CallbackPort int
	ConfigDir    string
	ConfigFile   string
	CacheDir     string

	AuthorLabel     string
	TimestampFormat string
	Theme           string
}

// Settings are the TUI-editable prefs persisted to YAML.
type Settings struct {
	AuthorLabel     string
	TimestampFormat string
	Theme           string
	PollInterval    time.Duration
}

// Load reads config from file, env, and defaults.
func Load() (*Config, error) {
	configDir, err := defaultConfigDir()
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(configDir)

	if custom := configFileOverride(); custom != "" {
		v.SetConfigFile(custom)
	}

	v.SetDefault("poll_interval", DefaultPoll.String())
	v.SetDefault("author_label", DefaultAuthorLabel)
	v.SetDefault("timestamp_format", DefaultTimestampFormat)
	v.SetDefault("theme", DefaultTheme)

	v.SetEnvPrefix("REMILIA")
	v.AutomaticEnv()

	// Missing file is fine (including REMITERM_CONFIG pointing at a new path).
	if err = v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	cfgFile := filepath.Join(configDir, "config.yaml")
	if custom := configFileOverride(); custom != "" {
		cfgFile = custom
	} else if used := v.ConfigFileUsed(); used != "" {
		cfgFile = used
	}

	s := normalizeSettings(Settings{
		AuthorLabel:     v.GetString("author_label"),
		TimestampFormat: v.GetString("timestamp_format"),
		Theme:           v.GetString("theme"),
		PollInterval:    parsePoll(v.GetString("poll_interval")),
	})

	cacheDir, err := defaultCacheDir()
	if err != nil {
		return nil, err
	}

	return &Config{
		APIBase:         envOrDefault("REMILIA_API_BASE", DefaultAPIBase),
		Issuer:          envOrDefault("REMILIA_ISSUER", DefaultIssuer),
		PollInterval:    s.PollInterval,
		ClientID:        envOrDefault("REMILIA_CLIENT_ID", DefaultClientID),
		CallbackPort:    envPortOrDefault("REMILIA_CALLBACK_PORT", DefaultPort),
		ConfigDir:       configDir,
		ConfigFile:      cfgFile,
		CacheDir:        cacheDir,
		AuthorLabel:     s.AuthorLabel,
		TimestampFormat: s.TimestampFormat,
		Theme:           s.Theme,
	}, nil
}

func defaultConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", AppName), nil
}

func defaultCacheDir() (string, error) {
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".cache", AppName), nil
}

// TokensPath is the file fallback for stored credentials.
func (c *Config) TokensPath() string {
	return filepath.Join(c.ConfigDir, "tokens.json")
}

// CachePath is the local SQLite chat cache (not next to tokens.json).
func (c *Config) CachePath() string {
	if c == nil || c.CacheDir == "" {
		return ""
	}
	return filepath.Join(c.CacheDir, "chat.sqlite")
}

// Settings returns the TUI-editable prefs.
func (c *Config) Settings() Settings {
	if c == nil {
		return normalizeSettings(Settings{})
	}
	return Settings{
		AuthorLabel:     c.AuthorLabel,
		TimestampFormat: c.TimestampFormat,
		Theme:           c.Theme,
		PollInterval:    c.PollInterval,
	}
}

// SaveSettings merges TUI prefs into the existing YAML file without copying
// env-resolved operator keys (api_base, issuer, client_id) onto disk.
func (c *Config) SaveSettings(s Settings) error {
	if c == nil {
		return errors.New("nil config")
	}
	if c.ConfigFile == "" {
		return errors.New("no config file path")
	}
	s = normalizeSettings(s)

	doc, err := readFileDoc(c.ConfigFile)
	if err != nil {
		return err
	}
	doc.PollInterval = s.PollInterval.String()
	doc.AuthorLabel = s.AuthorLabel
	doc.TimestampFormat = s.TimestampFormat
	doc.Theme = s.Theme

	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(c.ConfigFile), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	if err = os.WriteFile(c.ConfigFile, raw, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	c.AuthorLabel = s.AuthorLabel
	c.TimestampFormat = s.TimestampFormat
	c.Theme = s.Theme
	c.PollInterval = s.PollInterval
	return nil
}

// fileDoc is the on-disk YAML shape. omitempty keeps operator keys out of a
// newly created file; existing values are preserved on round-trip.
type fileDoc struct {
	APIBase         string `yaml:"api_base,omitempty"`
	Issuer          string `yaml:"issuer,omitempty"`
	ClientID        string `yaml:"client_id,omitempty"`
	CallbackPort    int    `yaml:"callback_port,omitempty"`
	PollInterval    string `yaml:"poll_interval,omitempty"`
	AuthorLabel     string `yaml:"author_label,omitempty"`
	TimestampFormat string `yaml:"timestamp_format,omitempty"`
	Theme           string `yaml:"theme,omitempty"`
}

func readFileDoc(path string) (fileDoc, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileDoc{}, nil
		}
		return fileDoc{}, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fileDoc{}, nil
	}
	var doc fileDoc
	if err = yaml.Unmarshal(raw, &doc); err != nil {
		return fileDoc{}, fmt.Errorf("parse config: %w", err)
	}
	return doc, nil
}

func parsePoll(s string) time.Duration {
	poll, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || poll <= 0 {
		return DefaultPoll
	}
	return poll
}

func normalizeSettings(s Settings) Settings {
	s.AuthorLabel = oneOf(strings.TrimSpace(s.AuthorLabel), AllowedAuthorLabels, DefaultAuthorLabel)
	s.TimestampFormat = oneOf(strings.TrimSpace(s.TimestampFormat), AllowedTimestampFormats, DefaultTimestampFormat)
	s.Theme = oneOf(strings.TrimSpace(s.Theme), AllowedThemes, DefaultTheme)
	s.PollInterval = clampPoll(s.PollInterval)
	return s
}

func clampPoll(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultPoll
	}
	if d < MinPoll {
		return MinPoll
	}
	if d > MaxPoll {
		return MaxPoll
	}
	return d
}

func oneOf(v string, allowed []string, fallback string) string {
	if slices.Contains(allowed, v) {
		return v
	}
	return fallback
}

func configFileOverride() string {
	if v := os.Getenv("REMITERM_CONFIG"); v != "" {
		return v
	}
	return os.Getenv("REMICHAT_CONFIG")
}

func envOrDefault(key, fallback string) string {
	if e := strings.TrimSpace(os.Getenv(key)); e != "" {
		return e
	}
	return fallback
}

func envPortOrDefault(key string, fallback int) int {
	e := strings.TrimSpace(os.Getenv(key))
	if e == "" {
		return fallback
	}
	n, err := strconv.Atoi(e)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
