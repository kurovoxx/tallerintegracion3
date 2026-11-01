package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// fakeRow permite construir un pgx.Row con un Scan programable.
type fakeRow struct{ scan func(dest ...any) error }

func (r fakeRow) Scan(dest ...any) error { return r.scan(dest...) }

// fakeDBTX captura las queries ejecutadas y devuelve filas programadas para el
// UPDATE ... RETURNING y para el SELECT EXISTS de desambiguación.
type fakeDBTX struct {
	queries   []string
	args      [][]any
	updateRow pgx.Row
	existsRow pgx.Row
}

func (f *fakeDBTX) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	f.queries = append(f.queries, sql)
	f.args = append(f.args, args)
	if strings.Contains(sql, "EXISTS") {
		return f.existsRow
	}
	return f.updateRow
}

func (f *fakeDBTX) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *fakeDBTX) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

// TestNoteRepositoryUpdateOptimisticVersionSQL verifica que Update emita el
// UPDATE condicionado por versión y devuelva ErrConflict cuando la fila existe
// pero la versión no coincide.
func TestNoteRepositoryUpdateOptimisticVersionSQL(t *testing.T) {
	ctx := context.Background()
	title := "nuevo"
	db := &fakeDBTX{
		updateRow: fakeRow{scan: func(dest ...any) error { return pgx.ErrNoRows }},
		existsRow: fakeRow{scan: func(dest ...any) error {
			*(dest[0].(*bool)) = true
			return nil
		}},
	}
	repo := &NoteRepository{}
	_, err := repo.Update(ctx, db, "note-1", &title, nil, 7)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("esperaba ErrConflict cuando la nota existe con version distinta, got %v", err)
	}
	if len(db.queries) != 2 {
		t.Fatalf("esperaba UPDATE + SELECT EXISTS, got %d queries", len(db.queries))
	}
	updateSQL := db.queries[0]
	if !strings.Contains(updateSQL, "version = version + 1") {
		t.Fatalf("el UPDATE debe incrementar version en la misma sentencia: %s", updateSQL)
	}
	if !strings.Contains(updateSQL, "WHERE id = $3 AND version = $2") {
		t.Fatalf("el UPDATE debe condicionar por id y version esperada: %s", updateSQL)
	}
	if len(db.args[0]) != 3 || db.args[0][1] != int64(7) || db.args[0][2] != "note-1" {
		t.Fatalf("argumentos inesperados en el UPDATE: %#v", db.args[0])
	}
}

// TestNoteRepositoryUpdateMissingNote verifica que una nota inexistente siga
// devolviendo (nil, nil) sin confundirse con un conflicto de versión.
func TestNoteRepositoryUpdateMissingNote(t *testing.T) {
	ctx := context.Background()
	db := &fakeDBTX{
		updateRow: fakeRow{scan: func(dest ...any) error { return pgx.ErrNoRows }},
		existsRow: fakeRow{scan: func(dest ...any) error {
			*(dest[0].(*bool)) = false
			return nil
		}},
	}
	repo := &NoteRepository{}
	note, err := repo.Update(ctx, db, "note-missing", nil, nil, 3)
	if err != nil || note != nil {
		t.Fatalf("nota inexistente debe devolver (nil, nil), got (%v, %v)", note, err)
	}
	if !strings.Contains(db.queries[0], "WHERE id = $2 AND version = $1") {
		t.Fatalf("sin campos de metadata el UPDATE debe seguir condicionando por versión: %s", db.queries[0])
	}
	if len(db.args[0]) != 2 || db.args[0][0] != int64(3) || db.args[0][1] != "note-missing" {
		t.Fatalf("argumentos inesperados en el UPDATE sin campos: %#v", db.args[0])
	}
}

// TestNoteRepositoryUpdateSuccess verifica el camino feliz: el UPDATE devuelve
// la fila con la nueva versión.
func TestNoteRepositoryUpdateSuccess(t *testing.T) {
	ctx := context.Background()
	title := "ok"
	visibility := "public"
	db := &fakeDBTX{
		updateRow: fakeRow{scan: func(dest ...any) error {
			*(dest[0].(*string)) = "note-1"
			*(dest[1].(*string)) = "user-1"
			*(dest[3].(*string)) = title
			*(dest[5].(*string)) = visibility
			*(dest[9].(*int64)) = 5
			return nil
		}},
	}
	repo := &NoteRepository{}
	note, err := repo.Update(ctx, db, "note-1", &title, &visibility, 4)
	if err != nil {
		t.Fatalf("update exitoso no debe fallar: %v", err)
	}
	if note == nil || note.Version != 5 {
		t.Fatalf("esperaba la fila con version 5, got %+v", note)
	}
	if !strings.Contains(db.queries[0], "WHERE id = $4 AND version = $3") {
		t.Fatalf("con title+visibility la numeración debe ser id=$4/version=$3: %s", db.queries[0])
	}
	if len(db.args[0]) != 4 || db.args[0][2] != int64(4) || db.args[0][3] != "note-1" {
		t.Fatalf("argumentos inesperados: %#v", db.args[0])
	}
}
