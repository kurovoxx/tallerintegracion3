package test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	httpHandler "github.com/kurovoxx/tallerintegracion3/back/notes/internal/handler/http"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

// notesAPI es un harness HTTP de Notes para pruebas de flujo entre usuarios:
// mismos handlers, middleware JWT y servicio que producción, con stores en
// memoria y Drive simulado (MockClient). Cada usuario es solo un user_id
// distinto en su JWT.
type notesAPI struct {
	t      *testing.T
	router *gin.Engine
	svc    *service.NoteService
	social *service.MemorySocialResolver
	drive  *drive.MockClient
}

const harnessSecret = "notes-flow-secret"

func newNotesAPI(t *testing.T) *notesAPI {
	t.Helper()
	gin.SetMode(gin.TestMode)
	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(
		noteStore, service.NewMemoryAttachmentStore(), service.NewMemorySavedStore(),
		likeStore, service.NewMemorySharedStore(), driveMock, social,
	)
	auth := middleware.NewAuthMiddleware(&middleware.SimpleHS256Validator{
		Secret: []byte(harnessSecret), Issuer: e2eIssuer, Audience: e2eAudience,
	})
	h := httpHandler.NewNoteHandler(svc)
	r := gin.New()
	p := r.Group("")
	p.Use(auth.RequireAuth())
	// Mismo inventario y orden que cmd/server/main.go (estáticas primero).
	p.POST("/notes", h.Create)
	p.GET("/notes/me", h.ListMy)
	p.GET("/notes/saved", h.ListSaved)
	p.GET("/notes/public", h.ListPublic)
	p.POST("/notes/:id/save", h.Save)
	p.DELETE("/notes/:id/save", h.Unsave)
	p.GET("/notes/:id/stats", h.Stats)
	p.POST("/notes/:id/copy", h.Copy)
	p.POST("/notes/:id/like", h.Like)
	p.DELETE("/notes/:id/like", h.Unlike)
	p.POST("/notes/:id/share", h.Share)
	p.PATCH("/notes/:id", h.Patch)
	p.DELETE("/notes/:id", h.Delete)
	p.GET("/notes/:id", h.Get)
	p.DELETE("/notes/shared/:sharedNoteId", h.Unshare)
	p.GET("/groups/:id/notes", h.ListGroupNotes)
	return &notesAPI{t: t, router: r, svc: svc, social: social, drive: driveMock}
}

func (a *notesAPI) token(userID string) string {
	a.t.Helper()
	claims := middleware.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    e2eIssuer,
			Audience:  jwt.ClaimStrings{e2eAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(harnessSecret))
	if err != nil {
		a.t.Fatal(err)
	}
	return tok
}

// call ejecuta una petición como userID y devuelve el status y el body JSON
// decodificado (nil si no hay body).
func (a *notesAPI) call(userID, method, path string, body any, headers ...string) (int, map[string]any) {
	a.t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Authorization", "Bearer "+a.token(userID))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, req)
	if w.Body.Len() == 0 {
		return w.Code, nil
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		a.t.Fatalf("%s %s: body no JSON (%d): %s", method, path, w.Code, w.Body.String())
	}
	return w.Code, out
}

// must exige el status esperado y devuelve el body.
func (a *notesAPI) must(want int, userID, method, path string, body any, headers ...string) map[string]any {
	a.t.Helper()
	got, out := a.call(userID, method, path, body, headers...)
	if got != want {
		a.t.Fatalf("%s %s como %s: status %d, esperado %d (body=%v)", method, path, userID[:8], got, want, out)
	}
	return out
}

// createNote crea un apunte vía POST /notes y devuelve su id.
func (a *notesAPI) createNote(owner, title, visibility, content string) string {
	a.t.Helper()
	out := a.must(http.StatusCreated, owner, http.MethodPost, "/notes", map[string]any{
		"title": title, "visibility": visibility, "content": content,
	})
	id, _ := out["note_id"].(string)
	if id == "" {
		a.t.Fatalf("POST /notes sin note_id: %v", out)
	}
	return id
}

func errCode(body map[string]any) string {
	e, _ := body["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func items(body map[string]any) []map[string]any {
	raw, _ := body["items"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, it := range raw {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func newUser() string { return uuid.NewString() }
