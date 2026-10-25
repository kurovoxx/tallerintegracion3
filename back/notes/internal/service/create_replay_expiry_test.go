package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

// Auditoría Sprint 3 (notas duplicadas): el cliente Flutter reenvía la MISMA
// X-Idempotency-Key cuando no recibió la respuesta del POST /notes (timeout,
// cierre de la app). Ese reintento puede llegar después de la ventana de replay
// de 24 h. Se verifica que el backend NO crea una segunda nota ni un segundo
// archivo: el claim vencido conserva resource_id (COALESCE) y Create adopta la
// nota ya sincronizada.
func TestCreateRetryAfterReplayWindowAdoptsExistingNote(t *testing.T) {
	d := drive.NewMockClient()
	svc, noteStore := newServiceWithDrive(d)
	ctx := context.Background()
	userID := uuid.NewString()
	const key = "flutter-note-create-1700000000000"

	first, err := svc.Create(ctx, userID, "Timeout", nil, "private", stringPtr("cuerpo"), key)
	if err != nil {
		t.Fatalf("create inicial: %v", err)
	}

	// Simula que pasaron más de 24 h: el registro 'completed' venció.
	noteStore.mu.Lock()
	rec := noteStore.idempotencyKeys[idemMemoryKey(userID, "create", key)]
	if rec == nil {
		noteStore.mu.Unlock()
		t.Fatal("la clave debe existir tras el create")
	}
	rec.ExpiresAt = time.Now().UTC().Add(-time.Minute)
	noteStore.mu.Unlock()

	again, err := svc.Create(ctx, userID, "Timeout", nil, "private", stringPtr("cuerpo"), key)
	if err != nil {
		t.Fatalf("retry tras vencer la ventana: %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("el retry debe devolver la MISMA nota: first=%s again=%s", first.ID, again.ID)
	}
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("no debe crearse una segunda nota, got %d", got)
	}
	if first.ExternalFileID == nil || again.ExternalFileID == nil || *first.ExternalFileID != *again.ExternalFileID {
		t.Fatalf("no debe crearse un segundo archivo en Drive: %v vs %v", first.ExternalFileID, again.ExternalFileID)
	}
}

// Reintentos concurrentes con la misma clave (dos pantallas o REFRESCAR + sync
// automática): exactamente una nota; el resto recibe replay o 409 "en
// progreso", nunca una nota nueva.
func TestCreateConcurrentRetriesSameKeyCreateOneNote(t *testing.T) {
	d := drive.NewMockClient()
	svc, noteStore := newServiceWithDrive(d)
	ctx := context.Background()
	userID := uuid.NewString()
	const key = "flutter-note-create-1700000000001"

	const workers = 8
	ids := make(chan string, workers)
	errs := make(chan error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		go func() {
			<-start
			n, err := svc.Create(ctx, userID, "Concurrente", nil, "private", stringPtr("cuerpo"), key)
			if err != nil {
				errs <- err
				return
			}
			ids <- n.ID
		}()
	}
	close(start)
	seen := map[string]bool{}
	for i := 0; i < workers; i++ {
		select {
		case id := <-ids:
			seen[id] = true
		case <-errs:
			// 409 "en progreso": esperado para los que llegan durante el claim.
		case <-time.After(10 * time.Second):
			t.Fatal("timeout esperando los creates concurrentes")
		}
	}
	if len(seen) > 1 {
		t.Fatalf("todos los replays deben apuntar a la misma nota, got %v", seen)
	}
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("exactamente una nota, got %d", got)
	}
}
