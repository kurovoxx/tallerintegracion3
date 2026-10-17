// Package grpc implementa notes.v1.NotesServiceServer delegando en el
// *service.NoteService existente: la capa gRPC es un adaptador fino (mapeo de
// request/response + errores), sin reimplementar reglas de negocio ni tocar
// stores/Drive directamente. Coexiste con el handler HTTP (dual-serve); ambos
// consumen el mismo servicio, así que su comportamiento es idéntico.
package grpc

import (
	"context"
	"strings"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// NotesServer implementa notes.v1.NotesServiceServer.
type NotesServer struct {
	notesv1.UnimplementedNotesServiceServer
	svc *service.NoteService
}

// NewNotesServer construye el adaptador gRPC sobre el servicio de Notes.
func NewNotesServer(svc *service.NoteService) *NotesServer {
	return &NotesServer{svc: svc}
}

// caller extrae el user_id autenticado del contexto. Lo puebla el interceptor
// de JWT (middleware.WithUserID); si falta, el RPC llegó sin autenticar.
func caller(ctx context.Context) (string, error) {
	userID, ok := middleware.GetUserIDFromContext(ctx)
	if !ok || strings.TrimSpace(userID) == "" {
		return "", status.Error(codes.Unauthenticated, "user_id ausente en el contexto")
	}
	return userID, nil
}

func (s *NotesServer) CreateNote(ctx context.Context, req *notesv1.CreateNoteRequest) (*notesv1.CreateNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := s.svc.Create(ctx, userID, req.GetTitle(), req.SubjectId, req.GetVisibility(), req.Content, req.GetIdempotencyKey())
	if err != nil {
		return nil, mapError(err)
	}
	return &notesv1.CreateNoteResponse{NoteId: note.ID}, nil
}

func (s *NotesServer) GetNote(ctx context.Context, req *notesv1.GetNoteRequest) (*notesv1.GetNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := s.svc.Get(ctx, userID, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return &notesv1.GetNoteResponse{Note: toProtoNote(note)}, nil
}

func (s *NotesServer) ListMyNotes(ctx context.Context, req *notesv1.ListMyNotesRequest) (*notesv1.ListMyNotesResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	notes, next, err := s.svc.ListMy(ctx, userID, req.GetPage().GetPageToken(), int(req.GetPage().GetPageSize()))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*notesv1.Note, 0, len(notes))
	for _, n := range notes {
		out = append(out, toProtoNote(n))
	}
	return &notesv1.ListMyNotesResponse{
		Notes: out,
		Page:  &notesv1.PageResponse{NextPageToken: next},
	}, nil
}

func (s *NotesServer) UpdateNote(ctx context.Context, req *notesv1.UpdateNoteRequest) (*notesv1.UpdateNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	note, err := s.svc.UpdateWithExpectedVersion(ctx, userID, req.GetId(), req.Title, req.Visibility, req.Content, req.GetExpectedVersion(), req.GetIdempotencyKey())
	if err != nil {
		return nil, mapError(err)
	}
	return &notesv1.UpdateNoteResponse{Note: toProtoNote(note)}, nil
}

func (s *NotesServer) DeleteNote(ctx context.Context, req *notesv1.DeleteNoteRequest) (*notesv1.DeleteNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.Delete(ctx, userID, req.GetId()); err != nil {
		return nil, mapError(err)
	}
	return &notesv1.DeleteNoteResponse{}, nil
}

func (s *NotesServer) ShareNote(ctx context.Context, req *notesv1.ShareNoteRequest) (*notesv1.ShareNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	shared, err := s.svc.Share(ctx, userID, req.GetNoteId(), req.GetGroupId(), req.GetAccessMode())
	if err != nil {
		return nil, mapError(err)
	}
	return &notesv1.ShareNoteResponse{SharedNoteId: shared.ID}, nil
}

func (s *NotesServer) UnshareNote(ctx context.Context, req *notesv1.UnshareNoteRequest) (*notesv1.UnshareNoteResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.svc.Unshare(ctx, userID, req.GetSharedNoteId()); err != nil {
		return nil, mapError(err)
	}
	return &notesv1.UnshareNoteResponse{}, nil
}

func (s *NotesServer) ListGroupNotes(ctx context.Context, req *notesv1.ListGroupNotesRequest) (*notesv1.ListGroupNotesResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	pairs, next, err := s.svc.ListGroupNotePairs(ctx, userID, req.GetGroupId(), req.GetPage().GetPageToken(), int(req.GetPage().GetPageSize()))
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*notesv1.NoteWithShare, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, &notesv1.NoteWithShare{
			Note:  toProtoNote(p.Note),
			Share: toProtoSharedNote(p.Shared),
		})
	}
	return &notesv1.ListGroupNotesResponse{
		Notes: out,
		Page:  &notesv1.PageResponse{NextPageToken: next},
	}, nil
}

func (s *NotesServer) GetNoteAccess(ctx context.Context, req *notesv1.GetNoteAccessRequest) (*notesv1.GetNoteAccessResponse, error) {
	userID, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	access, err := s.svc.GetAccess(ctx, userID, req.GetId())
	if err != nil {
		return nil, mapError(err)
	}
	return &notesv1.GetNoteAccessResponse{Access: toProtoAccess(access)}, nil
}

// UnshareAll es servicio-a-servicio (Social → Notes): el interceptor de
// x-internal-key ya autenticó al llamante, así que aquí no hay user_id del JWT.
// El user_id del request es el usuario objetivo de la limpieza.
func (s *NotesServer) UnshareAll(ctx context.Context, req *notesv1.UnshareAllRequest) (*notesv1.UnshareAllResponse, error) {
	if err := s.svc.UnshareAll(ctx, req.GetUserId(), req.GetGroupId()); err != nil {
		return nil, mapError(err)
	}
	return &notesv1.UnshareAllResponse{}, nil
}

// --- mapeo modelo → proto ---

func toProtoNote(n *model.Note) *notesv1.Note {
	if n == nil {
		return nil
	}
	p := &notesv1.Note{
		Id:               n.ID,
		UserId:           n.UserID,
		SubjectId:        n.SubjectID,
		Title:            n.Title,
		ExternalFileId:   n.ExternalFileID,
		Visibility:       n.Visibility,
		LikesCount:       int64(n.LikesCount),
		ForkedFromNoteId: n.ForkedFromNoteID,
		SyncStatus:       n.SyncStatus,
		Version:          n.Version,
		CreatedAt:        formatTime(n.CreatedAt),
		UpdatedAt:        formatTime(n.UpdatedAt),
		Content:          n.Content,
	}
	if n.ExternalFileID != nil && strings.TrimSpace(*n.ExternalFileID) != "" {
		u := "https://drive.google.com/file/d/" + *n.ExternalFileID + "/view"
		p.DriveUrl = &u
	}
	return p
}

func toProtoSharedNote(sh *model.SharedNote) *notesv1.SharedNote {
	if sh == nil {
		return nil
	}
	return &notesv1.SharedNote{
		Id:                      sh.ID,
		NoteId:                  sh.NoteID,
		GroupId:                 sh.GroupID,
		IsAdminNote:             sh.IsAdminNote,
		AccessMode:              sh.AccessMode,
		AuthorFollowersSnapshot: int32(sh.AuthorFollowersSnapshot),
		PermissionSyncStatus:    sh.PermissionSyncStatus,
		SharedAt:                formatTime(sh.SharedAt),
	}
}

func toProtoAccess(a *model.NoteAccessResponse) *notesv1.NoteAccess {
	if a == nil {
		return nil
	}
	return &notesv1.NoteAccess{
		CanRead:                 a.CanRead,
		CanWrite:                a.CanWrite,
		Visibility:              a.Visibility,
		AccessMode:              a.AccessMode,
		DriveSyncStatus:         a.DriveSyncStatus,
		DriveAccessVerified:     a.DriveAccessVerified,
		DriveConnectionRequired: a.DriveConnectionRequired,
		DriveUrl:                a.DriveURL,
	}
}

// formatTime serializa a RFC3339 UTC (mismo formato que el handler HTTP);
// zero -> "" para no emitir "0001-01-01T00:00:00Z".
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
