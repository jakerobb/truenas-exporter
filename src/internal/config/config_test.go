package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fp = "AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89:AB:CD:EF:01:23:45:67:89"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := LoadFromEnv(env(map[string]string{
		"TRUENAS_URL":     "wss://truenas.lan/api/current",
		"TRUENAS_API_KEY": " key \n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPPort != 8080 || cfg.Timeout != 10*time.Second || cfg.APIKey != "key" || cfg.TLSFingerprint != nil {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadKeyFileAndFingerprint(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "api-key")
	if err := os.WriteFile(keyFile, []byte("from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromEnv(env(map[string]string{
		"TRUENAS_URL":             "wss://truenas.lan/api/current",
		"TRUENAS_API_KEY":         "ignored",
		"TRUENAS_API_KEY_FILE":    keyFile,
		"TRUENAS_TLS_FINGERPRINT": fp,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "from-file" {
		t.Errorf("APIKey = %q, want from-file", cfg.APIKey)
	}
	if len(cfg.TLSFingerprint) != 32 || cfg.TLSFingerprint[0] != 0xab {
		t.Errorf("unexpected fingerprint %x", cfg.TLSFingerprint)
	}
}

func TestLoadErrors(t *testing.T) {
	base := map[string]string{
		"TRUENAS_URL":     "wss://truenas.lan/api/current",
		"TRUENAS_API_KEY": "key",
	}
	cases := map[string]struct {
		override map[string]string
		wantErr  string
	}{
		"missing url":         {map[string]string{"TRUENAS_URL": ""}, "TRUENAS_URL is required"},
		"missing key":         {map[string]string{"TRUENAS_API_KEY": ""}, "API key is required"},
		"plain ws":            {map[string]string{"TRUENAS_URL": "ws://truenas.lan/api/current"}, "revoke"},
		"https scheme":        {map[string]string{"TRUENAS_URL": "https://truenas.lan/api/current"}, "must use wss://"},
		"short fingerprint":   {map[string]string{"TRUENAS_TLS_FINGERPRINT": "abcd"}, "SHA-256"},
		"pin and skip verify": {map[string]string{"TRUENAS_TLS_FINGERPRINT": fp, "TRUENAS_TLS_SKIP_VERIFY": "true"}, "mutually exclusive"},
		"bad port":            {map[string]string{"HTTP_PORT": "http"}, "HTTP_PORT"},
		"bad timeout":         {map[string]string{"TRUENAS_TIMEOUT": "10"}, "TRUENAS_TIMEOUT"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			m := map[string]string{}
			for k, v := range base {
				m[k] = v
			}
			for k, v := range tc.override {
				m[k] = v
			}
			_, err := LoadFromEnv(env(m))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestPlainWSAllowedWhenOptedIn(t *testing.T) {
	_, err := LoadFromEnv(env(map[string]string{
		"TRUENAS_URL":            "ws://truenas.lan/api/current",
		"TRUENAS_API_KEY":        "key",
		"TRUENAS_ALLOW_INSECURE": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
}
