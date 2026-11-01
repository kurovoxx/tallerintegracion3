package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// definitiveFailDriveClient falla las primeras N llamadas a CreateFile con un
// error DEFINITIVO (OAuth revocado, 4xx, 413): Google Drive garantiza que no
// creó el archivo. Después delega en el MockClient.
type definitiveFailDriveClient struct {
	*drive.MockClient
	mu       sync.Mutex
	failures int
	err      error
}

func (d *definitiveFailDriveClient) CreateFile(
	ctx context.Context,
	userID, noteID, title, content string,
) (string, error) {
	d.mu.Lock()
	if d.failures > 0 {
		d.failures--
		err := d.err
		d.mu.Unlock()
		return "", err
	}
	d.mu.Unlock()
	return d.MockClient.CreateFile(ctx, userID, noteID, title, content)
}

func newServiceWithDrive(d drive.Client) (*NoteService, *MemoryNoteStore) {
	noteStore := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	svc := NewNoteService(
		noteStore,
		NewMemoryAttachmentStore(),
		NewMemorySavedStore(),
		likeStore,
		NewMemorySharedStore(),
		d,
		NewMemorySocialResolver(),
	)
	return svc, noteStore
}

// Fase 1 §P2: un fallo DEFINITIVO de Drive antes de crear el archivo no debe
// dejar una fila fantasma en notes.notes. La intención se libera y un retry con
// la misma Idempotency-Key vuelve a crear exactamente una nota (sin duplicar).
func TestCreateDefinitiveDriveFailureDoesNotLeavePhantomNote(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"oauth", &drive.OAuthError{Message: "sin conexión"}},
		{"too_large", &drive.DriveError{Code: 413, Message: "archivo demasiado grande"}},
		{"invalid_request", &drive.DriveError{Code: 400, Message: "petición inválida"}},
		{"forbidden", &drive.DriveError{Code: 403, Message: "sin permiso"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := drive.NewMockClient()
			flaky := &definitiveFailDriveClient{
				MockClient: base,
				failures:   1,
				err:        tc.err,
			}
			svc, noteStore := newServiceWithDrive(flaky)
			ctx := context.Background()
			userID := uuid.NewString()
			key := "create-definitivo-" + tc.name

			if _, err := svc.Create(ctx, userID, "Fantasma", nil, "private", stringPtr("cuerpo"), key); err == nil {
				t.Fatal("el primer intento debe fallar por Drive")
			}
			if got := countNotes(noteStore); got != 0 {
				t.Fatalf("un fallo definitivo no debe dejar fila fantasma, got %d", got)
			}
			if rec, loaded := idemRecordStored(svc, userID, "create", key); loaded {
				t.Fatalf("la clave debe liberarse tras el fallo definitivo, got %+v", rec)
			}

			// Retry con la MISMA clave: una sola nota, con archivo asociado.
			note, err := svc.Create(ctx, userID, "Fantasma", nil, "private", stringPtr("cuerpo"), key)
			if err != nil {
				t.Fatalf("el retry debe crear la nota: %v", err)
			}
			if note == nil || note.ExternalFileID == nil || *note.ExternalFileID == "" {
				t.Fatalf("la nota creada debe tener archivo de Drive, got %+v", note)
			}
			if got := countNotes(noteStore); got != 1 {
				t.Fatalf("el retry debe dejar exactamente una nota, got %d", got)
			}
		})
	}
}

// Fase 1 §P2: un fallo AMBIGUO (5xx/timeout: el archivo pudo crearse) conserva
// la fila failed_sync para la reconciliación idempotente, y el retry reutiliza
// la misma nota en lugar de crear otra.
func TestCreateAmbiguousDriveFailureKeepsRecoverableRow(t *testing.T) {
	base := drive.NewMockClient()
	flaky := &flakyDriveClient{
		MockClient:     base,
		createFailures: driveRetryMaxAttempts,
	}
	svc, noteStore := newFlakyService(flaky)
	ctx := context.Background()
	userID := uuid.NewString()
	key := "create-ambiguo"

	if _, err := svc.Create(ctx, userID, "Ambiguo", nil, "private", stringPtr("cuerpo"), key); err == nil {
		t.Fatal("el primer intento debe fallar por Drive")
	}
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("un fallo ambiguo debe conservar la fila recuperable, got %d", got)
	}
	rec, loaded := idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusRecoverable {
		t.Fatalf("debe quedar recoverable, got %+v", rec)
	}
	if rec.ResourceID == nil || *rec.ResourceID == "" {
		t.Fatal("recoverable debe preservar el resource_id")
	}

	note, err := svc.Create(ctx, userID, "Ambiguo", nil, "private", stringPtr("cuerpo"), key)
	if err != nil {
		t.Fatalf("el retry debe completar la misma nota: %v", err)
	}
	if note == nil || note.ID != *rec.ResourceID {
		t.Fatalf("el retry debe reutilizar la nota %s, got %+v", *rec.ResourceID, note)
	}
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("no debe duplicar la fila en el retry, got %d", got)
	}
}
