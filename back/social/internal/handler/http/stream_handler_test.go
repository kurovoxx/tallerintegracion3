package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/stream"
)

var errStreamTestBoom = errors.New("boom stream test")

type streamEnv struct {
	router *gin.Engine
	store  *service.MemoryMeetingStore
	mock   *stream.MockClient
}

func newStreamEnv(secret string) *streamEnv {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryMeetingStore()
	mock := stream.NewMockClient()
	svc := service.NewStreamTokenServiceWithKey(store, secret, "test-key")
	svc.SetClient(mock)
	h := NewStreamHandler(svc)

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	protected.GET("/groups/:id/stream-token", h.Token)
	return &streamEnv{router: r, store: store, mock: mock}
}

func (e *streamEnv) get(path, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func TestStreamToken_Contrato(t *testing.T) {
	e := newStreamEnv("handler-secret")
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	stranger := uuid.NewString()
	e.store.AddGroup(gid, admin, member)
	e.store.SetStreamChannel(gid, service.ChannelIDForGroup(gid))

	// 200 miembro: token + channel + api_key, sin secret en el body.
	w := e.get("/groups/"+gid+"/stream-token", member)
	expect(t, w, http.StatusOK)
	got := decode(t, w)
	if got["token"] == nil || got["token"] == "" {
		t.Fatalf("falta token: %v", got)
	}
	if got["channel_id"] != service.ChannelIDForGroup(gid) {
		t.Fatalf("channel_id inesperado: %v", got)
	}
	if strings.Contains(w.Body.String(), "handler-secret") {
		t.Fatal("el secret no debe filtrarse en la respuesta")
	}
	// El miembro quedó sincronizado en el canal de SU grupo.
	found := false
	for _, c := range e.mock.Ensured {
		if c.ChannelID == service.ChannelIDForGroup(gid) && c.UserID == member {
			found = true
		}
	}
	if !found {
		t.Fatalf("miembro no sincronizado: %+v", e.mock.Ensured)
	}

	// 403 ajeno: sin tocar Stream.
	n := len(e.mock.Ensured)
	w = e.get("/groups/"+gid+"/stream-token", stranger)
	expect(t, w, http.StatusForbidden)
	if len(e.mock.Ensured) != n {
		t.Fatal("ajeno no debe tocar Stream")
	}

	// 401 sin token.
	w = e.get("/groups/"+gid+"/stream-token", "")
	expect(t, w, http.StatusUnauthorized)

	// 404 grupo inexistente.
	w = e.get("/groups/"+uuid.NewString()+"/stream-token", member)
	expect(t, w, http.StatusNotFound)

	// 400 grupo mal formado.
	w = e.get("/groups/no-uuid/stream-token", member)
	expect(t, w, http.StatusBadRequest)
}

func TestStreamToken_SinSecret_503(t *testing.T) {
	e := newStreamEnv("  ")
	admin := uuid.NewString()
	gid := uuid.NewString()
	e.store.AddGroup(gid, admin)

	w := e.get("/groups/"+gid+"/stream-token", admin)
	expect(t, w, http.StatusServiceUnavailable)
}

func TestStreamToken_SyncFalla_503(t *testing.T) {
	e := newStreamEnv("handler-secret")
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	e.store.AddGroup(gid, admin, member)
	e.store.SetStreamChannel(gid, service.ChannelIDForGroup(gid))
	e.mock.EnsureErr = errStreamTestBoom

	w := e.get("/groups/"+gid+"/stream-token", member)
	expect(t, w, http.StatusServiceUnavailable)
	got := decode(t, w)
	if got["code"] != "stream_sync_failed" {
		t.Fatalf("code esperado stream_sync_failed: %v", got)
	}
}
