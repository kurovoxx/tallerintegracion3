package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Directorio falso con modo estricto configurable.
type fakeStrictDirectory struct {
	emails []string
	err    error
}

func (f *fakeStrictDirectory) LookupUsers(ctx context.Context, ids []string) (map[string]service.UserPublic, error) {
	return map[string]service.UserPublic{}, nil
}

func (f *fakeStrictDirectory) LookupEmailsStrict(ctx context.Context, ids []string) ([]string, error) {
	return f.emails, f.err
}

type internalEnv struct {
	router *gin.Engine
	svc    *service.GroupService
	store  *service.MemoryGroupStore
}

func newInternalEnv(dir service.UserDirectory) *internalEnv {
	gin.SetMode(gin.TestMode)
	store := service.NewMemoryGroupStore()
	svc := service.NewGroupService(store, nil)
	if dir != nil {
		svc.SetUserDirectory(dir)
	}
	h := NewInternalHandler(svc)
	r := gin.New()
	internal := r.Group("/internal")
	internal.Use(middleware.RequireInternalKey("k"))
	internal.GET("/groups/:id/members", h.Members)
	internal.GET("/groups/:id/member-emails", h.MemberEmails)
	return &internalEnv{router: r, svc: svc, store: store}
}

func (e *internalEnv) call(method, path, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	w := httptest.NewRecorder()
	e.router.ServeHTTP(w, req)
	return w
}

func (e *internalEnv) groupWithMembers(t *testing.T, owner string, members ...string) string {
	t.Helper()
	g, err := e.svc.Create(context.Background(), owner, "G", nil)
	if err != nil {
		t.Fatalf("crear grupo: %v", err)
	}
	for _, m := range members {
		st, err := e.store.GetByID(context.Background(), g.ID)
		if err != nil {
			t.Fatalf("leer grupo: %v", err)
		}
		if err := e.svc.JoinGroup(context.Background(), g.ID, st.InviteToken, m); err != nil {
			t.Fatalf("join: %v", err)
		}
	}
	return g.ID
}

func TestInternal_Members(t *testing.T) {
	e := newInternalEnv(nil)
	admin, member := uuid.NewString(), uuid.NewString()
	gid := e.groupWithMembers(t, admin, member)

	// Sin key → 401; grupo inexistente → 404; id malo → 400.
	expect(t, e.call(http.MethodGet, "/internal/groups/"+gid+"/members", ""), http.StatusUnauthorized)
	expect(t, e.call(http.MethodGet, "/internal/groups/"+uuid.NewString()+"/members", "k"), http.StatusNotFound)
	expect(t, e.call(http.MethodGet, "/internal/groups/no-uuid/members", "k"), http.StatusBadRequest)

	w := e.call(http.MethodGet, "/internal/groups/"+gid+"/members", "k")
	expect(t, w, http.StatusOK)
	var got []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("debe ser array JSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("se esperaban 2 miembros, got %s", w.Body.String())
	}
	roles := map[string]string{}
	for _, m := range got {
		uid, _ := m["user_id"].(string)
		role, _ := m["role"].(string)
		roles[uid] = role
	}
	if roles[admin] != "admin" || roles[member] != "member" {
		t.Fatalf("forma [{user_id, role}] esperada, got %s", w.Body.String())
	}
	// Sin key configurada en el server → 503 (fail closed).
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g2 := r.Group("/internal")
	g2.Use(middleware.RequireInternalKey(""))
	g2.GET("/groups/:id/members", NewInternalHandler(e.svc).Members)
	req := httptest.NewRequest(http.MethodGet, "/internal/groups/"+gid+"/members", nil)
	req.Header.Set("X-Internal-Key", "k")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req)
	expect(t, w2, http.StatusServiceUnavailable)
}

func TestInternal_MemberEmails(t *testing.T) {
	newEnvWith := func(dir service.UserDirectory) (*internalEnv, string) {
		e := newInternalEnv(dir)
		admin := uuid.NewString()
		return e, e.groupWithMembers(t, admin, uuid.NewString())
	}

	// OK: emails del directorio.
	e, gid := newEnvWith(&fakeStrictDirectory{emails: []string{"a@x.com"}})
	w := e.call(http.MethodGet, "/internal/groups/"+gid+"/member-emails", "k")
	expect(t, w, http.StatusOK)
	var got []string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil || len(got) != 1 || got[0] != "a@x.com" {
		t.Fatalf("esperado [a@x.com], got %s err=%v", w.Body.String(), err)
	}

	// Grupo inexistente → 404.
	expect(t, e.call(http.MethodGet, "/internal/groups/"+uuid.NewString()+"/member-emails", "k"), http.StatusNotFound)

	// Directorio caído → 500 (fail-closed, Notes no calcula permisos a ciegas).
	eDown, gidDown := newEnvWith(&fakeStrictDirectory{err: errors.New("identity caído")})
	w = eDown.call(http.MethodGet, "/internal/groups/"+gidDown+"/member-emails", "k")
	expect(t, w, http.StatusInternalServerError)

	// Sin directorio configurado → 500.
	eNil, gidNil := newEnvWith(nil)
	w = eNil.call(http.MethodGet, "/internal/groups/"+gidNil+"/member-emails", "k")
	expect(t, w, http.StatusInternalServerError)

	// Sin key → 401.
	expect(t, e.call(http.MethodGet, "/internal/groups/"+gid+"/member-emails", ""), http.StatusUnauthorized)

	// Respuesta siempre array (grupo sin emails → [], nunca null).
	eEmpty, gidEmpty := newEnvWith(&fakeStrictDirectory{emails: []string{}})
	w = eEmpty.call(http.MethodGet, "/internal/groups/"+gidEmpty+"/member-emails", "k")
	expect(t, w, http.StatusOK)
	if strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("vacío debe ser [], got %s", w.Body.String())
	}
}
