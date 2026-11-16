package repository

import (
	"context"
	"strings"
	"testing"
)

// Los métodos batch deben resolver una página completa en UNA consulta con
// ANY($n::uuid[]); estos tests fijan la forma del SQL sin PostgreSQL real.
func TestLikeRepository_ExistsByNotesSQLEnLote(t *testing.T) {
	db := &captureQueryDB{}
	ids := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}
	_, err := NewLikeRepository(nil).ExistsByNotes(context.Background(), db, "33333333-3333-4333-8333-333333333333", ids)
	if err == nil {
		t.Fatal("se esperaba el error de captura")
	}
	sql := normalizeSQL(db.sql)
	for _, want := range []string{
		"SELECT note_id FROM notes.note_likes",
		"user_id = $1",
		"note_id = ANY($2::uuid[])",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL sin %q: %s", want, sql)
		}
	}
	if len(db.args) != 2 {
		t.Fatalf("args = %d, want 2", len(db.args))
	}
	if got, ok := db.args[1].([]string); !ok || len(got) != len(ids) {
		t.Fatalf("arg de ids = %#v", db.args[1])
	}
}

func TestSavedRepository_CountByNotesSQLEnLote(t *testing.T) {
	db := &captureQueryDB{}
	ids := []string{"11111111-1111-4111-8111-111111111111"}
	_, err := NewSavedRepository(nil).CountByNotes(context.Background(), db, ids)
	if err == nil {
		t.Fatal("se esperaba el error de captura")
	}
	sql := normalizeSQL(db.sql)
	for _, want := range []string{
		"SELECT note_id, count(*) FROM notes.saved_notes",
		"note_id = ANY($1::uuid[])",
		"GROUP BY note_id",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL sin %q: %s", want, sql)
		}
	}
	if len(db.args) != 1 {
		t.Fatalf("args = %d, want 1", len(db.args))
	}
}

func TestSavedRepository_ExistsByNotesSQLEnLote(t *testing.T) {
	db := &captureQueryDB{}
	ids := []string{"11111111-1111-4111-8111-111111111111"}
	_, err := NewSavedRepository(nil).ExistsByNotes(context.Background(), db, "33333333-3333-4333-8333-333333333333", ids)
	if err == nil {
		t.Fatal("se esperaba el error de captura")
	}
	sql := normalizeSQL(db.sql)
	for _, want := range []string{
		"SELECT note_id FROM notes.saved_notes",
		"user_id = $1",
		"note_id = ANY($2::uuid[])",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL sin %q: %s", want, sql)
		}
	}
	if len(db.args) != 2 {
		t.Fatalf("args = %d, want 2", len(db.args))
	}
}
