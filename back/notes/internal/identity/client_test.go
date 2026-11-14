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
