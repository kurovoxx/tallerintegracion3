package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Ids fijos con formato uuid válido.
const (
	hAdmin  = "aaaaaaaa-0000-4000-8000-000000000001"
	hMember = "bbbbbbbb-0000-4000-8000-000000000001"
	hOther  = "bbbbbbbb-0000-4000-8000-000000000002"
	hStrang = "cccccccc-0000-4000-8000-000000000001"
)

// stubValidator trata el propio token como user_id; "malo" es inválido.
type stubValidator struct{}

func (stubValidator) ValidarAccessToken(token string) (string, string, error) {
	if token == "malo" {
		return "", "", errors.New("token inválido")
	}
	return token, "", nil
}

type env struct {
	router *gin.Engine
	store  *service.MemoryGroupStore
}

func newEnv() *env {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryGroupStore()
	h := NewGroupHandler(service.NewGroupService(store, nil))

	r := gin.New()
	protected := r.Group("")
	protected.Use(middleware.NewAuthMiddleware(stubValidator{}).RequireAuth())
	h.RegisterRoutes(protected)
	return &env{router: r, store: store}
}

// do ejecuta una petición autenticada como user (token vacío = sin header).
func (e *env) do(method, path, user string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.Header.Set("Authorization", "Bearer "+user)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("JSON inválido %q: %v", w.Body.String(), err)
	}
	return m
}

func (e *env) createGroup(t *testing.T, owner string) string {
	t.Helper()
	w := e.do(http.MethodPost, "/groups", owner, map[string]any{"name": "Grupo"})
	if w.Code != http.StatusCreated {
		t.Fatalf("crear grupo: %d %s", w.Code, w.Body)
	}
	return decode(t, w)["group_id"].(string)
}

func (e *env) join(t *testing.T, groupID, user string) {
	t.Helper()
	tok := e.inviteToken(t, groupID)
	if w := e.do(http.MethodPost, "/groups/"+groupID+"/join", user, map[string]string{"invite_token": tok}); w.Code != http.StatusCreated {
		t.Fatalf("join: %d %s", w.Code, w.Body)
	}
}

func (e *env) inviteToken(t *testing.T, groupID string) string {
	t.Helper()
	g, _ := e.store.GetByID(context.Background(), groupID)
	return g.InviteToken
}

func expect(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	if w.Code != code {
		t.Fatalf("status = %d (%s), se esperaba %d", w.Code, strings.TrimSpace(w.Body.String()), code)
	}
}

func TestUnauthenticatedRequestsGet401(t *testing.T) {
	e := newEnv()
	gid := "dddddddd-0000-4000-8000-000000000001"
	routes := []struct{ method, path string }{
		{http.MethodPost, "/groups"},
		{http.MethodGet, "/groups/me"},
		{http.MethodGet, "/groups/" + gid},
		{http.MethodPost, "/groups/" + gid + "/join"},
		{http.MethodPost, "/groups/" + gid + "/invite/regenerate"},
		{http.MethodGet, "/groups/" + gid + "/members"},
		{http.MethodPost, "/groups/" + gid + "/members/" + hMember + "/kick"},
		{http.MethodPost, "/groups/" + gid + "/members/" + hMember + "/ban"},
		{http.MethodPatch, "/groups/" + gid + "/members/" + hMember + "/role"},
		{http.MethodPost, "/groups/" + gid + "/transfer-admin"},
		{http.MethodPost, "/groups/" + gid + "/leave"},
		{http.MethodPost, "/groups/account-deletion"},
	}
	for _, r := range routes {
		expect(t, e.do(r.method, r.path, "", nil), http.StatusUnauthorized)
		expect(t, e.do(r.method, r.path, "malo", nil), http.StatusUnauthorized)
	}
}

func TestCreateGroupContract(t *testing.T) {
	e := newEnv()

	w := e.do(http.MethodPost, "/groups", hAdmin, map[string]any{"name": "Cálculo", "description": "  apuntes  "})
	expect(t, w, http.StatusCreated)
	if id, _ := decode(t, w)["group_id"].(string); id == "" {
		t.Fatalf("debe devolver {group_id}: %s", w.Body)
	}

	expect(t, e.do(http.MethodPost, "/groups", hAdmin, map[string]any{"name": "   "}), http.StatusBadRequest)
	expect(t, e.do(http.MethodPost, "/groups", hAdmin, map[string]any{}), http.StatusBadRequest)
}

func TestListMyGroupsContract(t *testing.T) {
	e := newEnv()
	w := e.do(http.MethodGet, "/groups/me", hMember, nil)
	expect(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("sin grupos debe ser [], no %s", w.Body)
	}

	gid := e.createGroup(t, hAdmin)
	w = e.do(http.MethodGet, "/groups/me", hAdmin, nil)
	var got []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 || got[0]["group_id"] != gid || got[0]["role"] != "admin" || got[0]["name"] != "Grupo" {
		t.Fatalf("contrato [{group_id,name,role}] incumplido: %s", w.Body)
	}
}

func TestGetGroupHidesInviteTokenFromNonAdmins(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)
	tok := e.inviteToken(t, gid)

	w := e.do(http.MethodGet, "/groups/"+gid, hAdmin, nil)
	expect(t, w, http.StatusOK)
	if decode(t, w)["invite_token"] != tok {
		t.Fatalf("el admin debe ver el token: %s", w.Body)
	}

	w = e.do(http.MethodGet, "/groups/"+gid, hMember, nil)
	expect(t, w, http.StatusOK)
	if strings.Contains(w.Body.String(), "invite_token") || strings.Contains(w.Body.String(), tok) {
		t.Fatalf("el miembro no debe ver el token: %s", w.Body)
	}

	expect(t, e.do(http.MethodGet, "/groups/"+gid, hStrang, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodGet, "/groups/dddddddd-0000-4000-8000-000000000001", hAdmin, nil), http.StatusNotFound)
	expect(t, e.do(http.MethodGet, "/groups/no-es-uuid", hAdmin, nil), http.StatusNotFound)
}

func TestJoinContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	tok := e.inviteToken(t, gid)

	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hMember, map[string]string{"invite_token": tok}), http.StatusCreated)
	// token inválido y mal formado -> 404 (nunca 500)
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hOther, map[string]string{"invite_token": "dddddddd-0000-4000-8000-000000000001"}), http.StatusNotFound)
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hOther, map[string]string{"invite_token": "basura"}), http.StatusNotFound)
	expect(t, e.do(http.MethodPost, "/groups/basura/join", hOther, map[string]string{"invite_token": tok}), http.StatusNotFound)
	// sin body
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hOther, nil), http.StatusBadRequest)

	// baneado -> 403
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/members/"+hMember+"/ban", hAdmin, nil), http.StatusNoContent)
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hMember, map[string]string{"invite_token": tok}), http.StatusForbidden)
}

func TestRegenerateInviteContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)
	old := e.inviteToken(t, gid)

	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/invite/regenerate", hMember, nil), http.StatusForbidden)

	w := e.do(http.MethodPost, "/groups/"+gid+"/invite/regenerate", hAdmin, nil)
	expect(t, w, http.StatusOK)
	tok, _ := decode(t, w)["new_invite_token"].(string)
	if tok == "" || tok == old {
		t.Fatalf("contrato {new_invite_token} incumplido: %s", w.Body)
	}
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/join", hOther, map[string]string{"invite_token": old}), http.StatusNotFound)
}

func TestMembersContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)

	w := e.do(http.MethodGet, "/groups/"+gid+"/members", hMember, nil)
	expect(t, w, http.StatusOK)
	var ms []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &ms)
	if len(ms) != 2 || ms[0]["user_id"] != hAdmin || ms[0]["role"] != "admin" || ms[1]["role"] != "member" || ms[0]["joined_at"] == nil {
		t.Fatalf("contrato [{user_id,role,joined_at}] incumplido: %s", w.Body)
	}
	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/members", hStrang, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodGet, "/groups/basura/members", hAdmin, nil), http.StatusNotFound)
}

func TestKickAndBanContract(t *testing.T) {
	for _, action := range []string{"kick", "ban"} {
		t.Run(action, func(t *testing.T) {
			e := newEnv()
			gid := e.createGroup(t, hAdmin)
			e.join(t, gid, hMember)
			e.join(t, gid, hOther)
			path := func(target string) string { return "/groups/" + gid + "/members/" + target + "/" + action }

			expect(t, e.do(http.MethodPost, path(hOther), hMember, nil), http.StatusForbidden) // no admin
			expect(t, e.do(http.MethodPost, path(hOther), hStrang, nil), http.StatusForbidden) // no miembro
			expect(t, e.do(http.MethodPost, path(hAdmin), hAdmin, nil), http.StatusBadRequest) // a sí mismo
			expect(t, e.do(http.MethodPost, path(hStrang), hAdmin, nil), http.StatusNotFound)  // objetivo ausente
			expect(t, e.do(http.MethodPost, path("basura"), hAdmin, nil), http.StatusNotFound) // id mal formado
			expect(t, e.do(http.MethodPost, path(hMember), hAdmin, nil), http.StatusNoContent) // contrato: 204

			// promover a hOther y comprobar que otro admin no puede ser expulsado (400)
			expect(t, e.do(http.MethodPatch, "/groups/"+gid+"/members/"+hOther+"/role", hAdmin, map[string]string{"role": "admin"}), http.StatusOK)
			expect(t, e.do(http.MethodPost, path(hOther), hAdmin, nil), http.StatusBadRequest)
			if action == "ban" {
				if by, ok := e.store.BannedBy(gid, hMember); !ok || by != hAdmin {
					t.Fatalf("el baneo debe registrar al admin (banned_by): %q %v", by, ok)
				}
				if _, ok := e.store.BannedBy(gid, hOther); ok {
					t.Fatal("un admin no debe quedar baneado")
				}
			}
		})
	}
}

func TestChangeRoleContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)
	path := "/groups/" + gid + "/members/" + hMember + "/role"

	expect(t, e.do(http.MethodPatch, path, hMember, map[string]string{"role": "admin"}), http.StatusForbidden)
	expect(t, e.do(http.MethodPatch, path, hAdmin, map[string]string{"role": "teacher"}), http.StatusBadRequest)
	expect(t, e.do(http.MethodPatch, path, hAdmin, nil), http.StatusBadRequest)
	expect(t, e.do(http.MethodPatch, "/groups/"+gid+"/members/"+hAdmin+"/role", hAdmin, map[string]string{"role": "member"}), http.StatusBadRequest)
	expect(t, e.do(http.MethodPatch, "/groups/"+gid+"/members/"+hStrang+"/role", hAdmin, map[string]string{"role": "admin"}), http.StatusNotFound)
	expect(t, e.do(http.MethodPatch, path, hAdmin, map[string]string{"role": "admin"}), http.StatusOK)
}

func TestTransferAdminContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)
	path := "/groups/" + gid + "/transfer-admin"

	expect(t, e.do(http.MethodPost, path, hMember, map[string]string{"new_admin_user_id": hMember}), http.StatusForbidden)
	expect(t, e.do(http.MethodPost, path, hAdmin, nil), http.StatusBadRequest)
	expect(t, e.do(http.MethodPost, path, hAdmin, map[string]string{"new_admin_user_id": hAdmin}), http.StatusBadRequest)
	expect(t, e.do(http.MethodPost, path, hAdmin, map[string]string{"new_admin_user_id": hStrang}), http.StatusNotFound)
	expect(t, e.do(http.MethodPost, path, hAdmin, map[string]string{"new_admin_user_id": hMember}), http.StatusOK)

	if r, _ := e.store.GetMemberRole(context.Background(), gid, hAdmin); r != model.RoleMember {
		t.Fatal("el admin anterior debe quedar como member")
	}
}

func TestLeaveContract(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)
	path := "/groups/" + gid + "/leave"

	expect(t, e.do(http.MethodPost, path, hStrang, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodPost, path, hAdmin, nil), http.StatusNoContent)  // sucesión automática
	expect(t, e.do(http.MethodPost, path, hMember, nil), http.StatusNoContent) // body vacío permitido
	if e.store.GroupExists(gid) {
		t.Fatal("el último miembro al salir elimina el grupo")
	}
	expect(t, e.do(http.MethodPost, "/groups/basura/leave", hAdmin, nil), http.StatusNotFound)
}

// Tarea 2_3_9: la ruta estática no debe ser capturada por /groups/:id/...
func TestAccountDeletionRoute(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	e.join(t, gid, hMember)

	expect(t, e.do(http.MethodPost, "/groups/account-deletion", hAdmin, nil), http.StatusNoContent)
	if r, _ := e.store.GetMemberRole(context.Background(), gid, hMember); r != model.RoleAdmin {
		t.Fatalf("el miembro más antiguo debe ser el nuevo admin, rol = %q", r)
	}
	// idempotente
	expect(t, e.do(http.MethodPost, "/groups/account-deletion", hAdmin, nil), http.StatusNoContent)
	// sin membresías: no-op
	expect(t, e.do(http.MethodPost, "/groups/account-deletion", hStrang, nil), http.StatusNoContent)
}

// Aislamiento extremo a extremo: baneado no reingresa ni lee el grupo.
func TestBannedUserIsolationEndToEnd(t *testing.T) {
	e := newEnv()
	gid := e.createGroup(t, hAdmin)
	oldTok := e.inviteToken(t, gid)
	e.join(t, gid, hMember)
	expect(t, e.do(http.MethodPost, "/groups/"+gid+"/members/"+hMember+"/ban", hAdmin, nil), http.StatusNoContent)

	join := func(tok string) int {
		return e.do(http.MethodPost, "/groups/"+gid+"/join", hMember, map[string]string{"invite_token": tok}).Code
	}
	if code := join(oldTok); code != http.StatusForbidden {
		t.Fatalf("token viejo: %d", code)
	}
	w := e.do(http.MethodPost, "/groups/"+gid+"/invite/regenerate", hAdmin, nil)
	fresh, _ := decode(t, w)["new_invite_token"].(string)
	if code := join(fresh); code != http.StatusForbidden {
		t.Fatalf("token nuevo: %d", code)
	}
	expect(t, e.do(http.MethodGet, "/groups/"+gid, hMember, nil), http.StatusForbidden)
	expect(t, e.do(http.MethodGet, "/groups/"+gid+"/members", hMember, nil), http.StatusForbidden)
}
