package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/utils"
)

// Gate de confirmación en Login y Refresh (EMAIL_VERIFICATION_REQUIRED).

type emailGateFake struct {
	verified map[string]bool
	err      error
	calls    int
}

func (g *emailGateFake) IsEmailVerified(_ context.Context, userID string) (bool, error) {
	g.calls++
	return g.verified[userID], g.err
}

type accountGateFake struct{ active bool }

func (g accountGateFake) AcquireRequest(context.Context, string) (func(), bool, error) {
	return func() {}, g.active, nil
}

func newGatedAuthService(t *testing.T) (*AuthService, *mockUserRepo, *mockRefreshRepo, *emailGateFake) {
	t.Helper()
	users, refresh := newMockUserRepo(), newMockRefreshRepo()
	svc := NewAuthService(users, refresh, newJWTForTest(t), 900, 604800)
	gate := &emailGateFake{verified: map[string]bool{}}
	svc.SetEmailVerificationGate(gate)
	return svc, users, refresh, gate
}

func serviceCode(err error) string {
	var se *ServiceError
	if errors.As(err, &se) {
		return se.Code
	}
	return ""
}

func TestEmailGate_LoginBlocksUnverifiedAndAllowsVerified(t *testing.T) {
	svc, _, refresh, gate := newGatedAuthService(t)
	ctx := context.Background()
	user, err := svc.Register(ctx, "gate@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Login(ctx, "gate@example.invalid", "Pass1234")
	if serviceCode(err) != utils.ErrEmailNotVerified || res != nil {
		t.Fatalf("cuenta sin confirmar: %v %v", res, err)
	}
	if len(refresh.tokens) != 0 {
		t.Fatal("no debe emitirse ningún refresh token")
	}
	if utils.StatusForCode(utils.ErrEmailNotVerified) != 403 {
		t.Fatal("email_not_verified se mapea a 403 en login")
	}
	gate.verified[user.ID] = true
	if res, err := svc.Login(ctx, "gate@example.invalid", "Pass1234"); err != nil || res.AccessToken == "" {
		t.Fatalf("cuenta confirmada debe iniciar sesión: %v", err)
	}
}

func TestEmailGate_CredentialErrorsTakePrecedence(t *testing.T) {
	svc, _, _, gate := newGatedAuthService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "prec@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ email, password string }{
		{"prec@example.invalid", "WrongPass1"}, // contraseña incorrecta de una cuenta sin confirmar
		{"nadie@example.invalid", "Pass1234"},  // cuenta inexistente
	} {
		gate.calls = 0
		if _, err := svc.Login(ctx, tc.email, tc.password); serviceCode(err) != utils.ErrInvalidCredentials {
			t.Fatalf("%s: debe ser invalid_credentials (no revelar la cuenta), got %v", tc.email, err)
		}
		if gate.calls != 0 {
			t.Fatal("el gate solo se consulta con credenciales válidas")
		}
	}
	// Cuenta en borrado: sigue siendo invalid_credentials, antes del gate.
	svc.SetAccountAccessGate(accountGateFake{active: false})
	if _, err := svc.Login(ctx, "prec@example.invalid", "Pass1234"); serviceCode(err) != utils.ErrInvalidCredentials || gate.calls != 0 {
		t.Fatalf("cuenta en borrado: %v (gate calls %d)", err, gate.calls)
	}
}

func TestEmailGate_LoginGateErrorIsInternal(t *testing.T) {
	svc, _, refresh, gate := newGatedAuthService(t)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "gerr@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	gate.err = errors.New("connection refused")
	if _, err := svc.Login(ctx, "gerr@example.invalid", "Pass1234"); serviceCode(err) != "internal_error" {
		t.Fatalf("fallo al consultar: internal_error, got %v", err)
	}
	if len(refresh.tokens) != 0 {
		t.Fatal("ante la duda no se emite sesión (fail-closed)")
	}
}

func TestEmailGate_DisabledByDefault(t *testing.T) {
	svc := NewAuthService(newMockUserRepo(), newMockRefreshRepo(), newJWTForTest(t), 900, 604800)
	ctx := context.Background()
	if _, err := svc.Register(ctx, "off@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Login(ctx, "off@example.invalid", "Pass1234")
	if err != nil {
		t.Fatalf("sin gate el login no cambia: %v", err)
	}
	if _, err := svc.Refresh(ctx, res.RefreshToken); err != nil {
		t.Fatalf("sin gate el refresh no cambia: %v", err)
	}
}

func TestEmailGate_RefreshCannotExtendUnverifiedSession(t *testing.T) {
	users, refresh := newMockUserRepo(), newMockRefreshRepo()
	jwtSvc := newJWTForTest(t)
	ctx := context.Background()
	// Sesión obtenida antes de activar el gate.
	before := NewAuthService(users, refresh, jwtSvc, 900, 604800)
	user, err := before.Register(ctx, "old-session@example.invalid", "Pass1234", nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	login, err := before.Login(ctx, "old-session@example.invalid", "Pass1234")
	if err != nil {
		t.Fatal(err)
	}
	// Gate activado.
	after := NewAuthService(users, refresh, jwtSvc, 900, 604800)
	gate := &emailGateFake{verified: map[string]bool{}}
	after.SetEmailVerificationGate(gate)
	if _, err := after.Refresh(ctx, login.RefreshToken); serviceCode(err) != utils.ErrEmailNotVerified {
		t.Fatalf("refresh de cuenta sin confirmar: %v", err)
	}
	// El refresh no se revoca: tras confirmar, la misma sesión continúa.
	gate.verified[user.ID] = true
	time.Sleep(1100 * time.Millisecond) // iat distinto en el JWT rotado
	if res, err := after.Refresh(ctx, login.RefreshToken); err != nil || res.RefreshToken == "" {
		t.Fatalf("tras confirmar, refresh válido: %v", err)
	}
}

func TestEmailGate_RefreshInvalidTokenTakesPrecedence(t *testing.T) {
	svc, _, _, gate := newGatedAuthService(t)
	if _, err := svc.Refresh(context.Background(), "no-existe"); serviceCode(err) != utils.ErrInvalidToken || gate.calls != 0 {
		t.Fatalf("token inexistente: invalid_token sin consultar el gate, got %v", err)
	}
}
