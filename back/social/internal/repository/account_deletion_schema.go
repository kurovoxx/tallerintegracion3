package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// SchemaQuerier es lo mínimo que necesita la verificación (pool o tx).
type SchemaQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type schemaRequirement struct {
	object    string
	migration string
	query     string
	args      []any
}

func relationRequirement(name, migration string) schemaRequirement {
	return schemaRequirement{
		object:    name,
		migration: migration,
		query:     `SELECT to_regclass($1) IS NOT NULL`,
		args:      []any{name},
	}
}

func columnRequirement(schema, table, column, migration string) schemaRequirement {
	return schemaRequirement{
		object:    schema + "." + table + "." + column,
		migration: migration,
		query: `SELECT EXISTS (SELECT 1 FROM information_schema.columns
			WHERE table_schema=$1 AND table_name=$2 AND column_name=$3)`,
		args: []any{schema, table, column},
	}
}

var accountDeletionSchema = []schemaRequirement{
	relationRequirement("social.account_deletion_users", "back/social/db/migrations/002_account_deletion.sql"),
	relationRequirement("social.account_deletion_groups", "back/social/db/migrations/002_account_deletion.sql"),
	columnRequirement("social", "account_deletion_groups", "stream_removed", "back/social/db/migrations/003_account_deletion_stream.sql"),
}

// VerifyAccountDeletionSchema comprueba al arrancar que existan las tablas del
// borrado de cuenta. Sin ellas el gate de las rutas autenticadas responde 503
// y PrepareAccountDeletion falla siempre, dejando la cuenta pending en
// Identity. init.sql solo corre con un volumen nuevo, así que una base local
// anterior al PR #94 queda sin estas tablas.
func VerifyAccountDeletionSchema(ctx context.Context, q SchemaQuerier) error {
	var missing []string
	for _, req := range accountDeletionSchema {
		var ok bool
		if err := q.QueryRow(ctx, req.query, req.args...).Scan(&ok); err != nil {
			return fmt.Errorf("social: no se pudo verificar %s: %w", req.object, err)
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("%s (%s)", req.object, req.migration))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("social: esquema incompleto para el borrado de cuenta, falta %s; aplica esas migraciones antes de iniciar el servicio",
			strings.Join(missing, ", "))
	}
	return nil
}
