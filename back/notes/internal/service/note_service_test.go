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

// helper para crear servicio con stores en memoria
func newTestService() (*NoteService, *drive.MockClient, *MemoryNoteStore, *MemoryAttachmentStore, *MemorySavedStore, *MemoryLikeStore, *MemorySharedStore, *MemorySocialResolver) {
	driveMock := drive.NewMockClient()
	noteStore := NewMemoryNoteStore()
	attStore := NewMemoryAttachmentStore()
	savedStore := NewMemorySavedStore()
	likeStore := NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := NewMemorySharedStore()
	social := NewMemorySocialResolver()
	svc := NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	return svc, driveMock, noteStore, attStore, savedStore, likeStore, sharedStore, social
}

func TestCreateSuccess(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	note, err := svc.Create(ctx, userID, "Mi primer apunte", nil, "private", stringPtr("# Hola"), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if note.ID == "" {
		t.Fatal("note ID vacío")
	}
	if note.Title != "Mi primer apunte" {
		t.Fatalf("title mismatch got %q", note.Title)
	}
	if note.Visibility != "private" {
		t.Fatalf("visibility mismatch")
	}
	if note.ExternalFileID == nil || *note.ExternalFileID == "" {
		t.Fatal("external_file_id vacío")
	}
	// verificar que Drive tiene el archivo
	if !driveMock.HasFile(*note.ExternalFileID) {
		t.Fatal("Drive no contiene archivo creado")
	}
	content, _ := driveMock.GetFileContent(ctx, userID, *note.ExternalFileID)
	if !strings.Contains(content, "Hola") {
		t.Fatalf("contenido Drive no coincide: %q", content)
	}
}

func TestCreateValidations(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	tests := []struct {
		name       string
		title      string
		visibility string
		subjectID  *string
		wantCode   string
	}{
		{"titulo vacío", "", "private", nil, "invalid_title"},
		{"titulo espacios", "   ", "public", nil, "invalid_title"},
		{"visibilidad inválida", "ok", "invalida", nil, "invalid_visibility"},
		{"subjectID inválido", "ok", "private", stringPtr("no-uuid"), "invalid_subject_id"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, userID, tc.title, tc.subjectID, tc.visibility, nil, nil)
			if err == nil {
				t.Fatal("esperaba error validación")
			}
			se, ok := err.(*ServiceError)
			if !ok {
				t.Fatalf("esperaba ServiceError, got %T %v", err, err)
			}
			if se.Code != tc.wantCode {
				t.Fatalf("code esperado %q got %q", tc.wantCode, se.Code)
			}
		})
	}
}

func TestCreateRollbackOnDBFailure(t *testing.T) {
	// Simular fallo DB tras crear archivo Drive: debe borrar huérfano
	driveMock := drive.NewMockClient()
	// failing store que siempre falla Create
	failingNoteStore := &failingNoteStore{err: fmt.Errorf("db insert failed")}
	svc := NewNoteService(failingNoteStore, NewMemoryAttachmentStore(), NewMemorySavedStore(), NewMemoryLikeStore(), NewMemorySharedStore(), driveMock, NewMemorySocialResolver())
	ctx := context.Background()
	userID := uuid.NewString()
	_, err := svc.Create(ctx, userID, "Rollback test", nil, "private", nil, nil)
	if err == nil {
		t.Fatal("esperaba error DB")
	}
	// verificar que no quedó archivo huérfano en Drive (debería haber sido eliminado)
	if driveMock.FileCount() != 0 {
		t.Fatalf("rollback falló, drive aún tiene %d archivos", driveMock.FileCount())
	}
}

type failingNoteStore struct{ err error }

func (f *failingNoteStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string) (*model.Note, error) {
	return nil, f.err
}
func (f *failingNoteStore) GetByID(ctx context.Context, id string) (*model.Note, error) { return nil, nil }
func (f *failingNoteStore) ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error) {
	return nil, "", nil
}
func (f *failingNoteStore) Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error) {
	return nil, nil
}
func (f *failingNoteStore) Delete(ctx context.Context, id string) error { return nil }
func (f *failingNoteStore) IncrementLikes(ctx context.Context, noteID string, delta int) error {
	return nil
}
func (f *failingNoteStore) UpdateExternalFileID(ctx context.Context, noteID, fileID string) error { return nil }

func TestGetWithContent(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Get content", nil, "public", stringPtr("contenido markdown"), nil)
	got, err := svc.Get(ctx, author, note.ID)
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Content == nil || *got.Content != "contenido markdown" {
		t.Fatalf("contenido esperado 'contenido markdown', got %v", got.Content)
	}
}

func TestGetDriveNotFound(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Drive missing", nil, "public", stringPtr("x"), nil)
	// simular borrado manual en Drive
	driveMock.InjectGetError(*note.ExternalFileID, &drive.DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"})
	_, err := svc.Get(ctx, author, note.ID)
	if err == nil {
		t.Fatal("esperaba error 404 drive")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("esperaba ServiceError got %T", err)
	}
	if se.Code != "note_unavailable" {
		t.Fatalf("code esperado note_unavailable got %q", se.Code)
	}
	if !strings.Contains(se.Message, "Nota no disponible") {
		t.Fatalf("mensaje debe contener 'Nota no disponible', got %q", se.Message)
	}
	_ = noteStore
}

func TestGetDriveForbidden(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Drive forbidden", nil, "public", stringPtr("secret"), nil)
	driveMock.InjectGetError(*note.ExternalFileID, &drive.DriveError{Code: 403, Message: "Nota no disponible en almacenamiento remoto"})
	_, err := svc.Get(ctx, author, note.ID)
	if err == nil {
		t.Fatal("esperaba error 403 drive")
	}
	se := err.(*ServiceError)
	if se.Code != "note_unavailable" {
		t.Fatalf("esperaba note_unavailable, got %q", se.Code)
	}
}

func TestUpdateOnlyAuthor(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	other := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Original", nil, "private", stringPtr("old"), nil)
	// other intenta editar
	_, err := svc.Update(ctx, other, note.ID, stringPtr("hack"), nil, nil)
	if err == nil {
		t.Fatal("esperaba 403 forbidden")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden, got %q", se.Code)
	}
	// author edita título y contenido
	newContent := "new content"
	updated, err := svc.Update(ctx, author, note.ID, stringPtr("Nuevo título"), stringPtr("public"), &newContent)
	if err != nil {
		t.Fatalf("update author failed: %v", err)
	}
	if updated.Title != "Nuevo título" {
		t.Fatalf("title no actualizado")
	}
	if updated.Visibility != "public" {
		t.Fatalf("visibility no actualizado")
	}
	// verificar Drive sync
	got, _ := svc.Get(ctx, author, note.ID)
	if got.Content == nil || *got.Content != newContent {
		t.Fatalf("Drive contenido no sincronizado, got %v", got.Content)
	}
}

func TestDeleteOnlyAuthor(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	other := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Borrable", nil, "private", nil, nil)
	fileID := *note.ExternalFileID
	// other falla
	err := svc.Delete(ctx, other, note.ID)
	if err == nil {
		t.Fatal("esperaba forbidden")
	}
	// author borra
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatalf("delete author failed: %v", err)
	}
	if driveMock.HasFile(fileID) {
		t.Fatal("Drive archivo no eliminado")
	}
	// verificar borrado
	n, _ := svc.Get(ctx, author, note.ID)
	_ = n // Get debería fallar not_found ahora porque nota no existe, pero Get hace HasAnyShare check antes?
	// Mejor verificar direct store
	if _, err := svc.Get(ctx, author, note.ID); err == nil {
		t.Fatal("nota debería estar borrada")
	}
}

func TestListMyPagination(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	userID := uuid.NewString()
	for i := 0; i < 5; i++ {
		_, _ = svc.Create(ctx, userID, fmt.Sprintf("Apunte %d", i), nil, "private", nil, nil)
	}
	notes, next, err := svc.ListMy(ctx, userID, "", 2)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("esperaba 2, got %d", len(notes))
	}
	if next == "" {
		t.Fatal("next cursor vacío")
	}
	notes2, next2, _ := svc.ListMy(ctx, userID, next, 2)
	if len(notes2) != 2 {
		t.Fatalf("segunda página esperaba 2, got %d", len(notes2))
	}
	_ = next2
}

func TestAttachmentUploadAndDelete(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Con adjunto", nil, "private", nil, nil)
	data := []byte("fake image data")
	att, err := svc.AddAttachment(ctx, author, note.ID, "foto.png", "image/png", data, false)
	if err != nil {
		t.Fatalf("add attachment failed: %v", err)
	}
	if att.FileURL == "" {
		t.Fatal("file_url vacío")
	}
	if att.FileType != "image/png" {
		t.Fatalf("file_type mismatch")
	}
	list, _ := svc.ListAttachments(ctx, note.ID)
	if len(list) != 1 {
		t.Fatalf("esperaba 1 attachment, got %d", len(list))
	}
	// otro usuario no puede borrar
	other := uuid.NewString()
	err = svc.RemoveAttachment(ctx, other, note.ID, att.ID)
	if err == nil {
		t.Fatal("esperaba forbidden al borrar otro")
	}
	// autor borra
	if err := svc.RemoveAttachment(ctx, author, note.ID, att.ID); err != nil {
		t.Fatalf("delete attachment author failed: %v", err)
	}
	list2, _ := svc.ListAttachments(ctx, note.ID)
	if len(list2) != 0 {
		t.Fatalf("esperaba 0 tras borrar, got %d", len(list2))
	}
}

func TestAttachmentTooLarge(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Grande", nil, "private", nil, nil)
	big := make([]byte, 11*1024*1024)
	_, err := svc.AddAttachment(ctx, author, note.ID, "big.pdf", "application/pdf", big, false)
	if err == nil {
		t.Fatal("esperaba file_too_large")
	}
	se := err.(*ServiceError)
	if se.Code != "file_too_large" {
		t.Fatalf("esperaba file_too_large got %q", se.Code)
	}
}

func TestSaveAndAlreadySaved(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	saver := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Guardable", nil, "public", nil, nil)
	_, err := svc.Save(ctx, saver, note.ID)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	_, err = svc.Save(ctx, saver, note.ID)
	if err == nil {
		t.Fatal("esperaba already_saved 409")
	}
	se := err.(*ServiceError)
	if se.Code != "already_saved" {
		t.Fatalf("esperaba already_saved got %q", se.Code)
	}
}

func TestCopyClonesFile(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Original copy", nil, "public", stringPtr("contenido original"), nil)
	origFileID := *note.ExternalFileID
	newNote, err := svc.Copy(ctx, copier, note.ID)
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if newNote.UserID != copier {
		t.Fatalf("nuevo owner debe ser copier")
	}
	if newNote.ForkedFromNoteID == nil || *newNote.ForkedFromNoteID != note.ID {
		t.Fatalf("forked_from_note_id debe apuntar al original")
	}
	if newNote.ExternalFileID == nil || *newNote.ExternalFileID == origFileID {
		t.Fatal("nuevo fileID debe ser distinto")
	}
	if !driveMock.HasFile(*newNote.ExternalFileID) {
		t.Fatal("Drive no tiene archivo clonado")
	}
	content, _ := driveMock.GetFileContent(ctx, copier, *newNote.ExternalFileID)
	if content != "contenido original" {
		t.Fatalf("contenido clonado mismatch: %q", content)
	}
	// contenido original intacto
	origContent, _ := driveMock.GetFileContent(ctx, author, origFileID)
	if origContent != "contenido original" {
		t.Fatalf("original alterado")
	}
}

func TestCopyPrivateWithoutAccessFails(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Privada", nil, "private", nil, nil)
	_, err := svc.Copy(ctx, copier, note.ID)
	if err == nil {
		t.Fatal("esperaba forbidden copy privada")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
}

func TestLikeUnlikeTransactional(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	liker := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Likeable", nil, "public", nil, nil)
	if note.LikesCount != 0 {
		t.Fatalf("likes inicial 0")
	}
	if err := svc.Like(ctx, liker, note.ID); err != nil {
		t.Fatalf("like failed: %v", err)
	}
	n, _ := noteStore.GetByID(ctx, note.ID)
	if n.LikesCount != 1 {
		t.Fatalf("likes_count debe ser 1, got %d", n.LikesCount)
	}
	// duplicate like 409
	if err := svc.Like(ctx, liker, note.ID); err == nil {
		t.Fatal("esperaba already_liked")
	} else {
		se := err.(*ServiceError)
		if se.Code != "already_liked" {
			t.Fatalf("esperaba already_liked got %q", se.Code)
		}
	}
	// unlike decrements
	if err := svc.Unlike(ctx, liker, note.ID); err != nil {
		t.Fatalf("unlike failed: %v", err)
	}
	n2, _ := noteStore.GetByID(ctx, note.ID)
	if n2.LikesCount != 0 {
		t.Fatalf("likes_count debe volver a 0, got %d", n2.LikesCount)
	}
	// unlike idempotente si ya no likeado (no error, no decrementa)
	if err := svc.Unlike(ctx, liker, note.ID); err != nil {
		t.Fatalf("unlike idempotente no debe fallar: %v", err)
	}
	n3, _ := noteStore.GetByID(ctx, note.ID)
	if n3.LikesCount != 0 {
		t.Fatalf("likes_count no debe bajar de 0, got %d", n3.LikesCount)
	}
}

func TestShareAndAccessControl(t *testing.T) {
	svc, _, _, _, _, _, sharedStore, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	member := uuid.NewString()
	other := uuid.NewString()
	groupID := uuid.NewString()
	// author es miembro y admin del grupo
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	// crear nota privada
	note, _ := svc.Create(ctx, author, "Privada share", nil, "private", stringPtr("secreto"), nil)
	// member sin share no puede leer
	_, err := svc.Get(ctx, member, note.ID)
	if err == nil {
		t.Fatal("member sin share debería 403")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
	// author comparte con grupo link
	shared, err := svc.Share(ctx, author, note.ID, groupID, "link")
	if err != nil {
		t.Fatalf("share failed: %v", err)
	}
	if shared.AccessMode != "link" {
		t.Fatalf("access_mode mismatch")
	}
	if shared.IsAdminNote != true {
		t.Fatalf("is_admin_note debe ser true porque author es admin")
	}
	// ahora member puede leer
	got, err := svc.Get(ctx, member, note.ID)
	if err != nil {
		t.Fatalf("member con share debería leer: %v", err)
	}
	if got.Content == nil || *got.Content != "secreto" {
		t.Fatalf("contenido no coincide tras share")
	}
	// other fuera del grupo con link SÍ puede leer (link = cualquiera con enlace)
	gotOther, err := svc.Get(ctx, other, note.ID)
	if err != nil {
		t.Fatalf("other fuera grupo con link debería poder leer: %v", err)
	}
	if gotOther.Content == nil || *gotOther.Content != "secreto" {
		t.Fatalf("contenido other link no coincide")
	}
	// author puede revocar
	if err := svc.Unshare(ctx, author, shared.ID); err != nil {
		t.Fatalf("unshare failed: %v", err)
	}
	_, err = svc.Get(ctx, member, note.ID)
	if err == nil {
		t.Fatal("tras unshare member no debe leer")
	}
	_ = sharedStore
}

func TestShareForbiddenIfNotAuthor(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	other := uuid.NewString()
	groupID := uuid.NewString()
	social.AddMember(author, groupID)
	social.AddMember(other, groupID)
	note, _ := svc.Create(ctx, author, "No share ajeno", nil, "private", nil, nil)
	_, err := svc.Share(ctx, other, note.ID, groupID, "link")
	if err == nil {
		t.Fatal("other no autor no puede compartir")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
}

func TestShareForbiddenIfNotMember(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	groupID := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Sin membresía", nil, "private", nil, nil)
	_, err := svc.Share(ctx, author, note.ID, groupID, "link")
	if err == nil {
		t.Fatal("author no miembro no puede compartir")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
}

func TestShareInvalidAccessMode(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	groupID := uuid.NewString()
	social.AddMember(author, groupID)
	note, _ := svc.Create(ctx, author, "Invalid mode", nil, "private", nil, nil)
	_, err := svc.Share(ctx, author, note.ID, groupID, "invalid")
	if err == nil {
		t.Fatal("esperaba invalid_access_mode")
	}
	se := err.(*ServiceError)
	if se.Code != "invalid_access_mode" {
		t.Fatalf("esperaba invalid_access_mode got %q", se.Code)
	}
}

func TestUnshareAll(t *testing.T) {
	svc, _, noteStore, _, _, _, sharedStore, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	// crear dos notas y compartir ambas
	n1, _ := svc.Create(ctx, author, "N1", nil, "private", nil, nil)
	n2, _ := svc.Create(ctx, author, "N2", nil, "private", nil, nil)
	_, _ = svc.Share(ctx, author, n1.ID, groupID, "link")
	_, _ = svc.Share(ctx, author, n2.ID, groupID, "link")
	list, _ := sharedStore.ListByNote(ctx, n1.ID)
	if len(list) != 1 {
		t.Fatalf("esperaba 1 share n1")
	}
	// unshare-all
	if err := svc.UnshareAll(ctx, author, groupID); err != nil {
		t.Fatalf("unshare-all failed: %v", err)
	}
	// verificar que ambos shares borrados (aunque memory DeleteByUserAndGroup borra todos del grupo)
	has1, _ := sharedStore.HasAnyShare(ctx, n1.ID)
	has2, _ := sharedStore.HasAnyShare(ctx, n2.ID)
	if has1 || has2 {
		t.Fatalf("shares deberían estar borrados, has1=%v has2=%v", has1, has2)
	}
	_ = noteStore
}

func TestGetAccess(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	note, _ := svc.Create(ctx, author, "Access test", nil, "private", nil, nil)
	// author can_read
	access, err := svc.GetAccess(ctx, author, note.ID)
	if err != nil {
		t.Fatalf("getAccess author failed: %v", err)
	}
	if access["can_read"] != true {
		t.Fatalf("author debe poder leer")
	}
	// member sin share 403
	_, err = svc.GetAccess(ctx, member, note.ID)
	if err == nil {
		t.Fatal("member sin share debe 403")
	}
	// tras share
	_, _ = svc.Share(ctx, author, note.ID, groupID, "restricted")
	access2, err := svc.GetAccess(ctx, member, note.ID)
	if err != nil {
		t.Fatalf("member con share debe tener acceso: %v", err)
	}
	if access2["can_read"] != true {
		t.Fatalf("member con share can_read true")
	}
	if access2["access_mode"] != "restricted" {
		t.Fatalf("access_mode debe ser restricted")
	}
}

func TestPublicNoteAccessibleWithoutShare(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	other := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Publica", nil, "public", stringPtr("public content"), nil)
	got, err := svc.Get(ctx, other, note.ID)
	if err != nil {
		t.Fatalf("public note debe ser accesible: %v", err)
	}
	if got.Content == nil || *got.Content != "public content" {
		t.Fatalf("contenido public mismatch")
	}
}

func TestEditPrivateNoteForbidden(t *testing.T) {
	svc, _, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	other := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Private edit", nil, "private", nil, nil)
	_, err := svc.Update(ctx, other, note.ID, stringPtr("hacked"), nil, nil)
	if err == nil {
		t.Fatal("other no debe editar privada")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
}

func TestListGroupNotes(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	outsider := uuid.NewString()
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	// author crea y comparte
	n, _ := svc.Create(ctx, author, "Grupo note", nil, "private", nil, nil)
	_, _ = svc.Share(ctx, author, n.ID, groupID, "link")
	// member lista
	notes, _, err := svc.ListGroupNotes(ctx, member, groupID, "", 10)
	if err != nil {
		t.Fatalf("list group notes member failed: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != n.ID {
		t.Fatalf("member debe ver 1 nota")
	}
	// outsider 403
	_, _, err = svc.ListGroupNotes(ctx, outsider, groupID, "", 10)
	if err == nil {
		t.Fatal("outsider debe 403")
	}
	se := err.(*ServiceError)
	if se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden got %q", se.Code)
	}
}

func TestConcurrentLikes(t *testing.T) {
	svc, _, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Concurrent likes", nil, "public", nil, nil)
	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	errs := make([]error, workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			uid := uuid.NewString()
			errs[idx] = svc.Like(ctx, uid, note.ID)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d like failed: %v", i, err)
		}
	}
	n, _ := noteStore.GetByID(ctx, note.ID)
	if n.LikesCount != workers {
		t.Fatalf("likes_count esperado %d, got %d", workers, n.LikesCount)
	}
	// verificar que segundo intento concurrente con mismo usuario da 409 sin desfasar contador
	dupUser := uuid.NewString()
	_ = svc.Like(ctx, dupUser, note.ID)
	var wg2 sync.WaitGroup
	wg2.Add(5)
	dupErrs := make([]error, 5)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			defer wg2.Done()
			dupErrs[idx] = svc.Like(ctx, dupUser, note.ID)
		}(i)
	}
	wg2.Wait()
	bad := 0
	for _, e := range dupErrs {
		if e != nil {
			if se, ok := e.(*ServiceError); ok && se.Code == "already_liked" {
				bad++
			}
		}
	}
	if bad != 5 {
		t.Fatalf("esperaba 5 already_liked, got %d", bad)
	}
	n2, _ := noteStore.GetByID(ctx, note.ID)
	if n2.LikesCount != workers+1 {
		t.Fatalf("likes_count tras duplicados debe ser %d, got %d", workers+1, n2.LikesCount)
	}
}

func TestAccessModeLinkVsRestricted(t *testing.T) {
	svc, _, _, _, _, _, _, social := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	nonMember := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	// nota private compartida con link -> nonMember debe leer
	noteLink, _ := svc.Create(ctx, author, "Link note", nil, "private", stringPtr("link content"), nil)
	_, err := svc.Share(ctx, author, noteLink.ID, groupID, "link")
	if err != nil {
		t.Fatalf("share link failed: %v", err)
	}
	got, err := svc.Get(ctx, nonMember, noteLink.ID)
	if err != nil {
		t.Fatalf("nonMember con link debería leer: %v", err)
	}
	if got.Content == nil || *got.Content != "link content" {
		t.Fatalf("contenido link no coincide")
	}
	// copy también debe permitir link sin membresía
	_, err = svc.Copy(ctx, nonMember, noteLink.ID)
	if err != nil {
		t.Fatalf("nonMember copy con link debería permitir: %v", err)
	}
	// nota private compartida con restricted -> nonMember 403, member 200
	noteRestr, _ := svc.Create(ctx, author, "Restricted note", nil, "private", stringPtr("restricted content"), nil)
	_, err = svc.Share(ctx, author, noteRestr.ID, groupID, "restricted")
	if err != nil {
		t.Fatalf("share restricted failed: %v", err)
	}
	_, err = svc.Get(ctx, nonMember, noteRestr.ID)
	if err == nil {
		t.Fatal("nonMember con restricted debe recibir 403")
	}
	if se, ok := err.(*ServiceError); !ok || se.Code != "forbidden" {
		t.Fatalf("esperaba forbidden, got %v", err)
	}
	_, err = svc.Copy(ctx, nonMember, noteRestr.ID)
	if err == nil {
		t.Fatal("nonMember copy con restricted debe 403")
	}
	got2, err := svc.Get(ctx, member, noteRestr.ID)
	if err != nil {
		t.Fatalf("member con restricted debería leer: %v", err)
	}
	if got2.Content == nil || *got2.Content != "restricted content" {
		t.Fatalf("contenido restricted member no coincide")
	}
}

func TestListGroupNotesOrdering(t *testing.T) {
	svc, _, noteStore, _, _, _, sharedStore, social := newTestService()
	ctx := context.Background()
	authorAdmin := uuid.NewString()
	authorMember := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(authorAdmin, groupID)
	social.AddMember(authorMember, groupID)
	social.AddMember(member, groupID)
	// crear 4 notas
	nAdminHigh, _ := svc.Create(ctx, authorAdmin, "Admin high", nil, "private", nil, nil)
	nAdminLow, _ := svc.Create(ctx, authorAdmin, "Admin low", nil, "private", nil, nil)
	nMemberHigh, _ := svc.Create(ctx, authorMember, "Member high", nil, "private", nil, nil)
	nMemberLow, _ := svc.Create(ctx, authorMember, "Member low", nil, "private", nil, nil)
	// compartir todas en el mismo grupo
	_, _ = svc.Share(ctx, authorAdmin, nAdminHigh.ID, groupID, "link")
	_, _ = svc.Share(ctx, authorAdmin, nAdminLow.ID, groupID, "link")
	_, _ = svc.Share(ctx, authorMember, nMemberHigh.ID, groupID, "link")
	_, _ = svc.Share(ctx, authorMember, nMemberLow.ID, groupID, "link")
	// dar likes: AdminHigh 5, MemberHigh 10, AdminLow 1, MemberLow 2
	// Para controlar shared_at, ajustamos shared_at manualmente via store (memoria)
	// Primero asignamos likes via LikeAtomic con usuarios ficticios
	for i := 0; i < 5; i++ {
		_ = svc.Like(ctx, uuid.NewString(), nAdminHigh.ID)
	}
	for i := 0; i < 1; i++ {
		_ = svc.Like(ctx, uuid.NewString(), nAdminLow.ID)
	}
	for i := 0; i < 10; i++ {
		_ = svc.Like(ctx, uuid.NewString(), nMemberHigh.ID)
	}
	for i := 0; i < 2; i++ {
		_ = svc.Like(ctx, uuid.NewString(), nMemberLow.ID)
	}
	// Ajustar SharedAt para desempate: asegurar orden determinístico haciendo que shared de MemberHigh sea más reciente que AdminHigh? Pero orden debe priorizar likes dentro de cada bucket admin/non-admin.
	// Fuerza shared_at: manipular directamente el store memory
	// Obtener shareds y setear tiempos
	sharedStore.mu.Lock()
	for _, sh := range sharedStore.shared {
		switch sh.NoteID {
		case nAdminHigh.ID:
			sh.SharedAt = parseTime("2026-01-10T10:00:00Z")
			sh.IsAdminNote = true
		case nAdminLow.ID:
			sh.SharedAt = parseTime("2026-01-10T11:00:00Z")
			sh.IsAdminNote = true
		case nMemberHigh.ID:
			sh.SharedAt = parseTime("2026-01-10T12:00:00Z")
			sh.IsAdminNote = false
		case nMemberLow.ID:
			sh.SharedAt = parseTime("2026-01-10T09:00:00Z")
			sh.IsAdminNote = false
		}
	}
	sharedStore.mu.Unlock()
	// también asegurar likes_count reflecte realidad (ya lo hace)
	// listar
	notes, _, err := svc.ListGroupNotes(ctx, member, groupID, "", 10)
	if err != nil {
		t.Fatalf("list group notes failed: %v", err)
	}
	if len(notes) != 4 {
		t.Fatalf("esperaba 4 notas, got %d", len(notes))
	}
	// Orden esperado: Admin bucket primero ordenado por likes desc: AdminHigh (5) antes que AdminLow (1)
	// Luego non-admin bucket ordenado por likes desc: MemberHigh (10) antes que MemberLow (2)
	expected := []string{nAdminHigh.ID, nAdminLow.ID, nMemberHigh.ID, nMemberLow.ID}
	for i, expID := range expected {
		if notes[i].ID != expID {
			t.Fatalf("orden incorrecto en pos %d: esperado %s (%s) got %s (%s)", i, expID, idToTitle(noteStore, expID), notes[i].ID, notes[i].Title)
		}
	}
	// --- Desempate por fecha: mismo likes (3) y mismo is_admin_note, debe ordenar por shared_at DESC ---
	nTieRecent, _ := svc.Create(ctx, authorMember, "Tie recent", nil, "private", nil, nil)
	nTieOld, _ := svc.Create(ctx, authorMember, "Tie old", nil, "private", nil, nil)
	_, _ = svc.Share(ctx, authorMember, nTieRecent.ID, groupID, "link")
	_, _ = svc.Share(ctx, authorMember, nTieOld.ID, groupID, "link")
	for i := 0; i < 3; i++ {
		_ = svc.Like(ctx, uuid.NewString(), nTieRecent.ID)
		_ = svc.Like(ctx, uuid.NewString(), nTieOld.ID)
	}
	// Ajustar shared_at para los 2 nuevos: TieRecent más reciente que TieOld, ambos con mismo is_admin_note=false
	sharedStore.mu.Lock()
	for _, sh := range sharedStore.shared {
		switch sh.NoteID {
		case nTieRecent.ID:
			sh.SharedAt = parseTime("2026-01-10T13:00:00Z")
			sh.IsAdminNote = false
		case nTieOld.ID:
			sh.SharedAt = parseTime("2026-01-10T10:30:00Z")
			sh.IsAdminNote = false
		}
	}
	sharedStore.mu.Unlock()
	notesTie, _, err := svc.ListGroupNotes(ctx, member, groupID, "", 10)
	if err != nil {
		t.Fatalf("list group notes tie failed: %v", err)
	}
	if len(notesTie) != 6 {
		t.Fatalf("esperaba 6 notas tras tie, got %d", len(notesTie))
	}
	// Orden esperado completo: admin bucket (5,1) luego non-admin bucket (10,3tieRecent,3tieOld,2)
	expectedTie := []string{nAdminHigh.ID, nAdminLow.ID, nMemberHigh.ID, nTieRecent.ID, nTieOld.ID, nMemberLow.ID}
	for i, expID := range expectedTie {
		if notesTie[i].ID != expID {
			t.Fatalf("orden tie incorrecto en pos %d: esperado %s (%s) got %s (%s)", i, expID, idToTitle(noteStore, expID), notesTie[i].ID, notesTie[i].Title)
		}
	}
	// Verificación adicional: entre los dos empatados, el más reciente primero
	var idxRecent, idxOld int = -1, -1
	for i, n := range notesTie {
		if n.ID == nTieRecent.ID {
			idxRecent = i
		}
		if n.ID == nTieOld.ID {
			idxOld = i
		}
	}
	if idxRecent == -1 || idxOld == -1 {
		t.Fatalf("no se encontraron notas tie en listado")
	}
	if idxRecent > idxOld {
		t.Fatalf("desempate por shared_at falló: TieRecent (13:00) debe aparecer antes que TieOld (10:30), idxRecent=%d idxOld=%d", idxRecent, idxOld)
	}
	_ = noteStore
}

// --- Suite Drive integration: ciclo de vida POST/PATCH/DELETE/GET/copy ---

func TestDriveCreateSyncsPostgresAndDrive(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Drive sync", nil, "private", stringPtr("# md body"), nil)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if note.ExternalFileID == nil || *note.ExternalFileID == "" {
		t.Fatal("drive_file_id (external_file_id) debe persistirse en Postgres")
	}
	stored, _ := noteStore.GetByID(ctx, note.ID)
	if stored == nil || stored.ExternalFileID == nil || *stored.ExternalFileID != *note.ExternalFileID {
		t.Fatal("Postgres no guarda el drive_file_id devuelto por Drive")
	}
	if !driveMock.HasFile(*note.ExternalFileID) {
		t.Fatal("Drive debe contener el .md creado")
	}
	body, _ := driveMock.GetFileContent(ctx, author, *note.ExternalFileID)
	if body != "# md body" {
		t.Fatalf("contenido Drive mismatch: %q", body)
	}
}

func TestDriveUpdateSyncsContent(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Editable", nil, "private", stringPtr("v1"), nil)
	newBody := "v2 actualizado"
	updated, err := svc.Update(ctx, author, note.ID, stringPtr("Editable v2"), nil, &newBody)
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if updated.Title != "Editable v2" {
		t.Fatalf("título PG no actualizado")
	}
	driveBody, _ := driveMock.GetFileContent(ctx, author, *note.ExternalFileID)
	if driveBody != newBody {
		t.Fatalf("Drive no sincronizado: got %q", driveBody)
	}
	got, _ := svc.Get(ctx, author, note.ID)
	if got.Content == nil || *got.Content != newBody {
		t.Fatalf("GET híbrido debe devolver contenido de Drive")
	}
}

func TestDriveUpdateRecreatesMissingFile(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Legacy sin file", nil, "private", stringPtr("orig"), nil)
	// Simular nota legacy sin drive_file_id
	stored, _ := noteStore.GetByID(ctx, note.ID)
	stored.ExternalFileID = nil
	noteStore.mu.Lock()
	noteStore.notes[note.ID].ExternalFileID = nil
	noteStore.mu.Unlock()
	newBody := "contenido recuperado"
	updated, err := svc.Update(ctx, author, note.ID, nil, nil, &newBody)
	if err != nil {
		t.Fatalf("update self-healing failed: %v", err)
	}
	if updated.ExternalFileID == nil || *updated.ExternalFileID == "" {
		t.Fatal("update debe crear drive_file_id cuando falta")
	}
	if !driveMock.HasFile(*updated.ExternalFileID) {
		t.Fatal("Drive debe contener el archivo recreado")
	}
	got, _ := svc.Get(ctx, author, note.ID)
	if got.Content == nil || *got.Content != newBody {
		t.Fatalf("contenido recreado mismatch: %v", got.Content)
	}
	_ = stored
}

func TestDriveUpdateNotFoundMapsNoteUnavailable(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Borrada externa", nil, "private", stringPtr("x"), nil)
	driveMock.InjectGetError(*note.ExternalFileID, &drive.DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"})
	// Forzar que UpdateFile falle con 404 eliminando el archivo del mock y usando otro owner path:
	// Mock UpdateFile devuelve 404 si el archivo no existe.
	// Eliminamos directamente vía DeleteFile con owner correcto y luego intentamos update.
	_ = driveMock.DeleteFile(ctx, author, *note.ExternalFileID)
	newBody := "intento"
	_, err := svc.Update(ctx, author, note.ID, nil, nil, &newBody)
	if err == nil {
		t.Fatal("esperaba note_unavailable al actualizar archivo borrado en Drive")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "note_unavailable" {
		t.Fatalf("esperaba note_unavailable, got %v", err)
	}
}

func TestDriveDeleteCascadesDriveAndPostgres(t *testing.T) {
	svc, driveMock, noteStore, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Eliminar cascada", nil, "private", stringPtr("bye"), nil)
	fileID := *note.ExternalFileID
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if driveMock.HasFile(fileID) {
		t.Fatal("Drive debe eliminar el archivo en cascada")
	}
	if stored, _ := noteStore.GetByID(ctx, note.ID); stored != nil {
		t.Fatal("Postgres debe eliminar la metadata")
	}
}

func TestDriveGetMaps403And404ToNoteUnavailable(t *testing.T) {
	for _, code := range []int{403, 404} {
		svc, driveMock, _, _, _, _, _, _ := newTestService()
		ctx := context.Background()
		author := uuid.NewString()
		note, _ := svc.Create(ctx, author, "Híbrida", nil, "public", stringPtr("c"), nil)
		driveMock.InjectGetError(*note.ExternalFileID, &drive.DriveError{Code: code, Message: "Nota no disponible en almacenamiento remoto"})
		_, err := svc.Get(ctx, author, note.ID)
		if err == nil {
			t.Fatalf("código %d: esperaba error", code)
		}
		se, ok := err.(*ServiceError)
		if !ok {
			t.Fatalf("código %d: esperaba ServiceError got %T", code, err)
		}
		if se.Code != "note_unavailable" {
			t.Fatalf("código %d: esperaba note_unavailable got %q", code, se.Code)
		}
		if !strings.Contains(se.Message, "Nota no disponible") {
			t.Fatalf("código %d: payload debe ser informativo, got %q", code, se.Message)
		}
	}
}

func TestDriveCopyCreatesIndependentFile(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Para copiar", nil, "public", stringPtr("original md"), nil)
	origID := *note.ExternalFileID
	cloned, err := svc.Copy(ctx, copier, note.ID)
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if cloned.UserID != copier {
		t.Fatalf("autor del clon debe ser quien copia")
	}
	if cloned.ExternalFileID == nil || *cloned.ExternalFileID == origID {
		t.Fatal("clon debe tener nuevo drive_file_id independiente")
	}
	if !driveMock.HasFile(*cloned.ExternalFileID) {
		t.Fatal("Drive del copiador debe contener la copia")
	}
	body, _ := driveMock.GetFileContent(ctx, copier, *cloned.ExternalFileID)
	if body != "original md" {
		t.Fatalf("contenido clonado mismatch: %q", body)
	}
}

func TestDriveCopyMissingSourceMapsNotFound(t *testing.T) {
	svc, driveMock, _, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	copier := uuid.NewString()
	note, _ := svc.Create(ctx, author, "Origen frágil", nil, "public", stringPtr("x"), nil)
	_ = driveMock.DeleteFile(ctx, author, *note.ExternalFileID)
	_, err := svc.Copy(ctx, copier, note.ID)
	if err == nil {
		t.Fatal("esperaba error al copiar origen borrado en Drive")
	}
	se, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("esperaba ServiceError got %T", err)
	}
	if se.Code != "not_found" && se.Code != "note_unavailable" {
		t.Fatalf("código semántico esperado not_found/note_unavailable, got %q", se.Code)
	}
}

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
func idToTitle(store *MemoryNoteStore, id string) string {
	n, _ := store.GetByID(context.Background(), id)
	if n == nil {
		return "?"
	}
	return n.Title
}

func stringPtr(s string) *string { return &s }
