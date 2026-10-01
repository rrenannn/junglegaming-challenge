package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rrenannn/junglegaming-challenge/internal/infrastructure/config"
)

func NewPool(cfg config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}
	poolConfig.MaxConns = cfg.Database.MaxConnections
	poolConfig.MinConns = cfg.Database.MinConnections
	poolConfig.ConnConfig.ConnectTimeout = cfg.Database.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	return pool, nil
}

type HealthCheck struct {
	pool *pgxpool.Pool
}

func NewHealthCheck(pool *pgxpool.Pool) *HealthCheck {
	return &HealthCheck{pool: pool}
}

func (h *HealthCheck) Name() string {
	return "postgres"
}

func (h *HealthCheck) Check(ctx context.Context) error {
	if err := h.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping postgres: %w", err)
	}
	return nil
}
