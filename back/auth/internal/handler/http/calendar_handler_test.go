package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

type mockCalendarRepo struct {
	conn        *model.OAuthConnection
	upsertErr   error
	upsertCalls int
	markCalls   int
}

func (m *mockCalendarRepo) UpsertGoogleCalendarConnection(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt *time.Time, externalEmail *string) error {
	m.upsertCalls++
	return m.upsertErr
}

func (m *mockCalendarRepo) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	return m.conn, nil
}

func (m *mockCalendarRepo) UpdateGoogleCalendarAccessToken(ctx context.Context, userID, accessToken string, refreshToken *string, expiresAt time.Time) error {
	return nil
}

func (m *mockCalendarRepo) MarkGoogleCalendarConnectionRevoked(ctx context.Context, userID string) error {
	m.markCalls++
	return nil
}

type mockCalendarProvider struct {
	exchangeResult *service.CalendarOAuthResult
	exchangeErr    error
	refreshResult  *service.CalendarOAuthResult
	refreshErr     error
}

func (m *mockCalendarProvider) Exchange(ctx context.Context, code string) (*service.CalendarOAuthResult, error) {
	return m.exchangeResult, m.exchangeErr
}

func (m *mockCalendarProvider) Refresh(ctx context.Context, refreshToken string) (*service.CalendarOAuthResult, error) {
	return m.refreshResult, m.refreshErr
}

func newCalendarConnectCtx(body string, withUser bool) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/auth/google-calendar/connect", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if withUser {
		c.Set(middleware.ContextUserIDKey, "user-123")
	}
	return c, w
}

func TestCalendarHandler_Connect_Valido_200(t *testing.T) {
	repo := &mockCalendarRepo{}
	exp := time.Now().Add(1 * time.Hour)
	email := "cal@gmail.com"
	refresh := "refreshcal"
	provider := &mockCalendarProvider{exchangeResult: &service.CalendarOAuthResult{
		AccessToken: "acccal", RefreshToken: &refresh, ExpiresAt: &exp, ExternalEmail: &email,
	}}
	h := NewCalendarHandler(service.NewCalendarOAuthService(repo, provider))
	c, w := newCalendarConnectCtx(`{"oauth_code":"code123"}`, true)
	h.Connect(c)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp["connected"] != true {
		t.Fatalf("connected true esperado, got %s", w.Body.String())
	}
	if repo.upsertCalls != 1 {
		t.Fatal("Upsert debe llamarse una vez")
	}
	if w.Body.String() != `{"connected":true}` {
		t.Fatalf("respuesta no debe exponer tokens: %s", w.Body.String())
	}
}

func TestCalendarHandler_Connect_SinUser_401(t *testing.T) {
	h := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{}))
	c, w := newCalendarConnectCtx(`{"oauth_code":"code123"}`, false)
	h.Connect(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

func TestCalendarHandler_Connect_CodigoVacio_400(t *testing.T) {
	h := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{}))
	for _, body := range []string{`{}`, `{"oauth_code":""}`} {
		c, w := newCalendarConnectCtx(body, true)
		h.Connect(c)
		if w.Code != 400 {
			t.Fatalf("esperado 400 para %s, got %d", body, w.Code)
		}
	}
}

func TestCalendarHandler_Connect_CodigoInvalido_400(t *testing.T) {
	provider := &mockCalendarProvider{exchangeErr: service.ErrInvalidOAuthCode}
	h := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, provider))
	c, w := newCalendarConnectCtx(`{"oauth_code":"bad"}`, true)
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestCalendarHandler_Connect_GoogleCaido_502(t *testing.T) {
	provider := &mockCalendarProvider{exchangeErr: service.ErrGoogleUnavailable}
	h := NewCalendarHandler(service.NewCalendarOAuthService(&mockCalendarRepo{}, provider))
	c, w := newCalendarConnectCtx(`{"oauth_code":"code123"}`, true)
	h.Connect(c)
	if w.Code != 502 {
		t.Fatalf("esperado 502, got %d %s", w.Code, w.Body.String())
	}
}
