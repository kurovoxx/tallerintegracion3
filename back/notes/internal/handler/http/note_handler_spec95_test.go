package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// patchWithIdem ejecuta PATCH /notes/{id} con header opcional X-Idempotency-Key.
func patchWithIdem(r *gin.Engine, noteID, token, body, key string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/notes/"+noteID, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("X-Idempotency-Key", key)
	}
	r.ServeHTTP(w, req)
	return w
}

// PATCH con X-Idempotency-Key: el primer request aplica el cambio; el replay
// con la misma clave y el mismo payload responde 200 desde el registro
// 'completed' sin volver a mutar el recurso; la misma clave con un body
// distinto responde 409 Conflict.
func TestHandlerPatchIdempotencyReplay(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	tok := genToken(author, "student")
	nid := createNoteHTTP(t, r, tok, "Patch Idem", "private", "v1")

	key := "patch-spec95-1"
	w1 := patchWithIdem(r, nid, tok, `{"title":"Primero"}`, key)
	if w1.Code != http.StatusOK {
		t.Fatalf("primer patch esperaba 200, got %d %s", w1.Code, w1.Body.String())
	}
	var r1 struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("respuesta patch inválida: %v", err)
	}
	if r1.Title != "Primero" {
		t.Fatalf("primer patch debe aplicar el título, got %q", r1.Title)
	}

	// Replay de red con la misma clave y el mismo payload.
	w2 := patchWithIdem(r, nid, tok, `{"title":"Primero"}`, key)
	if w2.Code != http.StatusOK {
		t.Fatalf("replay patch esperaba 200, got %d %s", w2.Code, w2.Body.String())
	}
	var r2 struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("respuesta replay inválida: %v", err)
	}
	if r2.Title != "Primero" {
		t.Fatalf("replay debe devolver el resultado cacheado (Primero), got %q", r2.Title)
	}

	// Misma clave con un body distinto: 409 Conflict, sin re-aplicar.
	w3 := patchWithIdem(r, nid, tok, `{"title":"Segundo"}`, key)
	if w3.Code != http.StatusConflict {
		t.Fatalf("misma clave con body distinto esperaba 409, got %d %s", w3.Code, w3.Body.String())
	}

	// El recurso no debe haber cambiado: GET devuelve el título aplicado una vez.
	wGet := doReq(r, http.MethodGet, "/notes/"+nid, tok, "")
	var got map[string]interface{}
	if err := json.Unmarshal(wGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta get inválida: %v", err)
	}
	if got["title"] != "Primero" {
		t.Fatalf("el replay no debe re-aplicar el cambio, title got %v", got["title"])
	}
}

// PATCH sin X-Idempotency-Key sigue funcionando igual que antes (idempotencia
// opcional): cada request aplica su cambio.
func TestHandlerPatchWithoutIdempotencyKeyAppliesEachChange(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	tok := genToken(author, "student")
	nid := createNoteHTTP(t, r, tok, "Patch no idem", "private", "v1")

	w1 := patchWithIdem(r, nid, tok, `{"title":"A"}`, "")
	if w1.Code != http.StatusOK {
		t.Fatalf("patch A esperaba 200, got %d", w1.Code)
	}
	w2 := patchWithIdem(r, nid, tok, `{"title":"B"}`, "")
	if w2.Code != http.StatusOK {
		t.Fatalf("patch B esperaba 200, got %d", w2.Code)
	}
	var r2 struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("respuesta inválida: %v", err)
	}
	if r2.Title != "B" {
		t.Fatalf("sin clave cada patch debe aplicar su cambio, got %q", r2.Title)
	}
}

// Una key distinta produce un nuevo resultado cacheado (no colisiona con la
// anterior) y los errores (nota inexistente) con clave propia se purgan y no
// bloquean un reintento posterior.
func TestHandlerPatchIdempotencyDistinctKeysAndErrorCleanup(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	tok := genToken(author, "student")
	nid := createNoteHTTP(t, r, tok, "Patch keys", "private", "v1")

	keyA := "patch-spec95-a"
	keyB := "patch-spec95-b"
	wA := patchWithIdem(r, nid, tok, `{"title":"AAA"}`, keyA)
	wB := patchWithIdem(r, nid, tok, `{"title":"BBB"}`, keyB)
	if wA.Code != http.StatusOK || wB.Code != http.StatusOK {
		t.Fatalf("patch con claves distintas esperaba 200, got %d/%d", wA.Code, wB.Code)
	}
	var rb struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(wB.Body.Bytes(), &rb); err != nil {
		t.Fatalf("respuesta inválida: %v", err)
	}
	if rb.Title != "BBB" {
		t.Fatalf("key distinta debe aplicar y cachear su propio resultado, got %q", rb.Title)
	}

	// Patch sobre nota inexistente con clave: 404 y la clave queda libre para
	// un reintento exitoso con la misma clave.
	missing := uuid.NewString()
	wFail := patchWithIdem(r, missing, tok, `{"title":"no"}`, "patch-spec95-c")
	if wFail.Code != http.StatusNotFound {
		t.Fatalf("patch de nota inexistente esperaba 404, got %d", wFail.Code)
	}
	wRetry := patchWithIdem(r, nid, tok, `{"title":"CCC"}`, "patch-spec95-c")
	if wRetry.Code != http.StatusOK {
		t.Fatalf("reintento con la misma clave tras el 404 debe proceder, got %d %s", wRetry.Code, wRetry.Body.String())
	}
}

// postJSONWithIdem ejecuta POST con header opcional X-Idempotency-Key.
func postJSONWithIdem(r *gin.Engine, path, token, body, key string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if key != "" {
		req.Header.Set("X-Idempotency-Key", key)
	}
	r.ServeHTTP(w, req)
	return w
}

// POST /notes con X-Idempotency-Key: el replay con el mismo payload devuelve el
// mismo note_id sin duplicar, y un payload distinto con la misma clave responde
// 409 Conflict (idempotencia durable en notes.idempotency_keys).
func TestHandlerCreateIdempotencyReplayAndConflict(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	tok := genToken(author, "student")
	key := "create-spec95-1"
	body := `{"title":"Idem Create","visibility":"private","content":"v1"}`
	w1 := postJSONWithIdem(r, "/notes", tok, body, key)
	if w1.Code != http.StatusCreated {
		t.Fatalf("primer create esperaba 201, got %d %s", w1.Code, w1.Body.String())
	}
	var r1 map[string]string
	if err := json.Unmarshal(w1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("respuesta create inválida: %v", err)
	}
	w2 := postJSONWithIdem(r, "/notes", tok, body, key)
	if w2.Code != http.StatusCreated {
		t.Fatalf("replay create esperaba 201, got %d %s", w2.Code, w2.Body.String())
	}
	var r2 map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("respuesta replay inválida: %v", err)
	}
	if r2["note_id"] != r1["note_id"] || r1["note_id"] == "" {
		t.Fatalf("el replay debe devolver el mismo note_id: %q vs %q", r2["note_id"], r1["note_id"])
	}
	w3 := postJSONWithIdem(r, "/notes", tok, `{"title":"Idem Create","visibility":"private","content":"v2"}`, key)
	if w3.Code != http.StatusConflict {
		t.Fatalf("payload distinto con la misma clave esperaba 409, got %d %s", w3.Code, w3.Body.String())
	}
}

// POST /notes/{id}/copy con X-Idempotency-Key: el replay devuelve el mismo clon
// y un origen distinto con la misma clave responde 409 Conflict.
func TestHandlerCopyIdempotencyReplayAndConflict(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	tokAuthor := genToken(author, "student")
	tokCopier := genToken(copier, "student")
	srcA := createNoteHTTP(t, r, tokAuthor, "Origen A", "public", "a")
	srcB := createNoteHTTP(t, r, tokAuthor, "Origen B", "public", "b")
	key := "copy-spec95-1"
	w1 := postJSONWithIdem(r, "/notes/"+srcA+"/copy", tokCopier, "", key)
	if w1.Code != http.StatusCreated {
		t.Fatalf("primer copy esperaba 201, got %d %s", w1.Code, w1.Body.String())
	}
	var r1 map[string]string
	if err := json.Unmarshal(w1.Body.Bytes(), &r1); err != nil {
		t.Fatalf("respuesta copy inválida: %v", err)
	}
	w2 := postJSONWithIdem(r, "/notes/"+srcA+"/copy", tokCopier, "", key)
	if w2.Code != http.StatusCreated {
		t.Fatalf("replay copy esperaba 201, got %d %s", w2.Code, w2.Body.String())
	}
	var r2 map[string]string
	if err := json.Unmarshal(w2.Body.Bytes(), &r2); err != nil {
		t.Fatalf("respuesta replay inválida: %v", err)
	}
	if r2["note_id"] != r1["note_id"] || r1["note_id"] == "" {
		t.Fatalf("el replay debe devolver el mismo clon: %q vs %q", r2["note_id"], r1["note_id"])
	}
	w3 := postJSONWithIdem(r, "/notes/"+srcB+"/copy", tokCopier, "", key)
	if w3.Code != http.StatusConflict {
		t.Fatalf("origen distinto con la misma clave esperaba 409, got %d %s", w3.Code, w3.Body.String())
	}
}
