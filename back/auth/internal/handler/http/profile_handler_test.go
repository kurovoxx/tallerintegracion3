package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

type mockProfileRepoHandler struct {
	store map[string]*model.Profile
}

func newMockRepoHandler() *mockProfileRepoHandler {
	return &mockProfileRepoHandler{store: make(map[string]*model.Profile)}
}

func (m *mockProfileRepoHandler) GetByUserID(ctx context.Context, userID string) (*model.Profile, error) {
	if p, ok := m.store[userID]; ok {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (m *mockProfileRepoHandler) Update(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error) {
	p, ok := m.store[userID]
	if !ok {
		return nil, nil
	}
	if req.DisplayName != nil {
		p.DisplayName = *req.DisplayName
	}
	if req.PhotoURL != nil {
		if *req.PhotoURL == "" {
			p.PhotoURL = nil
		} else {
			v := *req.PhotoURL
			p.PhotoURL = &v
		}
	}
	if req.Phone != nil {
		if *req.Phone == "" {
			p.Phone = nil
		} else {
			v := *req.Phone
			p.Phone = &v
		}
	}
	if req.Institution != nil {
		if *req.Institution == "" {
			p.Institution = nil
		} else {
			v := *req.Institution
			p.Institution = &v
		}
	}
	if req.Description != nil {
		if *req.Description == "" {
			p.Description = nil
		} else {
			v := *req.Description
			p.Description = &v
		}
	}
	if req.Visibility != nil {
		p.Visibility = *req.Visibility
	}
	cp := *p
	return &cp, nil
}

func strPtrH(s string) *string { return &s }

func init() { gin.SetMode(gin.TestMode) }

func setupProfileHandler(repo *mockProfileRepoHandler) (*ProfileHandler, *service.ProfileService) {
	svc := service.NewProfileService(repo)
	h := NewProfileHandler(svc)
	return h, svc
}

func newContextWithAuth(method, path string, body []byte, userID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	if userID != "" {
		c.Set(middleware.ContextUserIDKey, userID)
	}
	return c, w
}

// GET 1: perfil existente 200
func TestProfileHandler_GetProfile_200(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{
		UserID:      uid,
		DisplayName: "Agustín",
		PhotoURL:    strPtrH("https://cdn/photo.jpg"),
		Visibility:  "public",
	}
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("GET", "/profile/me", nil, uid)
	h.GetProfile(c)
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if resp["display_name"] != "Agustín" || resp["visibility"] != "public" {
		t.Fatalf("respuesta incorrecta %v", resp)
	}
	if resp["photo_url"] != "https://cdn/photo.jpg" {
		t.Fatalf("photo_url esperado, got %v", resp["photo_url"])
	}
}

// GET 2: perfil inexistente 404
func TestProfileHandler_GetProfile_404(t *testing.T) {
	repo := newMockRepoHandler()
	h, _ := setupProfileHandler(repo)
	uid := "550e8400-e29b-41d4-a716-446655440099"
	c, w := newContextWithAuth("GET", "/profile/me", nil, uid)
	h.GetProfile(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, got %d %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Fatalf("esperado {error:{code}} en 404, got %s", w.Body.String())
	}
}

// GET 3: sin auth 401
func TestProfileHandler_GetProfile_401_SinAuth(t *testing.T) {
	repo := newMockRepoHandler()
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("GET", "/profile/me", nil, "")
	h.GetProfile(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 sin auth, got %d %s", w.Code, w.Body.String())
	}
}

// GET 4: solo consulta user_id autenticado (aislamiento)
func TestProfileHandler_GetProfile_SoloAutenticado(t *testing.T) {
	repo := newMockRepoHandler()
	uidA := "550e8400-e29b-41d4-a716-446655440001"
	uidB := "550e8400-e29b-41d4-a716-446655440002"
	repo.store[uidA] = &model.Profile{UserID: uidA, DisplayName: "A", Visibility: "public"}
	repo.store[uidB] = &model.Profile{UserID: uidB, DisplayName: "B", Visibility: "private"}
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("GET", "/profile/me", nil, uidA)
	h.GetProfile(c)
	if w.Code != 200 {
		t.Fatalf("200 esperado got %d", w.Code)
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["display_name"] != "A" {
		t.Fatalf("debería devolver perfil de A, got %v", resp["display_name"])
	}
}

// PATCH 1: actualización parcial exitosa (solo display_name)
func TestProfileHandler_PatchProfile_Parcial_DisplayName(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Viejo", Visibility: "public", Phone: strPtrH("+5691")}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"display_name": "Nuevo"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["display_name"] != "Nuevo" {
		t.Fatalf("display_name no actualizado %v", resp)
	}
	if repo.store[uid].Phone == nil || *repo.store[uid].Phone != "+5691" {
		t.Fatal("phone no debía borrarse en patch parcial")
	}
}

// PATCH 2: varios campos exitosa
func TestProfileHandler_PatchProfile_VariosCampos(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "private"}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"display_name": "B", "phone": "+569999", "visibility": "public"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 200 {
		t.Fatalf("200 esperado, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["display_name"] != "B" || resp["visibility"] != "public" {
		t.Fatalf("varios campos fallaron %v", resp)
	}
}

// PATCH 3: visibility inválida 400
func TestProfileHandler_PatchProfile_VisibilityInvalida_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"visibility": "invisible"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d %s", w.Code, w.Body.String())
	}
	var bodyMap map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &bodyMap)
	if _, ok := bodyMap["error"]; !ok {
		t.Fatalf("esperado error code en 400")
	}
}

// PATCH 4: body vacío 400
func TestProfileHandler_PatchProfile_BodyVacio_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	c, w := newContextWithAuth("PATCH", "/profile/me", []byte(`{}`), uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 body vacío, got %d %s", w.Code, w.Body.String())
	}
	c2, w2 := newContextWithAuth("PATCH", "/profile/me", []byte(``), uid)
	h.PatchProfile(c2)
	if w2.Code != 400 {
		t.Fatalf("esperado 400 body nulo, got %d %s", w2.Code, w2.Body.String())
	}
}

// PATCH 5: no permite modificar otro user_id (intento de inyección)
func TestProfileHandler_PatchProfile_NoPermiteUserID_400(t *testing.T) {
	repo := newMockRepoHandler()
	uidA := "550e8400-e29b-41d4-a716-446655440001"
	uidB := "550e8400-e29b-41d4-a716-446655440002"
	repo.store[uidA] = &model.Profile{UserID: uidA, DisplayName: "A", Visibility: "public"}
	repo.store[uidB] = &model.Profile{UserID: uidB, DisplayName: "B", Visibility: "private"}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]interface{}{"display_name": "Hacked", "user_id": uidB})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uidA)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 al enviar user_id, got %d %s", w.Code, w.Body.String())
	}
	if repo.store[uidA].DisplayName != "A" || repo.store[uidB].DisplayName != "B" {
		t.Fatal("no debió modificar ningún perfil al intentar inyectar user_id")
	}
}

// PATCH 6: perfil inexistente 404
func TestProfileHandler_PatchProfile_PerfilInexistente_404(t *testing.T) {
	repo := newMockRepoHandler()
	h, _ := setupProfileHandler(repo)
	uid := "550e8400-e29b-41d4-a716-446655440099"
	body, _ := json.Marshal(map[string]string{"display_name": "X"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 404 {
		t.Fatalf("esperado 404 perfil inexistente, got %d %s", w.Code, w.Body.String())
	}
}

// PATCH 7: sin auth 401
func TestProfileHandler_PatchProfile_401_SinAuth(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"display_name": "X"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, "")
	h.PatchProfile(c)
	if w.Code != 401 {
		t.Fatalf("esperado 401 sin auth, got %d %s", w.Code, w.Body.String())
	}
}

// PATCH: display_name "" → 400 y no modifica
func TestProfileHandler_PatchProfile_DisplayNameVacio_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", PhotoURL: strPtrH("https://old.jpg")}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"display_name": ""})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 display_name vacío, got %d %s", w.Code, w.Body.String())
	}
	if repo.store[uid].DisplayName != "A" {
		t.Fatal("display_name vacío no debe modificar perfil")
	}
}

// PATCH: display_name >100 → 400
func TestProfileHandler_PatchProfile_DisplayNameLargo_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	h, _ := setupProfileHandler(repo)
	largo := ""
	for i := 0; i < 101; i++ {
		largo += "a"
	}
	body, _ := json.Marshal(map[string]string{"display_name": largo})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 display_name largo, got %d %s", w.Code, w.Body.String())
	}
	if repo.store[uid].DisplayName != "A" {
		t.Fatal("no debe modificar en PATCH inválido")
	}
}

// PATCH: photo_url >500 → 400 y no modifica
func TestProfileHandler_PatchProfile_PhotoURLLargo_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", PhotoURL: strPtrH("https://old.jpg")}
	h, _ := setupProfileHandler(repo)
	largo := ""
	for i := 0; i < 501; i++ {
		largo += "a"
	}
	body, _ := json.Marshal(map[string]string{"photo_url": largo})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 photo_url largo, got %d %s", w.Code, w.Body.String())
	}
	if *repo.store[uid].PhotoURL != "https://old.jpg" {
		t.Fatal("photo_url no debe modificarse en PATCH inválido")
	}
}

// PATCH: phone >30 → 400
func TestProfileHandler_PatchProfile_PhoneLargo_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Phone: strPtrH("+5691")}
	h, _ := setupProfileHandler(repo)
	largo := ""
	for i := 0; i < 31; i++ {
		largo += "1"
	}
	body, _ := json.Marshal(map[string]string{"phone": largo})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 phone largo, got %d %s", w.Code, w.Body.String())
	}
	if *repo.store[uid].Phone != "+5691" {
		t.Fatal("phone no debe modificarse")
	}
}

// PATCH: institution >200 → 400
func TestProfileHandler_PatchProfile_InstitutionLargo_400(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Institution: strPtrH("UCT")}
	h, _ := setupProfileHandler(repo)
	largo := ""
	for i := 0; i < 201; i++ {
		largo += "a"
	}
	body, _ := json.Marshal(map[string]string{"institution": largo})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400 institution largo, got %d %s", w.Code, w.Body.String())
	}
	if *repo.store[uid].Institution != "UCT" {
		t.Fatal("institution no debe modificarse")
	}
}

// PATCH: inválido no modifica parcialmente (atomicidad)
func TestProfileHandler_PatchProfile_InvalidoNoModificaParcial(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Phone: strPtrH("+5691")}
	h, _ := setupProfileHandler(repo)
	body, _ := json.Marshal(map[string]string{"display_name": "B", "visibility": "bad"})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 400 {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
	if repo.store[uid].DisplayName != "A" || *repo.store[uid].Phone != "+5691" {
		t.Fatal("PATCH inválido no debe modificar parcialmente")
	}
}

// PATCH: semántica "" → NULL para opcionales, campo omitido → no modifica
func TestProfileHandler_PatchProfile_SemanticaNullYOmitido(t *testing.T) {
	repo := newMockRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", PhotoURL: strPtrH("https://old.jpg"), Phone: strPtrH("+5691"), Institution: strPtrH("UCT"), Description: strPtrH("desc")}
	h, _ := setupProfileHandler(repo)
	// photo_url "" → NULL, phone omitido → permanece, institution "" → NULL
	body, _ := json.Marshal(map[string]interface{}{"photo_url": "", "institution": "   "})
	c, w := newContextWithAuth("PATCH", "/profile/me", body, uid)
	h.PatchProfile(c)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d %s", w.Code, w.Body.String())
	}
	if repo.store[uid].PhotoURL != nil {
		t.Fatalf("photo_url \"\" debe ser NULL, got %v", *repo.store[uid].PhotoURL)
	}
	if repo.store[uid].Institution != nil {
		t.Fatalf("institution \"   \" debe ser NULL")
	}
	if *repo.store[uid].Phone != "+5691" {
		t.Fatal("phone omitido no debe borrarse")
	}
	if repo.store[uid].DisplayName != "A" {
		t.Fatal("display_name omitido no debe cambiar")
	}
	// description "" → NULL
	body2, _ := json.Marshal(map[string]string{"description": ""})
	c2, w2 := newContextWithAuth("PATCH", "/profile/me", body2, uid)
	h.PatchProfile(c2)
	if w2.Code != 200 {
		t.Fatalf("200 esperado para description \"\", got %d", w2.Code)
	}
	if repo.store[uid].Description != nil {
		t.Fatal("description \"\" debe ser NULL")
	}
}
