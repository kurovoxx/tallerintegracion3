package fake

import (
	"context"
	"testing"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFakeServer_AddUserAndGetUser(t *testing.T) {
	srv := NewServer()
	srv.AddUser("u1", "alice@example.com", "Alice")

	u, err := srv.GetUser(context.Background(), &identityv1.GetUserRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("GetUser no debe fallar: %v", err)
	}
	if u.GetUserId() != "u1" || u.GetEmail() != "alice@example.com" || u.GetDisplayName() != "Alice" {
		t.Fatalf("usuario inesperado: %+v", u)
	}
}

func TestFakeServer_GetUserNotFound(t *testing.T) {
	srv := NewServer()

	_, err := srv.GetUser(context.Background(), &identityv1.GetUserRequest{UserId: "no-existe"})
	if err == nil {
		t.Fatal("debería devolver error")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("esperaba NotFound, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_GetUserEmptyID(t *testing.T) {
	srv := NewServer()

	_, err := srv.GetUser(context.Background(), &identityv1.GetUserRequest{UserId: "  "})
	if err == nil {
		t.Fatal("debería devolver error por ID vacío")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("esperaba InvalidArgument, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_LookupUsers(t *testing.T) {
	srv := NewServer()
	srv.AddUser("u1", "alice@example.com", "Alice")
	srv.AddUser("u2", "bob@example.com", "Bob")

	resp, err := srv.LookupUsers(context.Background(), &identityv1.LookupUsersRequest{UserIds: []string{"u1", "u2"}})
	if err != nil {
		t.Fatalf("LookupUsers no debe fallar: %v", err)
	}
	if len(resp.GetUsers()) != 2 {
		t.Fatalf("esperaba 2 usuarios, obtuvo %d", len(resp.GetUsers()))
	}
}

func TestFakeServer_LookupUsersSkipsMissing(t *testing.T) {
	srv := NewServer()
	srv.AddUser("u1", "alice@example.com")

	resp, err := srv.LookupUsers(context.Background(), &identityv1.LookupUsersRequest{UserIds: []string{"u1", "no-existe"}})
	if err != nil {
		t.Fatalf("LookupUsers no debe fallar por usuarios faltantes: %v", err)
	}
	if len(resp.GetUsers()) != 1 {
		t.Fatalf("esperaba 1 usuario, obtuvo %d", len(resp.GetUsers()))
	}
}

func TestFakeServer_VerifyToken(t *testing.T) {
	srv := NewServer()
	srv.AddToken("tok-valido", "u1")

	userID, err := srv.VerifyToken(context.Background(), &identityv1.VerifyTokenRequest{Token: "tok-valido"})
	if err != nil {
		t.Fatalf("VerifyToken no debe fallar para token válido: %v", err)
	}
	if userID.GetUserId() != "u1" || !userID.GetValid() {
		t.Fatalf("usuario inesperado: %+v", userID)
	}
}

func TestFakeServer_VerifyTokenInvalid(t *testing.T) {
	srv := NewServer()
	srv.AddToken("tok-valido", "u1")

	_, err := srv.VerifyToken(context.Background(), &identityv1.VerifyTokenRequest{Token: "tok-malo"})
	if err == nil {
		t.Fatal("debería devolver error para token inválido")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("esperaba Unauthenticated, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_VerifyTokenEmpty(t *testing.T) {
	srv := NewServer()

	_, err := srv.VerifyToken(context.Background(), &identityv1.VerifyTokenRequest{Token: "  "})
	if err == nil {
		t.Fatal("debería devolver error para token vacío")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("esperaba InvalidArgument, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_SetFailVerify(t *testing.T) {
	srv := NewServer()
	srv.AddToken("tok-valido", "u1")
	srv.SetFailVerify(true)

	_, err := srv.VerifyToken(context.Background(), &identityv1.VerifyTokenRequest{Token: "tok-valido"})
	if err == nil {
		t.Fatal("debería devolver error con failVerify=true")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("esperaba Unauthenticated, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_GetOAuthToken(t *testing.T) {
	srv := NewServer()
	srv.AddOAuthToken("u1", "access-token-abc")

	resp, err := srv.GetOAuthToken(context.Background(), &identityv1.GetOAuthTokenRequest{UserId: "u1", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	if err != nil {
		t.Fatalf("GetOAuthToken no debe fallar: %v", err)
	}
	if resp.GetAccessToken() != "access-token-abc" {
		t.Fatalf("access token inesperado: %q", resp.GetAccessToken())
	}
}

func TestFakeServer_GetOAuthTokenNotFound(t *testing.T) {
	srv := NewServer()

	_, err := srv.GetOAuthToken(context.Background(), &identityv1.GetOAuthTokenRequest{UserId: "u1", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR})
	if err == nil {
		t.Fatal("debería devolver error para usuario sin OAuth")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("esperaba NotFound, obtuvo %v", status.Code(err))
	}
}

func TestFakeServer_ReportCalendarRevoked(t *testing.T) {
	srv := NewServer()
	srv.AddOAuthToken("u1", "access-token-abc")

	_, err := srv.ReportCalendarRevoked(context.Background(), &identityv1.ReportCalendarRevokedRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("ReportCalendarRevoked no debe fallar: %v", err)
	}

	// El token OAuth debe estar borrado (idempotente)
	if _, err := srv.GetOAuthToken(context.Background(), &identityv1.GetOAuthTokenRequest{UserId: "u1", Provider: identityv1.Provider_PROVIDER_GOOGLE_CALENDAR}); err == nil {
		t.Fatal("después de revocar, GetOAuthToken debería dar NotFound")
	}
}

func TestFakeClient_VerifyToken(t *testing.T) {
	srv, client := NewServerAndClient()
	defer client.Close()

	srv.AddToken("tok-cliente", "u-cliente")

	userID, err := client.VerifyToken(context.Background(), "tok-cliente")
	if err != nil {
		t.Fatalf("VerifyToken no debe fallar: %v", err)
	}
	if userID != "u-cliente" {
		t.Fatalf("usuario inesperado: %s", userID)
	}
}

func TestFakeClient_GetUser(t *testing.T) {
	srv, client := NewServerAndClient()
	defer client.Close()

	srv.AddUser("u-cliente", "cliente@example.com", "Cliente")

	u, err := client.GetUser(context.Background(), "u-cliente")
	if err != nil {
		t.Fatalf("GetUser no debe fallar: %v", err)
	}
	if u.UserID != "u-cliente" || u.Email != "cliente@example.com" || u.DisplayName == nil || *u.DisplayName != "Cliente" {
		t.Fatalf("usuario inesperado: %+v", u)
	}
}

func TestFakeClient_GetUserNotFound(t *testing.T) {
	_, client := NewServerAndClient()
	defer client.Close()

	_, err := client.GetUser(context.Background(), "no-existe")
	if err == nil {
		t.Fatal("debería devolver error")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("esperaba NotFound, obtuvo %v", status.Code(err))
	}
}
