package drive

import (
	"context"
	"testing"
)

// CheckAccess solo consulta el token (Identity); nunca llama a la API de Drive.
func TestRealDriveClientCheckAccess(t *testing.T) {
	ok := &stubTokenProvider{token: "t"}
	if err := NewRealDriveClient(ok).CheckAccess(context.Background(), "u"); err != nil {
		t.Fatalf("con token vigente no debe fallar: %v", err)
	}
	if ok.calls != 1 {
		t.Fatalf("debe pedir el token una vez, calls=%d", ok.calls)
	}

	none := &stubTokenProvider{err: &OAuthError{Message: "Conecte o renueve su Google Drive"}}
	if err := NewRealDriveClient(none).CheckAccess(context.Background(), "u"); !IsOAuthError(err) {
		t.Fatalf("sin conexión debe devolver OAuthError, got %v", err)
	}

	down := &stubTokenProvider{err: &DriveError{Code: 500, Message: "Servicio de almacenamiento no disponible"}}
	err := NewRealDriveClient(down).CheckAccess(context.Background(), "u")
	if err == nil || IsOAuthError(err) {
		t.Fatalf("Identity caído no debe confundirse con «no conectado»: %v", err)
	}
}

func TestMockClientCheckAccess(t *testing.T) {
	m := NewMockClient()
	if err := m.CheckAccess(context.Background(), "u"); err != nil {
		t.Fatalf("el mock conecta a todos por defecto: %v", err)
	}
	m.Disconnect("u")
	if err := m.CheckAccess(context.Background(), "u"); !IsOAuthError(err) {
		t.Fatalf("tras Disconnect debe devolver OAuthError, got %v", err)
	}
}
