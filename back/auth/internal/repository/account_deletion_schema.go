package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SchemaQuerier es lo mínimo que necesita la verificación (pool o tx).
type SchemaQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const accountDeletionMigration = "back/auth/db/migrations/004_account_deletion.sql"

// VerifyAccountDeletionSchema comprueba al arrancar que exista
// identity.account_deletions. Sin ella el gate de RequireAuth responde 503 en
// cada petición autenticada y DELETE /auth/account no puede iniciar. init.sql
// solo corre con un volumen nuevo: una base anterior al PR #94 necesita la
// migración. Notes y Social verifican sus propias tablas del mismo modo.
func VerifyAccountDeletionSchema(ctx context.Context, q SchemaQuerier) error {
	const table = "identity.account_deletions"
	var ok bool
	if err := q.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&ok); err != nil {
		return fmt.Errorf("auth: no se pudo verificar %s: %w", table, err)
	}
	if !ok {
		return fmt.Errorf("auth: esquema incompleto para el borrado de cuenta, falta %s (%s); aplica esa migración antes de iniciar el servicio",
			table, accountDeletionMigration)
	}
	return nil
}
