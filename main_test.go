package main

import (
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("BASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.port != "8080" {
		t.Errorf("want default port 8080, got %q", cfg.port)
	}
	if cfg.baseURL != "http://localhost:8080" {
		t.Errorf("want default baseURL derived from port, got %q", cfg.baseURL)
	}
	if cfg.sentryDSN != "" {
		t.Errorf("sentryDSN should be empty without SENTRY_DSN, got %q", cfg.sentryDSN)
	}
}

func TestLoadConfigCustomValues(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("BASE_URL", "https://short.io")
	t.Setenv("DATABASE_URL", "postgres://prod/db")
	t.Setenv("SENTRY_DSN", "https://key@sentry.io/1")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.port != "9999" || cfg.baseURL != "https://short.io" || cfg.dsn != "postgres://prod/db" || cfg.sentryDSN != "https://key@sentry.io/1" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigBaseURLFallsBackToCustomPort(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("BASE_URL", "")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.baseURL != "http://localhost:9999" {
		t.Errorf("baseURL must use custom port, got %q", cfg.baseURL)
	}
}

func TestLoadConfigRequiresDatabaseURL(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("BASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error without DATABASE_URL")
	} else if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Errorf("error must mention DATABASE_URL, got: %v", err)
	}
}
