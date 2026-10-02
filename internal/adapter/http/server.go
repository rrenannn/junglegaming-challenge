package httpadapter

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"

	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/handler"
	"github.com/rrenannn/junglegaming-challenge/internal/adapter/http/middleware"
	"github.com/rrenannn/junglegaming-challenge/internal/application/port"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

type Server struct {
	server *http.Server
	logger *slog.Logger
}

func NewServer(cfg config.Config, logger *slog.Logger, checks []port.HealthCheck, verifier port.IdentityVerifier, walletHandler *handler.Wallet) *Server {
	health := handler.NewHealth(checks, cfg.HTTP.ReadinessTimeout, logger)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", health.Live)
	mux.HandleFunc("GET /health/ready", health.Ready)

	internalOnly := func(next http.HandlerFunc) http.Handler {
		return middleware.Authenticate(verifier)(middleware.RequireScope("wallets.manage")(next))
	}
	mux.Handle("POST /wallets", internalOnly(walletHandler.Open))
	mux.Handle("GET /wallets/{walletId}", internalOnly(walletHandler.Get))

	return &Server{
		server: &http.Server{
			Addr:         cfg.HTTP.Address,
			Handler:      mux,
			ReadTimeout:  cfg.HTTP.ReadTimeout,
			WriteTimeout: cfg.HTTP.WriteTimeout,
			IdleTimeout:  cfg.HTTP.IdleTimeout,
		},
		logger: logger,
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}

	s.logger.Info("HTTP server starting", "address", listener.Addr().String())
	go func() {
		if err := s.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("HTTP server stopped unexpectedly", "error", err)
		}
	}()
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("HTTP server shutting down")
	return s.server.Shutdown(ctx)
}
