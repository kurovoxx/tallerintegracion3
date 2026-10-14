package service

import (
	"context"
	"net"
	"testing"

	socialv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/socialv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	grpcGroup   = "11111111-1111-4111-8111-111111111111"
	grpcAdmin   = "22222222-2222-4222-8222-222222222222"
	grpcMissing = "33333333-3333-4333-8333-333333333333"
	grpcDown    = "44444444-4444-4444-8444-444444444444"
)

// fakeSocial imita social.v1.SocialService: un grupo con un admin, uno
// inexistente (NotFound) y uno con Identity caído (Unavailable). Rechaza
// llamadas sin la clave interna como lo hace el interceptor real.
type fakeSocial struct {
	socialv1.UnimplementedSocialServiceServer
}

func (fakeSocial) check(ctx context.Context, groupID string) error {
	md, _ := metadata.FromIncomingContext(ctx)
	if k := md.Get("x-internal-key"); len(k) == 0 || k[0] != "k" {
		return status.Error(codes.Unauthenticated, "invalid internal key")
	}
	switch groupID {
	case grpcMissing:
		return status.Error(codes.NotFound, "group_not_found")
	case grpcDown:
		return status.Error(codes.Unavailable, "identity_unavailable")
	}
	return nil
}

func (f fakeSocial) IsMember(ctx context.Context, r *socialv1.IsMemberRequest) (*socialv1.IsMemberResponse, error) {
	if err := f.check(ctx, r.GroupId); err != nil {
		return nil, err
	}
	return &socialv1.IsMemberResponse{IsMember: r.UserId == grpcAdmin}, nil
}

func (f fakeSocial) IsAdmin(ctx context.Context, r *socialv1.IsAdminRequest) (*socialv1.IsAdminResponse, error) {
	if err := f.check(ctx, r.GroupId); err != nil {
		return nil, err
	}
	return &socialv1.IsAdminResponse{IsAdmin: r.UserId == grpcAdmin}, nil
}

func (f fakeSocial) GetMemberEmails(ctx context.Context, r *socialv1.GetMemberEmailsRequest) (*socialv1.GetMemberEmailsResponse, error) {
	if err := f.check(ctx, r.GroupId); err != nil {
		return nil, err
	}
	return &socialv1.GetMemberEmailsResponse{Emails: []string{"a@x.com"}}, nil
}

func dialFakeSocial(t *testing.T, key string) *SocialGRPCAdapter {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	s := grpc.NewServer()
	socialv1.RegisterSocialServiceServer(s, fakeSocial{})
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return newSocialGRPCAdapter(socialv1.NewSocialServiceClient(conn), 0, key)
}

func TestSocialGRPCAdapter(t *testing.T) {
	a := dialFakeSocial(t, "k")
	ctx := context.Background()

	if ok, err := a.IsMember(ctx, grpcAdmin, grpcGroup); err != nil || !ok {
		t.Fatalf("IsMember admin = %v, %v", ok, err)
	}
	if ok, err := a.IsAdmin(ctx, "otro", grpcGroup); err != nil || ok {
		t.Fatalf("IsAdmin otro = %v, %v", ok, err)
	}
	// NotFound = ausencia, igual que el 404 del adaptador HTTP.
	if ok, err := a.IsMember(ctx, grpcAdmin, grpcMissing); err != nil || ok {
		t.Fatalf("IsMember grupo inexistente = %v, %v", ok, err)
	}
	if emails, err := a.ListMemberEmails(ctx, grpcMissing); err != nil || emails != nil {
		t.Fatalf("ListMemberEmails grupo inexistente = %v, %v", emails, err)
	}
	if emails, err := a.ListMemberEmails(ctx, grpcGroup); err != nil || len(emails) != 1 {
		t.Fatalf("ListMemberEmails = %v, %v", emails, err)
	}
	// Fail-closed (RN-SHARE-10): Identity caído nunca es lista vacía.
	if _, err := a.ListMemberEmails(ctx, grpcDown); err == nil {
		t.Fatal("ListMemberEmails con Identity caído debe fallar")
	}
	if n, err := a.GetFollowersCount(ctx, grpcAdmin); err != nil || n != 0 {
		t.Fatalf("GetFollowersCount = %v, %v", n, err)
	}
}

func TestSocialGRPCAdapterWrongKeyFailsClosed(t *testing.T) {
	a := dialFakeSocial(t, "mala")
	if _, err := a.IsMember(context.Background(), grpcAdmin, grpcGroup); err == nil {
		t.Fatal("clave inválida debe propagarse como error, no como no-miembro")
	}
}
