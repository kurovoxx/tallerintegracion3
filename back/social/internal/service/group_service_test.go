package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	notesv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/notesv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// Ids fijos con formato uuid válido: el servicio rechaza ids mal formados.
const (
	uAdmin  = "aaaaaaaa-0000-4000-8000-000000000001"
	uAdmin2 = "aaaaaaaa-0000-4000-8000-000000000002"
	uMember = "bbbbbbbb-0000-4000-8000-000000000001"
	uOther  = "bbbbbbbb-0000-4000-8000-000000000002"
	uThird  = "bbbbbbbb-0000-4000-8000-000000000003"
	uStrang = "cccccccc-0000-4000-8000-000000000001"
)

var ctx = context.Background()

func newTestSvc() (*GroupService, *MemoryGroupStore) {
	store := NewMemoryGroupStore()
	return NewGroupService(store, nil), store
}

// newGroup crea un grupo con owner como admin.
func newGroup(t *testing.T, svc *GroupService, owner string) *model.Group {
	t.Helper()
	g, err := svc.Create(ctx, owner, "Grupo de estudio", nil)
	if err != nil {
		t.Fatalf("crear grupo: %v", err)
	}
	return g
}

// tokenOf devuelve el invite_token vigente del grupo.
func tokenOf(t *testing.T, store *MemoryGroupStore, groupID string) string {
	t.Helper()
	g, _ := store.GetByID(ctx, groupID)
	return g.InviteToken
}

// joinAs une al usuario al grupo con el token vigente.
func joinAs(t *testing.T, svc *GroupService, store *MemoryGroupStore, groupID, userID string) {
	t.Helper()
	if err := svc.JoinGroup(ctx, groupID, tokenOf(t, store, groupID), userID); err != nil {
		t.Fatalf("join %s: %v", userID, err)
	}
}

func roleOf(t *testing.T, store *MemoryGroupStore, groupID, userID string) string {
	t.Helper()
	r, err := store.GetMemberRole(ctx, groupID, userID)
	if err != nil {
		return ""
	}
	return r
}

func wantErr(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, se esperaba %v", got, want)
	}
}

func TestCreate(t *testing.T) {
	blank := "   "
	desc := "  descripción  "
	tests := []struct {
		name     string
		gname    string
		desc     *string
		wantCode string
		wantDesc *string
	}{
		{name: "ok", gname: "Cálculo III"},
		{name: "recorta espacios", gname: "  Física  "},
		{name: "descripción vacía pasa a nil", gname: "G", desc: &blank},
		{name: "descripción se recorta", gname: "G", desc: &desc, wantDesc: strPtr("descripción")},
		{name: "nombre vacío", gname: "   ", wantCode: "invalid_name"},
		{name: "nombre demasiado largo", gname: strings.Repeat("a", 201), wantCode: "invalid_name"},
		{name: "nombre en el límite", gname: strings.Repeat("a", 200)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, store := newTestSvc()
			g, err := svc.Create(ctx, uAdmin, tc.gname, tc.desc)
			if tc.wantCode != "" {
				var se *ServiceError
				if !errors.As(err, &se) || se.Code != tc.wantCode {
					t.Fatalf("error = %v, código esperado %s", err, tc.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("no se esperaba error: %v", err)
			}
			if g.Name != strings.TrimSpace(tc.gname) {
				t.Fatalf("nombre = %q", g.Name)
			}
			if tc.desc != nil && (g.Description == nil) != (tc.wantDesc == nil) {
				t.Fatalf("descripción = %v, esperada %v", g.Description, tc.wantDesc)
			}
			if tc.wantDesc != nil && *g.Description != *tc.wantDesc {
				t.Fatalf("descripción = %q", *g.Description)
			}
			if roleOf(t, store, g.ID, uAdmin) != model.RoleAdmin {
				t.Fatal("el creador debe quedar como admin")
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestJoin(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	tok := tokenOf(t, store, g.ID)

	t.Run("token válido crea membresía como member", func(t *testing.T) {
		if err := svc.JoinGroup(ctx, g.ID, tok, uMember); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != model.RoleMember {
			t.Fatal("debe entrar como member")
		}
	})
	t.Run("reingresar siendo miembro es idempotente y no cambia el rol", func(t *testing.T) {
		if err := svc.JoinGroup(ctx, g.ID, tok, uMember); err != nil {
			t.Fatal(err)
		}
		if err := svc.JoinGroup(ctx, g.ID, tok, uAdmin); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uAdmin) != model.RoleAdmin {
			t.Fatal("un admin no debe degradarse al reingresar")
		}
		if members, _ := svc.ListMembers(ctx, g.ID, uAdmin); len(members) != 2 {
			t.Fatalf("no debe duplicar membresías: %d", len(members))
		}
	})
	t.Run("token incorrecto", func(t *testing.T) {
		wantErr(t, svc.JoinGroup(ctx, g.ID, "dddddddd-0000-4000-8000-000000000001", uOther), ErrInvalidInviteToken)
	})
	t.Run("token mal formado no llega a la base (404, no 500)", func(t *testing.T) {
		wantErr(t, svc.JoinGroup(ctx, g.ID, "no-es-un-uuid", uOther), ErrInvalidInviteToken)
	})
	t.Run("grupo mal formado", func(t *testing.T) {
		wantErr(t, svc.JoinGroup(ctx, "xyz", tok, uOther), ErrNotFound)
	})
	t.Run("grupo inexistente", func(t *testing.T) {
		wantErr(t, svc.JoinGroup(ctx, "eeeeeeee-0000-4000-8000-000000000001", tok, uOther), ErrInvalidInviteToken)
	})
	t.Run("sin usuario", func(t *testing.T) {
		wantErr(t, svc.JoinGroup(ctx, g.ID, tok, ""), ErrUnauthorized)
	})
}

func TestRegenerateInvite(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	old := tokenOf(t, store, g.ID)

	t.Run("miembro no admin recibe forbidden", func(t *testing.T) {
		_, err := svc.RegenerateInvite(ctx, g.ID, uMember)
		wantErr(t, err, ErrForbidden)
	})
	t.Run("no miembro recibe forbidden", func(t *testing.T) {
		_, err := svc.RegenerateInvite(ctx, g.ID, uStrang)
		wantErr(t, err, ErrForbidden)
	})
	t.Run("grupo mal formado", func(t *testing.T) {
		_, err := svc.RegenerateInvite(ctx, "xyz", uAdmin)
		wantErr(t, err, ErrNotFound)
	})
	t.Run("admin obtiene token nuevo e invalida el anterior", func(t *testing.T) {
		tok, err := svc.RegenerateInvite(ctx, g.ID, uAdmin)
		if err != nil {
			t.Fatal(err)
		}
		if tok == "" || tok == old {
			t.Fatalf("token nuevo inválido: %q", tok)
		}
		wantErr(t, svc.JoinGroup(ctx, g.ID, old, uOther), ErrInvalidInviteToken)
		if err := svc.JoinGroup(ctx, g.ID, tok, uOther); err != nil {
			t.Fatalf("el token nuevo debe servir: %v", err)
		}
	})
}

func TestGetGroup(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	tok := tokenOf(t, store, g.ID)

	t.Run("admin ve el invite_token", func(t *testing.T) {
		v, err := svc.GetGroup(ctx, g.ID, uAdmin)
		if err != nil {
			t.Fatal(err)
		}
		if v.InviteToken != tok {
			t.Fatal("el admin debe ver el invite_token")
		}
	})
	t.Run("miembro no admin no ve el invite_token", func(t *testing.T) {
		v, err := svc.GetGroup(ctx, g.ID, uMember)
		if err != nil {
			t.Fatal(err)
		}
		if v.InviteToken != "" {
			t.Fatal("el invite_token no debe filtrarse a miembros")
		}
		raw, _ := json.Marshal(v)
		if strings.Contains(string(raw), tok) || strings.Contains(string(raw), "invite_token") {
			t.Fatalf("el JSON no debe incluir el token: %s", raw)
		}
	})
	t.Run("no miembro recibe forbidden", func(t *testing.T) {
		_, err := svc.GetGroup(ctx, g.ID, uStrang)
		wantErr(t, err, ErrForbidden)
	})
	t.Run("grupo inexistente", func(t *testing.T) {
		_, err := svc.GetGroup(ctx, "eeeeeeee-0000-4000-8000-000000000001", uAdmin)
		wantErr(t, err, ErrNotFound)
	})
	t.Run("id mal formado", func(t *testing.T) {
		_, err := svc.GetGroup(ctx, "xyz", uAdmin)
		wantErr(t, err, ErrNotFound)
	})
	t.Run("sin usuario", func(t *testing.T) {
		_, err := svc.GetGroup(ctx, g.ID, "")
		wantErr(t, err, ErrUnauthorized)
	})
}

func TestListMembers(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	t.Run("miembro ve integrantes ordenados por ingreso", func(t *testing.T) {
		ms, err := svc.ListMembers(ctx, g.ID, uMember)
		if err != nil {
			t.Fatal(err)
		}
		if len(ms) != 2 || ms[0].UserID != uAdmin || ms[1].UserID != uMember {
			t.Fatalf("orden inesperado: %+v", ms)
		}
	})
	t.Run("no miembro recibe forbidden", func(t *testing.T) {
		_, err := svc.ListMembers(ctx, g.ID, uStrang)
		wantErr(t, err, ErrForbidden)
	})
	t.Run("id mal formado", func(t *testing.T) {
		_, err := svc.ListMembers(ctx, "xyz", uAdmin)
		wantErr(t, err, ErrNotFound)
	})
}

// adminAction describe una acción administrativa sobre un miembro objetivo.
type adminAction struct {
	name string
	run  func(svc *GroupService, groupID, actor, target string) error
}

func adminActions() []adminAction {
	return []adminAction{
		{"kick", func(s *GroupService, g, a, t string) error { return s.KickMember(ctx, g, a, t, false, "") }},
		{"ban", func(s *GroupService, g, a, t string) error { return s.BanMember(ctx, g, a, t, false, "") }},
		{"role", func(s *GroupService, g, a, t string) error { return s.ChangeRole(ctx, g, a, t, model.RoleMember) }},
		{"transfer", func(s *GroupService, g, a, t string) error { return s.TransferAdmin(ctx, g, a, t) }},
	}
}

// Las cuatro acciones exigen admin: un miembro común o un extraño reciben forbidden
// aunque el objetivo sea inválido (la autorización va primero).
func TestAdminActionsRequireAdmin(t *testing.T) {
	for _, act := range adminActions() {
		t.Run(act.name, func(t *testing.T) {
			svc, store := newTestSvc()
			g := newGroup(t, svc, uAdmin)
			joinAs(t, svc, store, g.ID, uMember)
			joinAs(t, svc, store, g.ID, uOther)

			wantErr(t, act.run(svc, g.ID, uMember, uOther), ErrForbidden)
			wantErr(t, act.run(svc, g.ID, uStrang, uOther), ErrForbidden)
			// no debe haber cambiado nada
			if roleOf(t, store, g.ID, uOther) != model.RoleMember || roleOf(t, store, g.ID, uAdmin) != model.RoleAdmin {
				t.Fatal("una acción rechazada no debe modificar roles")
			}
			if !store.GroupExists(g.ID) {
				t.Fatal("el grupo debe seguir existiendo")
			}
			wantErr(t, act.run(svc, g.ID, "", uOther), ErrUnauthorized)
		})
	}
}

func TestAdminActionsIdValidation(t *testing.T) {
	for _, act := range adminActions() {
		t.Run(act.name, func(t *testing.T) {
			svc, _ := newTestSvc()
			g := newGroup(t, svc, uAdmin)
			wantErr(t, act.run(svc, "xyz", uAdmin, uMember), ErrNotFound)
			wantErr(t, act.run(svc, g.ID, uAdmin, "no-uuid"), ErrTargetNotFound)
			wantErr(t, act.run(svc, g.ID, uAdmin, uAdmin), ErrCannotModifySelf)
			wantErr(t, act.run(svc, g.ID, uAdmin, uStrang), ErrTargetNotFound)
		})
	}
}

func TestKick(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	joinAs(t, svc, store, g.ID, uAdmin2)
	if err := svc.ChangeRole(ctx, g.ID, uAdmin, uAdmin2, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	t.Run("no se puede expulsar a otro admin", func(t *testing.T) {
		wantErr(t, svc.KickMember(ctx, g.ID, uAdmin, uAdmin2, false, ""), ErrTargetIsAdmin)
		if roleOf(t, store, g.ID, uAdmin2) != model.RoleAdmin {
			t.Fatal("el admin objetivo debe seguir en el grupo")
		}
	})
	t.Run("admin expulsa a un miembro y no queda baneado", func(t *testing.T) {
		if err := svc.KickMember(ctx, g.ID, uAdmin, uMember, false, ""); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != "" {
			t.Fatal("el miembro debe salir del grupo")
		}
		if _, banned := store.BannedBy(g.ID, uMember); banned {
			t.Fatal("kick no debe registrar baneo")
		}
	})
	t.Run("expulsar a quien ya no es miembro", func(t *testing.T) {
		wantErr(t, svc.KickMember(ctx, g.ID, uAdmin, uMember, false, ""), ErrTargetNotFound)
	})
}

func TestBan(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	joinAs(t, svc, store, g.ID, uAdmin2)
	if err := svc.ChangeRole(ctx, g.ID, uAdmin, uAdmin2, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	t.Run("no se puede banear a otro admin ni queda registrado", func(t *testing.T) {
		wantErr(t, svc.BanMember(ctx, g.ID, uAdmin, uAdmin2, false, ""), ErrTargetIsAdmin)
		if _, banned := store.BannedBy(g.ID, uAdmin2); banned {
			t.Fatal("un admin no debe quedar baneado")
		}
	})
	t.Run("banear a quien no es miembro", func(t *testing.T) {
		wantErr(t, svc.BanMember(ctx, g.ID, uAdmin, uStrang, false, ""), ErrTargetNotFound)
		if _, banned := store.BannedBy(g.ID, uStrang); banned {
			t.Fatal("no se banea a quien no es miembro")
		}
	})
	t.Run("ban expulsa y registra quién baneó", func(t *testing.T) {
		if err := svc.BanMember(ctx, g.ID, uAdmin2, uMember, false, ""); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != "" {
			t.Fatal("el baneado debe salir del grupo")
		}
		if by, banned := store.BannedBy(g.ID, uMember); !banned || by != uAdmin2 {
			t.Fatalf("banned_by = %q (baneado=%v), se esperaba %s", by, banned, uAdmin2)
		}
	})
}

func TestChangeRole(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	t.Run("rol inválido", func(t *testing.T) {
		wantErr(t, svc.ChangeRole(ctx, g.ID, uAdmin, uMember, "teacher"), ErrInvalidRole)
	})
	t.Run("un no-admin no distingue rol inválido de forbidden", func(t *testing.T) {
		wantErr(t, svc.ChangeRole(ctx, g.ID, uMember, uAdmin, "teacher"), ErrForbidden)
	})
	t.Run("ascender a admin", func(t *testing.T) {
		if err := svc.ChangeRole(ctx, g.ID, uAdmin, uMember, model.RoleAdmin); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
			t.Fatal("debe quedar como admin")
		}
	})
	t.Run("el nuevo admin puede degradar al anterior", func(t *testing.T) {
		if err := svc.ChangeRole(ctx, g.ID, uMember, uAdmin, model.RoleMember); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uAdmin) != model.RoleMember {
			t.Fatal("debe quedar como member")
		}
	})
}

func TestTransferAdmin(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	t.Run("destino que no es miembro", func(t *testing.T) {
		wantErr(t, svc.TransferAdmin(ctx, g.ID, uAdmin, uStrang), ErrTargetNotFound)
	})
	t.Run("transferir promueve al destino y degrada al admin", func(t *testing.T) {
		if err := svc.TransferAdmin(ctx, g.ID, uAdmin, uMember); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != model.RoleAdmin || roleOf(t, store, g.ID, uAdmin) != model.RoleMember {
			t.Fatal("los roles deben intercambiarse")
		}
		if n, _ := store.CountAdmins(ctx, g.ID); n != 1 {
			t.Fatalf("debe haber exactamente 1 admin, hay %d", n)
		}
	})
	t.Run("el admin degradado ya no puede transferir", func(t *testing.T) {
		wantErr(t, svc.TransferAdmin(ctx, g.ID, uAdmin, uMember), ErrForbidden)
	})
}

func TestLeave(t *testing.T) {
	t.Run("miembro común sale", func(t *testing.T) {
		svc, store := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		joinAs(t, svc, store, g.ID, uMember)
		if err := svc.LeaveGroup(ctx, g.ID, uMember, false, ""); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != "" {
			t.Fatal("debe salir del grupo")
		}
	})
	t.Run("admin con co-admin sale", func(t *testing.T) {
		svc, store := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		store.AddMember(g.ID, uMember, model.RoleMember)
		joinAs(t, svc, store, g.ID, uAdmin2)
		_ = svc.ChangeRole(ctx, g.ID, uAdmin, uAdmin2, model.RoleAdmin)
		if err := svc.LeaveGroup(ctx, g.ID, uAdmin, false, ""); err != nil {
			t.Fatal(err)
		}
		if n, _ := store.CountAdmins(ctx, g.ID); n != 1 {
			t.Fatalf("debe quedar 1 admin, hay %d", n)
		}
		updated, _ := store.GetByID(ctx, g.ID)
		if updated.OwnerUserID != uAdmin2 {
			t.Fatal("owner must become existing admin")
		}
		if roleOf(t, store, g.ID, uMember) != model.RoleMember {
			t.Fatal("unnecessary promotion")
		}
	})
	t.Run("admin único promueve al miembro restante", func(t *testing.T) {
		svc, store := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		joinAs(t, svc, store, g.ID, uMember)
		if err := svc.LeaveGroup(ctx, g.ID, uAdmin, false, ""); err != nil {
			t.Fatal(err)
		}
		if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
			t.Fatal("el miembro restante debe ser admin")
		}
	})
	t.Run("admin único que transfiere primero sí puede irse", func(t *testing.T) {
		svc, store := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		joinAs(t, svc, store, g.ID, uMember)
		_ = svc.TransferAdmin(ctx, g.ID, uAdmin, uMember)
		if err := svc.LeaveGroup(ctx, g.ID, uAdmin, false, ""); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("último miembro: el grupo se elimina", func(t *testing.T) {
		svc, store := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		if err := svc.LeaveGroup(ctx, g.ID, uAdmin, false, ""); err != nil {
			t.Fatal(err)
		}
		if store.GroupExists(g.ID) {
			t.Fatal("el grupo sin miembros debe eliminarse")
		}
	})
	t.Run("no miembro recibe forbidden", func(t *testing.T) {
		svc, _ := newTestSvc()
		g := newGroup(t, svc, uAdmin)
		wantErr(t, svc.LeaveGroup(ctx, g.ID, uStrang, false, ""), ErrForbidden)
	})
	t.Run("id mal formado", func(t *testing.T) {
		svc, _ := newTestSvc()
		wantErr(t, svc.LeaveGroup(ctx, "xyz", uAdmin, false, ""), ErrNotFound)
	})
}

// cleanup_shared_notes=true debe llamar a Notes con el usuario, el grupo y el JWT reenviado.
func TestLeaveCleansSharedNotes(t *testing.T) {
	type call struct {
		path, auth string
		body       map[string]string
	}
	got := make(chan call, 1)
	notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		got <- call{r.URL.Path, r.Header.Get("Authorization"), body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer notes.Close()

	svc, store := newTestSvc()
	svc.SetNotesBaseURL(notes.URL)
	// gRPC caído -> debe caer al fallback HTTP (lo que valida este test).
	svc.SetNotesUnsharer(&fakeUnsharer{err: errors.New("grpc down")})
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	if err := svc.LeaveGroup(ctx, g.ID, uMember, true, "jwt-del-usuario"); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-got:
		if c.path != "/notes/unshare-all" || c.auth != "Bearer jwt-del-usuario" ||
			c.body["user_id"] != uMember || c.body["group_id"] != g.ID {
			t.Fatalf("llamada inesperada a Notes: %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no se llamó a notes/unshare-all")
	}
}

func TestLeaveWithoutCleanupDoesNotCallNotes(t *testing.T) {
	called := make(chan struct{}, 1)
	notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called <- struct{}{} }))
	defer notes.Close()

	svc, store := newTestSvc()
	svc.SetNotesBaseURL(notes.URL)
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	if err := svc.LeaveGroup(ctx, g.ID, uMember, false, "jwt"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-called:
		t.Fatal("no se debe llamar a Notes sin cleanup_shared_notes")
	case <-time.After(150 * time.Millisecond):
	}
}

// Un fallo de Notes es best-effort: no impide salir del grupo.
func TestLeaveSucceedsWhenNotesIsDown(t *testing.T) {
	notes := httptest.NewServer(http.NotFoundHandler())
	url := notes.URL
	notes.Close() // servicio caído

	svc, store := newTestSvc()
	svc.SetNotesBaseURL(url)
	// Ambos transportes caídos: gRPC (fake) y HTTP (servidor cerrado).
	svc.SetNotesUnsharer(&fakeUnsharer{err: errors.New("grpc down")})
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	if err := svc.LeaveGroup(ctx, g.ID, uMember, true, "jwt"); err != nil {
		t.Fatalf("Notes caído no debe romper leave: %v", err)
	}
	if roleOf(t, store, g.ID, uMember) != "" {
		t.Fatal("el usuario debe haber salido")
	}
}

func TestListMyGroups(t *testing.T) {
	svc, store := newTestSvc()
	if gs, err := svc.ListMyGroups(ctx, uMember); err != nil || gs == nil || len(gs) != 0 {
		t.Fatalf("sin grupos debe ser slice vacío no nil: %v %v", gs, err)
	}
	g1 := newGroup(t, svc, uAdmin)
	g2 := newGroup(t, svc, uAdmin2)
	joinAs(t, svc, store, g1.ID, uMember)
	joinAs(t, svc, store, g2.ID, uMember)
	gs, err := svc.ListMyGroups(ctx, uMember)
	if err != nil || len(gs) != 2 {
		t.Fatalf("grupos = %v err=%v", gs, err)
	}
	if gs[0].GroupID != g2.ID {
		t.Fatal("el grupo más reciente va primero")
	}
	wantErr(t, func() error { _, e := svc.ListMyGroups(ctx, ""); return e }(), ErrUnauthorized)
}

// --- Sucesión automática de admin (tarea 2_3_9) ---

func TestAccountDeletionPromotesOldestMember(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember) // ingresa primero
	joinAs(t, svc, store, g.ID, uOther)
	joinAs(t, svc, store, g.ID, uThird)

	res, err := svc.HandleAccountDeletion(ctx, uAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].PromotedUserID == nil || *res[0].PromotedUserID != uMember || res[0].GroupDeleted {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
		t.Fatal("el miembro más antiguo debe ser admin")
	}
	if roleOf(t, store, g.ID, uOther) != model.RoleMember || roleOf(t, store, g.ID, uThird) != model.RoleMember {
		t.Fatal("los demás siguen como member")
	}
	if roleOf(t, store, g.ID, uAdmin) != "" {
		t.Fatal("el usuario eliminado debe salir del grupo")
	}
	if got, _ := store.GetByID(ctx, g.ID); got.OwnerUserID != uMember {
		t.Fatalf("owner_user_id = %s, debe pasar al sucesor", got.OwnerUserID)
	}
}

func TestAccountDeletionWithCoAdminDoesNotPromote(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	joinAs(t, svc, store, g.ID, uAdmin2)
	_ = svc.ChangeRole(ctx, g.ID, uAdmin, uAdmin2, model.RoleAdmin)

	res, err := svc.HandleAccountDeletion(ctx, uAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].PromotedUserID != nil || res[0].GroupDeleted {
		t.Fatalf("no debía promover ni eliminar: %+v", res)
	}
	if roleOf(t, store, g.ID, uMember) != model.RoleMember {
		t.Fatal("el miembro no debe ascender si queda otro admin")
	}
	// el dueño pasa al admin restante
	if got, _ := store.GetByID(ctx, g.ID); got.OwnerUserID != uAdmin2 {
		t.Fatalf("owner_user_id = %s, debe pasar al admin restante", got.OwnerUserID)
	}
}

func TestAccountDeletionLastMemberDeletesGroup(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	res, err := svc.HandleAccountDeletion(ctx, uAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || !res[0].GroupDeleted {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if store.GroupExists(g.ID) {
		t.Fatal("el grupo sin miembros debe eliminarse")
	}
}

func TestAccountDeletionOfRegularMember(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	res, err := svc.HandleAccountDeletion(ctx, uMember, "")
	if err != nil || len(res) != 1 || res[0].PromotedUserID != nil || res[0].GroupDeleted {
		t.Fatalf("resultado inesperado: %+v err=%v", res, err)
	}
	if roleOf(t, store, g.ID, uAdmin) != model.RoleAdmin {
		t.Fatal("el admin no cambia")
	}
}

func TestAccountDeletionWithoutGroupsIsNoOpAndIdempotent(t *testing.T) {
	svc, store := newTestSvc()
	res, err := svc.HandleAccountDeletion(ctx, uStrang, "")
	if err != nil || res == nil || len(res) != 0 {
		t.Fatalf("sin grupos debe devolver slice vacío: %v %v", res, err)
	}

	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	if _, err := svc.HandleAccountDeletion(ctx, uAdmin, ""); err != nil {
		t.Fatal(err)
	}
	res, err = svc.HandleAccountDeletion(ctx, uAdmin, "")
	if err != nil || len(res) != 0 {
		t.Fatalf("segunda llamada debe ser no-op: %v %v", res, err)
	}
	if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
		t.Fatal("la sucesión no debe repetirse ni revertirse")
	}
}

func TestAccountDeletionAcrossSeveralGroups(t *testing.T) {
	svc, store := newTestSvc()
	solo := newGroup(t, svc, uAdmin)       // último miembro -> se elimina
	withOthers := newGroup(t, svc, uAdmin) // sucesión
	asMember := newGroup(t, svc, uAdmin2)  // solo miembro común
	joinAs(t, svc, store, withOthers.ID, uMember)
	joinAs(t, svc, store, asMember.ID, uAdmin)
	// baneo del usuario en un tercer grupo: debe limpiarse
	banGroup := newGroup(t, svc, uAdmin2)
	joinAs(t, svc, store, banGroup.ID, uAdmin)
	if err := svc.BanMember(ctx, banGroup.ID, uAdmin2, uAdmin, false, ""); err != nil {
		t.Fatal(err)
	}

	res, err := svc.HandleAccountDeletion(ctx, uAdmin, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 { // solo, withOthers, asMember (banGroup ya no era membresía)
		t.Fatalf("se esperaban 3 grupos procesados: %+v", res)
	}
	if store.GroupExists(solo.ID) {
		t.Fatal("el grupo solitario debe eliminarse")
	}
	if roleOf(t, store, withOthers.ID, uMember) != model.RoleAdmin {
		t.Fatal("el grupo con miembros debe tener sucesor")
	}
	if roleOf(t, store, asMember.ID, uAdmin2) != model.RoleAdmin {
		t.Fatal("el admin del otro grupo no cambia")
	}
	if _, banned := store.BannedBy(banGroup.ID, uAdmin); banned {
		t.Fatal("los baneos del usuario eliminado deben limpiarse")
	}
}

func TestAccountDeletionRequiresUser(t *testing.T) {
	svc, _ := newTestSvc()
	_, err := svc.HandleAccountDeletion(ctx, "", "")
	wantErr(t, err, ErrUnauthorized)
}

// Borrado de cuenta e2e (tarea 3_5_7): sucesión en cada grupo + unshare-all
// en TODOS los grupos afectados (promovido o eliminado), con el JWT del
// usuario reenviado por Identity. Sin token solo hay sucesión.
func TestAccountDeletionCleansNotesInAllGroups(t *testing.T) {
	type call struct {
		body map[string]string
		auth string
	}
	setup := func(t *testing.T) (*GroupService, string, string, chan call) {
		t.Helper()
		got := make(chan call, 4)
		notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			got <- call{body, r.Header.Get("Authorization")}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(notes.Close)
		svc, store := newTestSvc()
		svc.SetNotesBaseURL(notes.URL)
		// Transporte HTTP explicito: este test valida el wiring de cleanup y el
		// payload/JWT del borrado de cuenta (no el transporte). En produccion
		// Social usa gRPC primero y cae a HTTP; aqui se fija HTTP para ser
		// determinista (mismo patron que TestKickBanCleanSharedNotes).
		svc.SetNotesUnsharer(nil)
		// g1: admin + miembro (promueve al miembro); g2: solo admin (se elimina).
		g1 := newGroup(t, svc, uAdmin)
		joinAs(t, svc, store, g1.ID, uMember)
		g2 := newGroup(t, svc, uAdmin)
		return svc, g1.ID, g2.ID, got
	}

	t.Run("con token limpia ambos grupos", func(t *testing.T) {
		svc, g1, g2, got := setup(t)
		res, err := svc.HandleAccountDeletion(ctx, uAdmin, "jwt-del-usuario")
		if err != nil {
			t.Fatal(err)
		}
		if len(res) != 2 {
			t.Fatalf("se esperaban 2 grupos afectados, got %d", len(res))
		}
		seen := map[string]bool{}
		for i := 0; i < 2; i++ {
			select {
			case c := <-got:
				if c.auth != "Bearer jwt-del-usuario" || c.body["user_id"] != uAdmin {
					t.Fatalf("llamada inesperada a Notes: %+v", c)
				}
				seen[c.body["group_id"]] = true
			case <-time.After(2 * time.Second):
				t.Fatal("faltan llamadas a notes/unshare-all")
			}
		}
		if !seen[g1] || !seen[g2] {
			t.Fatalf("unshare debe cubrir grupo promovido y eliminado, got %v", seen)
		}
	})
	t.Run("sin token solo sucesión", func(t *testing.T) {
		svc, _, _, got := setup(t)
		if _, err := svc.HandleAccountDeletion(ctx, uAdmin, ""); err != nil {
			t.Fatal(err)
		}
		select {
		case <-got:
			t.Fatal("sin token no se debe llamar a Notes")
		case <-time.After(150 * time.Millisecond):
		}
	})
}

func TestLeaveSuccessionOrderAndOwner(t *testing.T) {
	for _, tied := range []bool{false, true} {
		t.Run(fmtBool(tied), func(t *testing.T) {
			svc, store := newTestSvc()
			g := newGroup(t, svc, uAdmin)
			store.AddMember(g.ID, uOther, model.RoleMember)
			store.AddMember(g.ID, uMember, model.RoleMember)
			oldest := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			store.find(g.ID, uMember).JoinedAt = oldest
			store.find(g.ID, uOther).JoinedAt = oldest.Add(time.Hour)
			if tied {
				store.find(g.ID, uOther).JoinedAt = oldest
			}
			if err := svc.LeaveGroup(ctx, g.ID, uAdmin, false, ""); err != nil {
				t.Fatal(err)
			}
			if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
				t.Fatal("oldest/tie winner not promoted")
			}
			if roleOf(t, store, g.ID, uOther) != model.RoleMember {
				t.Fatal("unnecessary promotion")
			}
			updated, _ := store.GetByID(ctx, g.ID)
			if updated.OwnerUserID != uMember {
				t.Fatal("owner still references departing user")
			}
			if roleOf(t, store, g.ID, uAdmin) != "" {
				t.Fatal("departing admin remains")
			}
		})
	}
}
func fmtBool(tied bool) string {
	if tied {
		return "equal joined_at uses user_id"
	}
	return "oldest joined_at"
}

func TestConcurrentAdminLeaveRetainsAdminAndOwner(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	store.AddMember(g.ID, uAdmin2, model.RoleAdmin)
	store.AddMember(g.ID, uMember, model.RoleMember)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{uAdmin, uAdmin2} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); errs <- svc.LeaveGroup(ctx, g.ID, id, false, "") }(id)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if roleOf(t, store, g.ID, uMember) != model.RoleAdmin {
		t.Fatal("remaining group has no admin")
	}
	updated, _ := store.GetByID(ctx, g.ID)
	if updated.OwnerUserID != uMember {
		t.Fatal("owner not transferred")
	}
}

// --- Limpieza de apuntes al salir del grupo (migración unshare-all a gRPC) ---

type unsharerCall struct{ userID, groupID string }

type fakeUnsharer struct {
	mu    sync.Mutex
	calls []unsharerCall
	err   error
}

func (f *fakeUnsharer) UnshareAll(ctx context.Context, userID, groupID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, unsharerCall{userID: userID, groupID: groupID})
	return f.err
}

func (f *fakeUnsharer) snapshot() []unsharerCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]unsharerCall(nil), f.calls...)
}

// gRPC operativo: se usa el unsharer y NO se toca el HTTP legacy.
func TestLeavePrefersGRPCUnsharer(t *testing.T) {
	httpCalled := make(chan struct{}, 1)
	notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { httpCalled <- struct{}{} }))
	defer notes.Close()

	svc, store := newTestSvc()
	svc.SetNotesBaseURL(notes.URL)
	fake := &fakeUnsharer{}
	svc.SetNotesUnsharer(fake)
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)

	if err := svc.LeaveGroup(ctx, g.ID, uMember, true, "jwt"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(2 * time.Second)
	for len(fake.snapshot()) == 0 {
		select {
		case <-deadline:
			t.Fatal("no se llamó al unsharer gRPC")
		case <-time.After(10 * time.Millisecond):
		}
	}
	calls := fake.snapshot()
	if calls[0].userID != uMember || calls[0].groupID != g.ID {
		t.Fatalf("args inesperados: %+v", calls[0])
	}
	select {
	case <-httpCalled:
		t.Fatal("no debe usarse HTTP cuando gRPC funciona")
	case <-time.After(150 * time.Millisecond):
	}
}

// fakeNotesServer captura el RPC UnshareAll y su metadata.
type fakeNotesServer struct {
	notesv1.UnimplementedNotesServiceServer
	gotUser  string
	gotGroup string
	gotKey   string
	called   chan struct{}
}

func (s *fakeNotesServer) UnshareAll(ctx context.Context, req *notesv1.UnshareAllRequest) (*notesv1.UnshareAllResponse, error) {
	s.gotUser = req.GetUserId()
	s.gotGroup = req.GetGroupId()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if v := md.Get(metadataInternalKey); len(v) > 0 {
			s.gotKey = v[0]
		}
	}
	close(s.called)
	return &notesv1.UnshareAllResponse{}, nil
}

// El cliente gRPC real (bufconn) envía user_id, group_id y x-internal-key.
func TestNotesGRPCUnsharerSendsInternalKey(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	fake := &fakeNotesServer{called: make(chan struct{})}
	notesv1.RegisterNotesServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	u := newNotesGRPCUnsharer("bufnet", "secreto-interno")
	u.dial = func(target string) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := u.UnshareAll(callCtx, uMember, "grupo-1"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.called:
	case <-time.After(2 * time.Second):
		t.Fatal("el servidor gRPC no recibió el RPC")
	}
	if fake.gotUser != uMember || fake.gotGroup != "grupo-1" || fake.gotKey != "secreto-interno" {
		t.Fatalf("RPC inesperado: user=%q group=%q key=%q", fake.gotUser, fake.gotGroup, fake.gotKey)
	}
}

// cleanup_shared_notes=true en kick/ban debe llamar a Notes con el EXPULSADO
// como user_id y el JWT del admin actor (Notes lo autoriza vía IsGroupAdmin).
func TestKickBanCleanSharedNotes(t *testing.T) {
	type call struct {
		path, auth string
		body       map[string]string
	}
	setup := func(t *testing.T) (*GroupService, *MemoryGroupStore, string, chan call) {
		t.Helper()
		got := make(chan call, 1)
		notes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			got <- call{r.URL.Path, r.Header.Get("Authorization"), body}
			w.WriteHeader(http.StatusNoContent)
		}))
		t.Cleanup(notes.Close)
		svc, store := newTestSvc()
		svc.SetNotesBaseURL(notes.URL)
		// Transporte HTTP explícito (fallback): valida el wiring de cleanup y el
		// payload/JWT, sin depender de un Notes gRPC real (que en prod es 1º).
		svc.SetNotesUnsharer(nil)
		g := newGroup(t, svc, uAdmin)
		joinAs(t, svc, store, g.ID, uMember)
		return svc, store, g.ID, got
	}
	expectCleanup := func(t *testing.T, got chan call, gid string) {
		t.Helper()
		select {
		case c := <-got:
			if c.path != "/notes/unshare-all" || c.auth != "Bearer jwt-del-admin" ||
				c.body["user_id"] != uMember || c.body["group_id"] != gid {
				t.Fatalf("llamada inesperada a Notes: %+v", c)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("no se llamó a notes/unshare-all")
		}
	}

	t.Run("kick", func(t *testing.T) {
		svc, _, gid, got := setup(t)
		if err := svc.KickMember(ctx, gid, uAdmin, uMember, true, "jwt-del-admin"); err != nil {
			t.Fatal(err)
		}
		expectCleanup(t, got, gid)
	})
	t.Run("ban", func(t *testing.T) {
		svc, _, gid, got := setup(t)
		if err := svc.BanMember(ctx, gid, uAdmin, uMember, true, "jwt-del-admin"); err != nil {
			t.Fatal(err)
		}
		expectCleanup(t, got, gid)
	})
	t.Run("sin flag no llama", func(t *testing.T) {
		svc, _, gid, got := setup(t)
		if err := svc.KickMember(ctx, gid, uAdmin, uMember, false, "jwt"); err != nil {
			t.Fatal(err)
		}
		select {
		case <-got:
			t.Fatal("no se debe llamar a Notes sin cleanup_shared_notes")
		case <-time.After(150 * time.Millisecond):
		}
	})
	t.Run("kick fallido no llama", func(t *testing.T) {
		svc, _, gid, got := setup(t)
		if err := svc.KickMember(ctx, gid, uAdmin, uStrang, true, "jwt"); err == nil {
			t.Fatal("se esperaba error al expulsar no-miembro")
		}
		select {
		case <-got:
			t.Fatal("no se debe llamar a Notes si el kick falló")
		case <-time.After(150 * time.Millisecond):
		}
	})
}
