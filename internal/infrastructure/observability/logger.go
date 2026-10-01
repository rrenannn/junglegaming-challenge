package observability

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

func NewLogger(cfg config.Config) (*slog.Logger, error) {
	var level slog.Level
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return nil, fmt.Errorf("unsupported LOG_LEVEL %q", cfg.LogLevel)
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With("service", "jungle-gaming", "environment", cfg.Environment), nil
}
