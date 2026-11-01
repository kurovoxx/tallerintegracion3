package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

// Reconciliación Drive→App: solo el missing CONFIRMADO (404 o papelera)
// elimina metadata reutilizando el flujo normal de borrado. Los errores
// temporales (timeout, OAuth, 403, 5xx) no borran nada.
func TestReconcileKeepsExistingNote(t *testing.T) {
	svc, _, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, " intacta", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 0 || sum.RemovedAttachments != 0 || sum.Pending != 0 {
		t.Fatalf("nada que reconciliar: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("la nota existente debe permanecer")
	}
}

func TestReconcileRemovesNoteWithMissingMd(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "borrada en Drive", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mdID := ""
	if note.ExternalFileID != nil {
		mdID = *note.ExternalFileID
	}
	if mdID == "" {
		t.Fatal("sin external_file_id no hay caso")
	}
	// Borrado externo: el .md ya no existe en Drive (404 confirmado).
	if err := mock.DeleteFile(ctx, owner, mdID); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 1 || len(sum.RemovedNoteIDs) != 1 || sum.RemovedNoteIDs[0] != note.ID {
		t.Fatalf("resumen inesperado: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got != nil {
		t.Fatal("la metadata debe eliminarse")
	}
}

func TestReconcileMissingMdCleansAttachmentsViaNormalCleanup(t *testing.T) {
	svc, mock, notes, attachments, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "con adjuntos", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	att1, err := svc.AddAttachment(ctx, owner, note.ID, "a.png", "image/png", []byte("img"), true)
	if err != nil {
		t.Fatal(err)
	}
	att2, err := svc.AddAttachment(ctx, owner, note.ID, "b.pdf", "application/pdf", []byte("pdf"), false)
	if err != nil {
		t.Fatal(err)
	}
	mdID := *note.ExternalFileID
	if err := mock.DeleteFile(ctx, owner, mdID); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 1 {
		t.Fatalf("resumen inesperado: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got != nil {
		t.Fatal("nota debe eliminarse")
	}
	if got, _ := attachments.GetByID(ctx, att1.ID); got != nil {
		t.Fatal("adjunto 1 debe eliminarse")
	}
	if got, _ := attachments.GetByID(ctx, att2.ID); got != nil {
		t.Fatal("adjunto 2 debe eliminarse")
	}
	// Limpieza inmediata best-effort: los binarios restantes de Drive salen.
	if mock.HasFile(att1.ExternalFileID) || mock.HasFile(att2.ExternalFileID) {
		t.Fatal("los adjuntos Drive deben limpiarse con el borrado normal")
	}
	// Outbox durable coherente: delete_file + 2 delete_attachment.
	jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
	if err != nil || len(jobs) != 3 {
		t.Fatalf("outbox incoherente: %v %v", jobs, err)
	}
}

func TestReconcileTrashedCountsAsMissing(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "papelera", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mdID := *note.ExternalFileID
	// Mover a papelera en Drive (borrado desde la UI, no definitivo):
	// para la app cuenta como eliminado.
	mock.Trash(mdID)
	if gone, err := mock.FileGone(ctx, owner, mdID); err != nil || !gone {
		t.Fatalf("papelera debe ser missing confirmado: %v %v", gone, err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 1 {
		t.Fatalf("resumen inesperado: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got != nil {
		t.Fatal("la nota en papelera debe reconciliarse")
	}
}

func TestReconcileDrive404CountsAsMissing(t *testing.T) {
	mock := drive.NewMockClient()
	gone, err := mock.FileGone(context.Background(), uuid.NewString(), "inexistente")
	if err != nil || !gone {
		t.Fatalf("404 debe ser missing confirmado: %v %v", gone, err)
	}
}

func TestReconcileTimeoutNeverDeletes(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "timeout", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mdID := *note.ExternalFileID
	// El archivo sale del snapshot (papelera simulada) pero la confirmación
	// falla con timeout: debe contarse pendiente, jamás eliminarse.
	mock.Trash(mdID)
	mock.InjectGetError(mdID, context.DeadlineExceeded)
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 0 || sum.Pending != 1 {
		t.Fatalf("timeout debe ser pendiente sin borrar: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("timeout no debe borrar la nota")
	}
	mock.ClearGetError(mdID)
	mock.Restore(mdID)
}

func TestReconcileRevokedOAuthNeverDeletes(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "sin oauth", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mock.Disconnect(owner)
	_, err = svc.ReconcileDriveDeletions(ctx, owner)
	if !drive.IsOAuthError(err) {
		t.Fatalf("OAuth revocado debe abortar con OAuthError: %v", err)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("OAuth revocado no debe borrar la nota")
	}
	mock.Reconnect(owner)
}

func TestReconcileForbiddenNeverDeletes(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "forbidden", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mdID := *note.ExternalFileID
	mock.Trash(mdID)
	mock.InjectGetError(mdID, &drive.DriveError{Code: 403, Message: "x"})
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 0 || sum.Pending != 1 {
		t.Fatalf("403 debe ser pendiente sin borrar: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("403 no debe borrar la nota")
	}
	mock.ClearGetError(mdID)
	mock.Restore(mdID)
}

func TestReconcileListFailureAbortsWithoutDeleting(t *testing.T) {
	svc, mock, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "lista rota", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	mock.ListErr = &drive.DriveError{Code: 500, Message: "boom"}
	_, err = svc.ReconcileDriveDeletions(ctx, owner)
	if err == nil {
		t.Fatal("fallo del listado debe abortar con error")
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("fallo del listado no debe borrar la nota")
	}
	mock.ListErr = nil
}

func TestReconcileRemovesOnlyMissingAttachment(t *testing.T) {
	svc, mock, notes, attachments, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "parcial", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	att1, err := svc.AddAttachment(ctx, owner, note.ID, "a.png", "image/png", []byte("img"), true)
	if err != nil {
		t.Fatal(err)
	}
	att2, err := svc.AddAttachment(ctx, owner, note.ID, "b.pdf", "application/pdf", []byte("pdf"), false)
	if err != nil {
		t.Fatal(err)
	}
	// Borrado externo solo de att1.
	if err := mock.DeleteAttachment(ctx, owner, att1.ExternalFileID); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedNotes != 0 || sum.RemovedAttachments != 1 {
		t.Fatalf("resumen inesperado: %+v", sum)
	}
	if got, _ := notes.GetByID(ctx, note.ID); got == nil {
		t.Fatal("la nota debe permanecer")
	}
	if got, _ := attachments.GetByID(ctx, att1.ID); got != nil {
		t.Fatal("att1 debe eliminarse")
	}
	if got, _ := attachments.GetByID(ctx, att2.ID); got == nil {
		t.Fatal("att2 debe permanecer")
	}
	if !mock.HasFile(att2.ExternalFileID) {
		t.Fatal("el binario de att2 debe permanecer en Drive")
	}
	jobs, err := notes.ClaimDriveOperations(ctx, 10, time.Minute)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("outbox debe tener el delete_attachment: %v %v", jobs, err)
	}
}

func TestReconcileCleansInlineMarkdownOfMissingAttachment(t *testing.T) {
	svc, mock, _, attachments, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	note, err := svc.Create(ctx, owner, "inline", nil, "private", stringPtr("hola"), "")
	if err != nil {
		t.Fatal(err)
	}
	att1, err := svc.AddAttachment(ctx, owner, note.ID, "a.png", "image/png", []byte("img"), true)
	if err != nil {
		t.Fatal(err)
	}
	att2, err := svc.AddAttachment(ctx, owner, note.ID, "b.pdf", "application/pdf", []byte("pdf"), false)
	if err != nil {
		t.Fatal(err)
	}
	mdID := *note.ExternalFileID
	withRefs := "antes\n\n![a](attachment:" + att1.ID + ")\n\n[b](attachment:" + att2.ID + ")"
	if err := mock.UpdateFile(ctx, owner, mdID, &withRefs, nil); err != nil {
		t.Fatal(err)
	}
	if err := mock.DeleteAttachment(ctx, owner, att1.ExternalFileID); err != nil {
		t.Fatal(err)
	}
	sum, err := svc.ReconcileDriveDeletions(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if sum.RemovedAttachments != 1 {
		t.Fatalf("resumen inesperado: %+v", sum)
	}
	content, err := mock.GetFileContent(ctx, owner, mdID)
	if err != nil {
		t.Fatal(err)
	}
	if containsRef(content, att1.ID) {
		t.Fatalf("la referencia inline del eliminado debe limpiarse: %q", content)
	}
	if !containsRef(content, att2.ID) {
		t.Fatalf("la referencia del que permanece debe conservarse: %q", content)
	}
	if got, _ := attachments.GetByID(ctx, att2.ID); got == nil {
		t.Fatal("att2 debe permanecer")
	}
}

func containsRef(content, id string) bool {
	for i := 0; i+len(id) <= len(content); i++ {
		if content[i:i+len(id)] == id {
			return true
		}
	}
	return false
}

func TestStripAttachmentRefs(t *testing.T) {
	const id = "abcdefab-1234-4234-8234-123456789abc"
	in := "a\n\n![x](attachment:" + id + ")\n\n[y](attachment:" + id + ")\n\n[z](attachment:11111111-1111-1111-1111-111111111111)"
	out := stripAttachmentRefs(in, id)
	if containsRef(out, id) {
		t.Fatalf("refs no limpiadas: %q", out)
	}
	if !containsRef(out, "11111111-1111-1111-1111-111111111111") {
		t.Fatalf("otra ref no debe tocarse: %q", out)
	}
	if stripAttachmentRefs(in, "") != in {
		t.Fatal("id vacío no debe modificar")
	}
}
