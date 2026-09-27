package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// failingCreateStore falla solo en Create (simula timeout/deadlock de Postgres
// después de que Drive ya respondió 200).
type failingCreateStore struct {
	*MemoryNoteStore
}

func (f *failingCreateStore) Create(ctx context.Context, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	return nil, fmt.Errorf("db insert failed (simulado)")
}

func TestCopyRollbackOnDBFailureNoOrphan(t *testing.T) {
	driveMock := drive.NewMockClient()
	noteStoreOK := NewMemoryNoteStore()
	attStore := NewMemoryAttachmentStore()
	savedStore := NewMemorySavedStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStoreOK)
	sharedStore := NewMemorySharedStore()
	social := NewMemorySocialResolver()
	svcOK := NewNoteService(noteStoreOK, attStore, savedStore, likeStore, sharedStore, driveMock, social)

	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	orig, err := svcOK.Create(ctx, author, "Origen rollback", nil, "public", stringPtr("contenido"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	before := driveMock.FileCount()

	// Mismo drive, pero store de notas que falla en Create (clon PG falla).
	failing := &failingCreateStore{MemoryNoteStore: NewMemoryNoteStore()}
	likeFail := NewMemoryLikeStore()
	svcFail := NewNoteService(failing, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeFail, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	// Inyectar el origen en el store fallido para que Copy encuentre metadata.
	// Copiamos la nota original al store fallido vía memoria directa.
	failing.MemoryNoteStore.notes[orig.ID] = &model.Note{
		ID: orig.ID, UserID: orig.UserID, Title: orig.Title,
		ExternalFileID: orig.ExternalFileID, Visibility: orig.Visibility,
	}
	_, err = svcFail.Copy(ctx, copier, orig.ID, "")
	if err == nil {
		t.Fatal("esperaba error DB en Copy")
	}
	if driveMock.FileCount() != before {
		t.Fatalf("rollback copy falló: archivo huérfano en Drive, antes=%d ahora=%d", before, driveMock.FileCount())
	}
}

// failingAttachmentStore falla en Create de adjuntos tras UploadAttachment OK.
type failingAttachmentStore struct {
	*MemoryAttachmentStore
}

func (f *failingAttachmentStore) Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error) {
	return nil, fmt.Errorf("db attachment insert failed (simulado)")
}

func TestAttachmentRollbackOnDBFailureNoOrphan(t *testing.T) {
	driveMock := drive.NewMockClient()
	noteStore := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	svc := NewNoteService(noteStore, &failingAttachmentStore{NewMemoryAttachmentStore()}, NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	author := uuid.NewString()
	// Crear nota con store sano primero usando servicio sano que comparte drive.
	svcOK, _, _, _, _, _, _, _ := newTestService()
	_ = svcOK
	note, err := NewNoteService(noteStore, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver()).Create(ctx, author, "Con adjunto rollback", nil, "private", nil, "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	before := driveMock.FileCount()
	_, err = svc.AddAttachment(ctx, author, note.ID, "f.png", "image/png", []byte("data"), false)
	if err == nil {
		t.Fatal("esperaba error DB en AddAttachment")
	}
	if driveMock.FileCount() != before {
		t.Fatalf("rollback attachment falló: adjunto huérfano en Drive, antes=%d ahora=%d", before, driveMock.FileCount())
	}
}

// failingExtIDStore falla en UpdateExternalFileID tras CreateFile OK (ruta
// self-healing de Update cuando la nota legacy no tiene drive_file_id).
type failingExtIDStore struct {
	*MemoryNoteStore
}

func (f *failingExtIDStore) UpdateExternalFileID(ctx context.Context, noteID, fileID string) error {
	return fmt.Errorf("db update external_file_id failed (simulado)")
}

// failingUpdateStore falla solo en Update (simula caída de PG justo después de
// que Drive ya aceptó el nuevo contenido).
type failingUpdateStore struct {
	*MemoryNoteStore
}

func (f *failingUpdateStore) Update(ctx context.Context, id string, title *string, visibility *string, expectedVersion int64) (*model.Note, error) {
	return nil, fmt.Errorf("db update failed (simulado)")
}

// Idempotencia robusta: el replay con la misma X-Idempotency-Key devuelve el
// puntero cacheado (*model.Note) sin volver a tocar PG/Drive.
func TestRefactorIdempotencyReplayReturnsCachedNote(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-replay-1"
	note1, err := svc.Create(ctx, userID, "Idem replay", nil, "private", stringPtr("x"), key)
	if err != nil {
		t.Fatalf("primer create failed: %v", err)
	}
	filesBefore := driveMock.FileCount()
	notesBefore := countNotes(noteStore)
	note2, err := svc.Create(ctx, userID, "Idem replay", nil, "private", stringPtr("x"), key)
	if err != nil {
		t.Fatalf("replay debe responder desde caché, got %v", err)
	}
	if note2.ID != note1.ID {
		t.Fatalf("replay debe devolver la misma nota: %s vs %s", note2.ID, note1.ID)
	}
	if driveMock.FileCount() != filesBefore || countNotes(noteStore) != notesBefore {
		t.Fatalf("replay no debe duplicar efectos en Drive/PG (files %d->%d, notes %d->%d)",
			filesBefore, driveMock.FileCount(), notesBefore, countNotes(noteStore))
	}
}

// flakyCreateStore falla la primera vez en Create y luego delega al store real.
type flakyCreateStore struct {
	*MemoryNoteStore
	failOnce bool
}

func (f *flakyCreateStore) Create(ctx context.Context, noteID string, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	if f.failOnce {
		f.failOnce = false
		return nil, fmt.Errorf("db insert failed (simulado)")
	}
	return f.MemoryNoteStore.Create(ctx, noteID, userID, subjectID, title, externalFileID, visibility, forkedFrom, syncStatus)
}

// Idempotencia: ante error, el defer libera la clave para permitir reintentos
// legítimos (no queda "solicitud en progreso" ni se retiene memoria).
func TestRefactorIdempotencyCacheFreesOnError(t *testing.T) {
	driveMock := drive.NewMockClient()
	store := &flakyCreateStore{MemoryNoteStore: NewMemoryNoteStore(), failOnce: true}
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(store.MemoryNoteStore)
	svc := NewNoteService(store, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-error-1"
	if _, err := svc.Create(ctx, userID, "Primer intento", nil, "private", nil, key); err == nil {
		t.Fatal("esperaba error de PG en el primer intento")
	}
	note, err := svc.Create(ctx, userID, "Reintento", nil, "private", nil, key)
	if err != nil {
		t.Fatalf("el reintento con la misma clave debe proceder tras el error: %v", err)
	}
	if note == nil || note.ID == "" {
		t.Fatal("reintento debe crear la nota")
	}
}

// Consistencia causal en Update: si PG falla tras actualizar Drive, el servicio
// debe compensar revirtiendo el contenido (y título) previos en Drive.
func TestRefactorUpdatePGFailureRevertsDrive(t *testing.T) {
	driveMock := drive.NewMockClient()
	base := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(base)
	ctx := context.Background()
	author := uuid.NewString()
	svcOK := NewNoteService(base, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	note, err := svcOK.Create(ctx, author, "Original", nil, "private", stringPtr("contenido original"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID
	svcFail := NewNoteService(&failingUpdateStore{base}, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	newContent := "contenido nuevo"
	if _, err := svcFail.Update(ctx, author, note.ID, stringPtr("Título nuevo"), nil, &newContent, ""); err == nil {
		t.Fatal("esperaba error interno por fallo de PG en Update")
	}
	// Drive revertido al estado previo (compensación explícita).
	got, err := driveMock.GetFileContent(ctx, author, fileID)
	if err != nil {
		t.Fatalf("lectura Drive post-compensación failed: %v", err)
	}
	if got != "contenido original" {
		t.Fatalf("Drive debe revertirse al contenido previo, got %q", got)
	}
	// PG intacto (el Update falló, no debe haber mutación parcial).
	stored, _ := base.GetByID(ctx, note.ID)
	if stored == nil || stored.Title != "Original" {
		t.Fatalf("PG no debe mutar tras fallo de Update, got %+v", stored)
	}
}

func TestUpdateSelfHealingRollbackOnDBFailure(t *testing.T) {
	driveMock := drive.NewMockClient()
	base := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(base)
	ctx := context.Background()
	author := uuid.NewString()
	svcCreate := NewNoteService(base, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	note, err := svcCreate.Create(ctx, author, "Legacy", nil, "private", stringPtr("orig"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// Simular legacy sin file.
	base.mu.Lock()
	base.notes[note.ID].ExternalFileID = nil
	base.mu.Unlock()
	before := driveMock.FileCount()
	// Servicio con UpdateExternalFileID fallido pero mismo base store y drive.
	failing := &failingExtIDStore{base}
	svcFail := NewNoteService(failing, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	newBody := "recuperado"
	if _, err := svcFail.Update(ctx, author, note.ID, nil, nil, &newBody, ""); err == nil {
		t.Fatal("esperaba error DB en self-healing")
	}
	// El archivo recién creado debe haberse compensado. Como el original sigue
	// existiendo (legacy solo limpió metadata), el conteo debe volver a `before`.
	if driveMock.FileCount() != before {
		t.Fatalf("rollback self-healing falló, antes=%d ahora=%d", before, driveMock.FileCount())
	}
}

// failingDeleteStore falla solo en Delete (simula caída de PG tras lectura OK).
// Sirve para evidenciar el orden canónico PG-primero: si PG falla, Drive debe
// quedar intacto (sin pérdida de datos).
type failingDeleteStore struct {
	*MemoryNoteStore
}

func (f *failingDeleteStore) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("db delete failed (simulado)")
}

func (f *failingDeleteStore) DeleteWithDriveCleanup(ctx context.Context, id, requesterID string) (string, error) {
	return "", fmt.Errorf("db delete failed (simulado)")
}

func countNotes(s *MemoryNoteStore) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.notes)
}

// Orden canónico: si PG falla al borrar, el archivo en Drive debe preservarse
// (operación reintentable, fila + archivo siguen consistentes, sin pérdida).
func TestDeletePGFailurePreservesDriveFile(t *testing.T) {
	driveMock := drive.NewMockClient()
	base := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(base)
	ctx := context.Background()
	author := uuid.NewString()
	svcOK := NewNoteService(base, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	note, err := svcOK.Create(ctx, author, "No perder", nil, "private", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID
	if !driveMock.HasFile(fileID) {
		t.Fatal("precondición: Drive debe tener el archivo")
	}
	// Mismo estado PG pero Delete fallido.
	svcFail := NewNoteService(&failingDeleteStore{base}, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	if err := svcFail.Delete(ctx, author, note.ID); err == nil {
		t.Fatal("esperaba error DB en Delete")
	}
	// Drive intacto (no se perdió el archivo) y fila intacta (reintentable).
	if !driveMock.HasFile(fileID) {
		t.Fatal("orden canónico violado: PG falló pero Drive ya había borrado (pérdida de datos)")
	}
	if stored, _ := base.GetByID(ctx, note.ID); stored == nil {
		t.Fatal("fila PG debe seguir existiendo tras fallo de Delete (reintentable)")
	}
}

// Best-effort: si Drive falla tras PG OK, el borrado PG no debe revertirse ni
// bloquearse (no queda fila huérfana visible; la basura Drive es tolerable).
func TestDeleteDriveFailureDoesNotBlockPGDelete(t *testing.T) {
	driveMock := drive.NewMockClient()
	noteStore := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	ctx := context.Background()
	author := uuid.NewString()
	svc := NewNoteService(noteStore, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	note, err := svc.Create(ctx, author, "Drive frágil", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// Simular 500 de Drive en borrado.
	driveMock.DeleteErr = &drive.DriveError{Code: 500, Message: "boom"}
	defer func() { driveMock.DeleteErr = nil }()
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatalf("fallo Drive tras PG OK no debe bloquear el borrado: %v", err)
	}
	if stored, _ := noteStore.GetByID(ctx, note.ID); stored != nil {
		t.Fatal("metadata PG debe estar borrada aunque Drive falle")
	}
}

// Idempotencia: segundo Delete tras éxito retorna not_found (sin panic ni 500).
func TestDeleteIdempotentSecondCallNotFound(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Idempotente", nil, "private", nil, "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatalf("primer delete failed: %v", err)
	}
	err = svc.Delete(ctx, author, note.ID)
	if err == nil {
		t.Fatal("segundo delete debe retornar not_found (idempotencia API)")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "not_found" {
		t.Fatalf("esperaba not_found, got %v", err)
	}
}

// Validación OAuth destino: clonador sin Drive conectado -> 403.
// Con patrón PENDING_DRIVE, la nota de clon se crea con sync_status='pending_drive'
// y luego se marca 'failed_sync' cuando Drive falla.
func TestCopyDstWithoutOAuthRejectedNoRecord(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen OAuth", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	beforeFiles := driveMock.FileCount()
	beforeNotes := countNotes(noteStore)
	driveMock.Disconnect(copier)
	defer driveMock.Reconnect(copier)
	_, err = svc.Copy(ctx, copier, note.ID, "")
	if err == nil {
		t.Fatal("esperaba 403 destino sin OAuth")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden, got %v", err)
	}
	if driveMock.FileCount() != beforeFiles {
		t.Fatalf("copy fallida no debe crear archivos, antes=%d ahora=%d", beforeFiles, driveMock.FileCount())
	}
	// La nota de clon existe pero con sync_status='failed_sync'
	if countNotes(noteStore) != beforeNotes+1 {
		t.Fatalf("copy fallida debe crear fila con failed_sync, antes=%d ahora=%d", beforeNotes, countNotes(noteStore))
	}
	// Verificar que la nota extra tiene failed_sync
	allNotes, _, _ := noteStore.ListByUser(ctx, copier, "", 10)
	found := false
	for _, n := range allNotes {
		if n.SyncStatus == "failed_sync" && n.Title == "Origen OAuth" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("nota de clon debe quedar en failed_sync para reconciliación")
	}
}

// Desacoplado intencional: autor sin OAuth NO bloquea el clon de un apunte
// público si el destino sí tiene Drive (validación de origen = legibilidad del
// archivo, no vigencia del token del autor).
func TestCopyDecoupledAuthorWithoutOAuthSucceeds(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, err := svc.Create(ctx, author, "Pública clonable", nil, "public", stringPtr("contenido público"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	driveMock.Disconnect(author)
	defer driveMock.Reconnect(author)
	cloned, err := svc.Copy(ctx, copier, note.ID, "")
	if err != nil {
		t.Fatalf("clon público con autor sin OAuth debe funcionar (desacoplado): %v", err)
	}
	if cloned.UserID != copier {
		t.Fatalf("autor del clon debe ser el copiador")
	}
	if cloned.ExternalFileID == nil || *cloned.ExternalFileID == *note.ExternalFileID {
		t.Fatal("clon debe tener drive_file_id nuevo e independiente")
	}
}

// Validación origen: fuente borrada en Drive -> 404/note_unavailable.
// Con patrón PENDING_DRIVE, la nota de clon se crea con sync_status='pending_drive'
// y luego se marca 'failed_sync' cuando Drive falla.
func TestCopySourceMissingNoRecordNoOrphan(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen frágil", nil, "public", stringPtr("x"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	// Borrar fuente en Drive (simula 404).
	_ = driveMock.DeleteFile(ctx, author, *note.ExternalFileID)
	beforeFiles := driveMock.FileCount()
	beforeNotes := countNotes(noteStore)
	_, err = svc.Copy(ctx, copier, note.ID, "")
	if err == nil {
		t.Fatal("esperaba error al copiar origen borrado en Drive")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("esperaba ServiceError got %T", err)
	}
	if se.Code != "not_found" && se.Code != "note_unavailable" {
		t.Fatalf("código esperado not_found/note_unavailable, got %q", se.Code)
	}
	if driveMock.FileCount() != beforeFiles {
		t.Fatalf("copy fallida no debe crear archivos, antes=%d ahora=%d", beforeFiles, driveMock.FileCount())
	}
	// La nota de clon existe pero con sync_status='failed_sync'
	if countNotes(noteStore) != beforeNotes+1 {
		t.Fatalf("copy fallida debe crear fila con failed_sync, antes=%d ahora=%d", beforeNotes, countNotes(noteStore))
	}
	// Verificar que la nota extra tiene failed_sync
	allNotes, _, _ := noteStore.ListByUser(ctx, copier, "", 10)
	found := false
	for _, n := range allNotes {
		if n.SyncStatus == "failed_sync" && n.Title == "Origen frágil" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("nota de clon debe quedar en failed_sync para reconciliación")
	}
}

// idemRecordStored lee el registro durable de idempotencia del store en
// memoria (helper de test).
func idemRecordStored(svc *NoteService, userID, operation, key string) (*model.IdempotencyKey, bool) {
	mem, ok := svc.notes.(*MemoryNoteStore)
	if !ok {
		return nil, false
	}
	mem.mu.RLock()
	defer mem.mu.RUnlock()
	rec, loaded := mem.idempotencyKeys[idemMemoryKey(userID, operation, key)]
	return cloneIdempotencyKey(rec), loaded
}

// seedIdemClaim inserta un claim durable en el store en memoria para simular
// estados de crash (lease vencido) o de operación en progreso sin PG.
func seedIdemClaim(svc *NoteService, userID, operation, key, requestHash, status string, expiresAt time.Time) {
	mem, ok := svc.notes.(*MemoryNoteStore)
	if !ok {
		panic("seedIdemClaim requiere un MemoryNoteStore")
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	if mem.idempotencyKeys == nil {
		mem.idempotencyKeys = make(map[string]*model.IdempotencyKey)
	}
	mem.idempotencyKeys[idemMemoryKey(userID, operation, key)] = &model.IdempotencyKey{
		UserID:         userID,
		Operation:      operation,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		Status:         status,
		ExpiresAt:      expiresAt,
	}
}

// seedIdemClaimWithResource inserta un claim durable ya preasociado a un
// resource_id (noteID) para simular un crash entre el claim y la
// materialización de la nota.
func seedIdemClaimWithResource(svc *NoteService, userID, operation, key, requestHash, status, resourceID string, expiresAt time.Time) {
	mem, ok := svc.notes.(*MemoryNoteStore)
	if !ok {
		panic("seedIdemClaimWithResource requiere un MemoryNoteStore")
	}
	mem.mu.Lock()
	defer mem.mu.Unlock()
	if mem.idempotencyKeys == nil {
		mem.idempotencyKeys = make(map[string]*model.IdempotencyKey)
	}
	rid := resourceID
	mem.idempotencyKeys[idemMemoryKey(userID, operation, key)] = &model.IdempotencyKey{
		UserID:         userID,
		Operation:      operation,
		IdempotencyKey: key,
		ResourceID:     &rid,
		RequestHash:    requestHash,
		Status:         status,
		ExpiresAt:      expiresAt,
	}
}

// --- Idempotencia durable sobre notes.idempotency_keys ---

// Un replay de Create con la misma clave y el mismo payload devuelve el
// recurso persistido sin duplicar filas PG ni archivos en Drive, y deja el
// registro durable 'completed' apuntando a la nota.
func TestIdempotencyCreateReplaySameHashReturnsResource(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-replay"
	first, err := svc.Create(ctx, userID, "Durable", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("primer create failed: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	second, err := svc.Create(ctx, userID, "Durable", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("replay debe responder desde la clave durable, got %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay debe devolver la misma nota: %s vs %s", second.ID, first.ID)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatalf("replay no debe duplicar efectos (notes %d->%d, files %d->%d)",
			beforeNotes, countNotes(noteStore), beforeFiles, driveMock.FileCount())
	}
	rec, loaded := idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil {
		t.Fatal("la clave durable debe quedar registrada")
	}
	if rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != first.ID {
		t.Fatalf("esperaba registro completed apuntando a %s, got %+v", first.ID, rec)
	}
}

// Misma clave de Create con un payload distinto => 409 Conflict sin efectos
// nuevos (no se crean filas ni archivos).
func TestIdempotencyCreateHashMismatchConflicts(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-conflict"
	if _, err := svc.Create(ctx, userID, "Original", nil, "private", stringPtr("v1"), key); err != nil {
		t.Fatalf("primer create failed: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	_, err := svc.Create(ctx, userID, "Distinta", nil, "private", stringPtr("v2"), key)
	if err == nil {
		t.Fatal("misma clave con payload distinto debe rechazarse")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" {
		t.Fatalf("esperaba conflict (409), got %v", err)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatal("el conflicto no debe crear filas ni archivos")
	}
}

// Un claim 'in_progress' vigente rechaza la solicitud concurrente con 409
// "solicitud en progreso" sin efectos; al liberarse la clave, el reintento se
// reprocesa con normalidad.
func TestIdempotencyInProgressClaimRejectsConcurrentCreate(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-inflight"
	seedIdemClaim(svc, userID, "create", key, "hash-en-vuelo", model.IdempotencyStatusInProgress, time.Now().Add(idemClaimTTL))
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	_, err := svc.Create(ctx, userID, "En vuelo", nil, "private", nil, key)
	if err == nil {
		t.Fatal("clave in-progress debe rechazar la solicitud concurrente")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" || se.Message != "solicitud en progreso" {
		t.Fatalf("esperaba conflict/solicitud en progreso, got %v", err)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatal("la solicitud rechazada no debe tener efectos")
	}
	// Al liberar el claim, la misma clave se puede reclamar.
	if err := svc.notes.DeleteIdempotencyKey(ctx, userID, "create", key); err != nil {
		t.Fatalf("liberar claim failed: %v", err)
	}
	if _, err := svc.Create(ctx, userID, "Tras liberar", nil, "private", nil, key); err != nil {
		t.Fatalf("tras liberar el claim la clave debe funcionar: %v", err)
	}
}

// Un claim 'in_progress' vencido (el proceso murió a mitad de la operación) se
// reclama en el reintento: Create procede y publica un replay fresco.
func TestIdempotencyExpiredClaimRecoveredOnCreate(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-expired"
	seedIdemClaim(svc, userID, "create", key, "hash-colgado", model.IdempotencyStatusInProgress, time.Now().Add(-time.Minute))
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	note, err := svc.Create(ctx, userID, "Recuperada", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("el claim vencido debe reclamarse y permitir reprocesar: %v", err)
	}
	if note == nil || note.ID == "" {
		t.Fatal("el reintento debe crear la nota")
	}
	if countNotes(noteStore) != beforeNotes+1 || driveMock.FileCount() != beforeFiles+1 {
		t.Fatalf("debe crear fila y archivo nuevos (notes %d->%d, files %d->%d)",
			beforeNotes, countNotes(noteStore), beforeFiles, driveMock.FileCount())
	}
	rec, loaded := idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != note.ID {
		t.Fatalf("el claim recuperado debe completarse con el nuevo recurso, got %+v", rec)
	}
}

// Copy: replay con la misma clave y el mismo origen devuelve el clon ya creado
// sin volver a copiar en Drive.
func TestIdempotencyCopyReplaySameSourceReturnsClone(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen durable", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "idem-copy-replay"
	first, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("primer copy failed: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	second, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("replay de copy debe responder desde la clave durable, got %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replay de copy debe devolver el mismo clon: %s vs %s", second.ID, first.ID)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatalf("replay de copy no debe duplicar efectos (notes %d->%d, files %d->%d)",
			beforeNotes, countNotes(noteStore), beforeFiles, driveMock.FileCount())
	}
}

// Copy: la misma clave con un origen distinto => 409 Conflict.
func TestIdempotencyCopyDifferentSourceConflicts(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	srcA, err := svc.Create(ctx, author, "Origen A", nil, "public", stringPtr("a"), "")
	if err != nil {
		t.Fatalf("create A failed: %v", err)
	}
	srcB, err := svc.Create(ctx, author, "Origen B", nil, "public", stringPtr("b"), "")
	if err != nil {
		t.Fatalf("create B failed: %v", err)
	}
	key := "idem-copy-conflict"
	if _, err := svc.Copy(ctx, copier, srcA.ID, key); err != nil {
		t.Fatalf("primer copy failed: %v", err)
	}
	_, err = svc.Copy(ctx, copier, srcB.ID, key)
	if err == nil {
		t.Fatal("misma clave con origen distinto debe rechazarse")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "conflict" {
		t.Fatalf("esperaba conflict (409), got %v", err)
	}
}

// Copy: un claim vencido por crash se recupera y el clon se reprocesa.
func TestIdempotencyExpiredClaimRecoveredOnCopy(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen crash", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "idem-copy-expired"
	seedIdemClaim(svc, copier, "copy", key, "hash-colgado", model.IdempotencyStatusInProgress, time.Now().Add(-time.Minute))
	beforeNotes := countNotes(noteStore)
	cloned, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("el claim vencido de copy debe reclamarse: %v", err)
	}
	if cloned == nil || cloned.ID == "" {
		t.Fatal("el reintento debe crear el clon")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear un clon nuevo tras recuperar el claim, antes=%d ahora=%d", beforeNotes, got)
	}
	rec, loaded := idemRecordStored(svc, copier, "copy", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != cloned.ID {
		t.Fatalf("el claim recuperado debe completarse con el clon, got %+v", rec)
	}
}

// Crash recovery lógico: si el proceso murió tras reclamar la clave y
// materializar la fila pending_drive, el reintento con la misma clave reutiliza
// el noteID preasociado (resource_id) y completa el alta en Drive sin duplicar
// ni la nota local ni el archivo remoto.
func TestIdempotencyCreateCrashRecoveryReusesPreassignedResourceID(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-crash"
	noteID := uuid.NewString()
	seedIdemClaimWithResource(svc, userID, "create", key, "hash-colgado", model.IdempotencyStatusInProgress, noteID, time.Now().Add(-time.Minute))
	if _, err := noteStore.Create(ctx, noteID, userID, nil, "Recuperada", nil, "private", nil, "pending_drive"); err != nil {
		t.Fatalf("seed nota pending_drive: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()

	note, err := svc.Create(ctx, userID, "Recuperada", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("el retry tras crash debe recuperar la nota preasociada: %v", err)
	}
	if note.ID != noteID {
		t.Fatalf("el retry debe reutilizar el noteID preasociado %s, got %s", noteID, note.ID)
	}
	if countNotes(noteStore) != beforeNotes {
		t.Fatalf("no debe duplicar la nota local: antes=%d ahora=%d", beforeNotes, countNotes(noteStore))
	}
	if driveMock.FileCount() != beforeFiles+1 {
		t.Fatalf("debe crear exactamente un archivo: antes=%d ahora=%d", beforeFiles, driveMock.FileCount())
	}
	stored, _ := noteStore.GetByID(ctx, noteID)
	if stored == nil || stored.SyncStatus != "synced" || stored.ExternalFileID == nil {
		t.Fatalf("la nota recuperada debe quedar synced con external_file_id: %+v", stored)
	}
	rec, loaded := idemRecordStored(svc, userID, "create", key)
	if !loaded || rec == nil || rec.Status != model.IdempotencyStatusCompleted || rec.ResourceID == nil || *rec.ResourceID != noteID {
		t.Fatalf("la clave debe completarse con el recurso preasociado, got %+v", rec)
	}
}

// Crash recovery lógico: si el crash ocurrió después del alta en Drive pero
// antes de persistir external_file_id, el reintento adopta el archivo huérfano
// (appProperties notes_note_id) en lugar de crear un duplicado remoto.
func TestIdempotencyCreateCrashRecoveryAdoptsOrphanFile(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "idem-create-crash-orphan"
	noteID := uuid.NewString()
	seedIdemClaimWithResource(svc, userID, "create", key, "hash-colgado", model.IdempotencyStatusInProgress, noteID, time.Now().Add(-time.Minute))
	if _, err := noteStore.Create(ctx, noteID, userID, nil, "Rescatada", nil, "private", nil, "pending_drive"); err != nil {
		t.Fatalf("seed nota pending_drive: %v", err)
	}
	orphan, err := driveMock.CreateFile(ctx, userID, noteID, "Rescatada.md", "contenido rescatado")
	if err != nil {
		t.Fatalf("seed archivo huérfano: %v", err)
	}
	beforeFiles := driveMock.FileCount()

	note, err := svc.Create(ctx, userID, "Rescatada", nil, "private", stringPtr("contenido rescatado"), key)
	if err != nil {
		t.Fatalf("el retry debe adoptar el huérfano: %v", err)
	}
	if note.ID != noteID || note.ExternalFileID == nil || *note.ExternalFileID != orphan {
		t.Fatalf("debe adoptar el archivo huérfano %s, got %+v", orphan, note)
	}
	if driveMock.FileCount() != beforeFiles {
		t.Fatalf("no debe crear archivos duplicados: antes=%d ahora=%d", beforeFiles, driveMock.FileCount())
	}
	if content, err := driveMock.GetFileContent(ctx, userID, orphan); err != nil || content != "contenido rescatado" {
		t.Fatalf("el contenido debe preservarse: %q %v", content, err)
	}
}

// Crash recovery lógico en Copy: el reintento reutiliza el clon preasociado y
// no vuelve a copiar el archivo en Drive.
func TestIdempotencyCopyCrashRecoveryReusesPreassignedClone(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen crash copy", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "idem-copy-crash"
	cloneID := uuid.NewString()
	seedIdemClaimWithResource(svc, copier, "copy", key, "hash-colgado", model.IdempotencyStatusInProgress, cloneID, time.Now().Add(-time.Minute))
	if _, err := noteStore.Create(ctx, cloneID, copier, nil, src.Title, nil, "private", &src.ID, "pending_drive"); err != nil {
		t.Fatalf("seed clon pending_drive: %v", err)
	}
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()

	cloned, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("el retry de copy tras crash debe recuperar el clon: %v", err)
	}
	if cloned.ID != cloneID {
		t.Fatalf("debe reutilizar el clon preasociado %s, got %s", cloneID, cloned.ID)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles+1 {
		t.Fatalf("sin duplicados locales y un único archivo remoto (notes %d->%d, files %d->%d)",
			beforeNotes, countNotes(noteStore), beforeFiles, driveMock.FileCount())
	}
	if cloned.ExternalFileID == nil || *cloned.ExternalFileID == *src.ExternalFileID {
		t.Fatal("el clon recuperado debe tener un archivo remoto nuevo")
	}
}

// El claim preasocia resource_id cuando es nuevo y PRESERVA el id del intento
// anterior al reclamar un lease vencido (paridad PG/memoria).
func TestIdempotencyPreassignedResourceIDClaimAndReclaim(t *testing.T) {
	store := NewMemoryNoteStore()
	ctx := context.Background()
	userID := uuid.NewString()
	first := uuid.NewString()
	rec, claimed, err := store.GetOrClaimIdempotencyKey(ctx, userID, "create", "preassigned-key", "hash-a", &first, time.Now().Add(-time.Minute))
	if err != nil || !claimed || rec == nil || rec.ResourceID == nil || *rec.ResourceID != first {
		t.Fatalf("claim nuevo debe preasociar resource_id: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
	second := uuid.NewString()
	rec, claimed, err = store.GetOrClaimIdempotencyKey(ctx, userID, "create", "preassigned-key", "hash-b", &second, time.Now().Add(idemClaimTTL))
	if err != nil || !claimed || rec == nil || rec.ResourceID == nil || *rec.ResourceID != first {
		t.Fatalf("el reclamo vencido debe preservar el resource_id previo: rec=%+v claimed=%v err=%v", rec, claimed, err)
	}
}

// sanitizeMarkdown debe neutralizar <script>, <iframe>, eventos onload/onerror
// (case-insensitive y con atributos/espacios) y links javascript:.
func TestSanitizeMarkdownNeutralizesXSSVectors(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		forbid  []string
		contain []string
	}{
		{
			name:    "script tag simple",
			input:   "antes <script>alert(1)</script> despues",
			forbid:  []string{"<script", "</script", "script>"},
			contain: []string{"&lt;script&gt;", "antes ", " despues"},
		},
		{
			name:    "script con atributos y mayusculas",
			input:   "x<SCRIPT src=\"http://evil\">y</SCRIPT>z",
			forbid:  []string{"<SCRIPT", "<script"},
			contain: []string{"&lt;script&gt;"},
		},
		{
			name:    "iframe",
			input:   "a<iframe src=\"http://evil\"></iframe>b",
			forbid:  []string{"<iframe", "</iframe", "iframe>"},
			contain: []string{"&lt;iframe&gt;"},
		},
		{
			name:    "eventos onload onerror",
			input:   "<img src=x onerror=alert(1)> body onload = alert(2)",
			forbid:  []string{"onerror=", "onload =", "onload="},
			contain: []string{"blocked="},
		},
		{
			name:    "javascript scheme",
			input:   "[click](JaVaScRiPt:alert(1)) javascript:alert(2)",
			forbid:  []string{"javascript:", "JaVaScRiPt:", "JAVASCRIPT:"},
			contain: []string{"blocked:"},
		},
		{
			name:    "markdown normal intacto",
			input:   "# Titulo\n\n**negrita** y [link](https://example.com)",
			forbid:  []string{"&lt;", "blocked"},
			contain: []string{"# Titulo", "**negrita**", "[link](https://example.com)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeMarkdown(tc.input)
			for _, f := range tc.forbid {
				if strings.Contains(got, f) {
					t.Fatalf("sanitizeMarkdown(%q) = %q no debe contener %q", tc.input, got, f)
				}
			}
			for _, c := range tc.contain {
				if !strings.Contains(got, c) {
					t.Fatalf("sanitizeMarkdown(%q) = %q debe contener %q", tc.input, got, c)
				}
			}
		})
	}
}

// El contenido persistido en Create/Update debe salir sanitizado del servicio.
func TestCreateAndUpdateSanitizeContent(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	note, err := svc.Create(ctx, userID, "XSS", nil, "private", stringPtr("<script>alert(1)</script><iframe></iframe>onerror=1"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	got, err := driveMock.GetFileContent(ctx, userID, *note.ExternalFileID)
	if err != nil {
		t.Fatalf("lectura Drive failed: %v", err)
	}
	if strings.Contains(got, "<script") || strings.Contains(got, "<iframe") || strings.Contains(got, "onerror=") {
		t.Fatalf("el contenido persistido debe estar sanitizado, got %q", got)
	}
	newContent := "javascript:alert(1) <SCRIPT>x</SCRIPT>"
	if _, err := svc.Update(ctx, userID, note.ID, nil, nil, &newContent, ""); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	got, err = driveMock.GetFileContent(ctx, userID, *note.ExternalFileID)
	if err != nil {
		t.Fatalf("lectura Drive post-update failed: %v", err)
	}
	if strings.Contains(got, "javascript:") || strings.Contains(got, "<SCRIPT") {
		t.Fatalf("el contenido actualizado debe estar sanitizado, got %q", got)
	}
}

// deadlineDriveClient registra si cada llamada crítica a Drive recibió un
// contexto con deadline (context.WithTimeout).
type deadlineDriveClient struct {
	*drive.MockClient
	mu       sync.Mutex
	calls    map[string]bool // nombre del método -> tiene deadline
	missing  []string
	recorded int
}

func newDeadlineDriveClient() *deadlineDriveClient {
	return &deadlineDriveClient{MockClient: drive.NewMockClient(), calls: make(map[string]bool)}
}

func (d *deadlineDriveClient) record(name string, ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := ctx.Deadline()
	d.calls[name] = ok
	d.recorded++
	if !ok {
		d.missing = append(d.missing, name)
	}
}

func (d *deadlineDriveClient) CreateFile(ctx context.Context, userID string, noteID string, title string, content string) (string, error) {
	d.record("CreateFile", ctx)
	return d.MockClient.CreateFile(ctx, userID, noteID, title, content)
}
func (d *deadlineDriveClient) GetFileContent(ctx context.Context, userID string, driveFileID string) (string, error) {
	d.record("GetFileContent", ctx)
	return d.MockClient.GetFileContent(ctx, userID, driveFileID)
}
func (d *deadlineDriveClient) UpdateFile(ctx context.Context, userID string, driveFileID string, newContent *string, newTitle *string) error {
	d.record("UpdateFile", ctx)
	return d.MockClient.UpdateFile(ctx, userID, driveFileID, newContent, newTitle)
}
func (d *deadlineDriveClient) DeleteFile(ctx context.Context, userID string, driveFileID string) error {
	d.record("DeleteFile", ctx)
	return d.MockClient.DeleteFile(ctx, userID, driveFileID)
}
func (d *deadlineDriveClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newNoteID string, newTitle string) (string, error) {
	d.record("CopyFile", ctx)
	return d.MockClient.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newNoteID, newTitle)
}

// Todas las llamadas críticas a Drive (CreateFile, GetFileContent, UpdateFile,
// DeleteFile, CopyFile) deben viajar con un contexto acotado por timeout para
// prevenir cuelgues indefinidos del upstream.
func TestDriveCallsUseBoundedContext(t *testing.T) {
	driveMock := newDeadlineDriveClient()
	noteStore := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	svc := NewNoteService(noteStore, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()

	note, err := svc.Create(ctx, author, "Timeout", nil, "public", stringPtr("v1"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if _, err := svc.Get(ctx, author, note.ID); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	newContent := "v2"
	if _, err := svc.Update(ctx, author, note.ID, nil, nil, &newContent, ""); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if _, err := svc.Copy(ctx, copier, note.ID, ""); err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	// Delete fuerza la llamada DeleteFile (best-effort) del archivo copiado.
	allNotes, _, _ := noteStore.ListByUser(ctx, copier, "", 10)
	if len(allNotes) == 0 {
		t.Fatal("el clon debe existir")
	}
	if err := svc.Delete(ctx, copier, allNotes[0].ID); err != nil {
		t.Fatalf("delete clon failed: %v", err)
	}

	driveMock.mu.Lock()
	defer driveMock.mu.Unlock()
	for _, name := range []string{"CreateFile", "GetFileContent", "UpdateFile", "DeleteFile", "CopyFile"} {
		hasDeadline, called := driveMock.calls[name]
		if !called {
			t.Fatalf("la llamada crítica %s no fue ejercitada por el test", name)
		}
		if !hasDeadline {
			t.Fatalf("la llamada crítica %s debe usar context.WithTimeout (sin deadline)", name)
		}
	}
	if len(driveMock.missing) != 0 {
		t.Fatalf("llamadas a Drive sin contexto acotado: %v", driveMock.missing)
	}
}
