package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeSchemaRow struct {
	ok  bool
	err error
}

func (r fakeSchemaRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*(dest[0].(*bool)) = r.ok
	return nil
}

type fakeSchema fakeSchemaRow

func (f fakeSchema) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeSchemaRow(f)
}

func TestVerifyAccountDeletionSchema_NamesMigration(t *testing.T) {
	ctx := context.Background()
	if err := VerifyAccountDeletionSchema(ctx, fakeSchema{ok: true}); err != nil {
		t.Fatalf("con la tabla: %v", err)
	}
	err := VerifyAccountDeletionSchema(ctx, fakeSchema{ok: false})
	if err == nil || !strings.Contains(err.Error(), "004_account_deletion.sql") ||
		!strings.Contains(err.Error(), "identity.account_deletions") {
		t.Fatalf("sin la tabla = %v", err)
	}
	queryErr := errors.New("pg caído")
	if err := VerifyAccountDeletionSchema(ctx, fakeSchema{err: queryErr}); !errors.Is(err, queryErr) {
		t.Fatalf("error de consulta = %v", err)
	}
}

// Contra PostgreSQL real (AUTH_TEST_DATABASE_URL, base dedicada, -p 1). Todo
// ocurre en una transacción que se revierte: crea la tabla con la migración
// 004 si hace falta, la quita y comprueba el diagnóstico.
func TestVerifyAccountDeletionSchema_Postgres(t *testing.T) {
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AUTH_TEST_DATABASE_URL no definido")
	}
	migration, err := os.ReadFile("../../db/migrations/004_account_deletion.sql")
	if err != nil {
		t.Fatalf("migración: %v", err)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS identity`); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := tx.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("aplicar 004: %v", err)
	}
	if err := VerifyAccountDeletionSchema(ctx, tx); err != nil {
		t.Fatalf("con 004 aplicada: %v", err)
	}
	if _, err := tx.Exec(ctx, `DROP TABLE identity.account_deletions`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := VerifyAccountDeletionSchema(ctx, tx); err == nil || !strings.Contains(err.Error(), "004_account_deletion.sql") {
		t.Fatalf("sin la tabla = %v", err)
	}
}
