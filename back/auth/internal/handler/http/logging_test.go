package http

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// captureLogs redirige el logger estándar durante la prueba.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func mustContain(t *testing.T, log, substr string) {
	t.Helper()
	if !strings.Contains(log, substr) {
		t.Fatalf("log debe contener %q, got:\n%s", substr, log)
	}
}

func mustNotContain(t *testing.T, log, substr string) {
	t.Helper()
	if strings.Contains(log, substr) {
		t.Fatalf("log NO debe contener %q, got:\n%s", substr, log)
	}
}

func TestLogging_ProfileGet_200(t *testing.T) {
	buf := captureLogs(t)
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Test", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("GET", "/profile/me", nil, uid)
	h.GetProfile(c)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=GetProfile")
	mustContain(t, out, "status=200")
	mustContain(t, out, "user_id="+uid)
}

func TestLogging_ProfilePatch_VisibilityInvalida_400(t *testing.T) {
	buf := captureLogs(t)
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440002"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Test", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("PATCH", "/profile/me", []byte(`{"visibility":"SUPER-VISIBLE-TEST"}`), uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=PatchProfile")
	mustContain(t, out, "status=400")
	mustContain(t, out, "code=invalid_visibility")
	mustContain(t, out, "has_visibility=true")
	mustContain(t, out, "has_display_name=false")
	mustNotContain(t, out, "SUPER-VISIBLE-TEST")
}

func TestLogging_ProfileGet_401_SinUser(t *testing.T) {
	buf := captureLogs(t)
	h, _ := setupProfileHandler(newMockRepoHandler())
	c, w := newContextWithAuth("GET", "/profile/me", nil, "")
	h.GetProfile(c)
	if w.Code != 401 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=GetProfile")
	mustContain(t, out, "status=401")
	mustContain(t, out, "user_id=-")
}

func TestLogging_DriveConnect_CodigoInvalido_SinFuga(t *testing.T) {
	buf := captureLogs(t)
	provider := &mockDriveProvider{err: service.ErrInvalidOAuthCode}
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, provider)
	c, w := newDriveContext("", `{"oauth_code":"FAKECODE-XYZ-999"}`)
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=DriveConnect")
	mustContain(t, out, "status=400")
	mustContain(t, out, "code=invalid_oauth_code")
	mustContain(t, out, "user_id=user-123")
	mustNotContain(t, out, "FAKECODE-XYZ-999")
}

func TestLogging_DriveConnect_200_Conectado(t *testing.T) {
	buf := captureLogs(t)
	exp := time.Now().Add(time.Hour)
	provider := &mockDriveProvider{result: &service.DriveOAuthResult{AccessToken: "a", ExpiresAt: &exp}}
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, provider)
	c, w := newDriveContext("", `{"oauth_code":"code123"}`)
	h.Connect(c)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=DriveConnect")
	mustContain(t, out, "status=200")
	mustContain(t, out, "connected=true")
}

func TestLogging_DriveStatus_Conectado_200(t *testing.T) {
	buf := captureLogs(t)
	repo := &mockDriveRepo{connections: map[string]*model.OAuthConnection{
		"user-123": {UserID: "user-123", Provider: model.ProviderGoogleDrive, AccessToken: "tok"},
	}}
	h := newDriveHandlerWithMocks(repo, &mockDriveProvider{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/google-drive/status", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Status(c)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=DriveStatus")
	mustContain(t, out, "connected=true")
	mustContain(t, out, "reconnect_required=false")
}

func TestLogging_DriveStatus_Revocado_Reconnect(t *testing.T) {
	buf := captureLogs(t)
	now := time.Now()
	repo := &mockDriveRepo{connections: map[string]*model.OAuthConnection{
		"user-123": {UserID: "user-123", Provider: model.ProviderGoogleDrive, AccessToken: "tok", RevokedAt: &now},
	}}
	h := newDriveHandlerWithMocks(repo, &mockDriveProvider{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/google-drive/status", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Status(c)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "connected=false")
	mustContain(t, out, "reconnect_required=true")
}

func TestLogging_DriveDisconnect_204(t *testing.T) {
	buf := captureLogs(t)
	h := newDriveHandlerWithMocks(&mockDriveRepo{}, &mockDriveProvider{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/auth/google-drive/connection", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Disconnect(c)
	if w.Code != 204 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=DriveDisconnect")
	mustContain(t, out, "status=204")
	mustContain(t, out, "user_id=user-123")
}

func TestLogging_CalendarConnect_CodigoInvalido_SinFuga(t *testing.T) {
	buf := captureLogs(t)
	provider := &mockCalendarProvider{exchangeErr: service.ErrInvalidOAuthCode}
	h := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, provider))
	c, w := newCalendarConnectCtx(`{"oauth_code":"FAKECAL-CODE-456"}`, true)
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=CalendarConnect")
	mustContain(t, out, "status=400")
	mustContain(t, out, "code=invalid_oauth_code")
	mustNotContain(t, out, "FAKECAL-CODE-456")
}

func TestLogging_CalendarStatus_EmailNoSeLoguea(t *testing.T) {
	buf := captureLogs(t)
	email := "leakcheck-calendar@example.com"
	repo := &mockCalendarRepo{conn: &model.OAuthConnection{
		UserID: "user-123", Provider: model.ProviderGoogleCalendar,
		AccessToken: "tok", ExternalAccountEmail: &email,
	}}
	h := NewCalendarHandler(service.NewCalendarOAuthService(repo, &mockCalendarProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/auth/google-calendar/status", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Status(c)
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	out := buf.String()
	mustContain(t, out, "op=CalendarStatus")
	mustContain(t, out, "connected=true")
	mustContain(t, out, "has_external_email=true")
	mustNotContain(t, out, "leakcheck-calendar@example.com")
}
