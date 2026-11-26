package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// publicNotesTestPool crea una base desechable (requiere NOTES_TEST_DATABASE_URL
// con permiso CREATE DATABASE) y el esquema mínimo notes.notes. Si la variable
// no está definida, la prueba se salta: no se simula una ejecución real.
func publicNotesTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	name := "notes_public_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, `
		CREATE SCHEMA notes;
		CREATE TABLE notes.notes (
			id uuid PRIMARY KEY,
			user_id uuid NOT NULL,
			subject_id uuid,
			title text NOT NULL,
			external_file_id text,
			visibility text NOT NULL,
			likes_count integer NOT NULL DEFAULT 0,
			forked_from_note_id uuid,
			sync_status text,
			version bigint NOT NULL DEFAULT 1,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		);`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

// TestListPublicByUserPostgres ejecuta la consulta keyset real contra
// PostgreSQL: filtra privados, pagina con el cursor de dos columnas y no
// devuelve filas ante cursores inexistentes, privados o de otro autor.
func TestListPublicByUserPostgres(t *testing.T) {
	pool := publicNotesTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	author := uuid.NewString()
	otherAuthor := uuid.NewString()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	insert := func(id, userID, title, visibility string, created time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO notes.notes
				(id, user_id, subject_id, title, external_file_id, visibility,
				 likes_count, forked_from_note_id, sync_status, version, created_at, updated_at)
			VALUES ($1, $2, NULL, $3, $4, $5, 5, NULL, 'synced', 2, $6, $6)`,
			id, userID, title, "drive-"+id, visibility, created)
		if err != nil {
			t.Fatal(err)
		}
	}
	insertSinArchivo := func(id, userID, title, visibility string, created time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO notes.notes
				(id, user_id, subject_id, title, external_file_id, visibility,
				 likes_count, forked_from_note_id, sync_status, version, created_at, updated_at)
			VALUES ($1, $2, NULL, $3, NULL, $4, 5, NULL, 'synced', 2, $5, $5)`,
			id, userID, title, visibility, created)
		if err != nil {
			t.Fatal(err)
		}
	}

	// ids deterministas 101..108 para desempatar por id DESC.
	noteID := func(n int) string {
		return "00000000-0000-4000-8000-00000000010" + string(rune('0'+n))
	}

	p1 := noteID(1)
	p2 := noteID(2)
	p3 := noteID(3)
	p4 := noteID(4) // empate con p5
	p5 := noteID(5)
	pPrivate := noteID(6)
	pOther := noteID(7)
	pNoFile := noteID(8)
	pNoFileMid := noteID(9)

	insert(p1, author, "p1", "public", base.Add(5*time.Minute))
	insert(p2, author, "p2", "public", base.Add(4*time.Minute))
	insert(p3, author, "p3", "public", base.Add(3*time.Minute))
	insert(p4, author, "p4", "public", base.Add(2*time.Minute))
	insert(p5, author, "p5", "public", base.Add(2*time.Minute))
	insert(pPrivate, author, "privada", "private", base.Add(6*time.Minute))
	insert(pOther, otherAuthor, "otro autor", "public", base.Add(7*time.Minute))
	// Públicas sin archivo: una más nueva que todas y otra entre p2 y p3.
	// Ninguna debe aparecer ni consumir cupo de paginación.
	insertSinArchivo(pNoFile, author, "pública sin archivo", "public", base.Add(8*time.Minute))
	insertSinArchivo(pNoFileMid, author, "pública sin archivo intermedia", "public", base.Add(210*time.Second))

	page1, next1, err := repo.ListPublicByUser(ctx, nil, author, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || page1[0].ID != p1 || page1[1].ID != p2 {
		t.Fatalf("página 1 = %+v, want [%s %s]", page1, p1, p2)
	}
	if next1 != p2 {
		t.Fatalf("next_cursor = %q, want %q", next1, p2)
	}
	for _, n := range page1 {
		if n.ID == pPrivate || n.ID == pOther {
			t.Fatalf("página 1 filtró una nota indebida: %+v", n)
		}
	}

	page2, next2, err := repo.ListPublicByUser(ctx, nil, author, next1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 || page2[0].ID != p3 || page2[1].ID != p5 {
		t.Fatalf("página 2 = %+v, want [%s %s] (empate resuelto por id DESC)", page2, p3, p5)
	}
	if next2 != p5 {
		t.Fatalf("next_cursor 2 = %q, want %q", next2, p5)
	}

	page3, next3, err := repo.ListPublicByUser(ctx, nil, author, next2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 1 || page3[0].ID != p4 || next3 != "" {
		t.Fatalf("página 3 = %+v next=%q, want [%s] y fin", page3, next3, p4)
	}

	for _, tc := range []struct {
		name   string
		cursor string
	}{
		{"inexistente", uuid.NewString()},
		{"privado", pPrivate},
		{"otro autor", pOther},
		{"público sin archivo", pNoFile},
		{"público sin archivo intermedio", pNoFileMid},
	} {
		notes, next, err := repo.ListPublicByUser(ctx, nil, author, tc.cursor, 20)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(notes) != 0 || next != "" {
			t.Fatalf("%s: cursor no válido debe dar lista vacía, got %+v next=%q", tc.name, notes, next)
		}
	}
}
