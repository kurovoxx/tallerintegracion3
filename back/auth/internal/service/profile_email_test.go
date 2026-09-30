package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
)

type fakeEmailLookup struct {
	email string
	err   error
}

func (f *fakeEmailLookup) GetEmailByID(ctx context.Context, userID string) (string, error) {
	return f.email, f.err
}

func TestProfileService_GetProfile_ConEmail(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440002"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Nombre", Visibility: model.VisibilityPublic}
	svc := NewProfileServiceWithUsers(repo, &fakeEmailLookup{email: "user@example.com"})
	p, err := svc.GetProfile(context.Background(), uid)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if p.Email == nil || *p.Email != "user@example.com" {
		t.Fatalf("email no completado: %+v", p)
	}
}

func TestProfileService_GetProfile_SinLookup_SinEmail(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440003"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Nombre", Visibility: model.VisibilityPublic}
	svc := NewProfileService(repo)
	p, err := svc.GetProfile(context.Background(), uid)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if p.Email != nil {
		t.Fatalf("sin lookup no debe traer email: %+v", p)
	}
}

func TestProfileService_GetProfile_LookupFalla_NoRompe(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440004"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Nombre", Visibility: model.VisibilityPublic}
	svc := NewProfileServiceWithUsers(repo, &fakeEmailLookup{err: errors.New("db caída")})
	p, err := svc.GetProfile(context.Background(), uid)
	if err != nil {
		t.Fatalf("fallo de email no debe romper el perfil, got %v", err)
	}
	if p.Email != nil {
		t.Fatalf("con error debe omitir email: %+v", p)
	}
}

func TestUsersService_Lookup_Vacio(t *testing.T) {
	svc := NewUsersService(nil)
	out, err := svc.Lookup(context.Background(), nil)
	if err != nil {
		t.Fatalf("sin ids no debe fallar, got %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("sin ids debe dar vacío, got %v", out)
	}
}
