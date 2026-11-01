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

// fakeSchema responde por nombre de objeto; lo no listado existe.
type fakeSchema struct {
	missing map[string]bool
	err     error
}

func (f fakeSchema) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	if f.err != nil {
		return fakeSchemaRow{err: f.err}
	}
	name, _ := args[0].(string)
	return fakeSchemaRow{ok: !f.missing[name]}
}

func TestVerifyAccountDeletionSchema_ReportsMissingTablesAndMigration(t *testing.T) {
	if err := VerifyAccountDeletionSchema(context.Background(), fakeSchema{}); err != nil {
		t.Fatalf("esquema completo: %v", err)
	}
	err := VerifyAccountDeletionSchema(context.Background(), fakeSchema{missing: map[string]bool{
		"notes.account_deletion_users": true,
	}})
	if err == nil {
		t.Fatal("sin notes.account_deletion_users debe fallar")
	}
	for _, want := range []string{"notes.account_deletion_users", "003_account_deletion.sql"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("el error debe nombrar %q: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "account_deletion_group_notes") {
		t.Fatalf("solo debe listar lo que falta: %v", err)
	}
	queryErr := errors.New("pg caído")
	if err := VerifyAccountDeletionSchema(context.Background(), fakeSchema{err: queryErr}); !errors.Is(err, queryErr) {
		t.Fatalf("error de consulta = %v", err)
	}
}

// Contra PostgreSQL real (NOTES_TEST_DATABASE_URL): reproduce la base local
// sin la migración 003 ocultando la tabla dentro de una transacción.
func TestVerifyAccountDeletionSchema_PostgresWithoutMigration(t *testing.T) {
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL no definido")
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
	if _, err := tx.Exec(ctx, `ALTER TABLE notes.account_deletion_users RENAME TO account_deletion_users_hidden`); err != nil {
		t.Fatalf("rename: %v", err)
	}
	err = VerifyAccountDeletionSchema(ctx, tx)
	if err == nil || !strings.Contains(err.Error(), "003_account_deletion.sql") {
		t.Fatalf("sin la tabla = %v", err)
	}
	// Mismo síntoma que en Docker local: Begin falla mientras falte la tabla.
	if _, err := tx.Exec(ctx, `INSERT INTO notes.account_deletion_users(user_id,status) VALUES(gen_random_uuid(),'pending')`); err == nil {
		t.Fatal("el INSERT de Begin debía fallar sin la tabla")
	}
}
