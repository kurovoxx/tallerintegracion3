// Package grpc expone social.v1.SocialService (membresía, admin y limpieza por
// borrado de cuenta) para llamadas servicio-a-servicio. La clave interna la
// exige el interceptor (internal/interceptor), no este handler.
package grpc

import (
	"context"
	"errors"
	"log"

	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SocialServer struct {
	socialv1.UnimplementedSocialServiceServer
	groups *service.GroupService
}

func NewSocialServer(groups *service.GroupService) *SocialServer {
	return &SocialServer{groups: groups}
}

func (s *SocialServer) IsMember(ctx context.Context, req *socialv1.IsMemberRequest) (*socialv1.IsMemberResponse, error) {
	role, err := s.role(ctx, req.GetGroupId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &socialv1.IsMemberResponse{IsMember: role != "", Role: toRole(role)}, nil
}

func (s *SocialServer) IsAdmin(ctx context.Context, req *socialv1.IsAdminRequest) (*socialv1.IsAdminResponse, error) {
	role, err := s.role(ctx, req.GetGroupId(), req.GetUserId())
	if err != nil {
		return nil, err
	}
	return &socialv1.IsAdminResponse{IsAdmin: role == model.RoleAdmin}, nil
}

func (s *SocialServer) GetMembers(ctx context.Context, req *socialv1.GetMembersRequest) (*socialv1.GetMembersResponse, error) {
	if !utils.ValidateUUID(req.GetGroupId()) {
		return nil, status.Error(codes.InvalidArgument, "group_id inválido (UUID esperado)")
	}
	views, err := s.groups.GroupMembers(ctx, req.GetGroupId())
	if err != nil {
		return nil, toStatus(err)
	}
	members := make([]*socialv1.Member, 0, len(views))
	for _, v := range views {
		members = append(members, &socialv1.Member{
			UserId:      v.UserID,
			Role:        toRole(v.Role),
			JoinedAt:    timestamppb.New(v.JoinedAt),
			Email:       v.Email,
			DisplayName: v.DisplayName,
		})
	}
	return &socialv1.GetMembersResponse{Members: members}, nil
}

func (s *SocialServer) AccountDeletionCleanup(ctx context.Context, req *socialv1.AccountDeletionCleanupRequest) (*socialv1.AccountDeletionCleanupResponse, error) {
	if !utils.ValidateUUID(req.GetUserId()) {
		return nil, status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
	}
	results, err := s.groups.HandleAccountDeletion(ctx, req.GetUserId())
	if err != nil {
		return nil, toStatus(err)
	}
	groups := make([]*socialv1.GroupSuccession, 0, len(results))
	for _, r := range results {
		groups = append(groups, &socialv1.GroupSuccession{
			GroupId:        r.GroupID,
			PromotedUserId: r.PromotedUserID,
			GroupDeleted:   r.GroupDeleted,
		})
	}
	return &socialv1.AccountDeletionCleanupResponse{Groups: groups}, nil
}

// role valida los ids y devuelve el rol del usuario ("" si no es miembro).
func (s *SocialServer) role(ctx context.Context, groupID, userID string) (string, error) {
	if !utils.ValidateUUID(groupID) {
		return "", status.Error(codes.InvalidArgument, "group_id inválido (UUID esperado)")
	}
	if !utils.ValidateUUID(userID) {
		return "", status.Error(codes.InvalidArgument, "user_id inválido (UUID esperado)")
	}
	role, err := s.groups.MembershipRole(ctx, groupID, userID)
	if err != nil {
		return "", toStatus(err)
	}
	return role, nil
}

func toRole(role string) socialv1.Role {
	switch role {
	case model.RoleAdmin:
		return socialv1.Role_ROLE_ADMIN
	case model.RoleMember:
		return socialv1.Role_ROLE_MEMBER
	}
	return socialv1.Role_ROLE_UNSPECIFIED
}

// toStatus no filtra detalles de BD al cliente: solo NotFound es de dominio.
func toStatus(err error) error {
	if errors.Is(err, service.ErrNotFound) {
		return status.Error(codes.NotFound, "group_not_found")
	}
	log.Printf("social grpc: %v", err)
	return status.Error(codes.Internal, "internal")
}
