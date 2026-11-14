package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/identity"
)

// fakePublicProfileResolver permite probar la ruta HTTP sin gRPC real.
type fakePublicProfileResolver struct {
	ids map[string]string
	err error
}

func (f *fakePublicProfileResolver) ResolvePublicProfile(_ context.Context, publicID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if id, ok := f.ids[publicID]; ok {
		return id, nil
	}
	return "", identity.ErrNotFound
}

func doPublicProfileNotesGET(t *testing.T, publicID, token, query string) *httptest.ResponseRecorder {
	t.Helper()
	r, _, _, _ := setupRouter()
	path := "/public-users/" + publicID + "/notes"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandlerListPublicByPublicID_SinJWT_401(t *testing.T) {
	w := doPublicProfileNotesGET(t, "a8kp92mqwx7r", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestHandlerListPublicByPublicID_200_SoloPublicosSinUUID(t *testing.T) {
	r, svc, _, _ := setupRouter()
	ctx := context.Background()
	owner := uuid.NewString()
	requester := uuid.NewString()

	pub, err := svc.Create(ctx, owner, "Apunte público", nil, "public", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, owner, "Apunte privado", nil, "private", nil, ""); err != nil {
		t.Fatal(err)
	}
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{ids: map[string]string{"a8kp92mqwx7r": owner}})

	req := httptest.NewRequest(http.MethodGet, "/public-users/A8KP92MQWX7R/notes", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(requester, "student"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var body struct {
		Notes      []map[string]json.RawMessage `json:"notes"`
		NextCursor string                       `json:"next_cursor"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notes) != 1 {
		t.Fatalf("solo se esperaba un apunte público, got %s", w.Body.String())
	}
	item := body.Notes[0]
	var id string
	_ = json.Unmarshal(item["id"], &id)
	if id != pub.ID {
		t.Fatalf("id = %q, want %q", id, pub.ID)
	}
	for _, forbidden := range []string{"user_id", "external_file_id", "sync_status", "forked_from_note_id", "content", "version"} {
		if _, ok := item[forbidden]; ok {
			t.Fatalf("el DTO por public_id no debe exponer %q: %s", forbidden, w.Body.String())
		}
	}
	for _, required := range []string{"id", "title", "visibility", "likes_count", "saves_count", "is_liked", "is_saved", "created_at", "updated_at"} {
		if _, ok := item[required]; !ok {
			t.Fatalf("falta el campo %q en el DTO: %s", required, w.Body.String())
		}
	}
	if len(item) != 9 {
		t.Fatalf("campos inesperados en el DTO: %s", w.Body.String())
	}
}

func TestHandlerListPublicByPublicID_404_IdentityNoEncuentra(t *testing.T) {
	r, svc, _, _ := setupRouter()
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{err: identity.ErrNotFound})
	req := httptest.NewRequest(http.MethodGet, "/public-users/a8kp92mqwx7r/notes", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(uuid.NewString(), "student"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerListPublicByPublicID_503_IdentityCaido(t *testing.T) {
	r, svc, _, _ := setupRouter()
	svc.SetPublicProfileResolver(&fakePublicProfileResolver{err: identity.ErrUnavailable})
	req := httptest.NewRequest(http.MethodGet, "/public-users/a8kp92mqwx7r/notes", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(uuid.NewString(), "student"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("esperado 503, got %d %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
