package service

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testDatabaseURL = "postgres://jungle_app:jungle-local-password@localhost:5432/jungle_gaming?sslmode=disable"

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = testDatabaseURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("postgres not reachable: %v", err)
	}

	applyMigrations(t, databaseURL)

	t.Cleanup(func() {
		resetSchema(t, pool)
		pool.Close()
	})
	resetSchema(t, pool)

	return pool
}

func applyMigrations(t *testing.T, databaseURL string) {
	t.Helper()

	m, err := migrate.New("file://../../../migrations", databaseURL)
	if err != nil {
		t.Fatalf("open migrate: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("apply migrations: %v", err)
	}
}

func resetSchema(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(ctx, `
		TRUNCATE TABLE wallet_ledger_entries, wager_transactions, wallets RESTART IDENTITY CASCADE
	`)
	if err != nil {
		t.Fatalf("reset schema: %v", err)
	}
}
