package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

func newInternalTokenCtx(userID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/internal/oauth/calendar-token?user_id="+userID, nil)
	return c, w
}

func calendarConn(token string, expiresAt *time.Time, revoked bool) *model.OAuthConnection {
	c := &model.OAuthConnection{AccessToken: token, ExpiresAt: expiresAt}
	if revoked {
		now := time.Now().Add(-1 * time.Hour)
		c.RevokedAt = &now
	}
	return c
}

func TestInternalOAuth_GetToken_Vigente_200(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour)
	repo := &mockCalendarRepo{conn: calendarConn("acccal-vigente", &exp, false)}
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(repo, &mockCalendarProvider{}))
	c, w := newInternalTokenCtx("user-1")
	h.GetCalendarToken(c)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp["access_token"] != "acccal-vigente" {
		t.Fatalf("access_token esperado, got %s", w.Body.String())
	}
}

func TestInternalOAuth_GetToken_SinUserID_400(t *testing.T) {
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{}))
	c, w := newInternalTokenCtx("")
	h.GetCalendarToken(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
}

func TestInternalOAuth_GetToken_SinConexion_404(t *testing.T) {
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(&mockCalendarRepo{conn: nil}, &mockCalendarProvider{}))
	c, w := newInternalTokenCtx("user-1")
	h.GetCalendarToken(c)
	if w.Code != 404 {
		t.Fatalf("esperado 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestInternalOAuth_GetToken_Revocado_403(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour)
	repo := &mockCalendarRepo{conn: calendarConn("acccal", &exp, true)}
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(repo, &mockCalendarProvider{}))
	c, w := newInternalTokenCtx("user-1")
	h.GetCalendarToken(c)
	if w.Code != 403 {
		t.Fatalf("esperado 403, got %d %s", w.Code, w.Body.String())
	}
}

func TestInternalOAuth_GetToken_GoogleCaido_502(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	rt := "refreshcal"
	repo := &mockCalendarRepo{conn: &model.OAuthConnection{AccessToken: "viejo", RefreshToken: &rt, ExpiresAt: &exp}}
	provider := &mockCalendarProvider{refreshErr: service.ErrGoogleUnavailable}
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(repo, provider))
	c, w := newInternalTokenCtx("user-1")
	h.GetCalendarToken(c)
	if w.Code != 502 {
		t.Fatalf("esperado 502, got %d %s", w.Code, w.Body.String())
	}
}

func TestInternalOAuth_ReportRevoked_204(t *testing.T) {
	repo := &mockCalendarRepo{}
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(repo, &mockCalendarProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/internal/oauth/calendar-revoked",
		bytes.NewBufferString(`{"user_id":"user-1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ReportCalendarRevoked(c)
	if w.Code != 204 {
		t.Fatalf("esperado 204, got %d %s", w.Code, w.Body.String())
	}
	if repo.markCalls != 1 {
		t.Fatalf("se esperaba 1 marcado de revocación, got %d", repo.markCalls)
	}
}

func TestInternalOAuth_ReportRevoked_SinUserID_400(t *testing.T) {
	h := NewInternalOAuthHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/internal/oauth/calendar-revoked",
		bytes.NewBufferString(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.ReportCalendarRevoked(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
}
