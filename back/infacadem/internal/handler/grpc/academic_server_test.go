package grpc

import (
	"context"
	"net"
	"os"
	"testing"

	academicv1 "github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/gen/academicv1"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const (
	grpcUserA = "11111111-1111-4111-8111-111111111111"
	grpcUserB = "22222222-2222-4222-8222-222222222222"
	grpcSubID = "33333333-3333-4333-8333-333333333333"
)

type fakeSubjectGetter struct {
	items  map[string]*model.Subject
	owners map[string]string
}

func (f *fakeSubjectGetter) GetByID(_ context.Context, id, userID string) (*model.Subject, error) {
	if s, ok := f.items[id]; ok && f.owners[id] == userID {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}

func dialBufconn(t *testing.T, srv *AcademicServer) academicv1.AcademicServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	academicv1.RegisterAcademicServiceServer(s, srv)
	go func() {
		if err := s.Serve(lis); err != nil {
			t.Logf("serve: %v", err)
		}
	}()
	t.Cleanup(s.GracefulStop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return academicv1.NewAcademicServiceClient(conn)
}

func newFakeServer() *AcademicServer {
	return NewAcademicServer(&fakeSubjectGetter{
		items: map[string]*model.Subject{
			grpcSubID: {ID: grpcSubID, Name: "Cálculo", Status: "in_progress"},
		},
		owners: map[string]string{grpcSubID: grpcUserA},
	})
}

func TestGetSubject_OK(t *testing.T) {
	client := dialBufconn(t, newFakeServer())
	resp, err := client.GetSubject(context.Background(), &academicv1.GetSubjectRequest{
		SubjectId: grpcSubID, RequesterUserId: grpcUserA,
	})
	if err != nil {
		t.Fatalf("GetSubject: %v", err)
	}
	if !resp.GetExists() || resp.GetSubjectId() != grpcSubID || resp.GetOwnerUserId() != grpcUserA ||
		resp.GetName() != "Cálculo" || resp.GetStatus() != "in_progress" {
		t.Fatalf("respuesta: %+v", resp)
	}
}

func TestGetSubject_NotFound_SinOraculo(t *testing.T) {
	client := dialBufconn(t, newFakeServer())
	// Ajena e inexistente responden igual (mismo código, sin oráculo).
	for _, tc := range []struct {
		name, sub, user string
	}{
		{"ajena", grpcSubID, grpcUserB},
		{"inexistente", "44444444-4444-4444-8444-444444444444", grpcUserA},
	} {
		_, err := client.GetSubject(context.Background(), &academicv1.GetSubjectRequest{
			SubjectId: tc.sub, RequesterUserId: tc.user,
		})
		st, _ := status.FromError(err)
		if st.Code() != codes.NotFound {
			t.Fatalf("%s: esperado NOT_FOUND, got %v", tc.name, err)
		}
	}
}

func TestGetSubject_InvalidArgument(t *testing.T) {
	client := dialBufconn(t, newFakeServer())
	for _, tc := range []struct {
		name, sub, user string
	}{
		{"uuid malo", "no-uuid", grpcUserA},
		{"uuid vacío", "", grpcUserA},
		{"sin solicitante", grpcSubID, ""},
		{"solicitante espacios", grpcSubID, "   "},
	} {
		_, err := client.GetSubject(context.Background(), &academicv1.GetSubjectRequest{
			SubjectId: tc.sub, RequesterUserId: tc.user,
		})
		st, _ := status.FromError(err)
		if st.Code() != codes.InvalidArgument {
			t.Fatalf("%s: esperado INVALID_ARGUMENT, got %v", tc.name, err)
		}
	}
}

// Apagado gracioso: con una llamada en curso, GracefulStop drena sin colgar.
func TestServer_GracefulStop(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	academicv1.RegisterAcademicServiceServer(s, newFakeServer())
	done := make(chan error, 1)
	go func() { done <- s.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := academicv1.NewAcademicServiceClient(conn)
	if _, err := client.GetSubject(context.Background(), &academicv1.GetSubjectRequest{
		SubjectId: grpcSubID, RequesterUserId: grpcUserA,
	}); err != nil {
		t.Fatal(err)
	}
	s.GracefulStop()
	if err := <-done; err != nil {
		t.Fatalf("serve tras GracefulStop: %v", err)
	}
}

// Contra Postgres real (opt-in, skip sin ella): mismo contrato con el
// repository de verdad. Requiere ACADEMIC_TEST_DATABASE_URL a BD vacía.
// Nota: el paquete service usa la misma BD y el mismo schema; con la env
// fijada, correr el suite completo con -p 1 (misma solución que back/auth).
func TestGetSubject_ConPostgresReal(t *testing.T) {
	url := os.Getenv("ACADEMIC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ACADEMIC_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := newTestPool(ctx, t, url)
	if err != nil {
		t.Fatal(err)
	}
	// Sin defer pool.Close(): el ownership (DROP SCHEMA + Close) es del
	// helper vía t.Cleanup. Cerrar aquí antes deja el schema huérfano
	// (el DROP posterior corre sobre el pool cerrado y falla silencioso).
	curSvc := service.NewCurriculumService(repository.NewCurriculumRepository(pool))
	subSvc := service.NewSubjectService(repository.NewSubjectRepository(pool))
	malla, err := curSvc.Create(ctx, grpcUserA, "Malla gRPC")
	if err != nil {
		t.Fatal(err)
	}
	name := "Física"
	mat, err := subSvc.Create(ctx, grpcUserA, malla.ID, service.SubjectInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}
	client := dialBufconn(t, NewAcademicServer(repository.NewSubjectRepository(pool)))
	resp, err := client.GetSubject(ctx, &academicv1.GetSubjectRequest{
		SubjectId: mat.ID, RequesterUserId: grpcUserA,
	})
	if err != nil {
		t.Fatalf("propia: %v", err)
	}
	if !resp.GetExists() || resp.GetName() != "Física" || resp.GetOwnerUserId() != grpcUserA {
		t.Fatalf("respuesta: %+v", resp)
	}
	_, err = client.GetSubject(ctx, &academicv1.GetSubjectRequest{
		SubjectId: mat.ID, RequesterUserId: grpcUserB,
	})
	if st, _ := status.FromError(err); st.Code() != codes.NotFound {
		t.Fatalf("ajena: esperado NOT_FOUND, got %v", err)
	}
}
