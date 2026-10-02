package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/response"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
)

type Health struct {
	checks  []port.HealthCheck
	timeout time.Duration
	logger  *slog.Logger
}

func NewHealth(checks []port.HealthCheck, timeout time.Duration, logger *slog.Logger) *Health {
	return &Health{checks: checks, timeout: timeout, logger: logger}
}

func (h *Health) Live(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, healthResponse{Status: "live"})
}

func (h *Health) Ready(w http.ResponseWriter, request *http.Request) {
	result := healthResponse{Status: "ready", Checks: make(map[string]string, len(h.checks))}
	status := http.StatusOK

	for _, check := range h.checks {
		// Each check gets its own fresh budget. A single context shared
		// across the loop would let one slow/unavailable dependency eat the
		// whole timeout and make later, perfectly healthy dependencies
		// report unavailable too, for the wrong reason.
		ctx, cancel := context.WithTimeout(request.Context(), h.timeout)
		err := check.Check(ctx)
		cancel()
		if err != nil {
			status = http.StatusServiceUnavailable
			result.Status = "not_ready"
			result.Checks[check.Name()] = "unavailable"
			h.logger.WarnContext(request.Context(), "readiness check failed", "dependency", check.Name(), "error", err)
			continue
		}
		result.Checks[check.Name()] = "available"
	}

	response.JSON(w, status, result)
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}
