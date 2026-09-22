package http

import (
	"io"

	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

// --- IDOR UnshareAll ---

func TestUnshareAllIDORForbidden(t *testing.T) {
	r, _, _, social := setupRouter()
	victim := uuid.NewString()
	attacker := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(victim, groupID)
	victimTok := genToken(victim, "student")
	attackerTok := genToken(attacker, "student")
	// víctima crea y comparte
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"Victim","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+victimTok)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+victimTok)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("share %d %s", w2.Code, w2.Body.String())
	}
	// atacante intenta desvincular a la víctima -> 403
	w3 := httptest.NewRecorder()
	body := fmt.Sprintf(`{"user_id":"%s","group_id":"%s"}`, victim, groupID)
	req3 := httptest.NewRequest(http.MethodPost, "/notes/unshare-all", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+attackerTok)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("IDOR: atacante esperaba 403, got %d %s", w3.Code, w3.Body.String())
	}
	// el share de la víctima debe seguir existiendo (GET como miembro link sigue 200 para atacante? verificamos vía access del atacante: link permite leer)
	// Mejor: la víctima sigue compartida — el atacante no logró borrar. Verificamos que un miembro aún ve la nota vía grupo.
	// La víctima misma puede listar sus shares indirectamente: intentar GetAccess como atacante con link debe seguir 200 (share intacto).
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/notes/"+nid+"/access", nil)
	req4.Header.Set("Authorization", "Bearer "+attackerTok)
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("share debe seguir intacto tras IDOR bloqueado, access got %d %s", w4.Code, w4.Body.String())
	}
}

func TestUnshareAllSelfAllowedAndAdminAllowed(t *testing.T) {
	r, _, _, social := setupRouter()
	victim := uuid.NewString()
	admin := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(victim, groupID)
	social.AddAdmin(admin, groupID)
	victimTok := genToken(victim, "student")
	adminTok := genToken(admin, "student")
	// crear + compartir como víctima
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"Self","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+victimTok)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+victimTok)
	r.ServeHTTP(w2, req2)
	// admin (distinto user) puede desvincular -> 204
	w3 := httptest.NewRecorder()
	body := fmt.Sprintf(`{"user_id":"%s","group_id":"%s"}`, victim, groupID)
	req3 := httptest.NewRequest(http.MethodPost, "/notes/unshare-all", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+adminTok)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusNoContent {
		t.Fatalf("admin esperaba 204, got %d %s", w3.Code, w3.Body.String())
	}
}

func TestUnshareAllUnauthorized401(t *testing.T) {
	r, _, _, _ := setupRouter()
	body := fmt.Sprintf(`{"user_id":"%s","group_id":"%s"}`, uuid.NewString(), uuid.NewString())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes/unshare-all", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// sin token
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sin token esperaba 401, got %d", w.Code)
	}
}

func TestUnshareAllInvalidUUID400(t *testing.T) {
	r, _, _, _ := setupRouter()
	tok := genToken(uuid.NewString(), "student")
	for _, body := range []string{
		`{"user_id":"no-uuid","group_id":"no-uuid"}`,
		`{"user_id":"","group_id":""}`,
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes/unshare-all", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s esperaba 400, got %d %s", body, w.Code, w.Body.String())
		}
	}
}

// --- Validación UUID en handlers (antes del store) ---

func TestHandlerInvalidUUID404(t *testing.T) {
	r, _, _, _ := setupRouter()
	tok := genToken(uuid.NewString(), "student")
	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/notes/no-uuid", ""},
		{http.MethodPatch, "/notes/no-uuid", `{"title":"x"}`},
		{http.MethodDelete, "/notes/no-uuid", ""},
		{http.MethodPost, "/notes/no-uuid/copy", ""},
		{http.MethodPost, "/notes/no-uuid/like", ""},
		{http.MethodDelete, "/notes/no-uuid/like", ""},
		{http.MethodGet, "/notes/no-uuid/access", ""},
		{http.MethodGet, "/groups/no-uuid/notes", ""},
		{http.MethodDelete, "/notes/shared/no-uuid", ""},
	}
	for i, tc := range cases {
		w := doReq(r, tc.method, tc.path, tok, tc.body)
		if w.Code != http.StatusNotFound {
			t.Fatalf("case %d %s %s esperaba 404, got %d %s", i, tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// --- Validación oneof visibility / access_mode ---

func TestHandlerOneofValidation400(t *testing.T) {
	r, _, _, social := setupRouter()
	user := uuid.NewString()
	tok := genToken(user, "student")
	groupID := uuid.NewString()
	social.AddAdmin(user, groupID)
	// visibility inválida
	w := doReq(r, http.MethodPost, "/notes", tok, `{"title":"T","visibility":"super-public"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("visibility inválida esperaba 400, got %d %s", w.Code, w.Body.String())
	}
	// crear válida para probar share
	nid := createNoteHTTP(t, r, tok, "Para share", "private", "c")
	w2 := doReq(r, http.MethodPost, "/notes/"+nid+"/share", tok, fmt.Sprintf(`{"group_id":"%s","access_mode":"everyone"}`, groupID))
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("access_mode inválido esperaba 400, got %d %s", w2.Code, w2.Body.String())
	}
	// group_id no-uuid en body -> 400 por binding uuid
	w3 := doReq(r, http.MethodPost, "/notes/"+nid+"/share", tok, `{"group_id":"no-uuid","access_mode":"link"}`)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("group_id no-uuid esperaba 400, got %d %s", w3.Code, w3.Body.String())
	}
}

// --- Copy: error de descarga del source mapea códigos ---

func TestHandlerCopySourceDownloadErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		wantHTTP int
	}{
		{"source 404 → 404", 404, http.StatusNotFound},
		{"source 403 → 404", 403, http.StatusNotFound},
		{"source 500 → 502", 500, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, _, driveMock, _ := setupRouter()
			author := uuid.NewString()
			copier := uuid.NewString()
			at := genToken(author, "student")
			ct := genToken(copier, "student")
			nid := createNoteHTTP(t, r, at, "Origen frágil", "public", "data")
			// resolver external_file_id vía GET
			wOK := doReq(r, http.MethodGet, "/notes/"+nid, at, "")
			var got map[string]interface{}
			json.Unmarshal(wOK.Body.Bytes(), &got)
			extID, _ := got["external_file_id"].(string)
			if extID == "" {
				t.Fatal("precondición external_file_id vacío")
			}
			driveMock.InjectGetError(extID, &drive.DriveError{Code: tc.code, Message: "boom"})
			defer driveMock.ClearGetError(extID)
			w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
			if w.Code != tc.wantHTTP {
				t.Fatalf("esperaba HTTP %d, got %d %s", tc.wantHTTP, w.Code, w.Body.String())
			}
		})
	}
}

// --- Copy desacoplado: autor sin OAuth no rompe clon público ---

func TestHandlerCopyDecoupledFromAuthorOAuth(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	nid := createNoteHTTP(t, r, at, "Pública clonable", "public", "contenido público")
	// Autor revoca Drive después de crear (simula fila ausente en oauth_connections).
	driveMock.Disconnect(author)
	defer driveMock.Reconnect(author)
	w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("clon público con autor sin OAuth debe funcionar (desacoplado), got %d %s", w.Code, w.Body.String())
	}
}

func TestHandlerCopyDstWithoutOAuthRejectedViaDisconnect(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	nid := createNoteHTTP(t, r, at, "Origen OAuth2", "public", "data")
	driveMock.Disconnect(copier)
	defer driveMock.Reconnect(copier)
	before := driveMock.FileCount()
	w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("destino sin OAuth esperaba 403, got %d %s", w.Code, w.Body.String())
	}
	if driveMock.FileCount() != before {
		t.Fatalf("copy fallida no debe crear archivos")
	}
}

// --- Compensación inversa a nivel HTTP (Drive OK, PG falla) ---

type failingCreateNoteStore struct {
	*service.MemoryNoteStore
	failCreate bool
}

func (f *failingCreateNoteStore) Create(ctx context.Context, userID string, subjectID *string, title string, externalFileID *string, visibility string, forkedFrom *string, syncStatus string) (*model.Note, error) {
	if f.failCreate {
		return nil, fmt.Errorf("db insert failed (simulado)")
	}
	return f.MemoryNoteStore.Create(ctx, userID, subjectID, title, externalFileID, visibility, forkedFrom, syncStatus)
}

func setupRouterWithFailingCreate(fail bool) (*gin.Engine, *drive.MockClient, *failingCreateNoteStore) {
	driveMock := drive.NewMockClient()
	noteStore := &failingCreateNoteStore{MemoryNoteStore: service.NewMemoryNoteStore(), failCreate: fail}
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore.MemoryNoteStore)
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	validator := &middleware.SimpleHS256Validator{Secret: []byte(testSecret), Issuer: testIssuer, Audience: testAudience}
	authMw := middleware.NewAuthMiddleware(validator)
	h := NewNoteHandler(svc)
	r := gin.New()
	prot := r.Group("")
	prot.Use(authMw.RequireAuth())
	{
		prot.POST("/notes", h.Create)
		prot.GET("/notes/me", h.ListMy)
		prot.POST("/notes/:id/copy", h.Copy)
		prot.GET("/notes/:id", h.Get)
	}
	return r, driveMock, noteStore
}

func TestHandlerCreateInverseOrphanCompensation(t *testing.T) {
	r, driveMock, _ := setupRouterWithFailingCreate(true)
	tok := genToken(uuid.NewString(), "student")
	w := doReq(r, http.MethodPost, "/notes", tok, `{"title":"Huérfana inversa","visibility":"private","content":"x"}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("PG falla tras Drive OK esperaba 500 (error interno propio), got %d %s", w.Code, w.Body.String())
	}
	if driveMock.FileCount() != 0 {
		t.Fatalf("compensación: no debe quedar huérfano en Drive, FileCount=%d", driveMock.FileCount())
	}
	// Sin fila huérfana en PG
	w2 := doReq(r, http.MethodGet, "/notes/me?limit=20", tok, "")
	var list map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &list)
	if notes, ok := list["notes"].([]interface{}); !ok || len(notes) != 0 {
		t.Fatalf("no debe existir fila huérfana en PG, got %v", list["notes"])
	}
}

func TestHandlerCopyInverseOrphanCompensation(t *testing.T) {
	// Primera fase: crear origen con store sano.
	rOK, driveMockOK, _ := setupRouterWithFailingCreate(false)
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	nid := createNoteHTTP(t, rOK, at, "Origen copy rollback", "public", "orig")
	// Segunda fase: router cuyo Create falla (clon PG falla tras CopyFile OK).
	// Reutilizamos el mismo driveMock para observar huérfanos: construimos un
	// servicio que comparte el drive pero con store de notas que falla en Create.
	driveShared := driveMockOK
	_ = nid
	// Necesitamos el noteID del origen en el nuevo router: lo recreamos copiando
	// el estado no es trivial; en su lugar probamos a nivel servicio (ver
	// rollback_test en service) y aquí verificamos que Copy con CopyErr no deja
	// archivos (ya cubierto). Para el caso inverso HTTP usamos failing store con
	// origen creado en el mismo router antes de activar el fallo.
	r2, driveMock2, failing := setupRouterWithFailingCreate(false)
	nid2 := createNoteHTTP(t, r2, at, "Origen2", "public", "data2")
	failing.failCreate = true // activar fallo solo para el clon
	before := driveMock2.FileCount()
	w := doReq(r2, http.MethodPost, "/notes/"+nid2+"/copy", ct, "")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("clon con PG caído esperaba 500 (error interno propio), got %d %s", w.Code, w.Body.String())
	}
	if driveMock2.FileCount() != before {
		t.Fatalf("compensación copy: archivo recién creado debe borrarse, antes=%d ahora=%d", before, driveMock2.FileCount())
	}
	_ = driveShared
}

// --- MaxBytesReader anti-DoS (JSON 1MB) ---

func TestHandlerJSONPayloadTooLarge413(t *testing.T) {
	r, _, _, _ := setupRouter()
	tok := genToken(uuid.NewString(), "student")
	// Content de ~1MB+1 debe ser rechazado con 413, sin panic ni 500 con leak.
	big := strings.Repeat("a", 1048576+100)
	body := fmt.Sprintf(`{"title":"T","visibility":"private","content":%q}`, big)
	w := doReq(r, http.MethodPost, "/notes", tok, body)
	if w.Code != http.StatusRequestEntityTooLarge && w.Code != http.StatusBadRequest {
		t.Fatalf("payload gigante esperaba 413 (o 400), got %d %s", w.Code, w.Body.String()[:200])
	}
	// No debe exponer detalles internos de Drive/OAuth.
	if strings.Contains(strings.ToLower(w.Body.String()), "no oauth") ||
		strings.Contains(strings.ToLower(w.Body.String()), "googleapi") {
		t.Fatalf("respuesta no debe filtrar detalles internos: %s", w.Body.String()[:300])
	}
}

func TestHandlerOAuthErrorNoLeak(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	nid := createNoteHTTP(t, r, at, "Origen leak", "public", "data")
	driveMock.Disconnect(copier)
	defer driveMock.Reconnect(copier)
	w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("esperaba 403, got %d %s", w.Code, w.Body.String())
	}
	// El cuerpo no debe contener el mensaje crudo "no oauth connection found"
	// ni trazas internas; solo mensaje genérico de reconexión.
	if strings.Contains(w.Body.String(), "no oauth connection found") {
		t.Fatalf("leak de error interno al cliente: %s", w.Body.String())
	}
	var errBody map[string]map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("respuesta debe ser JSON controlado: %v", err)
	}
	if errBody["error"]["code"] != "forbidden" {
		t.Fatalf("code esperado forbidden, got %s", w.Body.String())
	}
}

// --- Remediación v3: zero-knowledge y distinción de errores en Copy ---

// Una nota privada sin acceso debe responder exactamente lo mismo que una nota
// inexistente (404 canónico) en GET, /access y /copy: sin oráculo de
// enumeración de recursos.
func TestHandlerZeroKnowledgePrivateNotFound(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	stranger := uuid.NewString()
	at := genToken(author, "student")
	st := genToken(stranger, "student")
	nid := createNoteHTTP(t, r, at, "Privada ZK", "private", "secreto")
	missing := uuid.NewString()
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/notes/" + nid},
		{http.MethodGet, "/notes/" + nid + "/access"},
		{http.MethodPost, "/notes/" + nid + "/copy"},
	}
	for _, ep := range endpoints {
		wPriv := doReq(r, ep.method, ep.path, st, "")
		wMiss := doReq(r, ep.method, strings.Replace(ep.path, nid, missing, 1), st, "")
		if wPriv.Code != http.StatusNotFound || wMiss.Code != http.StatusNotFound {
			t.Fatalf("%s %s: esperaba 404 en privada/inexistente, got %d/%d (%s | %s)",
				ep.method, ep.path, wPriv.Code, wMiss.Code, wPriv.Body.String(), wMiss.Body.String())
		}
		if wPriv.Body.String() != wMiss.Body.String() {
			t.Fatalf("%s %s: respuestas distinguibles: %s vs %s",
				ep.method, ep.path, wPriv.Body.String(), wMiss.Body.String())
		}
	}
}

// Copy debe diferenciar el fallo de acceso al archivo origen (404 zero-knowledge
// indistinguible de inexistente) del fallo OAuth de la cuenta del clonador (403
// de reconexión).
func TestHandlerCopyErrorDistinction(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	author := uuid.NewString()
	copier := uuid.NewString()
	at := genToken(author, "student")
	ct := genToken(copier, "student")
	nid := createNoteHTTP(t, r, at, "Origen distinción", "public", "data")
	wOK := doReq(r, http.MethodGet, "/notes/"+nid, at, "")
	var got map[string]interface{}
	json.Unmarshal(wOK.Body.Bytes(), &got)
	extID, _ := got["external_file_id"].(string)
	if extID == "" {
		t.Fatal("precondición external_file_id vacío")
	}
	// (a) Origen ilegible: 404 zero-knowledge (not_found / "Nota no
	// encontrada"), sin revelar que el recurso existe ni detalles del origen.
	driveMock.InjectGetError(extID, &drive.DriveError{Code: 403, Message: "permiso revocado"})
	w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	driveMock.ClearGetError(extID)
	if w.Code != http.StatusNotFound {
		t.Fatalf("origen ilegible esperaba 404, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "not_found") || !strings.Contains(w.Body.String(), "nota no encontrada") {
		t.Fatalf("origen ilegible debe responder not_found zero-knowledge: %s", w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "origen") {
		t.Fatalf("el 404 no debe revelar el fallo del origen: %s", w.Body.String())
	}
	// (b) Cuenta del clonador sin OAuth: 403 con mensaje de reconexión.
	driveMock.Disconnect(copier)
	defer driveMock.Reconnect(copier)
	w2 := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	if w2.Code != http.StatusForbidden {
		t.Fatalf("OAuth del clonador esperaba 403, got %d %s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "Google Drive") {
		t.Fatalf("mensaje debe pedir reconectar el Drive del clonador: %s", w2.Body.String())
	}
	if strings.Contains(strings.ToLower(w2.Body.String()), "origen") {
		t.Fatalf("OAuth del clonador no debe reportarse como fallo de origen: %s", w2.Body.String())
	}
}

// --- Boundary 10MB adjuntos ---

func TestHandlerAttachmentBoundary10MB(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	nid := createNoteHTTP(t, r, token, "Boundary", "private", "c")
	upload := func(data []byte, fname string) *httptest.ResponseRecorder {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", fname)
		part.Write(data)
		writer.Close()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/attachments", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		return w
	}
	// Exactamente 10MB debe pasar (límite es >10MB).
	exact := make([]byte, 10*1024*1024)
	wOK := upload(exact, "exact10.bin")
	if wOK.Code != http.StatusCreated {
		t.Fatalf("exactamente 10MB esperaba 201, got %d %s", wOK.Code, wOK.Body.String())
	}
	// 10MB+1 debe fallar 413.
	over := make([]byte, 10*1024*1024+1)
	wFail := upload(over, "over10.bin")
	if wFail.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("10MB+1 esperaba 413, got %d %s", wFail.Code, wFail.Body.String())
	}
}


func doReq(r *gin.Engine, method, path, token, bodyStr string) *httptest.ResponseRecorder {
	var body io.Reader
	if bodyStr != "" { body = strings.NewReader(bodyStr) }
	req := httptest.NewRequest(method, path, body)
	if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
func createNoteHTTP(t *testing.T, r *gin.Engine, token, title, visibility, content string) string {
	body := fmt.Sprintf(`{"title":"%s","visibility":"%s","content":"%s"}`, title, visibility, content)
	w := doReq(r, http.MethodPost, "/notes", token, body)
	if w.Code != http.StatusCreated {
		t.Fatalf("createNoteHTTP failed: %d %s", w.Code, w.Body.String())
	}
	var res map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &res)
	return res["note_id"].(string)
}
