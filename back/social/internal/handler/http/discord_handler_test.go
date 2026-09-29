package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

type discordEnv struct {
	router *gin.Engine
	store  *service.MemoryMeetingStore
}

func newDiscordEnv() *discordEnv {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryMeetingStore()
	h := NewDiscordHandler(service.NewDiscordService(store))

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	h.RegisterRoutes(protected)
	return &discordEnv{router: r, store: store}
}

// do ejecuta una petición autenticada como user (token vacío = sin header).
func (e *discordEnv) do(method, path, user string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func TestPutDiscordConfig_Contrato(t *testing.T) {
	e := newDiscordEnv()
	admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid := uuid.NewString()
	e.store.AddGroup(gid, admin, member)
	body := map[string]any{
		"server_name": "Servidor",
		"invite_url":  "https://discord.gg/abc",
		"webhook_url": "https://discord.com/api/webhooks/1/x",
	}

	// 200 admin.
	w := e.do(http.MethodPut, "/groups/"+gid+"/discord-config", admin, body)
	expect(t, w, http.StatusOK)
	got := decode(t, w)
	if got["server_name"] != "Servidor" || got["webhook_url"] != "https://discord.com/api/webhooks/1/x" {
		t.Fatalf("respuesta inesperada: %v", got)
	}

	// 403 member y extraño.
	w = e.do(http.MethodPut, "/groups/"+gid+"/discord-config", member, body)
	expect(t, w, http.StatusForbidden)
	w = e.do(http.MethodPut, "/groups/"+gid+"/discord-config", stranger, body)
	expect(t, w, http.StatusForbidden)

	// 401 sin token.
	w = e.do(http.MethodPut, "/groups/"+gid+"/discord-config", "", body)
	expect(t, w, http.StatusUnauthorized)

	// 404 grupo inexistente.
	w = e.do(http.MethodPut, "/groups/"+uuid.NewString()+"/discord-config", admin, body)
	expect(t, w, http.StatusNotFound)

	// 400 body inválido (sin server_name) y webhook no-discord.
	w = e.do(http.MethodPut, "/groups/"+gid+"/discord-config", admin, map[string]any{"invite_url": "https://discord.gg/abc"})
	expect(t, w, http.StatusBadRequest)
	w = e.do(http.MethodPut, "/groups/"+gid+"/discord-config", admin, map[string]any{
		"server_name": "S", "invite_url": "https://discord.gg/abc", "webhook_url": "https://example.com/hook",
	})
	expect(t, w, http.StatusBadRequest)

	// 400 group id con formato inválido.
	w = e.do(http.MethodPut, "/groups/no-uuid/discord-config", admin, body)
	expect(t, w, http.StatusBadRequest)
}
