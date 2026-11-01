package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/config"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
)

// Usa el router productivo: detecta endpoints omitidos del inventario y rutas
// accidentalmente públicas, sin depender de routers recreados en los tests.
func TestHTTPRouterContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{Secret: []byte("router-test")})
	r := newHTTPRouter(nil, &config.Config{StorageMode: "mock"}, auth)
	expected := map[string]bool{
		"GET /health": true, "GET /health/drive": true,
		"POST /notes": false, "GET /notes/me": false,
		"POST /notes/unshare-all": false, "POST /notes/reconcile": false,
		"POST /notes/upload": false, "GET /notes/:id/access": false,
		"POST /notes/:id/attachments":                      false,
		"GET /notes/:id/attachments/:attachmentId/content": false,
		"DELETE /notes/:id/attachments/:attachmentId":      false,
		"POST /notes/:id/save":                             false, "POST /notes/:id/copy": false,
		"POST /notes/:id/like": false, "DELETE /notes/:id/like": false,
		"POST /notes/:id/share": false, "PATCH /notes/:id": false,
		"DELETE /notes/:id": false, "GET /notes/:id": false,
		"DELETE /notes/shared/:sharedNoteId": false, "GET /groups/:id/notes": false,
	}
	if len(r.Routes()) != len(expected) {
		t.Fatalf("rutas=%d, contrato=%d", len(r.Routes()), len(expected))
	}
	for _, route := range r.Routes() {
		key := route.Method + " " + route.Path
		public, exists := expected[key]
		if !exists {
			t.Fatalf("ruta sin contrato: %s", key)
		}
		t.Run(key, func(t *testing.T) {
			path := strings.NewReplacer(":attachmentId", "33333333-3333-4333-8333-333333333333", ":sharedNoteId", "44444444-4444-4444-8444-444444444444", ":id", "11111111-1111-4111-8111-111111111111").Replace(route.Path)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(route.Method, path, nil))
			want := 401
			if public {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("status=%d, want=%d body=%s", w.Code, want, w.Body.String())
			}
			if route.Path == "/health" && !strings.Contains(w.Body.String(), `"service":"notes-service"`) {
				t.Fatal(w.Body.String())
			}
			if route.Path == "/health/drive" && w.Body.String() != `{"storage_mode":"mock"}` {
				t.Fatal(w.Body.String())
			}
		})
	}
}
