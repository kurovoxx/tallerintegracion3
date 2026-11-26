package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// mockFollowRepoHandler implementa FollowRepository en memoria para tests del handler.
type mockFollowRepoHandler struct {
	// follows[followerID] = set de followingID que sigue
	follows map[string]map[string]bool
	// users[publicID] = uuid interno
	users map[string]string
}

func newMockFollowRepoHandler() *mockFollowRepoHandler {
	return &mockFollowRepoHandler{
		follows: make(map[string]map[string]bool),
		users:   make(map[string]string),
	}
}

func (m *mockFollowRepoHandler) Follow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	fid := followerID.String()
	tid := followingID.String()
	if m.follows[fid] == nil {
		m.follows[fid] = make(map[string]bool)
	}
	if m.follows[fid][tid] {
		return false, nil // ya existía
	}
	m.follows[fid][tid] = true
	return true, nil
}

func (m *mockFollowRepoHandler) Unfollow(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	fid := followerID.String()
	tid := followingID.String()
	if m.follows[fid] == nil || !m.follows[fid][tid] {
		return false, nil // no existía
	}
	delete(m.follows[fid], tid)
	return true, nil
}

func (m *mockFollowRepoHandler) IsFollowing(ctx context.Context, followerID, followingID pgtype.UUID) (bool, error) {
	fid := followerID.String()
	tid := followingID.String()
	return m.follows[fid] != nil && m.follows[fid][tid], nil
}

func (m *mockFollowRepoHandler) ListFollowers(ctx context.Context, followingID pgtype.UUID) ([]model.FollowedUser, error) {
	return nil, nil // no usado en tests de handler
}

func (m *mockFollowRepoHandler) ListFollowing(ctx context.Context, followeeID pgtype.UUID) ([]model.FollowedUser, error) {
	return nil, nil // no usado en tests de handler
}

func (m *mockFollowRepoHandler) GetUserIDByPublicID(ctx context.Context, publicID string) (pgtype.UUID, error) {
	uid, ok := m.users[publicID]
	if !ok {
		return pgtype.UUID{}, errors.New("usuario no encontrado")
	}
	id, err := uuid.Parse(uid)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}, nil
}

func setupFollowHandler(repo *mockFollowRepoHandler) (*FollowHandler, *service.FollowService) {
	svc := service.NewFollowService(repo)
	h := NewFollowHandler(svc)
	return h, svc
}

func newContextWithAuthFollow(method, path, userID, publicID string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = []gin.Param{{Key: "public_id", Value: publicID}}
	if userID != "" {
		c.Set(middleware.ContextUserIDKey, userID)
	}
	return c, w
}

func init() { gin.SetMode(gin.TestMode) }

// TestFollowHandler_Follow_200_Created: seguir a un usuario existente
func TestFollowHandler_Follow_200_Created(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("POST", "/profile/follow/"+targetPublicID, uid, targetPublicID)
	h.Follow(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if !resp.Following {
		t.Fatalf("esperado following=true, got %v", resp.Following)
	}
	if resp.PublicID != targetPublicID {
		t.Fatalf("public_id incorrecto: %s", resp.PublicID)
	}
}

// TestFollowHandler_Follow_200_AlreadyFollowing: seguir a alguien que ya se sigue
func TestFollowHandler_Follow_200_AlreadyFollowing(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID
	// ya existe la relación
	repo.follows[uid] = map[string]bool{targetUID: true}

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("POST", "/profile/follow/"+targetPublicID, uid, targetPublicID)
	h.Follow(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if !resp.Following {
		t.Fatalf("esperado following=true")
	}
}

// TestFollowHandler_Follow_400_Self: no puede seguirse a sí mismo
func TestFollowHandler_Follow_400_Self(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	repo.users[uid] = uid // su propio public_id es su uid (falso pero sirve para el test)

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("POST", "/profile/follow/"+uid, uid, uid)
	h.Follow(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, got %d body %s", w.Code, w.Body.String())
	}
}

// TestFollowHandler_Follow_404_UserNotFound: public_id inexistente
func TestFollowHandler_Follow_404_UserNotFound(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("POST", "/profile/follow/nonexistent12", uid, "nonexistent12")
	h.Follow(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, got %d body %s", w.Code, w.Body.String())
	}
}

// TestFollowHandler_Unfollow_200: dejar de seguir a alguien
func TestFollowHandler_Unfollow_200(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID
	repo.follows[uid] = map[string]bool{targetUID: true}

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("DELETE", "/profile/follow/"+targetPublicID, uid, targetPublicID)
	h.Unfollow(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if resp.Following {
		t.Fatalf("esperado following=false")
	}
}

// TestFollowHandler_Unfollow_200_NotFollowing: dejar de seguir a quien no se sigue
func TestFollowHandler_Unfollow_200_NotFollowing(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("DELETE", "/profile/follow/"+targetPublicID, uid, targetPublicID)
	h.Unfollow(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
}

// TestFollowHandler_FollowStatus_200_Following: status cuando sí sigue
func TestFollowHandler_FollowStatus_200_Following(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID
	repo.follows[uid] = map[string]bool{targetUID: true}

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("GET", "/profile/follow-status/"+targetPublicID, uid, targetPublicID)
	h.FollowStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if !resp.Following {
		t.Fatalf("esperado following=true")
	}
}

// TestFollowHandler_FollowStatus_200_NotFollowing: status cuando no sigue
func TestFollowHandler_FollowStatus_200_NotFollowing(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"
	targetUID := "660e8400-e29b-41d4-a716-446655440002"
	targetPublicID := "abcdefghijkl"
	repo.users[targetPublicID] = targetUID

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("GET", "/profile/follow-status/"+targetPublicID, uid, targetPublicID)
	h.FollowStatus(c)

	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d body %s", w.Code, w.Body.String())
	}
	var resp followResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json invalido %v", err)
	}
	if resp.Following {
		t.Fatalf("esperado following=false")
	}
}

// TestFollowHandler_FollowStatus_404: public_id inexistente
func TestFollowHandler_FollowStatus_404(t *testing.T) {
	repo := newMockFollowRepoHandler()
	uid := "550e8400-e29b-41d4-a716-446655440001"

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("GET", "/profile/follow-status/nonexistent12", uid, "nonexistent12")
	h.FollowStatus(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("esperado 404, got %d body %s", w.Code, w.Body.String())
	}
}

// TestFollowHandler_Follow_401: sin token de autenticación
func TestFollowHandler_Follow_401_NoAuth(t *testing.T) {
	repo := newMockFollowRepoHandler()
	targetPublicID := "abcdefghijkl"

	h, _ := setupFollowHandler(repo)
	c, w := newContextWithAuthFollow("POST", "/profile/follow/"+targetPublicID, "", targetPublicID)
	h.Follow(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d body %s", w.Code, w.Body.String())
	}
}
