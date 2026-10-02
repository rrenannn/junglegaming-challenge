package app

import (
	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http"
	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/handler"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	apprepository "github.com/rrenannn/junglegaming-challenge/internal/application/repository"
	"github.com/rrenannn/junglegaming-challenge/internal/application/service"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/auth"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/clock"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/idgen"
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
		fx.Annotate(clock.NewSystemClock, fx.As(new(port.Clock))),
		fx.Annotate(idgen.NewUUIDGenerator, fx.As(new(port.IDGenerator))),
		fx.Annotate(postgres.NewUnitOfWork, fx.As(new(apprepository.UnitOfWork))),
		fx.Annotate(auth.NewKeycloakVerifier, fx.As(new(port.IdentityVerifier))),
		service.NewProcessWagerService,
		service.NewOpenWalletService,
		service.NewGetWalletService,
		handler.NewWallet,
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
			fx.ParamTags("", "", `group:"readiness"`, "", ""),
		),
	),
	fx.Invoke(registerLifecycle),
)
