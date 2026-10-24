package http

// Contrato HTTP del borrado de cuenta que consume el frontend (3_2_16,
// front/lib/core/services/account_deletion_service.dart). Fija códigos y
// cuerpos de DELETE /auth/account y GET /auth/account-deletion, y que ambas
// rutas usan RequireIdentity (la cuenta ya bloqueada puede consultar/reanudar)
// mientras las rutas normales responden 423. Las rutas se montan igual que en
// cmd/server/main.go.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

const contractUserID = "550e8400-e29b-41d4-a716-446655440099"

type contractCleanup struct {
	beginNotes, beginSocial, social, notes service.AccountCleanupStatus
}

func (c contractCleanup) result(s service.AccountCleanupStatus) service.AccountCleanupResult {
	return service.AccountCleanupResult{Status: s}
}
func (c contractCleanup) BeginNotes(context.Context, string) service.AccountCleanupResult {
	return c.result(c.beginNotes)
}
func (c contractCleanup) BeginSocial(context.Context, string) service.AccountCleanupResult {
	return c.result(c.beginSocial)
}
func (c contractCleanup) CleanupSocial(context.Context, string) service.AccountCleanupResult {
	return c.result(c.social)
}
func (c contractCleanup) CleanupNotes(context.Context, string) service.AccountCleanupResult {
	return c.result(c.notes)
}

func allComplete() contractCleanup {
	return contractCleanup{
		beginNotes:  service.AccountCleanupComplete,
		beginSocial: service.AccountCleanupComplete,
		social:      service.AccountCleanupComplete,
		notes:       service.AccountCleanupComplete,
	}
}

type contractStore struct {
	state       *repository.AccountDeletionState
	beginErr    error
	stateErr    error
	completeErr error
	completed   int
}

func (s *contractStore) WithExclusiveUserLock(_ context.Context, _ string, fn func() error) error {
	return fn()
}
func (s *contractStore) Begin(context.Context, string) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	if s.state == nil {
		s.state = &repository.AccountDeletionState{Status: "pending", UpdatedAt: time.Now()}
	}
	return nil
}
func (s *contractStore) State(context.Context, string) (repository.AccountDeletionState, bool, error) {
	if s.stateErr != nil {
		return repository.AccountDeletionState{}, false, s.stateErr
	}
	if s.state == nil {
		return repository.AccountDeletionState{}, false, nil
	}
	return *s.state, true, nil
}
func (s *contractStore) RecordError(context.Context, string, error) error { return nil }
func (s *contractStore) CompleteAndDelete(context.Context, string) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	s.completed++
	s.state.Status = "complete"
	return nil
}
func (s *contractStore) PendingUsers(context.Context, int) ([]string, error) { return nil, nil }

// contractGate imita AccountDeletionRepository.AcquireRequest: inactiva desde
// que existe el tombstone.
type contractGate struct{ store *contractStore }

func (g contractGate) AcquireRequest(context.Context, string) (func(), bool, error) {
	return func() {}, g.store.state == nil, nil
}

func contractRouter(t *testing.T, cleanup contractCleanup, store *contractStore) (*gin.Engine, string) {
	t.Helper()
	jwtSvc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte("clave-de-prueba-contrato-borrado-32b"),
		Issuer:       "auth-service",
		Audience:     "taller-integracion",
		Duracion:     time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	token, err := jwtSvc.GenerarAccessToken(service.UsuarioAutenticado{ID: contractUserID})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	coordinator := service.NewAccountDeletionCoordinator(cleanup, store)
	h := NewAccountDeletionHandler(coordinator)
	mw := middleware.NewAuthMiddleware(jwtSvc, contractGate{store: store})
	r := gin.New()
	r.DELETE("/auth/account", mw.RequireIdentity(), h.Delete)
	r.GET("/auth/account-deletion", mw.RequireIdentity(), h.Status)
	r.GET("/auth/me", mw.RequireAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, token.AccessToken
}

func contractDo(t *testing.T, r *gin.Engine, method, path, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	body := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s %s: cuerpo no JSON %q", method, path, rec.Body.String())
		}
	}
	return rec.Code, body
}

func TestAccountDeletionContract_CompleteIsIdempotentAndBlocksNormalRoutes(t *testing.T) {
	store := &contractStore{}
	r, token := contractRouter(t, allComplete(), store)

	if code, _ := contractDo(t, r, http.MethodGet, "/auth/me", token); code != http.StatusOK {
		t.Fatalf("antes del borrado /auth/me = %d", code)
	}
	code, body := contractDo(t, r, http.MethodGet, "/auth/account-deletion", token)
	if code != http.StatusNotFound || body["status"] != "not_started" {
		t.Fatalf("estado inicial = %d %v", code, body)
	}

	code, body = contractDo(t, r, http.MethodDelete, "/auth/account", token)
	if code != http.StatusOK || body["status"] != "complete" {
		t.Fatalf("DELETE = %d %v, want 200 complete", code, body)
	}
	// Reintento con el mismo JWT (respuesta perdida): sigue siendo 200.
	code, body = contractDo(t, r, http.MethodDelete, "/auth/account", token)
	if code != http.StatusOK || body["status"] != "complete" {
		t.Fatalf("DELETE repetido = %d %v", code, body)
	}
	if store.completed != 1 {
		t.Fatalf("CompleteAndDelete ejecutado %d veces", store.completed)
	}
	code, body = contractDo(t, r, http.MethodGet, "/auth/account-deletion", token)
	if code != http.StatusOK || body["status"] != "complete" || body["updated_at"] == nil {
		t.Fatalf("estado final = %d %v", code, body)
	}
	code, body = contractDo(t, r, http.MethodGet, "/auth/me", token)
	errBody, _ := body["error"].(map[string]any)
	if code != http.StatusLocked || errBody["code"] != "account_deletion_in_progress" {
		t.Fatalf("/auth/me tras borrar = %d %v, want 423", code, body)
	}
}

func TestAccountDeletionContract_PendingStagesReturn202(t *testing.T) {
	cases := map[string]contractCleanup{
		"notes cleanup pendiente": {
			beginNotes: service.AccountCleanupComplete, beginSocial: service.AccountCleanupComplete,
			social: service.AccountCleanupComplete, notes: service.AccountCleanupPending,
		},
		"social con error": {
			beginNotes: service.AccountCleanupComplete, beginSocial: service.AccountCleanupComplete,
			social: service.AccountCleanupError, notes: service.AccountCleanupComplete,
		},
		"servidor antiguo sin estado explícito": {
			beginNotes: service.AccountCleanupComplete, beginSocial: service.AccountCleanupUnspecified,
			social: service.AccountCleanupComplete, notes: service.AccountCleanupComplete,
		},
	}
	for name, cleanup := range cases {
		t.Run(name, func(t *testing.T) {
			store := &contractStore{}
			r, token := contractRouter(t, cleanup, store)
			code, body := contractDo(t, r, http.MethodDelete, "/auth/account", token)
			if code != http.StatusAccepted || body["status"] != "pending" {
				t.Fatalf("DELETE = %d %v, want 202 pending", code, body)
			}
			if store.completed != 0 {
				t.Fatal("Identity borró filas sin confirmación de Social/Notes")
			}
			code, body = contractDo(t, r, http.MethodGet, "/auth/account-deletion", token)
			if code != http.StatusOK || body["status"] != "pending" {
				t.Fatalf("estado = %d %v", code, body)
			}
			if code, _ := contractDo(t, r, http.MethodGet, "/auth/me", token); code != http.StatusLocked {
				t.Fatalf("/auth/me con borrado pendiente = %d, want 423", code)
			}
		})
	}
}

func TestAccountDeletionContract_ErrorsAndAuth(t *testing.T) {
	t.Run("cuenta inexistente 404", func(t *testing.T) {
		r, token := contractRouter(t, allComplete(), &contractStore{beginErr: repository.ErrAccountNotFound})
		code, body := contractDo(t, r, http.MethodDelete, "/auth/account", token)
		if code != http.StatusNotFound || body["error"] != "account_not_found" {
			t.Fatalf("DELETE = %d %v", code, body)
		}
	})
	t.Run("estado ilegible 503 en ambas rutas", func(t *testing.T) {
		r, token := contractRouter(t, allComplete(), &contractStore{stateErr: errors.New("pg caído")})
		code, body := contractDo(t, r, http.MethodDelete, "/auth/account", token)
		if code != http.StatusServiceUnavailable || body["error"] != "account_deletion_unavailable" {
			t.Fatalf("DELETE = %d %v", code, body)
		}
		code, body = contractDo(t, r, http.MethodGet, "/auth/account-deletion", token)
		if code != http.StatusServiceUnavailable || body["error"] != "account_status_unavailable" {
			t.Fatalf("GET = %d %v", code, body)
		}
	})
	t.Run("503 tras limpiar servicios deja el borrado iniciado", func(t *testing.T) {
		// Por esto el frontend consulta el estado ante un 503 en vez de
		// asumir que no comenzó.
		store := &contractStore{completeErr: errors.New("commit perdido")}
		r, token := contractRouter(t, allComplete(), store)
		code, _ := contractDo(t, r, http.MethodDelete, "/auth/account", token)
		if code != http.StatusServiceUnavailable {
			t.Fatalf("DELETE = %d, want 503", code)
		}
		code, body := contractDo(t, r, http.MethodGet, "/auth/account-deletion", token)
		if code != http.StatusOK || body["status"] != "pending" {
			t.Fatalf("estado tras 503 = %d %v, want 200 pending", code, body)
		}
	})
	t.Run("sin token o token inválido 401", func(t *testing.T) {
		r, _ := contractRouter(t, allComplete(), &contractStore{})
		for _, token := range []string{"", "no-es-un-jwt"} {
			for _, route := range [][2]string{{http.MethodDelete, "/auth/account"}, {http.MethodGet, "/auth/account-deletion"}} {
				code, body := contractDo(t, r, route[0], route[1], token)
				errBody, _ := body["error"].(map[string]any)
				if code != http.StatusUnauthorized || errBody["code"] == nil {
					t.Fatalf("%s %s token=%q = %d %v", route[0], route[1], token, code, body)
				}
			}
		}
	})
}
