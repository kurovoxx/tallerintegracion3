package http

import (
	"testing"

	"github.com/google/uuid"
)

func TestToPGUUID_ConvertsUUID(t *testing.T) {
	for _, raw := range []string{
		"550e8400-e29b-41d4-a716-446655440001",
		"00000000-0000-0000-0000-000000000000",
		"ffffffff-ffff-ffff-ffff-ffffffffffff",
	} {
		id := uuid.MustParse(raw)
		got := toPGUUID(id)
		if !got.Valid {
			t.Fatalf("UUID %s quedó inválido", raw)
		}
		if got.String() != id.String() {
			t.Fatalf("UUID esperado %s, got %s", id.String(), got.String())
		}
	}
}
