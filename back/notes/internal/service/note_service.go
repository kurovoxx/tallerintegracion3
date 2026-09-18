package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// ServiceError representa error de dominio mapeable a HTTP.
type ServiceError struct {
	Code    string
	Message string
}

func (e *ServiceError) Error() string { return e.Code + ": " + e.Message }

func newServiceError(code string) *ServiceError {
	return &ServiceError{Code: code, Message: utils.MessageForCode(code)}
}
func newServiceErrorMsg(code, msg string) *ServiceError {
	return &ServiceError{Code: code, Message: msg}
}

// Store interfaces para desacoplar de pgx y permitir mocks en tests.
type NoteStore interface {
	Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string) (*model.Note, error)
	GetByID(ctx context.Context, id string) (*model.Note, error)
	ListByUser(ctx context.Context, userID, cursor string, limit int) ([]*model.Note, string, error)
	Update(ctx context.Context, id string, title *string, visibility *string) (*model.Note, error)
	Delete(ctx context.Context, id string) error
	IncrementLikes(ctx context.Context, noteID string, delta int) error
	UpdateExternalFileID(ctx context.Context, noteID, fileID string) error
}

type AttachmentStore interface {
	Create(ctx context.Context, noteID, externalFileID, fileURL, fileType string, fileName *string, fileSize *int, isInline bool) (*model.Attachment, error)
	GetByID(ctx context.Context, id string) (*model.Attachment, error)
	ListByNote(ctx context.Context, noteID string) ([]*model.Attachment, error)
	Delete(ctx context.Context, id string) error
}

type SavedStore interface {
	Save(ctx context.Context, userID, noteID string) (*model.SavedNote, error)
	Exists(ctx context.Context, userID, noteID string) (bool, error)
	Delete(ctx context.Context, userID, noteID string) error
}

type LikeStore interface {
	Create(ctx context.Context, noteID, userID string) (*model.NoteLike, error)
	Delete(ctx context.Context, noteID, userID string) (bool, error)
	Exists(ctx context.Context, noteID, userID string) (bool, error)
	LikeAtomic(ctx context.Context, noteID, userID string) error
	UnlikeAtomic(ctx context.Context, noteID, userID string) error
}

type SharedStore interface {
	Create(ctx context.Context, noteID, groupID string, isAdminNote bool, accessMode string, followersSnapshot int) (*model.SharedNote, error)
	GetByID(ctx context.Context, id string) (*model.SharedNote, error)
	ListByNote(ctx context.Context, noteID string) ([]*model.SharedNote, error)
	ListByGroup(ctx context.Context, groupID string, limit int, cursor string) ([]*model.SharedNote, string, error)
	Delete(ctx context.Context, id string) error
	DeleteAllByNote(ctx context.Context, noteID string) error
	DeleteByUserAndGroup(ctx context.Context, userID, groupID string) error
	HasAccess(ctx context.Context, noteID, groupID string) (bool, error)
	HasAnyShare(ctx context.Context, noteID string) (bool, error)
}

// SocialResolver verifica pertenencia/membership vía gRPC a Social (mock en tests).
type SocialResolver interface {
	IsMember(ctx context.Context, userID, groupID string) (bool, error)
	IsAdmin(ctx context.Context, userID, groupID string) (bool, error)
	GetFollowersCount(ctx context.Context, userID string) (int, error)
}

type noopSocialResolver struct{}

func (n *noopSocialResolver) IsMember(ctx context.Context, userID, groupID string) (bool, error) { return true, nil }
func (n *noopSocialResolver) IsAdmin(ctx context.Context, userID, groupID string) (bool, error)  { return false, nil }
func (n *noopSocialResolver) GetFollowersCount(ctx context.Context, userID string) (int, error) {
	return 0, nil
}

type NoteService struct {
	notes       NoteStore
	attachments AttachmentStore
	saved       SavedStore
	likes       LikeStore
	shared      SharedStore
	drive       drive.Client
	social      SocialResolver
}

func NewNoteService(notes NoteStore, attachments AttachmentStore, saved SavedStore, likes LikeStore, shared SharedStore, d drive.Client, social SocialResolver) *NoteService {
	if social == nil {
		social = &noopSocialResolver{}
	}
	if d == nil {
		d = drive.NewMockClient()
	}
	return &NoteService{
		notes:       notes,
		attachments: attachments,
		saved:       saved,
		likes:       likes,
		shared:      shared,
		drive:       d,
		social:      social,
	}
}

// --- helpers validación ---
func validateTitle(title string) error {
	if !utils.ValidateTitle(title) {
		return newServiceError(utils.ErrInvalidTitle)
	}
	return nil
}
func validateVisibility(v string) error {
	if !utils.ValidateVisibility(v) {
		return newServiceError(utils.ErrInvalidVisibility)
	}
	return nil
}
func validateSubjectID(s *string) error {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	if !utils.ValidateUUID(*s) {
		return newServiceError(utils.ErrInvalidSubjectID)
	}
	return nil
}

// Create: valida payload, crea archivo .md en Drive, inserta metadata, rollback si falla DB.
func (s *NoteService) Create(ctx context.Context, userID string, title string, subjectID *string, visibility string, content *string, tags *string) (*model.Note, error) {
	// validaciones (FASE 1)
	title = utils.NormalizeTitle(title)
	if err := validateTitle(title); err != nil {
		return nil, err
	}
	if strings.TrimSpace(visibility) == "" {
		visibility = "private"
	}
	if err := validateVisibility(visibility); err != nil {
		return nil, err
	}
	if err := validateSubjectID(subjectID); err != nil {
		return nil, err
	}
	if subjectID != nil {
		v := strings.TrimSpace(*subjectID)
		if v == "" {
			subjectID = nil
		} else {
			*subjectID = v
		}
	}
	// contenido por defecto
	mdContent := ""
	if content != nil {
		mdContent = *content
	} else {
		mdContent = fmt.Sprintf("# %s\n\n", title)
	}
	// 1. Crear archivo en Drive del autor
	driveFileID, err := s.drive.CreateFile(ctx, userID, title+".md", mdContent)
	if err != nil {
		// distinguir 413 etc?
		if de, ok := err.(*drive.DriveError); ok && de.Code == 413 {
			return nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("error creando archivo en Drive: %v", err)}
	}
	// 2. Insertar metadata en PG
	note, err := s.notes.Create(ctx, userID, subjectID, title, &driveFileID, visibility, nil)
	if err != nil {
		// rollback compensatorio: eliminar archivo huérfano en Drive
		_ = s.drive.DeleteFile(ctx, userID, driveFileID)
		return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("error insertando metadata: %v", err)}
	}
	return note, nil
}

// Get: lee metadata, verifica acceso, descarga contenido de Drive con manejo 404/403 estructurado.
func (s *NoteService) Get(ctx context.Context, requesterID string, noteID string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// control acceso: si privado y no es autor, verificar shared_notes diferenciando access_mode
	if note.Visibility == "private" && note.UserID != requesterID {
		has, err := s.shared.HasAnyShare(ctx, noteID)
		if err != nil {
			return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
		}
		if !has {
			return nil, newServiceError(utils.ErrForbidden)
		}
		sharedList, err := s.shared.ListByNote(ctx, noteID)
		if err != nil {
			return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
		}
		allowed := false
		for _, sh := range sharedList {
			if sh.AccessMode == "link" {
				// link: cualquier autenticado con el enlace puede leer
				allowed = true
				break
			} else if sh.AccessMode == "restricted" {
				if ok, _ := s.social.IsMember(ctx, requesterID, sh.GroupID); ok {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return nil, newServiceError(utils.ErrForbidden)
		}
	}
	// descargar contenido de Drive usando external_file_id
	if note.ExternalFileID == nil || strings.TrimSpace(*note.ExternalFileID) == "" {
		// sin file aún (offline-first brevemente) -> retornar sin contenido
		return note, nil
	}
	content, err := s.drive.GetFileContent(ctx, note.UserID, *note.ExternalFileID)
	if err != nil {
		if drive.IsNotFound(err) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		if drive.IsForbidden(err) {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		// fallback: si error contiene 403/404 textual
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "404") {
			return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
		}
		return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("error leyendo Drive: %v", err)}
	}
	note.Content = &content
	return note, nil
}

// ListMy: GET /notes/me paginado
func (s *NoteService) ListMy(ctx context.Context, userID string, cursor string, limit int) ([]*model.Note, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	// cursor validación: si no vacío debe ser uuid
	if cursor != "" && !utils.ValidateUUID(cursor) {
		return nil, "", newServiceErrorMsg(utils.ErrBadRequest, "cursor inválido")
	}
	notes, next, err := s.notes.ListByUser(ctx, userID, cursor, limit)
	if err != nil {
		return nil, "", &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if notes == nil {
		notes = []*model.Note{}
	}
	return notes, next, nil
}

// Update: solo autor, actualiza título/visibilidad en PG, contenido en Drive si cambia.
func (s *NoteService) Update(ctx context.Context, userID string, noteID string, title *string, visibility *string, content *string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	// validar campos opcionales
	var newTitle *string
	if title != nil {
		t := utils.NormalizeTitle(*title)
		if err := validateTitle(t); err != nil {
			return nil, err
		}
		newTitle = &t
	}
	if visibility != nil {
		v := strings.TrimSpace(*visibility)
		if err := validateVisibility(v); err != nil {
			return nil, err
		}
		visibility = &v
	}
	// Si cambia contenido, actualizar archivo en Drive
	if content != nil && note.ExternalFileID != nil {
		err := s.drive.UpdateFile(ctx, userID, *note.ExternalFileID, content, newTitle)
		if err != nil {
			if drive.IsNotFound(err) || drive.IsForbidden(err) {
				return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
			}
			return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("error actualizando Drive: %v", err)}
		}
		// si solo cambió título y contenido ya incluyó título, no duplicar update PG? igual necesitamos actualizar título en PG si cambió.
	} else if newTitle != nil && note.ExternalFileID != nil && content == nil {
		// solo renombrar en Drive
		err := s.drive.UpdateFile(ctx, userID, *note.ExternalFileID, nil, newTitle)
		if err != nil {
			if drive.IsNotFound(err) || drive.IsForbidden(err) {
				return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
			}
			return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("error renombrando en Drive: %v", err)}
		}
	}
	// actualizar metadata PG
	updated, err := s.notes.Update(ctx, noteID, newTitle, visibility)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return updated, nil
}

// Delete: solo autor, elimina archivo Drive y metadata PG (cascadas)
func (s *NoteService) Delete(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceError(utils.ErrForbidden)
	}
	// eliminar archivo en Drive (si existe)
	if note.ExternalFileID != nil && strings.TrimSpace(*note.ExternalFileID) != "" {
		_ = s.drive.DeleteFile(ctx, userID, *note.ExternalFileID)
		// ignorar error Drive 404 ya que metadata igual debe borrarse; pero si 403 no debería bloquear? según spec, delete debe borrar ambos.
	}
	// eliminar metadata
	if err := s.notes.Delete(ctx, noteID); err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
		}
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return nil
}

// Attachments
func (s *NoteService) AddAttachment(ctx context.Context, userID string, noteID string, fileName string, fileType string, data []byte, isInline bool) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	if len(data) == 0 {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "archivo vacío")
	}
	if len(data) > 10*1024*1024 {
		return nil, newServiceError(utils.ErrFileTooLarge)
	}
	if strings.TrimSpace(fileName) == "" {
		fileName = "attachment"
	}
	if strings.TrimSpace(fileType) == "" {
		fileType = "application/octet-stream"
	}
	extID, url, err := s.drive.UploadAttachment(ctx, userID, noteID, fileName, fileType, data, isInline)
	if err != nil {
		if de, ok := err.(*drive.DriveError); ok && de.Code == 413 {
			return nil, newServiceError(utils.ErrFileTooLarge)
		}
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	size := len(data)
	att, err := s.attachments.Create(ctx, noteID, extID, url, fileType, &fileName, &size, isInline)
	if err != nil {
		_ = s.drive.DeleteAttachment(ctx, userID, extID)
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return att, nil
}

func (s *NoteService) RemoveAttachment(ctx context.Context, userID string, noteID string, attachmentID string) error {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(attachmentID) {
		return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return newServiceError(utils.ErrForbidden)
	}
	att, err := s.attachments.GetByID(ctx, attachmentID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if att == nil || att.NoteID != noteID {
		return newServiceErrorMsg(utils.ErrNotFound, "adjunto no encontrado")
	}
	_ = s.drive.DeleteAttachment(ctx, userID, att.ExternalFileID)
	if err := s.attachments.Delete(ctx, attachmentID); err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return nil
}

func (s *NoteService) ListAttachments(ctx context.Context, noteID string) ([]*model.Attachment, error) {
	return s.attachments.ListByNote(ctx, noteID)
}

// AddAttachmentExternal registra un adjunto ya subido directo a Drive desde el cliente (external_file_id provisto).
func (s *NoteService) AddAttachmentExternal(ctx context.Context, userID string, noteID string, externalFileID string, fileName string, fileType string, isInline bool, fileSize *int) (*model.Attachment, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if strings.TrimSpace(externalFileID) == "" {
		return nil, newServiceErrorMsg(utils.ErrBadRequest, "external_file_id requerido")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != userID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	if strings.TrimSpace(fileType) == "" {
		fileType = "application/octet-stream"
	}
	fileURL := fmt.Sprintf("https://drive.google.com/file/d/%s/view", externalFileID)
	var fnPtr *string
	if strings.TrimSpace(fileName) != "" {
		fnPtr = &fileName
	}
	return s.attachments.Create(ctx, noteID, externalFileID, fileURL, fileType, fnPtr, fileSize, isInline)
}

// Save: bookmark sin clonar
func (s *NoteService) Save(ctx context.Context, userID string, noteID string) (*model.SavedNote, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// opcional: verificar acceso si es privada y no es autor
	if note.Visibility == "private" && note.UserID != userID {
		has, _ := s.shared.HasAnyShare(ctx, noteID)
		if !has {
			return nil, newServiceError(utils.ErrForbidden)
		}
	}
	saved, err := s.saved.Save(ctx, userID, noteID)
	if err != nil {
		if strings.Contains(err.Error(), "already_saved") || strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate") {
			return nil, newServiceError(utils.ErrAlreadySaved)
		}
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return saved, nil
}

func (s *NoteService) Unsave(ctx context.Context, userID string, noteID string) error {
	return s.saved.Delete(ctx, userID, noteID)
}

// Copy: clona apunte de tercero, descarga y crea nuevo archivo en Drive del copiador
func (s *NoteService) Copy(ctx context.Context, userID string, noteID string) (*model.Note, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	orig, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if orig == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	// verificar acceso si privada diferenciando link vs restricted
	if orig.Visibility == "private" && orig.UserID != userID {
		has, _ := s.shared.HasAnyShare(ctx, noteID)
		if !has {
			return nil, newServiceError(utils.ErrForbidden)
		}
		sharedList, _ := s.shared.ListByNote(ctx, noteID)
		allowed := false
		for _, sh := range sharedList {
			if sh.AccessMode == "link" {
				allowed = true
				break
			} else if sh.AccessMode == "restricted" {
				if ok, _ := s.social.IsMember(ctx, userID, sh.GroupID); ok {
					allowed = true
					break
				}
			}
		}
		if !allowed {
			return nil, newServiceError(utils.ErrForbidden)
		}
	}
	if orig.ExternalFileID == nil {
		return nil, newServiceErrorMsg(utils.ErrNoteUnavailable, "Nota no disponible en almacenamiento remoto")
	}
	newFileID, err := s.drive.CopyFile(ctx, orig.UserID, *orig.ExternalFileID, userID, orig.Title)
	if err != nil {
		if drive.IsNotFound(err) || drive.IsForbidden(err) {
			return nil, newServiceErrorMsg(utils.ErrNotFound, "original no disponible")
		}
		return nil, &ServiceError{Code: utils.ErrInternal, Message: fmt.Sprintf("copy drive: %v", err)}
	}
	newNote, err := s.notes.Create(ctx, userID, orig.SubjectID, orig.Title, &newFileID, "private", &orig.ID)
	if err != nil {
		_ = s.drive.DeleteFile(ctx, userID, newFileID)
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return newNote, nil
}

// Like / Unlike con contador transaccional real (Tx en PG, mutex en memoria)
func (s *NoteService) Like(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	err = s.likes.LikeAtomic(ctx, noteID, userID)
	if err != nil {
		if strings.Contains(err.Error(), "already_liked") || strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "duplicate") {
			return newServiceError(utils.ErrAlreadyLiked)
		}
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return nil
}

func (s *NoteService) Unlike(ctx context.Context, userID string, noteID string) error {
	if !utils.ValidateUUID(noteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if err := s.likes.UnlikeAtomic(ctx, noteID, userID); err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	return nil
}

// Share
func (s *NoteService) Share(ctx context.Context, ownerID string, noteID string, groupID string, accessMode string) (*model.SharedNote, error) {
	if !utils.ValidateUUID(noteID) || !utils.ValidateUUID(groupID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota o grupo no encontrado")
	}
	if !utils.ValidateAccessMode(accessMode) {
		return nil, newServiceError(utils.ErrInvalidAccessMode)
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	if note.UserID != ownerID {
		return nil, newServiceError(utils.ErrForbidden)
	}
	// verificar membresía en grupo
	isMember, err := s.social.IsMember(ctx, ownerID, groupID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if !isMember {
		return nil, newServiceError(utils.ErrForbidden)
	}
	isAdmin, _ := s.social.IsAdmin(ctx, ownerID, groupID)
	followers, _ := s.social.GetFollowersCount(ctx, ownerID)

	shared, err := s.shared.Create(ctx, noteID, groupID, isAdmin, accessMode, followers)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	// si access_mode=restricted, otorgar permisos en Drive a miembros (best-effort, no bloquea)
	if accessMode == "restricted" && note.ExternalFileID != nil {
		// En real, iterar miembros y GrantPermission; mock no-op
		_ = s.drive.GrantPermission(ctx, ownerID, *note.ExternalFileID, "member@example.com", "reader")
	}
	return shared, nil
}

func (s *NoteService) Unshare(ctx context.Context, userID string, sharedNoteID string) error {
	if !utils.ValidateUUID(sharedNoteID) {
		return newServiceErrorMsg(utils.ErrNotFound, "compartición no encontrada")
	}
	sh, err := s.shared.GetByID(ctx, sharedNoteID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if sh == nil {
		return newServiceErrorMsg(utils.ErrNotFound, "compartición no encontrada")
	}
	note, err := s.notes.GetByID(ctx, sh.NoteID)
	if err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		// nota ya borrada, borrar share
		_ = s.shared.Delete(ctx, sharedNoteID)
		return nil
	}
	// solo autor o admin del grupo puede retirar (simplificado: autor)
	// Verificamos si requester es autor o admin del grupo
	if note.UserID != userID {
		isAdmin, _ := s.social.IsAdmin(ctx, userID, sh.GroupID)
		if !isAdmin {
			return newServiceError(utils.ErrForbidden)
		}
	}
	if err := s.shared.Delete(ctx, sharedNoteID); err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	// revocar permiso Drive si restricted (best-effort)
	if sh.AccessMode == "restricted" && note.ExternalFileID != nil {
		_ = s.drive.RevokePermission(ctx, note.UserID, *note.ExternalFileID, "member@example.com")
	}
	return nil
}

func (s *NoteService) UnshareAll(ctx context.Context, userID string, groupID string) error {
	if !utils.ValidateUUID(groupID) || !utils.ValidateUUID(userID) {
		return newServiceErrorMsg(utils.ErrBadRequest, "ids inválidos")
	}
	// Sin validar autor individual: borra todos los shares de userID en groupID
	if err := s.shared.DeleteByUserAndGroup(ctx, userID, groupID); err != nil {
		return &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	// restaurar permisos Drive: revocar todos
	// Necesitaríamos listar notes de user y revocar; simplificado no-op
	return nil
}

func (s *NoteService) GetAccess(ctx context.Context, requesterID string, noteID string) (map[string]interface{}, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	note, err := s.notes.GetByID(ctx, noteID)
	if err != nil {
		return nil, &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if note == nil {
		return nil, newServiceErrorMsg(utils.ErrNotFound, "nota no encontrada")
	}
	canRead := false
	accessMode := "private"
	if note.UserID == requesterID {
		canRead = true
		accessMode = note.Visibility
	} else if note.Visibility == "public" {
		canRead = true
		accessMode = "public"
	} else {
		// private: verificar shares diferenciando link vs restricted
		list, _ := s.shared.ListByNote(ctx, noteID)
		for _, sh := range list {
			if sh.AccessMode == "link" {
				canRead = true
				accessMode = sh.AccessMode
				break
			} else if sh.AccessMode == "restricted" {
				if ok, _ := s.social.IsMember(ctx, requesterID, sh.GroupID); ok {
					canRead = true
					accessMode = sh.AccessMode
					break
				}
			}
		}
	}
	if !canRead {
		return nil, newServiceError(utils.ErrForbidden)
	}
	driveURL := ""
	if note.ExternalFileID != nil {
		driveURL = fmt.Sprintf("https://drive.google.com/file/d/%s/view", *note.ExternalFileID)
	}
	return map[string]interface{}{
		"access_mode": accessMode,
		"can_read":    canRead,
		"drive_url":   driveURL,
	}, nil
}

func (s *NoteService) ListGroupNotes(ctx context.Context, requesterID string, groupID string, cursor string, limit int) ([]*model.Note, string, error) {
	if !utils.ValidateUUID(groupID) {
		return nil, "", newServiceErrorMsg(utils.ErrNotFound, "grupo no encontrado")
	}
	// verificar membresía
	isMember, err := s.social.IsMember(ctx, requesterID, groupID)
	if err != nil {
		return nil, "", &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	if !isMember {
		return nil, "", newServiceError(utils.ErrForbidden)
	}
	// listar shared_notes del grupo
	sharedList, next, err := s.shared.ListByGroup(ctx, groupID, limit, cursor)
	if err != nil {
		return nil, "", &ServiceError{Code: utils.ErrInternal, Message: err.Error()}
	}
	// Asociar cada shared con su nota para ordenar por criterios formales
	type pair struct {
		note   *model.Note
		shared *model.SharedNote
	}
	var pairs []pair
	for _, sh := range sharedList {
		n, _ := s.notes.GetByID(ctx, sh.NoteID)
		if n != nil {
			pairs = append(pairs, pair{note: n, shared: sh})
		}
	}
	// Orden formal: 1) is_admin_note == true primero, 2) likes_count DESC, 3) shared_at DESC
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].shared.IsAdminNote != pairs[j].shared.IsAdminNote {
			return pairs[i].shared.IsAdminNote && !pairs[j].shared.IsAdminNote
		}
		if pairs[i].note.LikesCount != pairs[j].note.LikesCount {
			return pairs[i].note.LikesCount > pairs[j].note.LikesCount
		}
		return pairs[i].shared.SharedAt.After(pairs[j].shared.SharedAt)
	})
	var notes []*model.Note
	for _, p := range pairs {
		notes = append(notes, p.note)
	}
	return notes, next, nil
}

// Helper para validar UUID con mensaje
func MustUUID(s string) bool { _, err := uuid.Parse(s); return err == nil }
