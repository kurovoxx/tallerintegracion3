package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

func TestDriveHandler_Disconnect_204(t *testing.T) {
	h := NewDriveHandler(service.NewDriveOAuthService(&mockDriveRepo{}, &mockDriveProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/auth/google-drive/connection", nil)
	c.Set(middleware.ContextUserIDKey, "user-123")
	h.Disconnect(c)
	if w.Code != 204 {
		t.Fatalf("esperado 204, got %d %s", w.Code, w.Body.String())
	}
}

func TestDriveHandler_Disconnect_SinUser_401(t *testing.T) {
	h := NewDriveHandler(service.NewDriveOAuthService(&mockDriveRepo{}, &mockDriveProvider{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("DELETE", "/auth/google-drive/connection", nil)
	h.Disconnect(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}
