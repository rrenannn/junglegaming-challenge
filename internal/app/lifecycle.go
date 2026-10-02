package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	httpadapter "github.com/rrenannn/junglegaming-challenge/internal/adapter/http"
	sqsadapter "github.com/rrenannn/junglegaming-challenge/internal/adapter/sqs"
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
	Consumer  *sqsadapter.Consumer
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
			if err := dependencies.Consumer.Start(ctx); err != nil {
				return fmt.Errorf("start SQS consumer: %w", err)
			}
			dependencies.Logger.Info("application started")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			shutdownCtx, cancel := context.WithTimeout(ctx, dependencies.Config.HTTP.ShutdownTimeout)
			defer cancel()

			consumerErr := dependencies.Consumer.Shutdown(shutdownCtx)
			serverErr := dependencies.Server.Shutdown(shutdownCtx)
			dependencies.Pool.Close()
			dependencies.Logger.Info("application stopped")

			if err := errors.Join(consumerErr, serverErr); err != nil {
				return fmt.Errorf("shutdown: %w", err)
			}
			return nil
		},
	})
}
