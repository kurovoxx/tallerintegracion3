package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

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
	// other fuera del grupo no puede leer
	_, err = svc.Get(ctx, other, note.ID)
	if err == nil {
		t.Fatal("other fuera grupo debe 403")
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

func stringPtr(s string) *string { return &s }
