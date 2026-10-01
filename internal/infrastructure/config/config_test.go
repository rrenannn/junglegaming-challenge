package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadUsesLocalDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Address != ":8080" {
		t.Fatalf("HTTP address = %q, want :8080", cfg.HTTP.Address)
	}
	if cfg.Database.ConnectTimeout != 5*time.Second {
		t.Fatalf("database connect timeout = %s, want 5s", cfg.Database.ConnectTimeout)
	}
	if cfg.SQS.InputQueue != "wager-transactions.fifo" {
		t.Fatalf("input queue = %q", cfg.SQS.InputQueue)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Setenv("SHUTDOWN_TIMEOUT", "tomorrow")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SHUTDOWN_TIMEOUT") {
		t.Fatalf("Load() error = %v, want SHUTDOWN_TIMEOUT parse error", err)
	}
}

func TestValidateRejectsInvalidConnectionLimits(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Database.MinConnections = cfg.Database.MaxConnections + 1

	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_MIN_CONNECTIONS") {
		t.Fatalf("Validate() error = %v, want connection limits error", err)
	}
}
