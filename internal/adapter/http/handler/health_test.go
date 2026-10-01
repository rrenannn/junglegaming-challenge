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
