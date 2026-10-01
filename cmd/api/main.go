package main

import (
	"log/slog"

	"github.com/rrenannn/junglegaming-challenge/internal/app"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

func main() {
	fx.New(
		app.Module,
		fx.WithLogger(func(logger *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: logger}
		}),
	).Run()
}
