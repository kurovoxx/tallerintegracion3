package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

type mockCalendarRepo struct {
	conn        *model.OAuthConnection
	upsertErr   error
	updateErr   error
	markErr     error
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
	return m.updateErr
}

func (m *mockCalendarRepo) MarkGoogleCalendarConnectionRevoked(ctx context.Context, userID string) error {
	m.markCalls++
	return m.markErr
}

type mockCalendarProvider struct {
	exchangeResult *CalendarOAuthResult
	exchangeErr    error
	refreshResult  *CalendarOAuthResult
	refreshErr     error
}

func (m *mockCalendarProvider) Exchange(ctx context.Context, code string) (*CalendarOAuthResult, error) {
	return m.exchangeResult, m.exchangeErr
}

func (m *mockCalendarProvider) Refresh(ctx context.Context, refreshToken string) (*CalendarOAuthResult, error) {
	return m.refreshResult, m.refreshErr
}

func TestCalendarConnect_OK(t *testing.T) {
	repo := &mockCalendarRepo{}
	prov := &mockCalendarProvider{exchangeResult: &CalendarOAuthResult{AccessToken: "acc"}}
	svc := NewCalendarOAuthService(repo, prov)
	if err := svc.Connect(context.Background(), "user-1", "code123"); err != nil {
		t.Fatalf("Connect no debió fallar: %v", err)
	}
	if repo.upsertCalls != 1 {
		t.Fatalf("se esperaba 1 upsert, got %d", repo.upsertCalls)
	}
}

func TestCalendarConnect_CodigoVacio_BadRequest(t *testing.T) {
	svc := NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{})
	err := svc.Connect(context.Background(), "user-1", "  ")
	if err == nil {
		t.Fatal("se esperaba error por código vacío")
	}
	if se, ok := err.(*ServiceError); !ok || se.Code != "bad_request" {
		t.Fatalf("se esperaba bad_request, got %v", err)
	}
}

func TestCalendarConnect_CodigoInvalido(t *testing.T) {
	svc := NewCalendarOAuthService(&mockCalendarRepo{}, &mockCalendarProvider{exchangeErr: ErrInvalidOAuthCode})
	err := svc.Connect(context.Background(), "user-1", "bad")
	if se, ok := err.(*ServiceError); !ok || se.Code != "invalid_oauth_code" {
		t.Fatalf("se esperaba invalid_oauth_code, got %v", err)
	}
}

func TestCalendarToken_SinConexion_NotConnected(t *testing.T) {
	svc := NewCalendarOAuthService(&mockCalendarRepo{conn: nil}, &mockCalendarProvider{})
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if se, ok := err.(*ServiceError); !ok || se.Code != "not_connected" {
		t.Fatalf("se esperaba not_connected, got %v", err)
	}
}

func TestCalendarToken_Vigente_SinRefresh(t *testing.T) {
	exp := time.Now().Add(1 * time.Hour)
	repo := &mockCalendarRepo{conn: &model.OAuthConnection{AccessToken: "acc-vigente", ExpiresAt: &exp}}
	svc := NewCalendarOAuthService(repo, &mockCalendarProvider{refreshErr: errors.New("no debe llamarse")})
	tok, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil || tok != "acc-vigente" {
		t.Fatalf("se esperaba token vigente, got %q, %v", tok, err)
	}
}

func TestCalendarToken_Vencido_RefrescaYActualiza(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	rt := "refresh-1"
	repo := &mockCalendarRepo{conn: &model.OAuthConnection{AccessToken: "viejo", RefreshToken: &rt, ExpiresAt: &exp}}
	newExp := time.Now().Add(1 * time.Hour)
	prov := &mockCalendarProvider{refreshResult: &CalendarOAuthResult{AccessToken: "nuevo", ExpiresAt: &newExp}}
	svc := NewCalendarOAuthService(repo, prov)
	tok, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if err != nil || tok != "nuevo" {
		t.Fatalf("se esperaba token refrescado, got %q, %v", tok, err)
	}
}

func TestCalendarToken_RefreshInvalido_MarcaRevocado(t *testing.T) {
	exp := time.Now().Add(-1 * time.Hour)
	rt := "refresh-malo"
	repo := &mockCalendarRepo{conn: &model.OAuthConnection{AccessToken: "viejo", RefreshToken: &rt, ExpiresAt: &exp}}
	prov := &mockCalendarProvider{refreshErr: ErrCalendarConnectionInvalid}
	svc := NewCalendarOAuthService(repo, prov)
	_, err := svc.GetValidAccessToken(context.Background(), "user-1")
	if se, ok := err.(*ServiceError); !ok || se.Code != "calendar_connection_invalid" {
		t.Fatalf("se esperaba calendar_connection_invalid, got %v", err)
	}
	if repo.markCalls != 1 {
		t.Fatalf("se esperaba 1 marcado de revocación, got %d", repo.markCalls)
	}
}
