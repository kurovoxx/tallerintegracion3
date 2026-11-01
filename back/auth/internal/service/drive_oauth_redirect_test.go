package service

import (
	"context"
	"testing"
	"time"
)

func TestValidateRedirectURI(t *testing.T) {
	cfg := "https://ti3-brojas.example.dev/auth/google/callback"
	cases := []struct {
		name  string
		raw   string
		valid bool
	}{
		{"loopback efímero", "http://127.0.0.1:54321/callback", true},
		{"loopback otro puerto", "http://127.0.0.1:8081/callback", true},
		{"legacy exacto", legacyDesktopRedirect, true},
		{"configurado prod", cfg, true},
		{"host externo", "https://evil.com/callback", false},
		{"loopback https", "https://127.0.0.1:9999/callback", false},
		{"loopback otro path", "http://127.0.0.1:9999/otro", false},
		{"localhost puerto libre NO permitido", "http://localhost:9999/auth/google/callback", false},
		{"vacío", "", false},
		{"basura", "no-es-url", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRedirectURI(tc.raw, cfg, legacyDesktopRedirect)
			if (err == nil) != tc.valid {
				t.Fatalf("raw=%q válido=%v, got err=%v", tc.raw, tc.valid, err)
			}
		})
	}
}

func TestDriveConnect_RedirectDinamico_OK(t *testing.T) {
	repo := &mockOAuthRepo{}
	exp := time.Now().Add(time.Hour)
	email := "user@gmail.com"
	provider := &mockProvider{
		result: &DriveOAuthResult{AccessToken: "acc", ExpiresAt: &exp, ExternalEmail: &email},
	}
	// Provider mock sin redirect configurado: igual debe aceptar loopback.
	svc := NewDriveOAuthService(repo, provider)
	dyn := "http://127.0.0.1:51234/callback"
	if err := svc.Connect(context.Background(), "user-1", "code123", nil, dyn); err != nil {
		t.Fatalf("redirect loopback no debe fallar: %v", err)
	}
	if provider.lastRedirect != dyn {
		t.Fatalf("el exchange debió usar el redirect dinámico, got %q", provider.lastRedirect)
	}
	if !repo.upsertCalled {
		t.Fatal("debió guardar la conexión")
	}
}

func TestDriveConnect_RedirectExterno_400(t *testing.T) {
	repo := &mockOAuthRepo{}
	provider := &mockProvider{result: &DriveOAuthResult{AccessToken: "acc"}}
	svc := NewDriveOAuthService(repo, provider)
	err := svc.Connect(context.Background(), "user-1", "code123", nil, "https://evil.com/callback")
	if se, ok := err.(*ServiceError); !ok || se.Code != "invalid_redirect_uri" {
		t.Fatalf("se esperaba invalid_redirect_uri, got %v", err)
	}
	if repo.upsertCalled {
		t.Fatal("rechazado no debe guardar nada")
	}
	if provider.called {
		t.Fatal("rechazado no debe llamar a Google")
	}
}
