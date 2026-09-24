package config

import (
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPPort int

	// URL is the TrueNAS JSON-RPC websocket endpoint, e.g. wss://truenas.lan/api/current.
	URL    string
	APIKey string

	// TLSFingerprint, when set, pins the server certificate by its SHA-256
	// fingerprint instead of validating it against a CA. This is the
	// intended mode for TrueNAS's default self-signed certificate.
	TLSFingerprint []byte
	TLSSkipVerify  bool

	// AllowInsecure permits a plain ws:// URL. Off by default because
	// TrueNAS 25.04+ revokes API keys that are used over an unencrypted
	// connection.
	AllowInsecure bool

	Timeout time.Duration
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	return LoadFromEnv(os.Getenv)
}

// LoadFromEnv reads configuration using the given lookup function (os.Getenv in production).
func LoadFromEnv(getenv func(string) string) (*Config, error) {
	cfg := &Config{
		URL: getenv("TRUENAS_URL"),
	}

	var err error
	if cfg.HTTPPort, err = intOr(getenv, "HTTP_PORT", 8080); err != nil {
		return nil, err
	}
	if cfg.TLSSkipVerify, err = boolOr(getenv, "TRUENAS_TLS_SKIP_VERIFY", false); err != nil {
		return nil, err
	}
	if cfg.AllowInsecure, err = boolOr(getenv, "TRUENAS_ALLOW_INSECURE", false); err != nil {
		return nil, err
	}
	if cfg.Timeout, err = durationOr(getenv, "TRUENAS_TIMEOUT", 10*time.Second); err != nil {
		return nil, err
	}

	if fp := getenv("TRUENAS_TLS_FINGERPRINT"); fp != "" {
		// Accept the colon-separated uppercase form `openssl x509 -fingerprint` prints.
		cfg.TLSFingerprint, err = hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(fp), ":", ""))
		if err != nil || len(cfg.TLSFingerprint) != 32 {
			return nil, fmt.Errorf("TRUENAS_TLS_FINGERPRINT must be a SHA-256 fingerprint (64 hex digits, colons optional)")
		}
	}

	cfg.APIKey, err = loadAPIKey(getenv)
	if err != nil {
		return nil, err
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// loadAPIKey prefers TRUENAS_API_KEY_FILE (a mounted secret) over
// TRUENAS_API_KEY, so the key doesn't need to appear in the process environment.
func loadAPIKey(getenv func(string) string) (string, error) {
	if path := getenv("TRUENAS_API_KEY_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading TRUENAS_API_KEY_FILE %s: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return strings.TrimSpace(getenv("TRUENAS_API_KEY")), nil
}

func validate(cfg *Config) error {
	if cfg.URL == "" {
		return fmt.Errorf("TRUENAS_URL is required (e.g. wss://truenas.lan/api/current)")
	}
	u, err := url.Parse(cfg.URL)
	if err != nil {
		return fmt.Errorf("invalid TRUENAS_URL %q: %w", cfg.URL, err)
	}
	switch u.Scheme {
	case "wss":
	case "ws":
		if !cfg.AllowInsecure {
			return fmt.Errorf("TRUENAS_URL uses ws://, which sends the API key unencrypted and causes TrueNAS to revoke it; use wss:// or set TRUENAS_ALLOW_INSECURE=true")
		}
	default:
		return fmt.Errorf("TRUENAS_URL must use wss:// (or ws://), got %q", u.Scheme)
	}
	if cfg.APIKey == "" {
		return fmt.Errorf("an API key is required: set TRUENAS_API_KEY_FILE or TRUENAS_API_KEY")
	}
	if cfg.TLSFingerprint != nil && cfg.TLSSkipVerify {
		return fmt.Errorf("TRUENAS_TLS_FINGERPRINT and TRUENAS_TLS_SKIP_VERIFY are mutually exclusive")
	}
	return nil
}

func intOr(getenv func(string) string, key string, def int) (int, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return n, nil
}

func boolOr(getenv func(string) string, key string, def bool) (bool, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return b, nil
}

func durationOr(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return d, nil
}
