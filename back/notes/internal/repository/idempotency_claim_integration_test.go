package repository

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// Auditoría Sprint 3 (notas duplicadas), contra PostgreSQL real: el cliente
// reenvía la misma X-Idempotency-Key si no recibió la respuesta del POST.
// Aunque el registro 'completed' haya vencido (ventana de 24 h), el reclamo
// debe CONSERVAR el resource_id (COALESCE) para que Create adopte la nota
// existente en lugar de crear otra.
func TestIdempotencyClaimAfterExpiryPreservesResourceID(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	userID := uuid.NewString()
	firstNote := uuid.NewString()
	const op, key, hash = "create", "flutter-note-create-1700000000000", "hash-a"

	rec, claimed, err := repo.GetOrClaimIdempotencyKey(ctx, nil, userID, op, key, hash, &firstNote, time.Now().Add(2*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("claim inicial: claimed=%v err=%v", claimed, err)
	}
	if rec.ResourceID == nil || *rec.ResourceID != firstNote {
		t.Fatalf("resource_id preasignado esperado %s, got %v", firstNote, rec.ResourceID)
	}
	if err := repo.CompleteIdempotencyKey(ctx, nil, userID, op, key, firstNote); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Mismo intento dentro de la ventana: no se reclama (replay).
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, op, key, hash, ptr(uuid.NewString()), time.Now().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if claimed || rec.Status != model.IdempotencyStatusCompleted {
		t.Fatalf("dentro de la ventana debe ser replay: claimed=%v status=%s", claimed, rec.Status)
	}

	// Vence la ventana de replay.
	if _, err := pool.Exec(ctx, `UPDATE notes.idempotency_keys SET expires_at = now() - interval '1 minute'
		WHERE user_id = $1 AND operation = $2 AND idempotency_key = $3`, userID, op, key); err != nil {
		t.Fatal(err)
	}

	// Reintento tardío con OTRO id preasignado: el claim se otorga pero
	// conserva el resource_id original (Create adoptará esa nota).
	retryNote := uuid.NewString()
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, op, key, hash, &retryNote, time.Now().Add(2*time.Minute))
	if err != nil || !claimed {
		t.Fatalf("reclamo tras vencer: claimed=%v err=%v", claimed, err)
	}
	if rec.ResourceID == nil || *rec.ResourceID != firstNote {
		t.Fatalf("el reclamo debe conservar resource_id=%s, got %v", firstNote, rec.ResourceID)
	}
	if rec.Status != model.IdempotencyStatusInProgress {
		t.Fatalf("status esperado in_progress, got %s", rec.Status)
	}
}

func ptr(s string) *string { return &s }
