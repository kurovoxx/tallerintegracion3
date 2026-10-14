package grpc

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/google/uuid"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const testKey = "test-internal-key"

// fakeDirectory resuelve correos fijos; down simula Identity caído.
type fakeDirectory struct{ down bool }

func (f *fakeDirectory) LookupUsers(context.Context, []string) (map[string]service.UserPublic, error) {
	return nil, nil
}

func (f *fakeDirectory) LookupEmailsStrict(_ context.Context, ids []string) ([]string, error) {
	if f.down {
		return nil, errors.New("identity down")
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id+"@x.com")
	}
	return out, nil
}

// TestSocialServer recorre los 4 RPC sobre bufconn con la misma cadena de
// interceptores que main, usando el store en memoria.
func TestSocialServer(t *testing.T) {
	store := service.NewMemoryGroupStore()
	admin, member, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	g, _ := store.CreateWithOwner(context.Background(), "Grupo", nil, admin)
	store.AddMember(g.ID, member, model.RoleMember)

	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(nil),
		interceptor.NewInternalAuth(testKey),
	))
	dir := &fakeDirectory{}
	svc := service.NewGroupService(store, nil)
	svc.SetUserDirectory(dir)
	socialv1.RegisterSocialServiceServer(s, NewSocialServer(svc))
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	c := socialv1.NewSocialServiceClient(conn)

	ctx := metadata.AppendToOutgoingContext(context.Background(), interceptor.MetadataInternalKey, testKey)
	wantCode := func(t *testing.T, err error, code codes.Code) {
		t.Helper()
		if status.Code(err) != code {
			t.Fatalf("code = %v, want %v (err=%v)", status.Code(err), code, err)
		}
	}

	t.Run("sin clave interna", func(t *testing.T) {
		_, err := c.IsMember(context.Background(), &socialv1.IsMemberRequest{GroupId: g.ID, UserId: admin})
		wantCode(t, err, codes.Unauthenticated)
	})

	t.Run("IsMember/IsAdmin", func(t *testing.T) {
		cases := []struct {
			user            string
			isMember, admin bool
			role            socialv1.Role
		}{
			{admin, true, true, socialv1.Role_ROLE_ADMIN},
			{member, true, false, socialv1.Role_ROLE_MEMBER},
			{outsider, false, false, socialv1.Role_ROLE_UNSPECIFIED},
		}
		for _, tc := range cases {
			m, err := c.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: g.ID, UserId: tc.user})
			if err != nil || m.IsMember != tc.isMember || m.Role != tc.role {
				t.Fatalf("IsMember(%s) = %v, %v", tc.user, m, err)
			}
			a, err := c.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: g.ID, UserId: tc.user})
			if err != nil || a.IsAdmin != tc.admin {
				t.Fatalf("IsAdmin(%s) = %v, %v", tc.user, a, err)
			}
		}
	})

	t.Run("errores de entrada y grupo inexistente", func(t *testing.T) {
		_, err := c.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: "no-uuid", UserId: admin})
		wantCode(t, err, codes.InvalidArgument)
		_, err = c.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: uuid.NewString(), UserId: admin})
		wantCode(t, err, codes.NotFound)
		_, err = c.GetMembers(ctx, &socialv1.GetMembersRequest{GroupId: uuid.NewString()})
		wantCode(t, err, codes.NotFound)
		_, err = c.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{})
		wantCode(t, err, codes.InvalidArgument)
	})

	t.Run("GetMembers", func(t *testing.T) {
		resp, err := c.GetMembers(ctx, &socialv1.GetMembersRequest{GroupId: g.ID})
		if err != nil || len(resp.Members) != 2 {
			t.Fatalf("GetMembers = %v, %v", resp, err)
		}
		if resp.Members[0].UserId != admin || resp.Members[0].Role != socialv1.Role_ROLE_ADMIN || resp.Members[1].UserId != member {
			t.Fatalf("orden/rol inesperado: %v", resp.Members)
		}
	})

	t.Run("GetMemberEmails fail-closed", func(t *testing.T) {
		resp, err := c.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: g.ID})
		if err != nil || len(resp.Emails) != 2 || resp.Emails[0] != admin+"@x.com" {
			t.Fatalf("GetMemberEmails = %v, %v", resp, err)
		}
		_, err = c.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: uuid.NewString()})
		wantCode(t, err, codes.NotFound)
		dir.down = true
		_, err = c.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: g.ID})
		wantCode(t, err, codes.Unavailable)
		dir.down = false
	})

	t.Run("AccountDeletionCleanup promueve y es idempotente", func(t *testing.T) {
		resp, err := c.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: admin})
		if err != nil || len(resp.Groups) != 1 || resp.Groups[0].GetPromotedUserId() != member || resp.Groups[0].GroupDeleted {
			t.Fatalf("cleanup = %v, %v", resp, err)
		}
		a, err := c.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: g.ID, UserId: member})
		if err != nil || !a.IsAdmin {
			t.Fatalf("sucesor no es admin: %v, %v", a, err)
		}
		again, err := c.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: admin})
		if err != nil || len(again.Groups) != 0 {
			t.Fatalf("segunda llamada = %v, %v", again, err)
		}
	})
}
