package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

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

func (h *Health) Live(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, healthResponse{Status: "live"})
}

func (h *Health) Ready(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), h.timeout)
	defer cancel()

	result := healthResponse{Status: "ready", Checks: make(map[string]string, len(h.checks))}
	status := http.StatusOK

	for _, check := range h.checks {
		if err := check.Check(ctx); err != nil {
			status = http.StatusServiceUnavailable
			result.Status = "not_ready"
			result.Checks[check.Name()] = "unavailable"
			h.logger.WarnContext(ctx, "readiness check failed", "dependency", check.Name(), "error", err)
			continue
		}
		result.Checks[check.Name()] = "available"
	}

	writeJSON(response, status, result)
}

type healthResponse struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(value); err != nil {
		slog.Error("write JSON response", "error", err)
	}
}
