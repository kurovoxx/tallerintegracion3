package http

import (
	"strings"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

func TestDriveHandler_Connect_RedirectExterno_400(t *testing.T) {
	repo := &mockDriveRepo{}
	provider := &mockDriveProvider{result: &service.DriveOAuthResult{AccessToken: "acc"}}
	h := newDriveHandlerWithMocks(repo, provider)
	c, w := newDriveContext("tok", `{"oauth_code":"code123","redirect_uri":"https://evil.com/callback"}`)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Connect(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid_redirect_uri") {
		t.Fatalf("se esperaba code invalid_redirect_uri, got %s", w.Body.String())
	}
	if repo.upsertCalled {
		t.Fatal("rechazado no debe guardar nada")
	}
}
