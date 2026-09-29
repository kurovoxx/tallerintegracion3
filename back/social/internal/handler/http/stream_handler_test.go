package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

func newStreamEnv(secret string) (*gin.Engine, *service.MemoryMeetingStore) {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryMeetingStore()
	h := NewStreamHandler(service.NewStreamTokenService(store, secret))
	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.GET("/groups/:id/stream-token", h.Token)
	return r, store
}

func doStreamToken(t *testing.T, r *gin.Engine, groupID, user string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/groups/"+groupID+"/stream-token", nil)
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestStreamToken_Contrato(t *testing.T) {
	r, store := newStreamEnv("test-secret")
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid := uuid.NewString()
	store.AddGroup(gid, admin, member)
	store.SetStreamChannel(gid, "group-canal-1")

	// 200 miembro con token y channel.
	w := doStreamToken(t, r, gid, member)
	expect(t, w, http.StatusOK)
	got := decode(t, w)
	if got["token"] == nil || got["token"] == "" {
		t.Fatalf("falta token: %v", got)
	}
	if got["channel_id"] != "group-canal-1" {
		t.Fatalf("channel inesperado: %v", got)
	}

	// 403 extraño.
	w = doStreamToken(t, r, gid, stranger)
	expect(t, w, http.StatusForbidden)

	// 401 sin token.
	w = doStreamToken(t, r, gid, "")
	expect(t, w, http.StatusUnauthorized)

	// 404 grupo inexistente.
	w = doStreamToken(t, r, uuid.NewString(), member)
	expect(t, w, http.StatusNotFound)

	// 400 grupo mal formado.
	w = doStreamToken(t, r, "no-uuid", member)
	expect(t, w, http.StatusBadRequest)
}

func TestStreamToken_SinSecret_503(t *testing.T) {
	r, store := newStreamEnv("  ")
	admin := uuid.NewString()
	gid := uuid.NewString()
	store.AddGroup(gid, admin)

	w := doStreamToken(t, r, gid, admin)
	expect(t, w, http.StatusServiceUnavailable)
}
