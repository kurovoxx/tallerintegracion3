package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

// --- Fix 1: Update con snapshot previo obligatorio y marcado failed_sync ---

// Si el snapshot previo falla, Update debe abortar con ErrDriveUnavailable sin
// mutar Drive ni PG (no se permiten actualizaciones no reversibles).
func TestUpdateSnapshotFailureAbortsWithoutMutation(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Snapshot", nil, "private", stringPtr("contenido previo"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID
	driveMock.InjectGetError(fileID, &drive.DriveError{Code: 500, Message: "boom"})
	driveMock.UpdateErr = errors.New("UpdateFile no debe invocarse sin snapshot")
	defer func() {
		driveMock.ClearGetError(fileID)
		driveMock.UpdateErr = nil
	}()

	newBody := "contenido nuevo"
	_, err = svc.Update(ctx, author, note.ID, stringPtr("Título nuevo"), nil, &newBody, "")
	if err == nil {
		t.Fatal("esperaba abort por snapshot fallido")
	}
	if !errors.Is(err, ErrDriveUnavailable) {
		t.Fatalf("esperaba ErrDriveUnavailable, got %v", err)
	}
	// Drive intacto.
	driveMock.ClearGetError(fileID)
	got, err := driveMock.GetFileContent(ctx, author, fileID)
	if err != nil {
		t.Fatalf("lectura Drive post-abort falló: %v", err)
	}
	if got != "contenido previo" {
		t.Fatalf("Drive no debe mutar sin snapshot, got %q", got)
	}
	// PG intacto.
	stored, _ := noteStore.GetByID(ctx, note.ID)
	if stored.Title != "Snapshot" {
		t.Fatalf("PG no debe mutar sin snapshot, got title %q", stored.Title)
	}
	if stored.SyncStatus != "synced" {
		t.Fatalf("sin mutación en Drive no debe marcarse failed_sync, got %q", stored.SyncStatus)
	}
}

// Si PG falla tras el UpdateFile exitoso, la fila debe quedar failed_sync
// además de compensarse el contenido previo en Drive.
func TestUpdatePGFailureMarksFailedSync(t *testing.T) {
	driveMock := drive.NewMockClient()
	base := NewMemoryNoteStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(base)
	ctx := context.Background()
	author := uuid.NewString()
	svcOK := NewNoteService(base, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	note, err := svcOK.Create(ctx, author, "Original", nil, "private", stringPtr("v1"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID

	svcFail := NewNoteService(&failingUpdateStore{base}, NewMemoryAttachmentStore(), NewMemorySavedStore(), likeStore, NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	newBody := "v2"
	_, err = svcFail.Update(ctx, author, note.ID, nil, nil, &newBody, "")
	if !errors.Is(err, ErrInternalDatabase) {
		t.Fatalf("esperaba ErrInternalDatabase, got %v", err)
	}
	stored, _ := base.GetByID(ctx, note.ID)
	if stored.SyncStatus != "failed_sync" {
		t.Fatalf("PG debe quedar marcado failed_sync tras fallo de Update, got %q", stored.SyncStatus)
	}
	got, err := driveMock.GetFileContent(ctx, author, fileID)
	if err != nil {
		t.Fatalf("lectura Drive post-rollback falló: %v", err)
	}
	if got != "v1" {
		t.Fatalf("Drive debe revertirse a v1, got %q", got)
	}
}

// --- Fix 2: ReconcilePendingNotes con filtro anti-race de 15 minutos + mutex ---

func TestReconcilePendingNotesSkipsFreshPendingAndFailedSync(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()

	// pending_drive reciente: podría ser un Create en vuelo, no debe compensarse.
	fileFresh, _ := driveMock.CreateFile(ctx, userID, "", "fresh.md", "x")
	fresh, err := noteStore.Create(ctx, "", userID, nil, "fresh", &fileFresh, "private", nil, "pending_drive")
	if err != nil {
		t.Fatalf("seed fresh: %v", err)
	}
	// failed_sync antiguo: fuera del alcance del reconciliador de pending.
	fileFailed, _ := driveMock.CreateFile(ctx, userID, "", "failed.md", "x")
	failed, err := noteStore.Create(ctx, "", userID, nil, "failed", &fileFailed, "private", nil, "failed_sync")
	if err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	// pending_drive a medio camino (12 min): supera el TTL in-flight (2 min) y
	// el TTL de caché (10 min) pero NO el umbral de reconciliación de 15 min;
	// NO debe compensarse (margen anti-race del reconcilePendingMinAge).
	mid := time.Now().UTC().Add(-12 * time.Minute)
	fileMid, _ := driveMock.CreateFile(ctx, userID, "", "mid.md", "x")
	midNote, err := noteStore.Create(ctx, "", userID, nil, "mid", &fileMid, "private", nil, "pending_drive")
	if err != nil {
		t.Fatalf("seed mid: %v", err)
	}
	old := time.Now().UTC().Add(-20 * time.Minute)
	noteStore.mu.Lock()
	noteStore.notes[failed.ID].CreatedAt = old
	noteStore.notes[failed.ID].UpdatedAt = old
	noteStore.notes[midNote.ID].CreatedAt = mid
	noteStore.notes[midNote.ID].UpdatedAt = mid
	noteStore.mu.Unlock()

	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if n, _ := noteStore.GetByID(ctx, fresh.ID); n == nil {
		t.Fatal("pending_drive reciente no debe compensarse (colisión con Create en vuelo)")
	}
	if !driveMock.HasFile(fileFresh) {
		t.Fatal("archivo de pending reciente no debe borrarse")
	}
	if n, _ := noteStore.GetByID(ctx, midNote.ID); n == nil {
		t.Fatal("pending_drive de 12 min no debe compensarse: aún no supera el umbral de 15 min")
	}
	if !driveMock.HasFile(fileMid) {
		t.Fatal("archivo del pending de 12 min no debe borrarse")
	}
	if n, _ := noteStore.GetByID(ctx, failed.ID); n == nil {
		t.Fatal("failed_sync no debe ser tocado por ReconcilePendingNotes")
	}
	if !driveMock.HasFile(fileFailed) {
		t.Fatal("archivo de failed_sync no debe borrarse aquí")
	}
}

func TestReconcilePendingNotesCompensatesStalePending(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()

	fileID, _ := driveMock.CreateFile(ctx, userID, "", "stale.md", "x")
	n, err := noteStore.Create(ctx, "", userID, nil, "stale", &fileID, "private", nil, "pending_drive")
	if err != nil {
		t.Fatalf("seed stale: %v", err)
	}
	// El archivo no es legible en Drive (404): sin verificación previa exitosa
	// la nota se compensa (anti-eliminación agresiva no aplica porque no hay
	// contenido que preservar).
	driveMock.InjectGetError(fileID, &drive.DriveError{Code: 404, Message: "no encontrado"})
	defer driveMock.ClearGetError(fileID)
	// > reconcilePendingMinAge (15 min): solo así se compensa.
	old := time.Now().UTC().Add(-20 * time.Minute)
	noteStore.mu.Lock()
	noteStore.notes[n.ID].CreatedAt = old
	noteStore.notes[n.ID].UpdatedAt = old
	noteStore.mu.Unlock()

	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if got, _ := noteStore.GetByID(ctx, n.ID); got != nil {
		t.Fatal("pending_drive antiguo debe compensarse (fila PG borrada)")
	}
	if driveMock.HasFile(fileID) {
		t.Fatal("huérfano de Drive debe eliminarse al compensar")
	}
}

// --- Fix 3: zero-knowledge (404 en vez de 403) ---

// Privada sin acceso e inexistente deben ser indistinguibles a nivel de error
// canónico (mismo code y message) para todos los endpoints consultados.
func TestZeroKnowledgePrivateAndMissingIndistinguishable(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	stranger := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	priv, err := svc.Create(ctx, author, "Privada ZK", nil, "private", stringPtr("secreto"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	missing := uuid.NewString()

	checks := []struct {
		name string
		run  func() error
	}{
		{"get privada", func() error { _, err := svc.Get(ctx, stranger, priv.ID); return err }},
		{"get inexistente", func() error { _, err := svc.Get(ctx, stranger, missing); return err }},
		{"access privada", func() error { _, err := svc.GetAccess(ctx, stranger, priv.ID); return err }},
		{"access inexistente", func() error { _, err := svc.GetAccess(ctx, stranger, missing); return err }},
		{"copy privada", func() error { _, err := svc.Copy(ctx, stranger, priv.ID, ""); return err }},
		{"copy inexistente", func() error { _, err := svc.Copy(ctx, stranger, missing, ""); return err }},
		{"save privada", func() error { _, err := svc.Save(ctx, stranger, priv.ID); return err }},
		{"save inexistente", func() error { _, err := svc.Save(ctx, stranger, missing); return err }},
	}
	const want = "not_found:nota no encontrada"
	for _, tc := range checks {
		err := tc.run()
		se, ok := err.(*ServiceError)
		if !ok {
			t.Fatalf("%s: esperaba ServiceError, got %T %v", tc.name, err, err)
		}
		if got := se.Code + ":" + se.Message; got != want {
			t.Fatalf("%s: esperaba %q, got %q (oráculo de enumeración)", tc.name, want, got)
		}
	}
}

// --- Fix 4: zero-knowledge del origen vs OAuth del destino en Copy ---

func TestCopySourceAccessDeniedDistinctFromDstOAuth(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, err := svc.Create(ctx, author, "Origen restringido", nil, "public", stringPtr("data"), "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	fileID := *note.ExternalFileID

	// (a) El origen no es legible (403 de Drive): zero-knowledge absoluto,
	// idéntico a notFoundNote (404 not_found "nota no encontrada"), sin
	// mensajes que revelen el origen ni el estado del recurso.
	driveMock.InjectGetError(fileID, &drive.DriveError{Code: 403, Message: "permiso revocado"})
	_, err = svc.Copy(ctx, copier, note.ID, "")
	driveMock.ClearGetError(fileID)
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "not_found" || se.Message != "nota no encontrada" {
		t.Fatalf("esperaba not_found canónico zero-knowledge, got %v", err)
	}
	if strings.Contains(err.Error(), "origen") || strings.Contains(err.Error(), "Conecte o renueve") || strings.Contains(err.Error(), "note_unavailable") {
		t.Fatalf("el fallo de origen no debe filtrarse en el error: %v", err)
	}

	// (b) Cuenta del clonador sin OAuth: 403 forbidden con mensaje de
	// reconexión, distinto del fallo de origen.
	driveMock.Disconnect(copier)
	defer driveMock.Reconnect(copier)
	_, err = svc.Copy(ctx, copier, note.ID, "")
	se, ok = err.(*ServiceError)
	if !ok || se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden por OAuth del clonador, got %v", err)
	}
	if !strings.Contains(se.Message, "Google Drive") {
		t.Fatalf("mensaje debe pedir reconectar el Drive del clonador, got %q", se.Message)
	}
	if errors.Is(err, ErrSourceAccessDenied) {
		t.Fatalf("OAuth del clonador no debe clasificarse como acceso al origen")
	}
}
