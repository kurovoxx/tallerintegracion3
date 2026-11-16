package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// captureQueryDB implementa DBTX solo para capturar la SQL enviada y forzar un
// error temprano: permite verificar la forma exacta del keyset sin PostgreSQL.
type captureQueryDB struct {
	sql  string
	args []any
}

func (c *captureQueryDB) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	c.sql = sql
	c.args = args
	return nil, errors.New("captura: detener ejecución")
}

func (c *captureQueryDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}

func (c *captureQueryDB) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// El cursor debe compararse con una subconsulta de DOS columnas y mantener
// user_id/visibility/external_file_id dentro de la subconsulta. Esta prueba
// evita que se reintroduzca la variante inválida de una sola columna o que se
// pierda el filtro de archivo Drive.
func TestListPublicByUserSQL_CursorDeDosColumnas(t *testing.T) {
	db := &captureQueryDB{}
	repo := NewNoteRepository(nil)
	_, _, err := repo.ListPublicByUser(context.Background(), db, "00000000-0000-4000-8000-000000000001", "00000000-0000-4000-8000-000000000002", 20)
	if err == nil {
		t.Fatal("se esperaba el error de captura")
	}
	query := normalizeSQL(db.sql)
	for _, required := range []string{
		"WHERE user_id = $1 AND visibility = 'public' AND external_file_id IS NOT NULL",
		"AND (created_at, id) < (",
		"SELECT created_at, id FROM notes.notes WHERE id = $2 AND user_id = $1 AND visibility = 'public' AND external_file_id IS NOT NULL",
		"ORDER BY created_at DESC, id DESC LIMIT $3",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("la SQL no contiene %q:\n%s", required, query)
		}
	}
	if got := strings.Count(query, "external_file_id IS NOT NULL"); got != 2 {
		t.Fatalf("external_file_id IS NOT NULL debe aparecer en la consulta y en la subconsulta del cursor, got %d:\n%s", got, query)
	}
	if strings.Contains(query, "SELECT created_at FROM notes.notes") {
		t.Fatalf("la subconsulta del cursor debe devolver created_at e id:\n%s", query)
	}
	if len(db.args) != 3 {
		t.Fatalf("args = %v, want [user_id cursor limit+1]", db.args)
	}
}

func TestListPublicByUserSQL_SinCursorFiltraVisibilidadYArchivo(t *testing.T) {
	db := &captureQueryDB{}
	repo := NewNoteRepository(nil)
	_, _, err := repo.ListPublicByUser(context.Background(), db, "00000000-0000-4000-8000-000000000001", "", 20)
	if err == nil {
		t.Fatal("se esperaba el error de captura")
	}
	query := normalizeSQL(db.sql)
	if !strings.Contains(query, "WHERE user_id = $1 AND visibility = 'public' AND external_file_id IS NOT NULL") {
		t.Fatalf("la SQL sin cursor debe filtrar visibility='public' y external_file_id no nulo:\n%s", query)
	}
	if strings.Contains(query, "created_at, id) <") {
		t.Fatalf("la SQL sin cursor no debe incluir keyset:\n%s", query)
	}
	if len(db.args) != 2 {
		t.Fatalf("args = %v, want [user_id limit+1]", db.args)
	}
}
