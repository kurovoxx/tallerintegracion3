package service

import (
	"context"
	"strings"

	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// NoteTombstoneReader es implementado por los NoteStore que conocen las
// lápidas de apuntes borrados (PG: notes.note_tombstones; memoria: mapa). Es
// opcional: un store que no lo implementa degrada a 404 genérico.
type NoteTombstoneReader interface {
	GetTombstone(ctx context.Context, noteID string) (*model.NoteTombstone, error)
}

// PublicNoteLister es implementado por los NoteStore que pueden listar
// apuntes públicos de otros usuarios (GET /notes/public).
type PublicNoteLister interface {
	ListPublic(ctx context.Context, excludeUserID, query, subjectID, cursor string, limit int) ([]*model.Note, string, error)
}

// SavedBatchStore permite resolver saves_count, is_saved e is_liked de una
// página completa con un número constante de consultas (sin N+1). Es opcional:
// un store que no lo implementa degrada al camino por nota.
type SavedBatchStore interface {
	CountByNotes(ctx context.Context, noteIDs []string) (map[string]int, error)
	ExistsByNotes(ctx context.Context, userID string, noteIDs []string) (map[string]bool, error)
}

// LikeBatchStore permite resolver is_liked de una página en una consulta.
type LikeBatchStore interface {
	ExistsByNotes(ctx context.Context, userID string, noteIDs []string) (map[string]bool, error)
}

const (
	savedListDefaultLimit = 20
	savedListMaxLimit     = 50
)

func (s *NoteService) tombstoneFor(ctx context.Context, noteID string) *model.NoteTombstone {
	reader, ok := s.notes.(NoteTombstoneReader)
	if !ok {
		return nil
	}
	t, err := reader.GetTombstone(ctx, noteID)
	if err != nil {
		return nil
	}
	return t
}

// deletedNoteError es el error 410 canónico de un apunte borrado por su autor.
func deletedNoteError() *ServiceError {
	return newServiceError(utils.ErrNoteDeleted)
}

// missingNoteError decide qué responder cuando la fila del apunte no existe.
// Devuelve 410 note_deleted solo si hay lápida y el solicitante podía verlo:
// el apunte era público o el solicitante lo tenía guardado. En cualquier otro
// caso (nunca existió, era privado y ajeno, o es el propio autor que lo borró)
// se mantiene el 404 zero-knowledge de siempre.
func (s *NoteService) missingNoteError(ctx context.Context, requesterID, noteID string) *ServiceError {
	t := s.tombstoneFor(ctx, noteID)
	if t == nil || t.OwnerUserID == requesterID {
		return notFoundNote()
	}
	if t.Visibility == "public" {
		return deletedNoteError()
	}
	if saved, err := s.saved.Exists(ctx, requesterID, noteID); err == nil && saved {
		return deletedNoteError()
	}
	return notFoundNote()
}

// interactionTarget es la metadata mínima que ya trae la fila del apunte
// (likes_count desnormalizado) para armar las interacciones de una página.
type interactionTarget struct {
	id         string
	likesCount int
}

// interactionsForTargets arma los contadores exactos de una página sin
// verificar acceso (el llamador ya lo hizo). likes_count sale de la columna
// desnormalizada que LikeAtomic/UnlikeAtomic mantienen en la misma
// transacción; saves_count/is_liked/is_saved se resuelven en lote cuando el
// store lo soporta (3 consultas por página) y por nota como fallback.
func (s *NoteService) interactionsForTargets(ctx context.Context, requesterID string, targets []interactionTarget) (map[string]*model.NoteInteractions, error) {
	out := make(map[string]*model.NoteInteractions, len(targets))
	if len(targets) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.id)
	}

	saves := make(map[string]int, len(ids))
	if batch, ok := s.saved.(SavedBatchStore); ok {
		got, err := batch.CountByNotes(ctx, ids)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		saves = got
	} else {
		for _, t := range targets {
			count, err := s.saved.CountByNote(ctx, t.id)
			if err != nil {
				return nil, ErrInternalDatabase
			}
			saves[t.id] = count
		}
	}

	liked := make(map[string]bool, len(ids))
	if batch, ok := s.likes.(LikeBatchStore); ok {
		got, err := batch.ExistsByNotes(ctx, requesterID, ids)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		liked = got
	} else {
		for _, t := range targets {
			exists, err := s.likes.Exists(ctx, t.id, requesterID)
			if err != nil {
				return nil, ErrInternalDatabase
			}
			liked[t.id] = exists
		}
	}

	savedSet := make(map[string]bool, len(ids))
	if batch, ok := s.saved.(SavedBatchStore); ok {
		got, err := batch.ExistsByNotes(ctx, requesterID, ids)
		if err != nil {
			return nil, ErrInternalDatabase
		}
		savedSet = got
	} else {
		for _, t := range targets {
			exists, err := s.saved.Exists(ctx, requesterID, t.id)
			if err != nil {
				return nil, ErrInternalDatabase
			}
			savedSet[t.id] = exists
		}
	}

	for _, t := range targets {
		out[t.id] = &model.NoteInteractions{
			NoteID:     t.id,
			LikesCount: t.likesCount,
			SavesCount: saves[t.id],
			IsLiked:    liked[t.id],
			IsSaved:    savedSet[t.id],
		}
	}
	return out, nil
}

// interactionsFor conserva la firma de un solo apunte y reutiliza el camino
// batch (con fallback por nota), de modo que GET /notes/{id}/stats mantiene 3
// consultas exactas.
func (s *NoteService) interactionsFor(ctx context.Context, requesterID string, note *model.Note) (*model.NoteInteractions, error) {
	got, err := s.interactionsForTargets(ctx, requesterID, []interactionTarget{{id: note.ID, likesCount: note.LikesCount}})
	if err != nil {
		return nil, err
	}
	return got[note.ID], nil
}

// InteractionsForNotes resuelve en lote likes_count/is_liked (y guardados) de
// una página de apuntes que el llamador YA autorizó, p. ej. el listado de un
// grupo (ListGroupNotes verifica la membresía). Así la lista del grupo puede
// pintar el "me gusta" propio sin una petición por nota.
func (s *NoteService) InteractionsForNotes(ctx context.Context, requesterID string, notes []*model.Note) (map[string]*model.NoteInteractions, error) {
	targets := make([]interactionTarget, 0, len(notes))
	for _, n := range notes {
		targets = append(targets, interactionTarget{id: n.ID, likesCount: n.LikesCount})
	}
	return s.interactionsForTargets(ctx, requesterID, targets)
}

// Interactions devuelve likes_count, saves_count, is_liked e is_saved de un
// apunte legible por el solicitante (GET /notes/{id}/stats). No toca Drive.
func (s *NoteService) Interactions(ctx context.Context, requesterID, noteID string) (*model.NoteInteractions, error) {
	if !utils.ValidateUUID(noteID) {
		return nil, notFoundNote()
	}
	note, err := s.observedNotesGetByID(ctx, noteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		return nil, s.missingNoteError(ctx, requesterID, noteID)
	}
	allowed, err := s.hasReadAccess(ctx, note, requesterID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, notFoundNote()
	}
	return s.interactionsFor(ctx, requesterID, note)
}

// SavedFilter acota GET /notes/saved. Todos los campos son opcionales.
type SavedFilter struct {
	// Query busca (sin distinguir mayúsculas) en el título.
	Query string
	// SubjectID filtra por materia del apunte original.
	SubjectID string
	// AuthorID filtra por autor del apunte original.
	AuthorID string
	// Scope: "all" (defecto), "others" (solo apuntes de otros usuarios) u
	// "own" (solo los propios).
	Scope string
	// Availability: "all" (defecto), "available" o "unavailable".
	Availability string
}

// savedView arma un elemento del listado. Un guardado cuyo original ya no
// existe o dejó de ser accesible se devuelve con IsAvailable=false (nunca se
// omite ni produce 500) para que el usuario pueda quitarlo de sus guardados.
func (s *NoteService) savedView(ctx context.Context, userID string, saved *model.SavedNote) (*model.SavedNoteView, error) {
	view := &model.SavedNoteView{
		SavedID: saved.ID, NoteID: saved.NoteID, SavedAt: saved.SavedAt, IsSaved: true,
	}
	note, err := s.observedNotesGetByID(ctx, saved.NoteID)
	if err != nil {
		return nil, ErrInternalDatabase
	}
	if note == nil {
		view.UnavailableReason = model.UnavailableDeleted
		view.Title = "Apunte eliminado"
		if t := s.tombstoneFor(ctx, saved.NoteID); t != nil {
			view.Title = t.Title
			view.AuthorID = t.OwnerUserID
			view.SubjectID = t.SubjectID
			view.IsOwn = t.OwnerUserID == userID
			deletedAt := t.DeletedAt
			view.DeletedAt = &deletedAt
		}
		return view, nil
	}
	view.AuthorID = note.UserID
	view.IsOwn = note.UserID == userID
	allowed, err := s.hasReadAccess(ctx, note, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		// Zero-knowledge: no se exponen título ni metadata actuales de un
		// apunte que el usuario ya no puede leer.
		view.UnavailableReason = model.UnavailableNoAccess
		view.Title = "Apunte no disponible"
		return view, nil
	}
	view.IsAvailable = true
	view.Title = note.Title
	view.SubjectID = note.SubjectID
	view.Visibility = note.Visibility
	updatedAt := note.UpdatedAt
	view.UpdatedAt = &updatedAt
	stats, err := s.interactionsFor(ctx, userID, note)
	if err != nil {
		return nil, err
	}
	view.LikesCount = stats.LikesCount
	view.SavesCount = stats.SavesCount
	view.IsLiked = stats.IsLiked
	return view, nil
}

func (f SavedFilter) matches(v *model.SavedNoteView) bool {
	switch f.Scope {
	case "others":
		if v.IsOwn {
			return false
		}
	case "own":
		if !v.IsOwn {
			return false
		}
	}
	switch f.Availability {
	case "available":
		if !v.IsAvailable {
			return false
		}
	case "unavailable":
		if v.IsAvailable {
			return false
		}
	}
	if f.AuthorID != "" && v.AuthorID != f.AuthorID {
		return false
	}
	if f.SubjectID != "" && (v.SubjectID == nil || *v.SubjectID != f.SubjectID) {
		return false
	}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" && !strings.Contains(strings.ToLower(v.Title), q) {
		return false
	}
	return true
}

func (f SavedFilter) validate() error {
	switch f.Scope {
	case "", "all", "others", "own":
	default:
		return newServiceErrorMsg(utils.ErrBadRequest, "scope debe ser all, others u own")
	}
	switch f.Availability {
	case "", "all", "available", "unavailable":
	default:
		return newServiceErrorMsg(utils.ErrBadRequest, "availability debe ser all, available o unavailable")
	}
	if f.SubjectID != "" && !utils.ValidateUUID(f.SubjectID) {
		return newServiceError(utils.ErrInvalidSubjectID)
	}
	if f.AuthorID != "" && !utils.ValidateUUID(f.AuthorID) {
		return newServiceErrorMsg(utils.ErrBadRequest, "author_id debe ser un UUID válido")
	}
	if len(f.Query) > 300 {
		return newServiceErrorMsg(utils.ErrBadRequest, "q demasiado largo")
	}
	return nil
}

// ListSaved implementa GET /notes/saved: guardados del usuario (más recientes
// primero) con filtros y paginación por cursor (cursor = saved_id del último
// elemento de la página anterior). Incluye los no disponibles.
func (s *NoteService) ListSaved(ctx context.Context, userID string, filter SavedFilter, cursor string, limit int) ([]*model.SavedNoteView, string, error) {
	if limit <= 0 {
		limit = savedListDefaultLimit
	}
	if limit > savedListMaxLimit {
		limit = savedListMaxLimit
	}
	if cursor != "" && !utils.ValidateUUID(cursor) {
		return nil, "", newServiceErrorMsg(utils.ErrBadRequest, "cursor inválido")
	}
	if err := filter.validate(); err != nil {
		return nil, "", err
	}
	rows, err := s.saved.ListByUser(ctx, userID)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	start := 0
	if cursor != "" {
		start = len(rows)
		for i, r := range rows {
			if r.ID == cursor {
				start = i + 1
				break
			}
		}
	}
	out := make([]*model.SavedNoteView, 0, limit)
	next := ""
	for i := start; i < len(rows); i++ {
		view, err := s.savedView(ctx, userID, rows[i])
		if err != nil {
			return nil, "", err
		}
		if !filter.matches(view) {
			continue
		}
		if len(out) == limit {
			// Hay al menos un elemento más que cumple el filtro.
			next = out[len(out)-1].SavedID
			break
		}
		out = append(out, view)
	}
	authors := make([]string, 0, len(out))
	for _, v := range out {
		authors = append(authors, v.AuthorID)
	}
	codes := s.resolveAuthorPublicIDs(ctx, authors)
	for _, v := range out {
		v.AuthorPublicID = codes[v.AuthorID]
	}
	return out, next, nil
}

// ListPublic implementa GET /notes/public: apuntes públicos de OTROS usuarios
// (los propios ya están en "Mis apuntes") con contadores y estado del
// solicitante, para poder darles like o guardarlos desde la tarjeta.
func (s *NoteService) ListPublic(ctx context.Context, requesterID, query, subjectID, cursor string, limit int) ([]*model.PublicNoteView, string, error) {
	if limit <= 0 {
		limit = savedListDefaultLimit
	}
	if limit > savedListMaxLimit {
		limit = savedListMaxLimit
	}
	if cursor != "" && !utils.ValidateUUID(cursor) {
		return nil, "", newServiceErrorMsg(utils.ErrBadRequest, "cursor inválido")
	}
	if subjectID != "" && !utils.ValidateUUID(subjectID) {
		return nil, "", newServiceError(utils.ErrInvalidSubjectID)
	}
	if len(query) > 300 {
		return nil, "", newServiceErrorMsg(utils.ErrBadRequest, "q demasiado largo")
	}
	lister, ok := s.notes.(PublicNoteLister)
	if !ok {
		return []*model.PublicNoteView{}, "", nil
	}
	notes, next, err := lister.ListPublic(ctx, requesterID, query, subjectID, cursor, limit)
	if err != nil {
		return nil, "", ErrInternalDatabase
	}
	targets := make([]interactionTarget, 0, len(notes))
	for _, n := range notes {
		targets = append(targets, interactionTarget{id: n.ID, likesCount: n.LikesCount})
	}
	// Una sola resolución batch por página: nunca 3 consultas por apunte.
	stats, err := s.interactionsForTargets(ctx, requesterID, targets)
	if err != nil {
		return nil, "", err
	}
	authors := make([]string, 0, len(notes))
	for _, n := range notes {
		authors = append(authors, n.UserID)
	}
	codes := s.resolveAuthorPublicIDs(ctx, authors)
	out := make([]*model.PublicNoteView, 0, len(notes))
	for _, n := range notes {
		st := stats[n.ID]
		out = append(out, &model.PublicNoteView{
			ID: n.ID, UserID: n.UserID, AuthorPublicID: codes[n.UserID],
			Title: n.Title, SubjectID: n.SubjectID,
			Visibility: n.Visibility, LikesCount: st.LikesCount,
			SavesCount: st.SavesCount, IsLiked: st.IsLiked, IsSaved: st.IsSaved,
			CreatedAt: n.CreatedAt, UpdatedAt: n.UpdatedAt,
		})
	}
	return out, next, nil
}
