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

func (f *failingCreateStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
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

func (f *failingUpdateStore) Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error) {
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

func (f *flakyCreateStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	if f.failOnce {
		f.failOnce = false
		return nil, fmt.Errorf("db insert failed (simulado)")
	}
	return f.MemoryNoteStore.Create(ctx, userID, subjectID, title, externalFileID, visibility, forkedFrom, syncStatus)
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

// idemEntryStored lee una entrada del caché unificado de idempotencia bajo el
// RWMutex (helper de test para el nuevo idemStore).
func idemEntryStored(svc *NoteService, key string) (*idemEntry, bool) {
	svc.idemMu.RLock()
	defer svc.idemMu.RUnlock()
	e, ok := svc.idemStore[key]
	return e, ok
}

// --- TTL del caché de idempotencia (10 minutos) ---

// Una entrada *idemEntry con más de 10 minutos se purga al consultarla y la
// misma clave vuelve a ejecutar Create (no queda memoria retenida ni replay
// obsoleto).
func TestIdemCacheTTLPurgesExpiredEntryOnCreate(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "ttl-create-1"
	first, err := svc.Create(ctx, userID, "TTL create", nil, "private", stringPtr("v1"), key)
	if err != nil {
		t.Fatalf("primer create failed: %v", err)
	}
	// Simular que la entrada cumplió el TTL (11 minutos > 10 minutos).
	svc.storeIdemEntry(idemKey("create", key), &idemEntry{status: idemSuccess, note: first, createdAt: time.Now().Add(-11 * time.Minute)})
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	second, err := svc.Create(ctx, userID, "TTL create", nil, "private", stringPtr("v2"), key)
	if err != nil {
		t.Fatalf("create tras expiración debe proceder: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("entrada expirada debe purgarse y permitir re-ejecución")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear una fila nueva tras expirar, antes=%d ahora=%d", beforeNotes, got)
	}
	if got := driveMock.FileCount(); got != beforeFiles+1 {
		t.Fatalf("debe crear un archivo nuevo tras expirar, antes=%d ahora=%d", beforeFiles, got)
	}
	// La respuesta nueva queda cacheada como idemEntry fresca...
	entry, loaded := idemEntryStored(svc, idemKey("create", key))
	if !loaded || entry == nil {
		t.Fatal("tras el éxito debe quedar una entrada vigente en el caché")
	}
	if entry.note == nil || entry.note.ID != second.ID {
		t.Fatal("la entrada debe apuntar a la nota recién creada")
	}
	if age := time.Since(entry.createdAt); age > time.Minute {
		t.Fatalf("createdAt debe ser fresco (TTL reiniciado), edad=%v", age)
	}
	// ...y el replay inmediato responde desde el caché sin duplicar efectos.
	third, err := svc.Create(ctx, userID, "TTL create", nil, "private", stringPtr("v3"), key)
	if err != nil {
		t.Fatalf("replay vigente failed: %v", err)
	}
	if third.ID != second.ID {
		t.Fatalf("replay vigente debe devolver la nota cacheada: %s vs %s", third.ID, second.ID)
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("replay no debe duplicar filas, got %d", got)
	}
}

// Misma semántica de TTL para Copy: la entrada vencida se purga y se ejecuta un
// clon nuevo, manteniendo el replay vigente posterior.
func TestIdemCacheTTLPurgesExpiredEntryOnCopy(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen TTL", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "ttl-copy-1"
	first, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("primer copy failed: %v", err)
	}
	svc.storeIdemEntry(idemKey("copy", key), &idemEntry{status: idemSuccess, note: first, createdAt: time.Now().Add(-11 * time.Minute)})
	beforeNotes := countNotes(noteStore)
	second, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("copy tras expiración debe proceder: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("entrada expirada debe purgarse y permitir un clon nuevo")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear un clon nuevo tras expirar, antes=%d ahora=%d", beforeNotes, got)
	}
	third, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("replay vigente de copy failed: %v", err)
	}
	if third.ID != second.ID {
		t.Fatalf("replay vigente debe devolver el clon cacheado: %s vs %s", third.ID, second.ID)
	}
}

// La entrada in-flight unificada (*idemEntry con status=idemInFlight) rechaza
// la solicitud concurrente y se conserva mientras está vigente; al liberarse,
// la clave vuelve a ser utilizable.
func TestIdemCacheInFlightEntryRejectsConcurrent(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "inflight-1"
	svc.storeIdemEntry(idemKey("create", key), &idemEntry{status: idemInFlight, createdAt: time.Now()})
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	_, err := svc.Create(ctx, userID, "En vuelo", nil, "private", nil, key)
	if err == nil {
		t.Fatal("clave in-flight debe rechazar la solicitud concurrente")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "bad_request" || se.Message != "solicitud en progreso" {
		t.Fatalf("esperaba bad_request/solicitud en progreso, got %v", err)
	}
	if countNotes(noteStore) != beforeNotes || driveMock.FileCount() != beforeFiles {
		t.Fatal("la solicitud rechazada por in-flight no debe tener efectos")
	}
	// La entrada in-flight vigente se conserva (el TTL de 2 minutos no vence).
	entry, loaded := idemEntryStored(svc, idemKey("create", key))
	if !loaded || entry == nil {
		t.Fatal("la entrada in-flight vigente debe conservarse")
	}
	if entry.status != idemInFlight {
		t.Fatalf("la entrada in-flight debe ser *idemEntry{status: idemInFlight}, got %#v", entry)
	}
	// Al liberar la clave, el reintento se reprocesa con normalidad.
	svc.deleteIdemEntry(idemKey("create", key))
	if _, err := svc.Create(ctx, userID, "Tras liberar", nil, "private", nil, key); err != nil {
		t.Fatalf("tras liberar el in-flight la clave debe funcionar: %v", err)
	}
}

// Un in-flight colgado más de 2 minutos (proceso muerto a mitad de operación)
// se purga al consultarlo y el reintento vuelve a ejecutar Create sin quedar
// bloqueado por "solicitud en progreso".
func TestIdemCacheStaleInFlightPurgedOnCreate(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "inflight-stale-1"
	svc.storeIdemEntry(idemKey("create", key), &idemEntry{status: idemInFlight, createdAt: time.Now().Add(-3 * time.Minute)})
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	note, err := svc.Create(ctx, userID, "Reprocesar", nil, "private", nil, key)
	if err != nil {
		t.Fatalf("el in-flight colgado debe purgarse y permitir reprocesar: %v", err)
	}
	if note == nil || note.ID == "" {
		t.Fatal("el reintento debe crear la nota")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear una fila nueva tras purgar el in-flight, antes=%d ahora=%d", beforeNotes, got)
	}
	if got := driveMock.FileCount(); got != beforeFiles+1 {
		t.Fatalf("debe crear un archivo nuevo tras purgar el in-flight, antes=%d ahora=%d", beforeFiles, got)
	}
	// El resultado exitoso queda cacheado como replay unificado.
	entry, loaded := idemEntryStored(svc, idemKey("create", key))
	if !loaded || entry == nil {
		t.Fatal("tras el éxito debe quedar una entrada vigente en el caché")
	}
	if entry.status != idemSuccess || entry.note == nil || entry.note.ID != note.ID {
		t.Fatalf("esperaba *idemEntry{status: idemSuccess} apuntando a la nota nueva, got %#v", entry)
	}
}

// Una entrada con estado desconocido (corrupta/legado) se purga al consultarse
// y la operación se reprocesa: con el mapa unificado tipado el único estado
// ilegítimo posible es un idemStatus fuera del contrato, y nunca se interpreta
// como in-flight válido ni bloquea la clave.
func TestIdemCacheUnknownStatusPurgedAndReprocessed(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	key := "legacy-1"
	svc.storeIdemEntry(idemKey("create", key), &idemEntry{status: idemStatus(99), createdAt: time.Now()})
	beforeNotes := countNotes(noteStore)
	beforeFiles := driveMock.FileCount()
	note, err := svc.Create(ctx, userID, "Legado", nil, "private", nil, key)
	if err != nil {
		t.Fatalf("un valor legado debe purgarse y permitir reprocesar: %v", err)
	}
	if note == nil || note.ID == "" {
		t.Fatal("el reintento debe crear la nota")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear una fila nueva, antes=%d ahora=%d", beforeNotes, got)
	}
	if got := driveMock.FileCount(); got != beforeFiles+1 {
		t.Fatalf("debe crear un archivo nuevo, antes=%d ahora=%d", beforeFiles, got)
	}
	if entry, loaded := idemEntryStored(svc, idemKey("create", key)); !loaded || entry == nil {
		t.Fatal("tras el éxito debe quedar la entrada unificada vigente")
	}
}

// El mismo contrato unificado aplica a Copy: un in-flight colgado (>2 min) se
// purga y el clon se reprocesa.
func TestIdemCacheStaleInFlightPurgedOnCopy(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	src, err := svc.Create(ctx, author, "Origen in-flight", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create origen failed: %v", err)
	}
	key := "inflight-stale-copy-1"
	svc.storeIdemEntry(idemKey("copy", key), &idemEntry{status: idemInFlight, createdAt: time.Now().Add(-3 * time.Minute)})
	beforeNotes := countNotes(noteStore)
	cloned, err := svc.Copy(ctx, copier, src.ID, key)
	if err != nil {
		t.Fatalf("el in-flight colgado de copy debe purgarse y permitir reprocesar: %v", err)
	}
	if cloned == nil || cloned.ID == "" {
		t.Fatal("el reintento debe crear el clon")
	}
	if got := countNotes(noteStore); got != beforeNotes+1 {
		t.Fatalf("debe crear un clon nuevo tras purgar el in-flight, antes=%d ahora=%d", beforeNotes, got)
	}
	entry, loaded := idemEntryStored(svc, idemKey("copy", key))
	if !loaded || entry == nil {
		t.Fatal("tras el éxito debe quedar la entrada unificada vigente")
	}
	if entry.status != idemSuccess || entry.note == nil || entry.note.ID != cloned.ID {
		t.Fatalf("esperaba *idemEntry{status: idemSuccess} apuntando al clon, got %#v", entry)
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

func (d *deadlineDriveClient) CreateFile(ctx context.Context, userID string, title string, content string) (string, error) {
	d.record("CreateFile", ctx)
	return d.MockClient.CreateFile(ctx, userID, title, content)
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
func (d *deadlineDriveClient) CopyFile(ctx context.Context, srcUserID string, srcFileID string, dstUserID string, newTitle string) (string, error) {
	d.record("CopyFile", ctx)
	return d.MockClient.CopyFile(ctx, srcUserID, srcFileID, dstUserID, newTitle)
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
