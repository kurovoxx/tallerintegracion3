package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// flakyDriveClient falla las primeras N llamadas a CreateFile/CopyFile (5xx
// retryable) y luego delega en el MockClient. Permite simular Drive caído
// justo después de insertar la fila local: el intento agota los reintentos
// internos y devuelve error, mientras que el retry posterior de la misma
// Idempotency-Key encuentra el upstream sano.
type flakyDriveClient struct {
	*drive.MockClient
	mu             sync.Mutex
	createFailures int
	copyFailures   int
}

func (f *flakyDriveClient) CreateFile(ctx context.Context, userID, noteID, title, content string) (string, error) {
	f.mu.Lock()
	if f.createFailures > 0 {
		f.createFailures--
		f.mu.Unlock()
		return "", &drive.DriveError{Code: 500, Message: "upstream caído"}
	}
	f.mu.Unlock()
	return f.MockClient.CreateFile(ctx, userID, noteID, title, content)
}

func (f *flakyDriveClient) CopyFile(ctx context.Context, srcUserID, srcFileID, dstUserID, newNoteID, newTitle string) (string, error) {
	f.mu.Lock()
	if f.copyFailures > 0 {
		f.copyFailures--
		f.mu.Unlock()
		return "", &drive.DriveError{Code: 500, Message: "upstream caído"}
	}
	f.mu.Unlock()
	return f.MockClient.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newNoteID, newTitle)
}

func newFlakyService(flaky *flakyDriveClient) (*NoteService, *MemoryNoteStore) {
	noteStore := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	svc := NewNoteService(noteStore, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), flaky, NewMemorySocialResolver())
	return svc, noteStore
}

// Un registro 'recoverable' con el mismo request_hash se reclama de inmediato
// (aunque expires_at siga vigente) preservando estrictamente el resource_id
// preexistente; con un hash distinto nunca se reclama (409 en el servicio).
func TestIdempotencyRecoverableClaimImmediateWithoutLease(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-recoverable-claim"
	resourceID := uuid.NewString()
	// Lease deliberadamente VIGENTE: la reclamación recoverable no espera a que venza.
	seedIdemClaimWithResource(svc, userID, "create", key, "hash-original", model.IdempotencyStatusRecoverable, resourceID, time.Now().Add(time.Hour))

	otherPreassigned := uuid.NewString()
	rec, claimed, err := svc.notes.GetOrClaimIdempotencyKey(ctx, userID, "create", key, "hash-original", &otherPreassigned, time.Now().Add(idemClaimTTL))
	if err != nil || !claimed || rec == nil {
		t.Fatalf("recoverable con mismo hash debe reclamarse de inmediato: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	if rec.Status != model.IdempotencyStatusInProgress {
		t.Fatalf("el claim debe quedar in_progress, got %q", rec.Status)
	}
	if rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("debe preservar el resource_id preexistente %s, got %+v", resourceID, rec.ResourceID)
	}
	if *rec.ResourceID == otherPreassigned {
		t.Fatal("no debe reemplazar el recurso preexistente por el preasignado nuevo")
	}

	// Con hash distinto el registro recoverable no se reclama.
	if err := svc.notes.MarkIdempotencyRecoverable(ctx, userID, "create", key, resourceID, "drive 500"); err != nil {
		t.Fatalf("mark recoverable failed: %v", err)
	}
	rec, claimed, err = svc.notes.GetOrClaimIdempotencyKey(ctx, userID, "create", key, "hash-distinto", &otherPreassigned, time.Now().Add(idemClaimTTL))
	if err != nil || claimed || rec == nil {
		t.Fatalf("hash distinto no debe reclamar: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	if rec.Status != model.IdempotencyStatusRecoverable || rec.ResourceID == nil || *rec.ResourceID != resourceID {
		t.Fatalf("el registro debe seguir recoverable con su recurso, got %+v", rec)
	}
}

// Fallo de Drive DESPUÉS de insertar la fila local: la clave queda
// 'recoverable' con el noteID insertado y el reintento inmediato con la misma
// clave retoma esa misma nota (sin duplicar filas ni archivos) en lugar de
// liberar la clave y crear una nota nueva.
func TestIdempotencyCreateRecoverableImmediateRetryResumesNote(t *testing.T) {
	base := drive.NewMockClient()
	flaky := &flakyDriveClient{MockClient: base, createFailures: driveRetryMaxAttempts}
	svc, noteStore := newFlakyService(flaky)
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-recoverable-create"

	if _, err := svc.Create(ctx, userID, "Recuperable", nil, "private", stringPtr("v1"), key); err == nil {
		t.Fatal("el primer intento debe fallar por Drive tras insertar la fila")
	}
	rec, loaded := idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusRecoverable {
		t.Fatalf("el fallo post-inserción debe dejar la clave recoverable, got %+v", rec)
	}
	if rec.ResourceID == nil || *rec.ResourceID == "" {
		t.Fatal("recoverable debe preservar el resource_id insertado")
	}
	noteID := *rec.ResourceID
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("el primer intento debe insertar exactamente una nota, got %d", got)
	}
	if base.FileCount() != 0 {
		t.Fatalf("Drive no debe tener archivos tras el fallo, got %d", base.FileCount())
	}

	note, err := svc.Create(ctx, userID, "Recuperable", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("el retry inmediato con la misma clave debe retomar la nota, got %v", err)
	}
	if note.ID != noteID {
		t.Fatalf("el retry debe reutilizar la nota %s, got %s", noteID, note.ID)
	}
	if got := countNotes(noteStore); got != 1 {
		t.Fatalf("no debe duplicar la nota local, got %d", got)
	}
	if base.FileCount() != 1 {
		t.Fatalf("debe existir exactamente un archivo en Drive, got %d", base.FileCount())
	}
	rec, loaded = idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != noteID {
		t.Fatalf("la clave debe completarse con el recurso retomado, got %+v", rec)
	}
}

// Mientras la clave está 'recoverable', el mismo payload retoma el recurso
// existente y un payload distinto responde 409 Conflict sin efectos nuevos.
func TestIdempotencyRecoverableSameHashResumesAndDifferentHashConflicts(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-recoverable-conflict"
	first, err := svc.Create(ctx, userID, "Recuperable hash", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("create inicial failed: %v", err)
	}
	if err := svc.notes.MarkIdempotencyRecoverable(ctx, userID, "create", key, first.ID, "drive 500"); err != nil {
		t.Fatalf("mark recoverable failed: %v", err)
	}
	// Mismo hash: se retoma la nota publicada sin volver a tocar Drive.
	filesBefore := driveMock.FileCount()
	same, err := svc.Create(ctx, userID, "Recuperable hash", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("mismo hash debe retomar la nota existente, got %v", err)
	}
	if same.ID != first.ID {
		t.Fatalf("debe devolver la nota existente %s, got %s", first.ID, same.ID)
	}
	if driveMock.FileCount() != filesBefore {
		t.Fatalf("el retry no debe duplicar archivos en Drive (%d -> %d)", filesBefore, driveMock.FileCount())
	}
	// Hash distinto: 409 Conflict sin crear filas ni archivos.
	if err := svc.notes.MarkIdempotencyRecoverable(ctx, userID, "create", key, first.ID, "drive 500"); err != nil {
		t.Fatalf("re-mark recoverable failed: %v", err)
	}
	notesBefore := countNotes(noteStore)
	filesBefore = driveMock.FileCount()
	_, err = svc.Create(ctx, userID, "Payload distinto", nil, "private", stringPtr("v2"), key)
	if err == nil {
		t.Fatal("hash distinto sobre recoverable debe rechazarse")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" {
		t.Fatalf("esperaba conflict (409), got %v", err)
	}
	if countNotes(noteStore) != notesBefore || driveMock.FileCount() != filesBefore {
		t.Fatal("el conflicto no debe tener efectos")
	}
}

// Fallo de CopyFile DESPUÉS de insertar el clon: la clave queda 'recoverable'
// con el id del clon y el retry inmediato lo retoma sin duplicar el clon ni el
// archivo remoto.
func TestIdempotencyCopyRecoverableImmediateRetryResumesClone(t *testing.T) {
	base := drive.NewMockClient()
	flaky := &flakyDriveClient{MockClient: base, copyFailures: driveRetryMaxAttempts}
	svc, noteStore := newFlakyService(flaky)
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen recuperable", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "idem-recoverable-copy"

	if _, err := svc.Copy(ctx, copier, src.ID, key); err == nil {
		t.Fatal("el primer copy debe fallar por Drive tras insertar el clon")
	}
	rec, loaded := idemRecordStored(svc, copier, "copy", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusRecoverable {
		t.Fatalf("el fallo post-inserción de copy debe dejar la clave recoverable, got %+v", rec)
	}
	if rec.ResourceID == nil || *rec.ResourceID == "" {
		t.Fatal("recoverable debe preservar el id del clon")
	}
	cloneID := *rec.ResourceID
	if got := countNotes(noteStore); got != 2 {
		t.Fatalf("debe existir el origen y un clon, got %d notas", got)
	}

	cloned, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("el retry inmediato del copy debe retomar el clon, got %v", err)
	}
	if cloned.ID != cloneID {
		t.Fatalf("el retry debe reutilizar el clon %s, got %s", cloneID, cloned.ID)
	}
	if got := countNotes(noteStore); got != 2 {
		t.Fatalf("no debe duplicar el clon, got %d notas", got)
	}
	if base.FileCount() != 2 {
		t.Fatalf("debe existir el archivo origen y el clon, got %d", base.FileCount())
	}
	rec, loaded = idemRecordStored(svc, copier, "copy", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != cloneID {
		t.Fatalf("la clave de copy debe completarse con el clon retomado, got %+v", rec)
	}
}

// Un fallo ANTES de insertar la fila local sigue liberando la clave (DELETE):
// nada que retomar, el retry debe poder reclamar de nuevo sin quedar
// 'recoverable' fantasma.
func TestIdempotencyCreatePreInsertFailureStillReleasesKey(t *testing.T) {
	driveMock := drive.NewMockClient()
	store := &flakyCreateStore{MemoryNoteStore: NewMemoryNoteStore(), failOnce: true}
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(store.MemoryNoteStore)
	svc := NewNoteService(store, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-recoverable-preinsert"
	if _, err := svc.Create(ctx, userID, "Sin fila", nil, "private", nil, key); err == nil {
		t.Fatal("el primer intento debe fallar al insertar")
	}
	if rec, loaded := idemRecordStored(svc, userID, "create", key); loaded {
		t.Fatalf("un fallo pre-inserción no debe dejar registro durable, got %+v", rec)
	}
	if _, err := svc.Create(ctx, userID, "Sin fila", nil, "private", nil, key); err != nil {
		t.Fatalf("el retry debe proceder tras liberar la clave: %v", err)
	}
}
