package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/jakerobb/truenas-exporter/internal/api"
	"github.com/jakerobb/truenas-exporter/internal/config"
	"github.com/jakerobb/truenas-exporter/internal/metrics"
	"github.com/jakerobb/truenas-exporter/internal/truenas"
)

func main() {
	logLevel := slog.LevelInfo
	if strings.ToLower(os.Getenv("LOG_LEVEL")) == "debug" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	tlsMode := "ca"
	if cfg.TLSFingerprint != nil {
		tlsMode = "pinned-fingerprint"
	} else if cfg.TLSSkipVerify {
		tlsMode = "skip-verify"
	}
	slog.Info("configuration loaded",
		"http_port", cfg.HTTPPort,
		"truenas_url", cfg.URL,
		"tls_mode", tlsMode,
		"timeout", cfg.Timeout,
	)

	collector := metrics.NewCollector(truenas.Options{
		URL:            cfg.URL,
		APIKey:         cfg.APIKey,
		TLSFingerprint: cfg.TLSFingerprint,
		TLSSkipVerify:  cfg.TLSSkipVerify,
	}, cfg.Timeout)

	srv := api.New(collector, cfg.HTTPPort)
	slog.Info("HTTP server starting", "port", cfg.HTTPPort)
	if err := srv.Start(); err != nil {
		slog.Error("HTTP server failed", "err", err)
		os.Exit(1)
	}
}
