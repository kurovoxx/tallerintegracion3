package grpc

import (
	"context"
	"strings"

	"github.com/google/uuid"
	academicv1 "github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/gen/academicv1"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubjectGetter resuelve una materia propia (satisfecho por
// *repository.SubjectRepository). Nil = inexistente o ajena.
type SubjectGetter interface {
	GetByID(ctx context.Context, subjectID, userID string) (*model.Subject, error)
}

// AcademicServer implementa academic.v1.AcademicService para Notes:
// lookup de materia con ownership (ajeno e inexistente → NOT_FOUND,
// sin oráculo).
type AcademicServer struct {
	academicv1.UnimplementedAcademicServiceServer
	subjects SubjectGetter
}

func NewAcademicServer(subjects SubjectGetter) *AcademicServer {
	return &AcademicServer{subjects: subjects}
}

// GetSubject resuelve una materia propia del solicitante.
func (s *AcademicServer) GetSubject(ctx context.Context, req *academicv1.GetSubjectRequest) (*academicv1.GetSubjectResponse, error) {
	subjectID := strings.TrimSpace(req.GetSubjectId())
	requester := strings.TrimSpace(req.GetRequesterUserId())
	if _, err := uuid.Parse(subjectID); err != nil {
		return nil, status.Error(codes.InvalidArgument, "subject_id inválido (UUID esperado)")
	}
	if requester == "" {
		return nil, status.Error(codes.InvalidArgument, "requester_user_id requerido")
	}
	sub, err := s.subjects.GetByID(ctx, subjectID, requester)
	if err != nil {
		return nil, status.Error(codes.Internal, "error interno")
	}
	if sub == nil {
		return nil, status.Error(codes.NotFound, "materia no encontrada")
	}
	return &academicv1.GetSubjectResponse{
		Exists:      true,
		SubjectId:   sub.ID,
		OwnerUserId: requester,
		Name:        sub.Name,
		Status:      sub.Status,
	}, nil
}
