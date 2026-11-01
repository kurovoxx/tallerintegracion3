package service

import (
	"context"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

type disconnectRepo struct {
	mockOAuthRepo
	conn    *model.OAuthConnection
	deleted []string
}

func (m *disconnectRepo) GetByUserIDAndProvider(ctx context.Context, userID, provider string) (*model.OAuthConnection, error) {
	return m.conn, nil
}

func (m *disconnectRepo) DeleteGoogleDriveConnection(ctx context.Context, userID string) error {
	m.deleted = append(m.deleted, userID)
	return nil
}

func TestDriveDisconnect_BorraFila(t *testing.T) {
	rt := "refresh123"
	repo := &disconnectRepo{conn: &model.OAuthConnection{
		AccessToken: "ya29.falso", RefreshToken: &rt,
	}}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	if err := svc.Disconnect(context.Background(), "user-1"); err != nil {
		t.Fatalf("disconnect no debe fallar aunque Google sea inalcanzable: %v", err)
	}
	if len(repo.deleted) != 1 || repo.deleted[0] != "user-1" {
		t.Fatalf("debió borrar la fila, got %v", repo.deleted)
	}
}

func TestDriveDisconnect_SinFila_NoOp(t *testing.T) {
	repo := &disconnectRepo{conn: nil}
	svc := NewDriveOAuthService(repo, &mockProvider{})
	if err := svc.Disconnect(context.Background(), "user-1"); err != nil {
		t.Fatalf("sin fila debe ser éxito silencioso: %v", err)
	}
	if len(repo.deleted) != 1 {
		t.Fatalf("igual borra (idempotente), got %v", repo.deleted)
	}
}

func TestDriveDisconnect_SinUser_401(t *testing.T) {
	svc := NewDriveOAuthService(&disconnectRepo{}, &mockProvider{})
	err := svc.Disconnect(context.Background(), "  ")
	if se, ok := err.(*ServiceError); !ok || se.Code != "unauthorized" {
		t.Fatalf("se esperaba unauthorized, got %v", err)
	}
}
