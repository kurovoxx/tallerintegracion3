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

var accountDeletionSchema = []schemaRequirement{
	relationRequirement("notes.account_deletion_users", "back/notes/db/migrations/003_account_deletion.sql"),
	relationRequirement("notes.account_deletion_group_notes", "back/notes/db/migrations/003_account_deletion.sql"),
}

// VerifyAccountDeletionSchema comprueba al arrancar que existan las tablas del
// borrado de cuenta. Sin ellas el gate de RequireAuth responde 503 en cada
// petición autenticada y BeginAccountDeletion falla con Internal para siempre
// (Identity deja la cuenta en pending con notes_begin_incomplete). init.sql
// solo corre con un volumen nuevo, así que una base local anterior al PR #94
// queda sin estas tablas.
func VerifyAccountDeletionSchema(ctx context.Context, q SchemaQuerier) error {
	return verifySchema(ctx, q, "notes", accountDeletionSchema)
}

func verifySchema(ctx context.Context, q SchemaQuerier, service string, reqs []schemaRequirement) error {
	var missing []string
	for _, req := range reqs {
		var ok bool
		if err := q.QueryRow(ctx, req.query, req.args...).Scan(&ok); err != nil {
			return fmt.Errorf("%s: no se pudo verificar %s: %w", service, req.object, err)
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("%s (%s)", req.object, req.migration))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%s: esquema incompleto para el borrado de cuenta, falta %s; aplica esas migraciones antes de iniciar el servicio",
			service, strings.Join(missing, ", "))
	}
	return nil
}
