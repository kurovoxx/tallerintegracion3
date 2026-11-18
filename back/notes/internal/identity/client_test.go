package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/identityv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// fakeIdentityClient embebe la interfaz completa y solo implementa el RPC que
// usa Notes; así el test no depende de un servidor gRPC real.
type fakeIdentityClient struct {
	identityv1.IdentityServiceClient
	resp   *identityv1.ResolvePublicProfileResponse
	err    error
	gotReq *identityv1.ResolvePublicProfileRequest
	gotMD  metadata.MD
}

func (f *fakeIdentityClient) ResolvePublicProfile(ctx context.Context, in *identityv1.ResolvePublicProfileRequest, _ ...grpc.CallOption) (*identityv1.ResolvePublicProfileResponse, error) {
	f.gotReq = in
	f.gotMD, _ = metadata.FromOutgoingContext(ctx)
	return f.resp, f.err
}

func TestPublicProfileClient_ResuelveYNormaliza(t *testing.T) {
	fake := &fakeIdentityClient{resp: &identityv1.ResolvePublicProfileResponse{UserId: "550e8400-e29b-41d4-a716-446655440001"}}
	client := NewPublicProfileClient(fake, "clave-interna", time.Second)

	got, err := client.ResolvePublicProfile(context.Background(), "  A8KP92MQWX7R  ")
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if got != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("user_id = %q", got)
	}
	if fake.gotReq.GetPublicId() != "a8kp92mqwx7r" {
		t.Fatalf("public_id enviado = %q, want minúsculas", fake.gotReq.GetPublicId())
	}
	if values := fake.gotMD.Get(metadataInternalKey); len(values) != 1 || values[0] != "clave-interna" {
		t.Fatalf("metadata %s = %v", metadataInternalKey, values)
	}
}

func TestPublicProfileClient_MapeaErroresGRPC(t *testing.T) {
	cases := []struct {
		name string
		code codes.Code
		want error
	}{
		{"not found", codes.NotFound, ErrNotFound},
		{"invalid argument", codes.InvalidArgument, ErrInvalid},
		{"unavailable", codes.Unavailable, ErrUnavailable},
		{"deadline", codes.DeadlineExceeded, ErrUnavailable},
		{"unauthenticated", codes.Unauthenticated, ErrUnavailable},
		{"permission denied", codes.PermissionDenied, ErrUnavailable},
		{"internal", codes.Internal, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := NewPublicProfileClient(&fakeIdentityClient{err: status.Error(tc.code, "fallo")}, "k", time.Second)
			_, err := client.ResolvePublicProfile(context.Background(), "a8kp92mqwx7r")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// fakeLookupClient implementa LookupUsers para LookupPublicIDs.
type fakeLookupClient struct {
	identityv1.IdentityServiceClient
	users  []*identityv1.User
	err    error
	gotReq *identityv1.LookupUsersRequest
	gotMD  metadata.MD
}

func (f *fakeLookupClient) LookupUsers(ctx context.Context, in *identityv1.LookupUsersRequest, _ ...grpc.CallOption) (*identityv1.LookupUsersResponse, error) {
	f.gotReq = in
	f.gotMD, _ = metadata.FromOutgoingContext(ctx)
	if f.err != nil {
		return nil, f.err
	}
	return &identityv1.LookupUsersResponse{Users: f.users}, nil
}

func TestPublicProfileClient_LookupPublicIDs(t *testing.T) {
	code := "a8kp92mqwx7r"
	fake := &fakeLookupClient{users: []*identityv1.User{
		{UserId: "u-1", Email: "no-debe-salir@test.invalid", PublicId: &code},
		{UserId: "u-2", Email: "sin-codigo@test.invalid"},
	}}
	client := NewPublicProfileClient(fake, "clave-interna", time.Second)
	got, err := client.LookupPublicIDs(context.Background(), []string{"u-1", "u-2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["u-1"] != code {
		t.Fatalf("solo debe mapear códigos presentes: %v", got)
	}
	if values := fake.gotMD.Get(metadataInternalKey); len(values) != 1 || values[0] != "clave-interna" {
		t.Fatalf("metadata %s = %v", metadataInternalKey, values)
	}

	empty := &fakeLookupClient{}
	if got, err := NewPublicProfileClient(empty, "k", time.Second).LookupPublicIDs(context.Background(), nil); err != nil || len(got) != 0 || empty.gotReq != nil {
		t.Fatalf("sin ids no debe llamar a Identity: %v %v", got, err)
	}

	down := NewPublicProfileClient(&fakeLookupClient{err: status.Error(codes.Unavailable, "x")}, "k", time.Second)
	if _, err := down.LookupPublicIDs(context.Background(), []string{"u-1"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
