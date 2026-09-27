package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestRealLinkPermissionLifecycle(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "owner-token"})
	var calls []string
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		if token != "owner-token" {
			t.Fatalf("wrong token: %s", token)
		}
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls = append(calls, req.Method+" "+req.URL.Path)
			if req.Method == http.MethodPost {
				var body map[string]any
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["type"] != "anyone" || body["role"] != "reader" || body["allowFileDiscovery"] != false {
					t.Fatalf("wrong link policy: %v", body)
				}
				if req.URL.Query().Get("fields") != "id" {
					t.Fatal("permission ID must be requested")
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"link-id"}`))}, nil
			}
			if req.Method != http.MethodDelete || !strings.HasSuffix(req.URL.Path, "/files/file-id/permissions/link-id") {
				t.Fatalf("unexpected request: %s %s", req.Method, req.URL)
			}
			return &http.Response{StatusCode: 204, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})), nil
	}
	id, err := r.GrantLinkPermission(context.Background(), "owner", "file-id")
	if err != nil || id != "link-id" {
		t.Fatalf("grant = %q, %v", id, err)
	}
	if err := r.RevokePermissionByID(context.Background(), "owner", "file-id", id); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls: %v", calls)
	}
}

func TestRealLinkPermissionErrors(t *testing.T) {
	for _, code := range []int{403, 404, 500} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			r := NewRealDriveClient(&stubTokenProvider{token: "token"})
			r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
				return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) { return googleErrResp(code, "failed"), nil })), nil
			}
			if id, err := r.GrantLinkPermission(context.Background(), "owner", "file"); err == nil || id != "" {
				t.Fatalf("grant = %q, %v", id, err)
			}
			err := r.RevokePermissionByID(context.Background(), "owner", "file", "permission")
			if (err == nil) != (code == 404) {
				t.Fatalf("revoke status %d: %v", code, err)
			}
		})
	}
	r := NewRealDriveClient(nil)
	if _, err := r.GrantLinkPermission(context.Background(), "owner", "file"); !IsOAuthError(err) {
		t.Fatalf("OAuth: %v", err)
	}
	if err := r.RevokePermissionByID(context.Background(), "owner", "file", "permission"); !IsOAuthError(err) {
		t.Fatalf("OAuth: %v", err)
	}
}

func TestRealRevokeLinkAfterRestart(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "token"})
	var deleted []string
	r.newService = func(ctx context.Context, token string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodGet {
				body := `{"nextPageToken":"page2","permissions":[{"id":"owner","type":"user","role":"owner"},{"id":"member","type":"user","role":"reader"}]}`
				if req.URL.Query().Get("pageToken") == "page2" {
					body = `{"permissions":[{"id":"actual-link-id","type":"anyone","role":"reader"}]}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			}
			deleted = append(deleted, req.URL.Path)
			return googleErrResp(404, "already removed"), nil
		})), nil
	}
	if err := r.RevokePermissionByID(context.Background(), "owner", "file", ""); err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || !strings.HasSuffix(deleted[0], "/permissions/actual-link-id") {
		t.Fatalf("deleted wrong permissions: %v", deleted)
	}
}

func TestMockLinkPermissionLifecycle(t *testing.T) {
	m := NewMockClient()
	ctx := context.Background()
	id, err := m.GrantLinkPermission(ctx, "owner", "file")
	if err != nil || id == "" {
		t.Fatalf("grant: %q %v", id, err)
	}
	again, _ := m.GrantLinkPermission(ctx, "owner", "file")
	if again != id {
		t.Fatal("duplicate link permission")
	}
	if err := m.RevokePermissionByID(ctx, "owner", "file", id); err != nil {
		t.Fatal(err)
	}
	if len(m.permissions["file"]) != 0 {
		t.Fatal("link remains")
	}
	_, _ = m.GrantLinkPermission(ctx, "owner", "file")
	_ = m.GrantPermission(ctx, "owner", "file", "real@example.com", "reader")
	if err := m.RevokePermissionByID(ctx, "owner", "file", ""); err != nil {
		t.Fatal(err)
	}
	if len(m.permissions["file"]) != 1 {
		t.Fatal("must preserve nominal permission")
	}
	m.Disconnect("owner")
	if _, err := m.GrantLinkPermission(ctx, "owner", "file"); !IsOAuthError(err) {
		t.Fatalf("OAuth: %v", err)
	}
	if err := m.RevokePermissionByID(ctx, "owner", "file", id); !IsOAuthError(err) {
		t.Fatalf("OAuth: %v", err)
	}
}

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
	fid, err := m.CreateFile(ctx, author, "note-orig", "orig.md", "contenido")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	// Desconectar al AUTOR: la clonación desacoplada debe seguir funcionando
	// (caso apunte público cuyo autor revocó Drive).
	m.Disconnect(author)
	newID, err := m.CopyFile(ctx, author, fid, copier, "note-clon", "clon")
	if err != nil {
		t.Fatalf("copy desacoplada debe funcionar sin token del autor, got %v", err)
	}
	if !m.HasFile(newID) {
		t.Fatal("clon debe existir en Drive del copiador")
	}
	m.Reconnect(author)
	// Desconectar al DESTINO: debe rechazarse (caso TestHandlerCopyDestWithoutOAuthRejected).
	m.Disconnect(copier)
	if _, err := m.CopyFile(ctx, author, fid, copier, "note-clon-2", "clon2"); err == nil {
		t.Fatal("copy sin OAuth destino debe fallar")
	} else if !IsOAuthError(err) {
		t.Fatalf("debe ser *OAuthError tipado (sin texto interno), got %T %v", err, err)
	}
}

func TestMockCreateRequiresOAuth(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()
	m.Disconnect("u-sin-oauth")
	if _, err := m.CreateFile(ctx, "u-sin-oauth", "", "a.md", "x"); err == nil {
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
	newID, err := r.CopyFile(ctx, "src-user", "src123", "dst-user", "note-clon", "clon.md")
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
	_, err := r.CopyFile(ctx, "src-user", "missing", "dst-user", "note-clon", "clon.md")
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
	_, err := r.CopyFile(ctx, "src-user", "src123", "dst-user", "note-clon", "clon.md")
	if err == nil {
		t.Fatal("sin OAuth destino debe fallar antes del fallback")
	}
}

func TestRealCreateFileEmptyTitle400(t *testing.T) {
	r := NewRealDriveClient(&stubTokenProvider{token: "x"})
	if _, err := r.CreateFile(context.Background(), "u", "", "   ", "c"); err == nil {
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

// --- appProperties + FindFileByNoteID ---

// CreateFile indexa el .md con notes_note_id y notes_owner_user_id, de modo que
// FindFileByNoteID puede recuperarlo por nota y respeta el dueño.
func TestMockCreateFileIndexesAppPropertiesAndFindsByNoteID(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()
	owner := "owner-1"
	noteID := "note-abc"
	fid, err := m.CreateFile(ctx, owner, noteID, "nota.md", "contenido")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	props, ok := m.FileAppProperties(fid)
	if !ok {
		t.Fatal("el archivo debe existir")
	}
	if props["notes_note_id"] != noteID || props["notes_owner_user_id"] != owner {
		t.Fatalf("appProperties esperadas note=%q owner=%q, got %v", noteID, owner, props)
	}
	found, err := m.FindFileByNoteID(ctx, owner, noteID)
	if err != nil || found != fid {
		t.Fatalf("FindFileByNoteID debe encontrar el archivo: got %q err=%v", found, err)
	}
	// Otro dueño no puede encontrarlo, y una nota sin archivo devuelve "".
	if other, err := m.FindFileByNoteID(ctx, "otro", noteID); err != nil || other != "" {
		t.Fatalf("otro dueño no debe encontrarlo: got %q err=%v", other, err)
	}
	if missing, err := m.FindFileByNoteID(ctx, owner, "sin-archivo"); err != nil || missing != "" {
		t.Fatalf("nota sin archivo debe devolver vacío: got %q err=%v", missing, err)
	}
	// Sin OAuth del dueño la búsqueda falla tipada (paridad con Real).
	m.Disconnect(owner)
	if _, err := m.FindFileByNoteID(ctx, owner, noteID); !IsOAuthError(err) {
		t.Fatalf("sin OAuth debe ser OAuthError, got %v", err)
	}
}

// CopyFile indexa el clon con el notes_note_id de la NUEVA nota y el dueño
// destino, permitiendo recuperarlo tras un crash.
func TestMockCopyFileIndexesNewNoteID(t *testing.T) {
	ctx := context.Background()
	m := NewMockClient()
	srcFile, err := m.CreateFile(ctx, "author", "note-src", "src.md", "data")
	if err != nil {
		t.Fatalf("create src failed: %v", err)
	}
	cloneFile, err := m.CopyFile(ctx, "author", srcFile, "copier", "note-clone", "clon")
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	found, err := m.FindFileByNoteID(ctx, "copier", "note-clone")
	if err != nil || found != cloneFile {
		t.Fatalf("el clon debe quedar indexado: got %q err=%v", found, err)
	}
	props, _ := m.FileAppProperties(cloneFile)
	if props["notes_note_id"] != "note-clone" || props["notes_owner_user_id"] != "copier" {
		t.Fatalf("appProperties del clon incorrectas: %v", props)
	}
}

// FindFileByNoteID del cliente real arma la query de appProperties y filtra por
// notes_owner_user_id, eligiendo el ID menor de forma determinista.
func TestRealFindFileByNoteIDQueriesAppProperties(t *testing.T) {
	ctx := context.Background()
	prov := &perUserTokenProvider{tokens: map[string]string{"owner": "tok"}}
	r := NewRealDriveClient(prov)
	var gotQuery string
	r.newService = func(context.Context, string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			gotQuery = req.URL.Query().Get("q")
			body, _ := json.Marshal(map[string]interface{}{
				"files": []map[string]interface{}{
					{"id": "file-b", "appProperties": map[string]string{"notes_note_id": "note-1", "notes_owner_user_id": "owner"}},
					{"id": "file-a", "appProperties": map[string]string{"notes_note_id": "note-1", "notes_owner_user_id": "owner"}},
					{"id": "file-ajeno", "appProperties": map[string]string{"notes_note_id": "note-1", "notes_owner_user_id": "otro"}},
				},
			})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		})), nil
	}
	got, err := r.FindFileByNoteID(ctx, "owner", "note-1")
	if err != nil || got != "file-a" {
		t.Fatalf("esperaba file-a (dueño correcto, ID menor), got %q err=%v", got, err)
	}
	wantQuery := "appProperties has { key='notes_note_id' and value='note-1' }"
	if !strings.Contains(gotQuery, wantQuery) || !strings.Contains(gotQuery, "trashed = false") {
		t.Fatalf("query de appProperties incorrecta: %q", gotQuery)
	}
}

// Sin resultados (o sin OAuth) FindFileByNoteID no inventa un fileID.
func TestRealFindFileByNoteIDEmptyAndOAuth(t *testing.T) {
	ctx := context.Background()
	prov := &perUserTokenProvider{tokens: map[string]string{"owner": "tok"}}
	r := NewRealDriveClient(prov)
	if got, err := r.FindFileByNoteID(ctx, "owner", ""); err != nil || got != "" {
		t.Fatalf("noteID vacío debe devolver vacío sin error: %q %v", got, err)
	}
	r.newService = func(context.Context, string) (*drive.Service, error) {
		return driveServiceWithTransport(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(map[string]interface{}{"files": []interface{}{}})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{"Content-Type": []string{"application/json"}}}, nil
		})), nil
	}
	if got, err := r.FindFileByNoteID(ctx, "owner", "note-sin-archivo"); err != nil || got != "" {
		t.Fatalf("sin coincidencias debe devolver vacío sin error: %q %v", got, err)
	}
	stub := &stubTokenProvider{err: context.DeadlineExceeded}
	if _, err := NewRealDriveClient(stub).FindFileByNoteID(ctx, "owner", "note-1"); err == nil {
		t.Fatal("sin OAuth FindFileByNoteID debe fallar")
	}
}
