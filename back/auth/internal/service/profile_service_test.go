package service

import (
	"context"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

// mockProfileRepo es un mock en memoria para tests unitarios sin BD.
type mockProfileRepo struct {
	store map[string]*model.Profile
	// para inyectar error
	err error
}

func newMockRepo() *mockProfileRepo {
	return &mockProfileRepo{store: make(map[string]*model.Profile)}
}

func (m *mockProfileRepo) GetByUserID(ctx context.Context, userID string) (*model.Profile, error) {
	if m.err != nil {
		return nil, m.err
	}
	if p, ok := m.store[userID]; ok {
		// copiar para evitar mutación
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (m *mockProfileRepo) Update(ctx context.Context, userID string, req repository.ProfileUpdateRequest) (*model.Profile, error) {
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.store[userID]
	if !ok {
		return nil, nil
	}
	// aplicar cambios parciales (igual que repo real)
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

func strPtr(s string) *string { return &s }

func TestProfileService_GetProfile_Existente(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{
		UserID:      uid,
		DisplayName: "Agustín Vega",
		PhotoURL:    strPtr("https://cdn.test/photo.jpg"),
		Phone:       strPtr("+56912345678"),
		Institution: strPtr("UCT"),
		Description: strPtr("Estudiante"),
		Visibility:  model.VisibilityPublic,
	}
	svc := NewProfileService(repo)
	p, err := svc.GetProfile(context.Background(), uid)
	if err != nil {
		t.Fatalf("esperado sin error, got %v", err)
	}
	if p.DisplayName != "Agustín Vega" || p.Visibility != "public" || *p.PhotoURL != "https://cdn.test/photo.jpg" || *p.Phone != "+56912345678" {
		t.Fatalf("perfil no coincide: %+v", p)
	}
}

func TestProfileService_GetProfile_Inexistente_404(t *testing.T) {
	repo := newMockRepo()
	svc := NewProfileService(repo)
	_, err := svc.GetProfile(context.Background(), "550e8400-e29b-41d4-a716-446655440099")
	if err == nil {
		t.Fatal("esperado error profile_not_found")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "profile_not_found" {
		t.Fatalf("esperado ServiceError profile_not_found, got %v", err)
	}
}

func TestProfileService_GetProfile_SinUserID_401(t *testing.T) {
	repo := newMockRepo()
	svc := NewProfileService(repo)
	_, err := svc.GetProfile(context.Background(), "   ")
	if err == nil {
		t.Fatal("esperado unauthorized")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "unauthorized" {
		t.Fatalf("esperado unauthorized, got %v", err)
	}
}

func TestProfileService_UpdateProfile_Parcial_SoloDisplayName(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "Viejo", Visibility: "public"}
	svc := NewProfileService(repo)
	newName := "Nuevo Nombre"
	updated, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{DisplayName: &newName})
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	if updated.DisplayName != "Nuevo Nombre" || updated.Visibility != "public" {
		t.Fatalf("no se actualizó correctamente: %+v", updated)
	}
	// verificar que repo refleja cambio
	if repo.store[uid].DisplayName != "Nuevo Nombre" {
		t.Fatal("repositorio no actualizado")
	}
}

func TestProfileService_UpdateProfile_VariosCampos(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "private"}
	svc := NewProfileService(repo)
	req := repository.ProfileUpdateRequest{
		DisplayName: strPtr("B"),
		Phone:       strPtr("+569999"),
		Visibility:  strPtr("public"),
		PhotoURL:    strPtr("https://a.b/c.jpg"),
	}
	updated, err := svc.UpdateProfile(context.Background(), uid, req)
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if updated.DisplayName != "B" || *updated.Phone != "+569999" || updated.Visibility != "public" || *updated.PhotoURL != "https://a.b/c.jpg" {
		t.Fatalf("varios campos no actualizados: %+v", updated)
	}
}

func TestProfileService_UpdateProfile_VisibilityInvalida_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	svc := NewProfileService(repo)
	invalid := "solo_yo"
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{Visibility: &invalid})
	if err == nil {
		t.Fatal("esperado invalid_visibility")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_visibility" {
		t.Fatalf("esperado invalid_visibility, got %v", err)
	}
}

func TestProfileService_UpdateProfile_BodyVacio_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	svc := NewProfileService(repo)
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{})
	if err == nil {
		t.Fatal("esperado bad_request por body vacío")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "bad_request" {
		t.Fatalf("esperado bad_request, got %v", err)
	}
}

func TestProfileService_UpdateProfile_DisplayNameVacio_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	svc := NewProfileService(repo)
	empty := "   "
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{DisplayName: &empty})
	if err == nil {
		t.Fatal("esperado invalid_display_name")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_display_name" {
		t.Fatalf("esperado invalid_display_name, got %v", err)
	}
	// verificar que no modificó
	if repo.store[uid].DisplayName != "A" {
		t.Fatal("perfil no debe modificarse en PATCH inválido")
	}
}

func TestProfileService_UpdateProfile_DisplayNameLargo_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	svc := NewProfileService(repo)
	largo := string(make([]byte, 101)) // 101 >100
	for i := range largo {
		largo = largo[:i] + "a" + largo[i+1:]
	}
	// generar string 101 a
	largo = ""
	for i := 0; i < 101; i++ {
		largo += "a"
	}
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{DisplayName: &largo})
	if err == nil {
		t.Fatal("esperado invalid_display_name por >100")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_display_name" {
		t.Fatalf("esperado invalid_display_name, got %v", err)
	}
	if repo.store[uid].DisplayName != "A" {
		t.Fatal("no debe modificar en PATCH inválido por longitud")
	}
}

func TestProfileService_UpdateProfile_PhotoURLLargo_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", PhotoURL: strPtr("https://old.jpg")}
	svc := NewProfileService(repo)
	largo := ""
	for i := 0; i < 501; i++ {
		largo += "a"
	}
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{PhotoURL: &largo})
	if err == nil {
		t.Fatal("esperado invalid_photo_url por >500")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_photo_url" {
		t.Fatalf("esperado invalid_photo_url, got %v", err)
	}
	if *repo.store[uid].PhotoURL != "https://old.jpg" {
		t.Fatal("photo_url no debe modificarse en PATCH inválido")
	}
}

func TestProfileService_UpdateProfile_PhoneLargo_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Phone: strPtr("+5691")}
	svc := NewProfileService(repo)
	largo := ""
	for i := 0; i < 31; i++ {
		largo += "1"
	}
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{Phone: &largo})
	if err == nil {
		t.Fatal("esperado invalid_phone por >30")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_phone" {
		t.Fatalf("esperado invalid_phone, got %v", err)
	}
	if *repo.store[uid].Phone != "+5691" {
		t.Fatal("phone no debe modificarse en PATCH inválido")
	}
}

func TestProfileService_UpdateProfile_InstitutionLargo_400(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Institution: strPtr("UCT")}
	svc := NewProfileService(repo)
	largo := ""
	for i := 0; i < 201; i++ {
		largo += "a"
	}
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{Institution: &largo})
	if err == nil {
		t.Fatal("esperado invalid_institution por >200")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "invalid_institution" {
		t.Fatalf("esperado invalid_institution, got %v", err)
	}
	if *repo.store[uid].Institution != "UCT" {
		t.Fatal("institution no debe modificarse en PATCH inválido")
	}
}

func TestProfileService_UpdateProfile_InvalidoNoModificaParcial(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", Phone: strPtr("+5691"), PhotoURL: strPtr("https://old.jpg")}
	svc := NewProfileService(repo)
	// intentar PATCH con display_name válido + visibility inválida → debe fallar completo
	validName := "B"
	invalidVis := "bad"
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{DisplayName: &validName, Visibility: &invalidVis})
	if err == nil {
		t.Fatal("esperado error por visibility inválida")
	}
	// verificar que NINGÚN campo se modificó (atomicidad de validación)
	if repo.store[uid].DisplayName != "A" || *repo.store[uid].Phone != "+5691" || *repo.store[uid].PhotoURL != "https://old.jpg" {
		t.Fatal("PATCH inválido no debe modificar parcialmente el perfil")
	}
}

func TestProfileService_UpdateProfile_EmptyOptionalToNull(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public", PhotoURL: strPtr("https://old.jpg"), Phone: strPtr("+5691"), Institution: strPtr("UCT"), Description: strPtr("desc")}
	svc := NewProfileService(repo)
	empty := ""
	// photo_url "" → NULL, display_name omitido
	updated, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{PhotoURL: &empty})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if updated.PhotoURL != nil {
		t.Fatalf("photo_url \"\" debe guardarse como NULL, got %v", *updated.PhotoURL)
	}
	if *updated.Phone != "+5691" {
		t.Fatal("phone omitido no debe borrarse")
	}
	if updated.DisplayName != "A" {
		t.Fatal("display_name omitido no debe cambiar")
	}
	// phone "" también → NULL
	empty2 := "   "
	updated2, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{Phone: &empty2})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if updated2.Phone != nil {
		t.Fatalf("phone \"   \" debe ser NULL")
	}
}

func TestProfileService_UpdateProfile_DescriptionSinLimite(t *testing.T) {
	repo := newMockRepo()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.store[uid] = &model.Profile{UserID: uid, DisplayName: "A", Visibility: "public"}
	svc := NewProfileService(repo)
	// description text sin límite: 5000 chars debe ser válido
	largo := ""
	for i := 0; i < 5000; i++ {
		largo += "a"
	}
	updated, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{Description: &largo})
	if err != nil {
		t.Fatalf("description larga no debe fallar, got %v", err)
	}
	if *updated.Description != largo {
		t.Fatal("description no guardada correctamente")
	}
}

func TestProfileService_UpdateProfile_PerfilInexistente_404(t *testing.T) {
	repo := newMockRepo()
	svc := NewProfileService(repo)
	uid := "550e8400-e29b-41d4-a716-446655440099"
	_, err := svc.UpdateProfile(context.Background(), uid, repository.ProfileUpdateRequest{DisplayName: strPtr("X")})
	if err == nil {
		t.Fatal("esperado profile_not_found")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "profile_not_found" {
		t.Fatalf("esperado profile_not_found, got %v", err)
	}
}

func TestProfileService_UpdateProfile_NoPermiteOtroUserID(t *testing.T) {
	// La autorización se basa en el userID pasado al Service (inyectado por middleware).
	// Si el handler ignora user_id del body, el Service nunca ve otro ID.
	// Aquí verificamos que actualizar con uid A no afecta uid B.
	repo := newMockRepo()
	uidA := "550e8400-e29b-41d4-a716-446655440001"
	uidB := "550e8400-e29b-41d4-a716-446655440002"
	repo.store[uidA] = &model.Profile{UserID: uidA, DisplayName: "A", Visibility: "public"}
	repo.store[uidB] = &model.Profile{UserID: uidB, DisplayName: "B", Visibility: "private"}
	svc := NewProfileService(repo)
	updated, err := svc.UpdateProfile(context.Background(), uidA, repository.ProfileUpdateRequest{DisplayName: strPtr("A2")})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if updated.UserID != uidA || updated.DisplayName != "A2" {
		t.Fatalf("uid incorrecto: %+v", updated)
	}
	if repo.store[uidB].DisplayName != "B" {
		t.Fatal("perfil de otro usuario fue modificado, viola aislamiento")
	}
}

func TestProfileService_UpdateProfile_SinAuth_401(t *testing.T) {
	repo := newMockRepo()
	svc := NewProfileService(repo)
	_, err := svc.UpdateProfile(context.Background(), "", repository.ProfileUpdateRequest{DisplayName: strPtr("X")})
	if err == nil {
		t.Fatal("esperado unauthorized sin user_id")
	}
	se, ok := err.(*ServiceError)
	if !ok || se.Code != "unauthorized" {
		t.Fatalf("esperado unauthorized, got %v", err)
	}
}
