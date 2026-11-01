package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Mantiene el store real en memoria y simula fallos de persistencia solo en
// los métodos consultados por estos RPC.
type failingGroupStore struct {
	*service.MemoryGroupStore
}

var errPrivateStore = errors.New("postgres password=private-test-value")

func (s *failingGroupStore) GetByID(context.Context, string) (*model.Group, error) {
	return nil, errPrivateStore
}

func (s *failingGroupStore) ListMembers(context.Context, string) ([]*model.GroupMembership, error) {
	return nil, errPrivateStore
}

func (s *failingGroupStore) HandleAccountDeletion(context.Context, string) ([]model.SuccessionResult, error) {
	return nil, errPrivateStore
}

func TestSocialServerDoesNotExposePersistenceErrors(t *testing.T) {
	server := NewSocialServer(service.NewGroupService(&failingGroupStore{service.NewMemoryGroupStore()}, nil))
	id := uuid.NewString()
	ctx := context.Background()
	tests := []struct {
		name string
		call func() error
		code codes.Code
		msg  string
	}{
		{"IsMember", func() error {
			_, err := server.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: id, UserId: id})
			return err
		}, codes.Internal, "internal"},
		{"IsAdmin", func() error {
			_, err := server.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: id, UserId: id})
			return err
		}, codes.Internal, "internal"},
		{"GetMembers", func() error {
			_, err := server.GetMembers(ctx, &socialv1.GetMembersRequest{GroupId: id})
			return err
		}, codes.Internal, "internal"},
		{"GetMemberEmails", func() error {
			_, err := server.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: id})
			return err
		}, codes.Unavailable, "identity_unavailable"},
		{"AccountDeletionCleanup", func() error {
			_, err := server.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: id})
			return err
		}, codes.Internal, "internal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := status.Convert(tc.call())
			if got.Code() != tc.code || got.Message() != tc.msg {
				t.Fatalf("status = %v; esperado %v: %s", got, tc.code, tc.msg)
			}
		})
	}
}

func TestGetMemberEmailsWithoutIdentityFailsClosed(t *testing.T) {
	store := service.NewMemoryGroupStore()
	g, err := store.CreateWithOwner(context.Background(), "Grupo", nil, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	server := NewSocialServer(service.NewGroupService(store, nil))
	resp, err := server.GetMemberEmails(context.Background(), &socialv1.GetMemberEmailsRequest{GroupId: g.ID})
	if status.Code(err) != codes.Unavailable || resp != nil {
		t.Fatalf("sin Identity: respuesta=%v error=%v; esperado Unavailable sin lista parcial", resp, err)
	}
}
