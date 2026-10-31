package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// 3_2_15 (ventana de notas compartidas), contra PostgreSQL real: el keyset de
// ListByGroup debe recorrer TODAS las comparticiones aunque varias tengan el
// mismo shared_at y aunque el cursor caiga en una nota de admin. Antes el
// cursor solo comparaba shared_at y omitía filas.
func TestSharedListByGroupKeysetTiesAndAdmin(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		ALTER TABLE notes.shared_notes
			ADD COLUMN id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			ADD COLUMN group_id uuid NOT NULL,
			ADD COLUMN is_admin_note boolean NOT NULL DEFAULT false,
			ADD COLUMN access_mode varchar(20) NOT NULL DEFAULT 'link',
			ADD COLUMN author_followers_snapshot int NOT NULL DEFAULT 0,
			ADD COLUMN permission_sync_status text NOT NULL DEFAULT 'pending',
			ADD COLUMN shared_at timestamptz NOT NULL DEFAULT now()`); err != nil {
		t.Fatal(err)
	}
	group := uuid.NewString()
	same := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	// 2 de admin (una más antigua) + 4 normales, tres con el MISMO instante y
	// una más reciente que las de admin.
	rows := []struct {
		admin bool
		at    time.Time
	}{
		{true, same.Add(-time.Hour)}, {true, same},
		{false, same}, {false, same}, {false, same}, {false, same.Add(time.Hour)},
	}
	want := map[string]bool{}
	for _, r := range rows {
		noteID := uuid.NewString()
		if _, err := pool.Exec(ctx, `INSERT INTO notes.notes (id, user_id) VALUES ($1, $2)`, noteID, uuid.NewString()); err != nil {
			t.Fatal(err)
		}
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO notes.shared_notes (note_id, group_id, is_admin_note, shared_at)
			VALUES ($1, $2, $3, $4) RETURNING id`, noteID, group, r.admin, r.at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		want[id] = true
	}

	repo := NewSharedRepository(pool)
	seen := map[string]bool{}
	var order []bool
	cursor := ""
	for page := 0; page < 10; page++ {
		got, next, err := repo.ListByGroup(ctx, nil, group, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range got {
			if seen[s.ID] {
				t.Fatalf("compartición repetida entre páginas: %s", s.ID)
			}
			seen[s.ID] = true
			order = append(order, s.IsAdminNote)
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if len(seen) != len(want) {
		t.Fatalf("paginación incompleta: %d/%d", len(seen), len(want))
	}
	if !order[0] || !order[1] || order[2] {
		t.Fatalf("las notas de admin deben ir primero: %v", order)
	}
}
