package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	httpadapter "github.com/rrenannn/junglegaming-challenge/internal/adapter/http"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
	"go.uber.org/fx"
)

type lifecycleDependencies struct {
	fx.In

	Lifecycle fx.Lifecycle
	Config    config.Config
	Logger    *slog.Logger
	Pool      *pgxpool.Pool
	Server    *httpadapter.Server
	Checks    []port.HealthCheck `group:"readiness"`
}

func registerLifecycle(dependencies lifecycleDependencies) {
	dependencies.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			for _, check := range dependencies.Checks {
				checkCtx, cancel := context.WithTimeout(ctx, dependencies.Config.HTTP.ReadinessTimeout)
				err := check.Check(checkCtx)
				cancel()
				if err != nil {
					return fmt.Errorf("dependency %s is not ready: %w", check.Name(), err)
				}
			}
			if err := dependencies.Server.Start(); err != nil {
				return fmt.Errorf("start HTTP server: %w", err)
			}
			dependencies.Logger.Info("application started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(ctx, dependencies.Config.HTTP.ShutdownTimeout)
			defer cancel()

			err := dependencies.Server.Shutdown(shutdownCtx)
			dependencies.Pool.Close()
			dependencies.Logger.Info("application stopped")
			if err != nil {
				return fmt.Errorf("shutdown HTTP server: %w", err)
			}
			return nil
		},
	})
}
