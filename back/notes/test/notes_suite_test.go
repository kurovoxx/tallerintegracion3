package test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// Suite de Notes sobre la API HTTP real (harness en notes_http_harness_test.go):
// likes/guardados idempotentes y con contadores exactos, filtros de guardados,
// copia a apuntes propios, share duplicado y búsqueda por ID.

// --- Likes y guardados ---

func TestSuiteLikeAndSaveAreIdempotentWithExactCounters(t *testing.T) {
	api := newNotesAPI(t)
	author, bob, carol := newUser(), newUser(), newUser()
	id := api.createNote(author, "Probabilidad", "public", "P(A|B)")

	like := api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+id+"/like", nil)
	if like["likes_count"] != float64(1) || like["is_liked"] != true {
		t.Fatalf("like: %v", like)
	}
	// Repetir no duplica: 409 controlado y el contador no cambia.
	if errCode(api.must(http.StatusConflict, bob, http.MethodPost, "/notes/"+id+"/like", nil)) != "already_liked" {
		t.Fatal("like repetido debía ser already_liked")
	}
	api.must(http.StatusCreated, carol, http.MethodPost, "/notes/"+id+"/like", nil)
	api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+id+"/save", nil)
	if errCode(api.must(http.StatusConflict, bob, http.MethodPost, "/notes/"+id+"/save", nil)) != "already_saved" {
		t.Fatal("save repetido debía ser already_saved")
	}

	stats := api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+id+"/stats", nil)
	if stats["likes_count"] != float64(2) || stats["saves_count"] != float64(1) ||
		stats["is_liked"] != true || stats["is_saved"] != true {
		t.Fatalf("stats: %v", stats)
	}
	carolStats := api.must(http.StatusOK, carol, http.MethodGet, "/notes/"+id+"/stats", nil)
	if carolStats["is_saved"] != false || carolStats["is_liked"] != true {
		t.Fatalf("el estado es por usuario: %v", carolStats)
	}

	// Quitar dos veces: 204 ambas, el contador nunca baja de lo real.
	for i := 0; i < 2; i++ {
		api.must(http.StatusNoContent, bob, http.MethodDelete, "/notes/"+id+"/like", nil)
		api.must(http.StatusNoContent, bob, http.MethodDelete, "/notes/"+id+"/save", nil)
	}
	stats = api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+id+"/stats", nil)
	if stats["likes_count"] != float64(1) || stats["saves_count"] != float64(0) || stats["is_liked"] != false {
		t.Fatalf("stats tras quitar: %v", stats)
	}
	// GET /notes/:id expone los mismos contadores.
	got := api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+id, nil)
	if got["likes_count"] != float64(1) || got["saves_count"] != float64(0) || got["is_own"] != false {
		t.Fatalf("GET /notes/:id: %v", got)
	}
}

func TestSuiteConcurrentLikesStayExact(t *testing.T) {
	api := newNotesAPI(t)
	author, spammer := newUser(), newUser()
	id := api.createNote(author, "Concurrencia", "public", "x")

	// Mismo usuario, 20 likes simultáneos: exactamente uno entra.
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, _ := api.call(spammer, http.MethodPost, "/notes/"+id+"/like", nil)
			mu.Lock()
			codes[code]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[http.StatusCreated] != 1 || codes[http.StatusConflict] != 19 {
		t.Fatalf("respuestas del like concurrente: %v", codes)
	}

	// 15 usuarios distintos a la vez: el contador termina exacto.
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			api.call(newUser(), http.MethodPost, "/notes/"+id+"/like", nil)
		}()
	}
	wg.Wait()
	stats := api.must(http.StatusOK, author, http.MethodGet, "/notes/"+id+"/stats", nil)
	if stats["likes_count"] != float64(16) {
		t.Fatalf("likes_count=%v, esperado 16", stats["likes_count"])
	}
}

func TestSuitePrivateNoteCannotBeLikedOrSavedByStrangers(t *testing.T) {
	api := newNotesAPI(t)
	author, stranger := newUser(), newUser()
	id := api.createNote(author, "Privada", "private", "x")
	for _, path := range []string{"/like", "/save"} {
		if errCode(api.must(http.StatusNotFound, stranger, http.MethodPost, "/notes/"+id+path, nil)) != "not_found" {
			t.Fatalf("%s sobre privada ajena debía ser 404", path)
		}
	}
}

// --- Filtros de guardados y públicos ---

func TestSuiteSavedFiltersOverHTTP(t *testing.T) {
	api := newNotesAPI(t)
	me, ana, beto := newUser(), newUser(), newUser()
	math := uuid.NewString()
	save := func(owner, title string, subject *string) string {
		t.Helper()
		body := map[string]any{"title": title, "visibility": "public", "content": "x"}
		if subject != nil {
			body["subject_id"] = *subject
		}
		out := api.must(http.StatusCreated, owner, http.MethodPost, "/notes", body)
		id := out["note_id"].(string)
		api.must(http.StatusCreated, me, http.MethodPost, "/notes/"+id+"/save", nil)
		return id
	}
	save(ana, "Integrales dobles", &math)
	save(beto, "Redes OSI", nil)
	save(me, "Mi resumen de integrales", nil)

	count := func(query string) int {
		t.Helper()
		return len(items(api.must(http.StatusOK, me, http.MethodGet, "/notes/saved"+query, nil)))
	}
	cases := map[string]int{
		"":                                 3,
		"?scope=others":                    2,
		"?scope=own":                       1,
		"?q=INTEGRALES":                    2,
		"?q=integrales&scope=others":       1,
		"?subject_id=" + math:              1,
		"?author_id=" + beto:               1,
		"?availability=available&limit=50": 3,
	}
	for q, want := range cases {
		if got := count(q); got != want {
			t.Errorf("GET /notes/saved%s: %d, esperado %d", q, got, want)
		}
	}
	for _, bad := range []string{"?scope=todos", "?availability=x", "?author_id=nope", "?cursor=nope"} {
		if errCode(api.must(http.StatusBadRequest, me, http.MethodGet, "/notes/saved"+bad, nil)) != "bad_request" {
			t.Errorf("%s debía ser 400 bad_request", bad)
		}
	}

	// Los públicos de otros se listan con su estado de guardado.
	pub := items(api.must(http.StatusOK, me, http.MethodGet, "/notes/public?q=redes", nil))
	if len(pub) != 1 || pub[0]["is_saved"] != true || pub[0]["user_id"] != beto {
		t.Fatalf("GET /notes/public: %v", pub)
	}
}

// --- Copia a apuntes propios ---

func TestSuiteCopySavedNoteChangesOwner(t *testing.T) {
	api := newNotesAPI(t)
	alice, bob, carol := newUser(), newUser(), newUser()
	origID := api.createNote(alice, "Álgebra lineal", "public", "# Matrices\n$A^{-1}$")
	api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+origID+"/save", nil)

	copy1 := api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+origID+"/copy", nil, "X-Idempotency-Key", "copy-1")
	cloneID, _ := copy1["note_id"].(string)
	if cloneID == "" || cloneID == origID || copy1["user_id"] != bob ||
		copy1["forked_from_note_id"] != origID || copy1["visibility"] != "private" {
		t.Fatalf("respuesta de copia: %v", copy1)
	}
	// Reintento con la misma clave: mismo clon, sin duplicar.
	copy2 := api.must(http.StatusCreated, bob, http.MethodPost, "/notes/"+origID+"/copy", nil, "X-Idempotency-Key", "copy-1")
	if copy2["note_id"] != cloneID {
		t.Fatalf("replay idempotente creó otro clon: %v vs %v", copy2["note_id"], cloneID)
	}

	clone := api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+cloneID, nil)
	if clone["user_id"] != bob || clone["is_own"] != true || clone["content"] != "# Matrices\n$A^{-1}$" {
		t.Fatalf("clon: %v", clone)
	}
	if clone["external_file_id"] == api.must(http.StatusOK, alice, http.MethodGet, "/notes/"+origID, nil)["external_file_id"] {
		t.Fatal("el clon comparte archivo de Drive con el original")
	}
	mine, _ := api.must(http.StatusOK, bob, http.MethodGet, "/notes/me", nil)["notes"].([]any)
	if len(mine) != 1 {
		t.Fatalf("B debía tener exactamente 1 apunte propio, tiene %d", len(mine))
	}
	// El clon es privado de B: un tercero no lo ve.
	api.must(http.StatusNotFound, carol, http.MethodGet, "/notes/"+cloneID, nil)
	// Y no se puede copiar el privado ajeno de nadie.
	priv := api.createNote(alice, "Privada de A", "private", "x")
	api.must(http.StatusNotFound, bob, http.MethodPost, "/notes/"+priv+"/copy", nil)
}

// --- Share duplicado ---

func TestSuiteShareSameNoteTwiceToGroupIsConflict(t *testing.T) {
	api := newNotesAPI(t)
	owner := newUser()
	group, other := uuid.NewString(), uuid.NewString()
	api.social.AddMember(owner, group)
	api.social.AddMember(owner, other)
	id := api.createNote(owner, "Para el grupo", "private", "x")
	share := func(groupID, mode string, want int) map[string]any {
		t.Helper()
		return api.must(want, owner, http.MethodPost, "/notes/"+id+"/share", map[string]any{
			"group_id": groupID, "access_mode": mode,
		})
	}

	first := share(group, "link", http.StatusCreated)
	for _, mode := range []string{"link", "restricted"} {
		dup := share(group, mode, http.StatusConflict)
		e := dup["error"].(map[string]any)
		if e["code"] != "already_shared" || e["message"] != "Este apunte ya está compartido en este grupo" {
			t.Fatalf("share duplicado (%s): %v", mode, dup)
		}
	}
	share(other, "link", http.StatusCreated)

	listed := api.must(http.StatusOK, owner, http.MethodGet, "/groups/"+group+"/notes", nil)
	if notes, _ := listed["notes"].([]any); len(notes) != 1 {
		t.Fatalf("el grupo debía listar la nota una sola vez: %v", listed)
	}

	// Tras dejar de compartir, se puede volver a compartir en ese grupo.
	api.must(http.StatusNoContent, owner, http.MethodDelete, fmt.Sprintf("/notes/shared/%s", first["shared_note_id"]), nil)
	share(group, "restricted", http.StatusCreated)
}

// --- Búsqueda por ID ---

func TestSuiteSearchByID(t *testing.T) {
	api := newNotesAPI(t)
	alice, bob := newUser(), newUser()
	group := uuid.NewString()
	api.social.AddMember(alice, group)

	public := api.createNote(alice, "Física I", "public", "# Cinemática")
	private := api.createNote(alice, "Privada", "private", "secreto")
	linked := api.createNote(alice, "Con enlace", "private", "por link")
	api.must(http.StatusCreated, alice, http.MethodPost, "/notes/"+linked+"/share", map[string]any{
		"group_id": group, "access_mode": "link",
	})

	// Éxito: público ajeno, privado propio y privado con share link.
	got := api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+public, nil)
	if got["title"] != "Física I" || got["content"] != "# Cinemática" || got["is_own"] != false {
		t.Fatalf("público: %v", got)
	}
	if api.must(http.StatusOK, alice, http.MethodGet, "/notes/"+private, nil)["is_own"] != true {
		t.Fatal("el autor debe encontrar su privado")
	}
	api.must(http.StatusOK, bob, http.MethodGet, "/notes/"+linked, nil)

	// Privado sin permiso e inexistente: el mismo 404 (no revela existencia).
	_, noPerm := api.call(bob, http.MethodGet, "/notes/"+private, nil)
	_, missing := api.call(bob, http.MethodGet, "/notes/"+uuid.NewString(), nil)
	if errCode(noPerm) != "not_found" || fmt.Sprint(noPerm) != fmt.Sprint(missing) {
		t.Fatalf("privado sin permiso=%v, inexistente=%v", noPerm, missing)
	}
	api.must(http.StatusNotFound, bob, http.MethodGet, "/notes/no-es-uuid", nil)
}
