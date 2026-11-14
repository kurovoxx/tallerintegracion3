package http

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// La segunda copia del mismo apunte por el mismo usuario responde 409
// already_copied; el replay de la MISMA clave sigue devolviendo 201 con el clon
// original (idempotencia no se confunde con duplicado).
func TestHandlerCopyDuplicateReturns409AlreadyCopied(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	src := createPublicNoteForRateLimit(t, r, at, "Duplicado")

	w1 := postJSONWithIdem(r, "/notes/"+src+"/copy", ct, "", "copy-dup-1")
	if w1.Code != http.StatusCreated {
		t.Fatalf("primera copia esperaba 201, got %d %s", w1.Code, w1.Body.String())
	}
	var first map[string]string
	if err := json.Unmarshal(w1.Body.Bytes(), &first); err != nil || first["note_id"] == "" {
		t.Fatalf("respuesta de copia inválida: %v %s", err, w1.Body.String())
	}

	w2 := postJSONWithIdem(r, "/notes/"+src+"/copy", ct, "", "copy-dup-2")
	if w2.Code != http.StatusConflict {
		t.Fatalf("segunda copia esperaba 409, got %d %s", w2.Code, w2.Body.String())
	}
	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("409 debe ser JSON válido: %v", err)
	}
	if errResp.Error.Code != "already_copied" {
		t.Fatalf("code esperado already_copied, got %q", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "Ya copiaste") {
		t.Fatalf("mensaje esperado de copia existente, got %q", errResp.Error.Message)
	}

	// Replay idempotente de la primera solicitud: mismo clon, sin 409.
	w3 := postJSONWithIdem(r, "/notes/"+src+"/copy", ct, "", "copy-dup-1")
	if w3.Code != http.StatusCreated {
		t.Fatalf("replay esperaba 201, got %d %s", w3.Code, w3.Body.String())
	}
	var replay map[string]string
	if err := json.Unmarshal(w3.Body.Bytes(), &replay); err != nil || replay["note_id"] != first["note_id"] {
		t.Fatalf("el replay debe devolver el mismo clon: %v %s", err, w3.Body.String())
	}
}
