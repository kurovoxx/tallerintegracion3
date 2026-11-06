package test

import (
	"net/http"
	"testing"
)

// Flujo completo "apunte no disponible al borrar el dueño":
// A crea un apunte público -> B lo guarda -> A lo elimina -> B sigue viendo
// la referencia en sus guardados marcada como no disponible, y abrir /
// copiar / likear / volver a guardar responde el 410 controlado. Lo único que
// B puede hacer es quitarlo de guardados.
func TestFlowOwnerDeletesSavedNoteBecomesUnavailable(t *testing.T) {
	api := newNotesAPI(t)
	alice, bob, carol := newUser(), newUser(), newUser()

	noteID := api.createNote(alice, "Cálculo II - Series", "public", "# Series\n$\\sum 1/n^2$")
	saved := api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+noteID+"/save", nil)
	if saved["saves_count"] != float64(1) || saved["is_saved"] != true {
		t.Fatalf("contadores tras guardar: %v", saved)
	}

	list := items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved", nil))
	if len(list) != 1 || list[0]["is_available"] != true || list[0]["title"] != "Cálculo II - Series" {
		t.Fatalf("guardado antes de borrar: %v", list)
	}

	// El dueño elimina su apunte.
	api.must(http.StatusNoContent, alice, http.MethodDelete, "/notes/"+noteID, nil)

	// B ve la referencia, marcada como no disponible (no desaparece ni da 500).
	list = items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved", nil))
	if len(list) != 1 {
		t.Fatalf("el guardado desapareció en silencio: %v", list)
	}
	item := list[0]
	if item["is_available"] != false || item["unavailable_reason"] != "deleted" ||
		item["title"] != "Cálculo II - Series" || item["deleted_at"] == nil || item["note_id"] != noteID {
		t.Fatalf("guardado no marcado como no disponible: %v", item)
	}
	if n := len(items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved?availability=unavailable", nil))); n != 1 {
		t.Fatalf("filtro unavailable: %d", n)
	}
	if n := len(items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved?availability=available", nil))); n != 0 {
		t.Fatalf("filtro available: %d", n)
	}

	// Abrir, copiar, likear o volver a guardar: error controlado 410.
	for _, op := range []struct{ method, path string }{
		{http.MethodGet, "/notes/" + noteID},
		{http.MethodPost, "/notes/" + noteID + "/copy"},
		{http.MethodPost, "/notes/" + noteID + "/like"},
		{http.MethodPost, "/notes/" + noteID + "/save"},
		{http.MethodGet, "/notes/" + noteID + "/stats"},
	} {
		body := api.must(http.StatusGone, bob, op.method, op.path, nil)
		if errCode(body) != "note_deleted" {
			t.Fatalf("%s %s: code=%q", op.method, op.path, errCode(body))
		}
	}
	// El intento de copia no dejó ningún clon a nombre de B.
	mine := api.must(http.StatusOK, bob, http.MethodGet, "/notes/me", nil)
	if notes, _ := mine["notes"].([]any); len(notes) != 0 {
		t.Fatalf("la copia fallida creó apuntes: %v", notes)
	}

	// El público también ve 410 (era público); el dueño ve 404.
	if errCode(api.must(http.StatusGone, carol, http.MethodGet, "/notes/"+noteID, nil)) != "note_deleted" {
		t.Fatal("un tercero debía ver note_deleted en un público borrado")
	}
	api.must(http.StatusNotFound, alice, http.MethodGet, "/notes/"+noteID, nil)

	// Única acción permitida: quitarlo de guardados (idempotente).
	api.must(http.StatusNoContent, bob, http.MethodDelete, "/notes/"+noteID+"/save", nil)
	api.must(http.StatusNoContent, bob, http.MethodDelete, "/notes/"+noteID+"/save", nil)
	if n := len(items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved", nil))); n != 0 {
		t.Fatalf("quitar de guardados no limpió la referencia: %d", n)
	}
}

// Un apunte privado borrado sigue siendo zero-knowledge para quien no lo
// tenía guardado: 404, nunca 410 (no confirma que existió).
func TestFlowDeletedPrivateNoteIsNotFoundForStrangers(t *testing.T) {
	api := newNotesAPI(t)
	alice, mallory := newUser(), newUser()
	noteID := api.createNote(alice, "Diario privado", "private", "secreto")
	api.must(http.StatusNoContent, alice, http.MethodDelete, "/notes/"+noteID, nil)
	body := api.must(http.StatusNotFound, mallory, http.MethodGet, "/notes/"+noteID, nil)
	if errCode(body) != "not_found" {
		t.Fatalf("code=%q", errCode(body))
	}
}

// Si el autor vuelve privado el apunte, el guardado queda no disponible sin
// filtrar su título actual, y abrirlo responde 404.
func TestFlowOwnerMakesSavedNotePrivate(t *testing.T) {
	api := newNotesAPI(t)
	alice, bob := newUser(), newUser()
	noteID := api.createNote(alice, "Apuntes compartidos", "public", "x")
	api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+noteID+"/save", nil)
	api.must(http.StatusOK, alice, http.MethodPatch, "/notes/"+noteID, map[string]any{
		"visibility": "private", "title": "Título secreto nuevo",
	})

	list := items(api.must(http.StatusOK, bob, http.MethodGet, "/notes/saved", nil))
	if len(list) != 1 || list[0]["is_available"] != false || list[0]["unavailable_reason"] != "no_access" {
		t.Fatalf("guardado privado: %v", list)
	}
	if list[0]["title"] == "Título secreto nuevo" {
		t.Fatal("se filtró el título actual de un apunte que ya no es accesible")
	}
	api.must(http.StatusNotFound, bob, http.MethodGet, "/notes/"+noteID, nil)
	api.must(http.StatusNoContent, bob, http.MethodDelete, "/notes/"+noteID+"/save", nil)
}
