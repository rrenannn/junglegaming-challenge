package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
)

type stubHealthCheck struct {
	name string
	err  error
}

func (s stubHealthCheck) Name() string                { return s.name }
func (s stubHealthCheck) Check(context.Context) error { return s.err }

// slowHealthCheck blocks until its context is cancelled, simulating a
// dependency that hangs (e.g. a TCP connect to a paused container) instead
// of failing fast.
type slowHealthCheck struct{ name string }

func (s slowHealthCheck) Name() string { return s.name }
func (s slowHealthCheck) Check(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestHealthLive(t *testing.T) {
	handler := NewHealth(nil, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()

	handler.Live(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"live"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHealthReadyReportsDependencyFailure(t *testing.T) {
	checks := []port.HealthCheck{
		stubHealthCheck{name: "postgres"},
		stubHealthCheck{name: "sqs", err: errors.New("offline")},
	}
	handler := NewHealth(checks, time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()

	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), `"sqs":"unavailable"`) {
		t.Fatalf("body = %s", response.Body.String())
	}
}

// Regression test: a slow/hanging dependency must not eat the timeout
// budget of later, perfectly healthy dependencies checked in the same
// request — each check needs its own fresh deadline.
func TestHealthReadyGivesEachCheckItsOwnTimeoutBudget(t *testing.T) {
	checks := []port.HealthCheck{
		slowHealthCheck{name: "postgres"},
		stubHealthCheck{name: "sqs"},
	}
	handler := NewHealth(checks, 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	response := httptest.NewRecorder()

	handler.Ready(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(response.Body.String(), `"postgres":"unavailable"`) {
		t.Fatalf("body = %s, want postgres reported unavailable", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"sqs":"available"`) {
		t.Fatalf("body = %s, want sqs still reported available even though postgres hung for the full timeout", response.Body.String())
	}
}
