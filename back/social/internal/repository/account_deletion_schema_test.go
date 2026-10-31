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

// fakeSchema responde por objeto (tabla o "tabla.columna"); lo no listado existe.
type fakeSchema struct {
	missing map[string]bool
	err     error
}

func (f fakeSchema) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	if f.err != nil {
		return fakeSchemaRow{err: f.err}
	}
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, a.(string))
	}
	return fakeSchemaRow{ok: !f.missing[strings.Join(parts, ".")]}
}

func TestVerifyAccountDeletionSchema_ReportsMissingObjectsAndMigrations(t *testing.T) {
	ctx := context.Background()
	if err := VerifyAccountDeletionSchema(ctx, fakeSchema{}); err != nil {
		t.Fatalf("esquema completo: %v", err)
	}
	err := VerifyAccountDeletionSchema(ctx, fakeSchema{missing: map[string]bool{
		"social.account_deletion_users":                  true,
		"social.account_deletion_groups":                 true,
		"social.account_deletion_groups.stream_removed": true,
	}})
	if err == nil {
		t.Fatal("una base sin migraciones 002/003 debe fallar")
	}
	for _, want := range []string{"002_account_deletion.sql", "003_account_deletion_stream.sql", "social.account_deletion_users"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("el error debe nombrar %q: %v", want, err)
		}
	}
	// Solo falta 003 (columna stream_removed).
	err = VerifyAccountDeletionSchema(ctx, fakeSchema{missing: map[string]bool{
		"social.account_deletion_groups.stream_removed": true,
	}})
	if err == nil || !strings.Contains(err.Error(), "003_account_deletion_stream.sql") || strings.Contains(err.Error(), "002_") {
		t.Fatalf("solo 003 pendiente = %v", err)
	}
	queryErr := errors.New("pg caído")
	if err := VerifyAccountDeletionSchema(ctx, fakeSchema{err: queryErr}); !errors.Is(err, queryErr) {
		t.Fatalf("error de consulta = %v", err)
	}
}

// Contra PostgreSQL real (SOCIAL_TEST_DATABASE_URL), dentro de una transacción
// que se revierte: sin la columna de 003 y sin las tablas de 002.
func TestVerifyAccountDeletionSchema_PostgresWithoutMigrations(t *testing.T) {
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL not configured")
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
	if err := VerifyAccountDeletionSchema(ctx, tx); err != nil {
		t.Fatalf("base de prueba con migraciones: %v", err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE social.account_deletion_groups DROP COLUMN stream_removed`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if err := VerifyAccountDeletionSchema(ctx, tx); err == nil || !strings.Contains(err.Error(), "003_account_deletion_stream.sql") {
		t.Fatalf("sin stream_removed = %v", err)
	}
	if _, err := tx.Exec(ctx, `ALTER TABLE social.account_deletion_users RENAME TO account_deletion_users_hidden`); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := VerifyAccountDeletionSchema(ctx, tx); err == nil || !strings.Contains(err.Error(), "002_account_deletion.sql") {
		t.Fatalf("sin account_deletion_users = %v", err)
	}
}
