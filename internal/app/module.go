package app

import (
	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/messaging"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/observability"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/postgres"
	"go.uber.org/fx"
)

var Module = fx.Module(
	"jungle-gaming",
	fx.Provide(
		config.Load,
		observability.NewLogger,
		postgres.NewPool,
		messaging.NewSQSClient,
		fx.Annotate(
			postgres.NewHealthCheck,
			fx.As(new(port.HealthCheck)),
			fx.ResultTags(`group:"readiness"`),
		),
		fx.Annotate(
			messaging.NewSQSHealthCheck,
			fx.As(new(port.HealthCheck)),
			fx.ResultTags(`group:"readiness"`),
		),
		fx.Annotate(
			httpadapter.NewServer,
			fx.ParamTags("", "", `group:"readiness"`),
		),
	),
	fx.Invoke(registerLifecycle),
)
