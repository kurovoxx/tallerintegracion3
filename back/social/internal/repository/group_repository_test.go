package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// El contexto ya cancelado impide adquirir una conexión: no requiere Postgres.
func TestListMembersPreservesDatabaseError(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	members, err := NewGroupRepository(pool).ListMembers(ctx, uuid.NewString())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("no debe confundirse un fallo de BD con grupo inexistente: %v", err)
	}
	if members != nil {
		t.Fatalf("no deben devolverse miembros ante error: %v", members)
	}
}
