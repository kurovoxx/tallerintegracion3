package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Búsqueda de un apunte por su ID exacto (GET /notes/{id}): el buscador del
// front pega un UUID y espera el apunte, un 404 sin filtrar existencia, o un
// 410 claro si el autor lo borró.

func getNoteByID(t *testing.T, h http.Handler, id, userID string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+genToken(userID, "student"))
	h.ServeHTTP(w, req)
	return w
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body inválido: %s", w.Body.String())
	}
	return body.Error.Code
}

func TestSearchByIDPublicNoteOfAnotherUser(t *testing.T) {
	r, svc, _, _ := setupRouter()
	ctx := context.Background()
	author, reader := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Física I", nil, "public", stringPtrForDelete("# Cinemática"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Like(ctx, author, note.ID); err != nil {
		t.Fatal(err)
	}

	// El ID se acepta en mayúsculas (copiado desde otra app).
	w := getNoteByID(t, r, strings.ToUpper(note.ID), reader)
	if w.Code != http.StatusOK {
		t.Fatalf("esperaba 200, got %d %s", w.Code, w.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["title"] != "Física I" || got["content"] != "# Cinemática" {
		t.Fatalf("payload inesperado: %v", got)
	}
	if got["is_own"] != false || got["is_liked"] != false || got["likes_count"] != float64(1) || got["saves_count"] != float64(0) {
		t.Fatalf("contadores/estado inesperados: %v", got)
	}
}

func TestSearchByIDPrivateWithoutPermissionIs404(t *testing.T) {
	r, svc, _, _ := setupRouter()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Privada", nil, "private", stringPtrForDelete("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	private := getNoteByID(t, r, note.ID, uuid.NewString())
	missing := getNoteByID(t, r, uuid.NewString(), uuid.NewString())
	for name, w := range map[string]*httptest.ResponseRecorder{"privada": private, "inexistente": missing} {
		if w.Code != http.StatusNotFound || errorCode(t, w) != "not_found" {
			t.Fatalf("%s: esperaba 404 not_found, got %d %s", name, w.Code, w.Body.String())
		}
	}
	// Zero-knowledge: privada sin permiso e inexistente son idénticas.
	if private.Body.String() != missing.Body.String() {
		t.Fatalf("las respuestas difieren: %s vs %s", private.Body.String(), missing.Body.String())
	}
	// El autor sí la encuentra.
	if w := getNoteByID(t, r, note.ID, author); w.Code != http.StatusOK {
		t.Fatalf("autor: %d %s", w.Code, w.Body.String())
	}
}

func TestSearchByIDDeletedSavedNoteIs410(t *testing.T) {
	r, svc, _, _ := setupRouter()
	ctx := context.Background()
	author, reader := uuid.NewString(), uuid.NewString()
	note, err := svc.Create(ctx, author, "Se borrará", nil, "public", stringPtrForDelete("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(ctx, reader, note.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, author, note.ID); err != nil {
		t.Fatal(err)
	}
	w := getNoteByID(t, r, note.ID, reader)
	if w.Code != http.StatusGone || errorCode(t, w) != "note_deleted" {
		t.Fatalf("esperaba 410 note_deleted, got %d %s", w.Code, w.Body.String())
	}
}

func TestSearchByIDMalformedIs404(t *testing.T) {
	r, _, _, _ := setupRouter()
	w := getNoteByID(t, r, "no-es-un-uuid", uuid.NewString())
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperaba 404, got %d", w.Code)
	}
}
