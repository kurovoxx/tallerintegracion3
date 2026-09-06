package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool crea un PgPool desde DATABASE_URL (pooler us-east-2) y valida con Ping.
// Cumple patrón MasterPrompt: Repository es la única capa que toca SQL (sqlc+pgx).
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, fmt.Errorf("DATABASE_URL vacío — revisa .env")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	// Tuning razonable para Supabase pooler (transaction mode)
	// Pool en modo transacción (Supavisor/pgbouncer) no soporta prepared statements con cache
	// → usar SimpleProtocol para evitar "prepared statement already exists" 42P05
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.MaxConns = 5
	cfg.MinConns = 1
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 15 * time.Minute
	cfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("crear pool: %w", err)
	}
	// Ping con timeout corto para validar conectividad al arrancar
	pingCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}
	return pool, nil
}
