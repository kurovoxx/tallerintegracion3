package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestMapGoogleErrorCodes(t *testing.T) {
	cases := []struct {
		name     string
		google   *googleapi.Error
		wantCode int
	}{
		{"404→404", &googleapi.Error{Code: 404}, 404},
		{"403→403", &googleapi.Error{Code: 403}, 403},
		{"401→403", &googleapi.Error{Code: 401}, 403},
		{"400→400", &googleapi.Error{Code: 400}, 400},
		{"413→413", &googleapi.Error{Code: 413}, 413},
		{"500→500", &googleapi.Error{Code: 500}, 500},
		{"429→500", &googleapi.Error{Code: 429}, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := mapGoogleError("op", tc.google)
			de, ok := err.(*DriveError)
			if !ok {
				t.Fatalf("esperaba *DriveError, got %T %v", err, err)
			}
			if de.Code != tc.wantCode {
				t.Fatalf("code esperado %d, got %d (%v)", tc.wantCode, de.Code, err)
			}
		})
	}
}

func TestMapGoogleErrorNilAndGeneric(t *testing.T) {
	if err := mapGoogleError("op", nil); err != nil {
		t.Fatalf("nil debe mapear a nil, got %v", err)
	}
	err := mapGoogleError("op", context.DeadlineExceeded)
	de, ok := err.(*DriveError)
	if !ok || de.Code != 500 {
		t.Fatalf("error genérico debe mapear a 500, got %v", err)
	}
}

func TestIsNotFoundForbiddenHelpers(t *testing.T) {
	if !IsNotFound(&DriveError{Code: 404}) {
		t.Fatal("IsNotFound debe ser true para 404")
	}
	if IsNotFound(&DriveError{Code: 403}) {
		t.Fatal("IsNotFound debe ser false para 403")
	}
	if !IsForbidden(&DriveError{Code: 403}) {
		t.Fatal("IsForbidden debe ser true para 403")
	}
	if IsForbidden(&DriveError{Code: 404}) {
		t.Fatal("IsForbidden debe ser false para 404")
	}
	if IsNotFound(context.DeadlineExceeded) || IsForbidden(context.DeadlineExceeded) {
		t.Fatal("helpers deben ser false para errores no-Drive")
	}
}

type stubTokenProvider struct {
	token string
	err   error
	calls int
}

func (s *stubTokenProvider) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	s.calls++
	if s.err != nil {
		return "", s.err
	}
	return s.token, nil
}

func TestServiceForNilProvider(t *testing.T) {
	r := &RealDriveClient{}
	// tokens nil via constructor directo (NewRealDriveClient exige non-nil, pero
	// serviceFor debe manejar nil sin panic).
	r.tokens = nil
	_, err := r.serviceFor(context.Background(), "user-x")
	if err == nil || !IsOAuthError(err) {
		t.Fatalf("esperaba *OAuthError tipado, got %v", err)
	}
}

func TestServiceForPropagatesTokenError(t *testing.T) {
	stub := &stubTokenProvider{err: context.DeadlineExceeded}
	r := NewRealDriveClient(stub)
	_, err := r.serviceFor(context.Background(), "user-x")
	if err == nil {
		t.Fatal("esperaba propagar error del TokenProvider")
	}
}

func TestRevokeAllPermissionsRequiresOAuth(t *testing.T) {
	stub := &stubTokenProvider{err: context.DeadlineExceeded}
	r := NewRealDriveClient(stub)
	if err := r.RevokeAllPermissions(context.Background(), "user-x", "file-x"); err == nil {
		t.Fatal("sin OAuth debe fallar antes de llamar a Google")
	}
	if err := r.RevokePermission(context.Background(), "user-x", "file-x", "a@x.com"); err == nil {
		t.Fatal("RevokePermission sin OAuth debe fallar")
	}
	if err := r.GrantPermission(context.Background(), "user-x", "file-x", "a@x.com", "reader"); err == nil {
		t.Fatal("GrantPermission sin OAuth debe fallar")
	}
}

func TestMockCopyRequiresDstOAuthOnly(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()
	author := "author-1"
	copier := "copier-1"
	// Crear archivo original con autor conectado.
	fid, err := m.CreateFile(ctx, author, "orig.md", "contenido")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// Desconectar al AUTOR: la clonación desacoplada debe seguir funcionando
	// (caso apunte público cuyo autor revocó Drive).
	m.Disconnect(author)
	newID, err := m.CopyFile(ctx, author, fid, copier, "clon")
	if err != nil {
		t.Fatalf("copy desacoplada debe funcionar sin token del autor, got %v", err)
	}
	if !m.HasFile(newID) {
		t.Fatal("clon debe existir en Drive del copiador")
	}
	m.Reconnect(author)
	// Desconectar al DESTINO: debe rechazarse (caso TestHandlerCopyDestWithoutOAuthRejected).
	m.Disconnect(copier)
	if _, err := m.CopyFile(ctx, author, fid, copier, "clon2"); err == nil {
		t.Fatal("copy sin OAuth destino debe fallar")
	} else if !IsOAuthError(err) {
		t.Fatalf("debe ser *OAuthError tipado (sin texto interno), got %T %v", err, err)
	}
}

func TestMockCreateRequiresOAuth(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()
	m.Disconnect("u-sin-oauth")
	if _, err := m.CreateFile(ctx, "u-sin-oauth", "a.md", "x"); err == nil {
		t.Fatal("CreateFile sin OAuth debe fallar como RealDriveClient")
	}
}

// --- RealDriveClient.CopyFile fallback srcSrv (downloadWith) ---

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func googleErrResp(code int, msg string) *http.Response {
	body, _ := json.Marshal(map[string]interface{}{
		"error": map[string]interface{}{"code": code, "message": msg},
	})
	return &http.Response{
		StatusCode: code,
		Status:     http.StatusText(code),
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

type perUserTokenProvider struct {
	tokens map[string]string
	errMap map[string]error
}

func (p *perUserTokenProvider) GetValidAccessToken(ctx context.Context, userID string) (string, error) {
	if p.errMap != nil {
		if err, ok := p.errMap[userID]; ok && err != nil {
			return "", err
		}
	}
	if p.tokens != nil {
		if tok, ok := p.tokens[userID]; ok {
			return tok, nil
		}
	}
	return "tok-" + userID, nil
}

func driveServiceWithTransport(t *testing.T, rt http.RoundTripper) *drive.Service {
	t.Helper()
	hc := &http.Client{Transport: rt}
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(hc))
	if err != nil {
		t.Fatalf("drive.NewService failed: %v", err)
	}
	return srv
}

func TestRealCopyFileFallbackDstFailsSrcSucceeds(t *testing.T) {
	ctx := context.Background()
	prov := &perUserTokenProvider{tokens: map[string]string{
		"src-user": "src-token",
		"dst-user": "dst-token",
	}}
	r := NewRealDriveClient(prov)
	// dst: download 403, create OK. src: download OK con contenido original.
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		switch token {
		case "dst-token":
			return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/files/") {
					return googleErrResp(403, "Forbidden"), nil
				}
				if req.Method == http.MethodPost && strings.Contains(req.URL.Path, "/files") {
					body, _ := json.Marshal(map[string]string{"id": "cloned-123"})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
				}
				// Upload con Media puede usar PUT/POST a /upload/...: responder clon.
				if strings.Contains(req.URL.Path, "/files") {
					body, _ := json.Marshal(map[string]string{"id": "cloned-123"})
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
				}
				return googleErrResp(500, "unexpected"), nil
			})), nil
		case "src-token":
			return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/files/") {
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("contenido original")), Header: http.Header{}}, nil
				}
				return googleErrResp(500, "unexpected src"), nil
			})), nil
		default:
			t.Fatalf("token inesperado %q", token)
			return nil, context.DeadlineExceeded
		}
	}
	newID, err := r.CopyFile(ctx, "src-user", "src123", "dst-user", "clon.md")
	if err != nil {
		t.Fatalf("fallback dst→src debe tener éxito, got %v", err)
	}
	if newID != "cloned-123" {
		t.Fatalf("id clonado esperado cloned-123, got %q", newID)
	}
}

func TestRealCopyFileFallbackBothFail(t *testing.T) {
	ctx := context.Background()
	prov := &perUserTokenProvider{tokens: map[string]string{
		"src-user": "src-token",
		"dst-user": "dst-token",
	}}
	r := NewRealDriveClient(prov)
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet && strings.Contains(req.URL.Path, "/files/") {
				return googleErrResp(404, "Not Found"), nil
			}
			return googleErrResp(500, "unexpected"), nil
		})), nil
	}
	_, err := r.CopyFile(ctx, "src-user", "missing", "dst-user", "clon.md")
	if err == nil {
		t.Fatal("si ambos (dst y src) fallan, CopyFile debe fallar")
	}
	if !IsNotFound(err) {
		t.Fatalf("doble 404 debe mapear a DriveError 404, got %v", err)
	}
}

func TestRealCopyFileDstNoOAuthNoFallback(t *testing.T) {
	ctx := context.Background()
	prov := &perUserTokenProvider{
		tokens: map[string]string{"src-user": "src-token"},
		errMap: map[string]error{"dst-user": context.DeadlineExceeded},
	}
	r := NewRealDriveClient(prov)
	// newService no debe llamarse para dst (falla antes); si se llama para src es error.
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		t.Fatal("sin OAuth destino no debe intentar fallback con src")
		return nil, context.DeadlineExceeded
	}
	_, err := r.CopyFile(ctx, "src-user", "src123", "dst-user", "clon.md")
	if err == nil {
		t.Fatal("sin OAuth destino debe fallar antes del fallback")
	}
}

func TestRealCreateFileEmptyTitle400(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "x"})
	if _, err := r.CreateFile(context.Background(), "u", "   ", "c"); err == nil {
		t.Fatal("título vacío debe fallar sin llamar a Drive")
	} else if de, ok := err.(*DriveError); !ok || de.Code != 400 {
		t.Fatalf("título vacío debe ser DriveError 400, got %v", err)
	}
}

func TestRealUploadAttachmentTooLarge413(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "x"})
	big := make([]byte, 10*1024*1024+1)
	if _, _, err := r.UploadAttachment(context.Background(), "u", "n", "f.bin", "application/octet-stream", big, false); err == nil {
		t.Fatal("adjunto >10MB debe fallar sin llamar a Drive")
	} else if de, ok := err.(*DriveError); !ok || de.Code != 413 {
		t.Fatalf("adjunto grande debe ser DriveError 413, got %v", err)
	}
}

func TestRealVerifyFileAccessRequiresOAuth(t *testing.T) {
	stub := &stubTokenProvider{err: context.DeadlineExceeded}
	r := NewRealDriveClient(stub)
	if err := r.VerifyFileAccess(context.Background(), "u", "f"); err == nil {
		t.Fatal("sin OAuth VerifyFileAccess debe fallar")
	}
}
