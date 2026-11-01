package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// TestIdempotencyKeyClaimCompleteAndReclaim valida el ciclo durable completo
// sobre notes.idempotency_keys contra PostgreSQL real: claim atómico, replay,
// hash distinto sin reclamar, recuperación de claim vencido (crash) y liberación.
func TestIdempotencyKeyClaimCompleteAndReclaim(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	userID, otherUser := uuid.NewString(), uuid.NewString()
	const key = "idem-key-1"
	expires := time.Now().Add(2 * time.Minute)

	rec, claimed, err := repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-a", nil, expires)
	if err != nil || !claimed || rec == nil || rec.Status != model.IdempotencyStatusInProgress {
		t.Fatalf("primer claim: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// La clave se namespacea por usuario: otro usuario reclama su propia fila.
	if _, otherClaimed, err := repo.GetOrClaimIdempotencyKey(ctx, nil, otherUser, "create", key, "hash-a", nil, expires); err != nil || !otherClaimed {
		t.Fatalf("la clave debe namespacearse por usuario: claimed=%v err=%v", otherClaimed, err)
	}
	// Claim vigente: no se reclama y devuelve el hash registrado.
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-b", nil, expires)
	if err != nil || claimed || rec == nil || rec.RequestHash != "hash-a" {
		t.Fatalf("claim vigente no debe reclamarse: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Completar asocia el recurso y habilita el replay.
	resourceID := uuid.NewString()
	if err := repo.CompleteIdempotencyKey(ctx, nil, userID, "create", key, resourceID); err != nil {
		t.Fatal(err)
	}
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-a", nil, expires)
	if err != nil || claimed || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("replay completado: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Un hash distinto con la misma clave sigue sin reclamar: el servicio
	// decide el 409 Conflict comparando request_hash.
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-distinto", nil, expires)
	if err != nil || claimed || rec == nil || rec.RequestHash != "hash-a" {
		t.Fatalf("hash distinto no debe reclamar: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Claim vencido por crash: un reintento puede reclamarlo de nuevo.
	if _, err := pool.Exec(ctx, `UPDATE notes.idempotency_keys SET expires_at = now() - interval '1 minute' WHERE user_id = $1 AND operation = 'create' AND idempotency_key = $2`, userID, key); err != nil {
		t.Fatal(err)
	}
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-reclamo", nil, time.Now().Add(2*time.Minute))
	if err != nil || !claimed || rec == nil || rec.RequestHash != "hash-reclamo" || rec.Status != model.IdempotencyStatusInProgress {
		t.Fatalf("claim vencido debe reclamarse: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Delete libera la clave para reintentos inmediatos.
	if err := repo.DeleteIdempotencyKey(ctx, nil, userID, "create", key); err != nil {
		t.Fatal(err)
	}
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-nuevo", nil, time.Now().Add(2*time.Minute))
	if err != nil || !claimed || rec == nil || rec.RequestHash != "hash-nuevo" {
		t.Fatalf("tras liberar debe reclamarse: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Complete de una clave inexistente => ErrNotFound.
	if err := repo.CompleteIdempotencyKey(ctx, nil, userID, "create", "no-existe", uuid.NewString()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("complete de clave ausente debe ser ErrNotFound, got %v", err)
	}
}

// TestIdempotencyPreassignedResourceID valida la pre-asociación del recurso al
// claim (crash recovery lógico): un claim nuevo persiste resource_id y un
// reclamo vencido PRESERVA el id del intento que murió.
func TestIdempotencyPreassignedResourceID(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	userID := uuid.NewString()
	first := uuid.NewString()
	rec, claimed, err := repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", "idem-preassigned", "hash-a", &first, time.Now().Add(2*time.Minute))
	if err != nil || !claimed || rec == nil || rec.ResourceID == nil || *rec.ResourceID != first {
		t.Fatalf("claim nuevo debe preasociar resource_id: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE notes.idempotency_keys SET expires_at = now() - interval '1 minute' WHERE user_id = $1 AND operation = 'create' AND idempotency_key = $2`, userID, "idem-preassigned"); err != nil {
		t.Fatal(err)
	}
	second := uuid.NewString()
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", "idem-preassigned", "hash-b", &second, time.Now().Add(2*time.Minute))
	if err != nil || !claimed || rec == nil || rec.ResourceID == nil || *rec.ResourceID != first {
		t.Fatalf("reclamo vencido debe preservar el resource_id previo: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
}

// TestIdempotencyRecoverableImmediateClaim valida la máquina de estados
// recoverable sobre PostgreSQL real: MarkIdempotencyRecoverable asocia el
// recurso ya persistido, el claim con el MISMO request_hash es inmediato
// (aunque expires_at siga vigente) preservando estrictamente el resource_id, y
// un request_hash distinto nunca reclama el registro.
func TestIdempotencyRecoverableImmediateClaim(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	repo := NewNoteRepository(pool)
	userID := uuid.NewString()
	const key = "idem-recoverable-1"
	resourceID := uuid.NewString()
	expires := time.Now().Add(2 * time.Minute)

	rec, claimed, err := repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-a", &resourceID, expires)
	if err != nil || !claimed || rec == nil || rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("claim inicial: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	if err := repo.MarkIdempotencyRecoverable(ctx, nil, userID, "create", key, resourceID, "drive 500"); err != nil {
		t.Fatalf("mark recoverable: %v", err)
	}
	// Hash distinto: nunca se reclama (el servicio responde 409 Conflict).
	other := uuid.NewString()
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-distinto", &other, expires)
	if err != nil || claimed || rec == nil || rec.Status != model.IdempotencyStatusRecoverable || rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("hash distinto no debe reclamar recoverable: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	// Mismo hash: claim inmediato aunque el lease siga vigente, preservando el recurso.
	rec, claimed, err = repo.GetOrClaimIdempotencyKey(ctx, nil, userID, "create", key, "hash-a", &other, expires)
	if err != nil || !claimed || rec == nil || rec.Status != model.IdempotencyStatusInProgress {
		t.Fatalf("mismo hash debe reclamar recoverable de inmediato: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	if rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("el claim recoverable debe preservar el resource_id %s, got %+v", resourceID, rec.ResourceID)
	}
	// Mark sobre una clave inexistente => ErrNotFound.
	if err := repo.MarkIdempotencyRecoverable(ctx, nil, userID, "create", "no-existe", uuid.NewString(), "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mark de clave ausente debe ser ErrNotFound, got %v", err)
	}
}

// TestUpdateExternalFileIDMissingOrDeleted exige RowsAffected == 1: una fila
// ausente o ya borrada se reporta ErrNotFound en lugar de un éxito silencioso.
func TestUpdateExternalFileIDMissingOrDeleted(t *testing.T) {
	pool := outboxTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `ALTER TABLE notes.notes ADD COLUMN IF NOT EXISTS sync_status text, ADD COLUMN IF NOT EXISTS updated_at timestamptz`); err != nil {
		t.Fatal(err)
	}
	repo := NewNoteRepository(pool)
	if err := repo.UpdateExternalFileID(ctx, nil, uuid.NewString(), "file-x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nota ausente debe ser ErrNotFound, got %v", err)
	}
	noteID, owner := uuid.NewString(), uuid.NewString()
	if _, err := pool.Exec(ctx, `INSERT INTO notes.notes (id, user_id, external_file_id) VALUES ($1, $2, NULL)`, noteID, owner); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateExternalFileID(ctx, nil, noteID, "file-y"); err != nil {
		t.Fatalf("update válido failed: %v", err)
	}
	var file *string
	var status *string
	if err := pool.QueryRow(ctx, `SELECT external_file_id, sync_status FROM notes.notes WHERE id = $1`, noteID).Scan(&file, &status); err != nil {
		t.Fatal(err)
	}
	if file == nil || *file != "file-y" || status == nil || *status != "synced" {
		t.Fatalf("external_file_id/sync_status no persistidos: file=%v status=%v", file, status)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM notes.notes WHERE id = $1`, noteID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateExternalFileID(ctx, nil, noteID, "file-z"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("nota borrada debe ser ErrNotFound, got %v", err)
	}
}
