package service

import (
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// Tarea 2_3_12: aislamiento. Un usuario baneado no debe reingresar con ningún
// link, viejo o nuevo, ni acceder a datos del grupo.

// setupBanned deja un grupo con admin, un miembro baneado y el token que el
// baneado conocía (el vigente al momento del baneo).
func setupBanned(t *testing.T) (*GroupService, *MemoryGroupStore, *model.Group, string) {
	t.Helper()
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	oldToken := tokenOf(t, store, g.ID)
	joinAs(t, svc, store, g.ID, uMember)
	if err := svc.BanMember(ctx, g.ID, uAdmin, uMember); err != nil {
		t.Fatal(err)
	}
	return svc, store, g, oldToken
}

func TestBannedUserCannotRejoinWithOldToken(t *testing.T) {
	svc, _, g, oldToken := setupBanned(t)
	wantErr(t, svc.JoinGroup(ctx, g.ID, oldToken, uMember), ErrBanned)
}

func TestBannedUserCannotRejoinWithFreshToken(t *testing.T) {
	svc, _, g, _ := setupBanned(t)
	fresh, err := svc.RegenerateInvite(ctx, g.ID, uAdmin)
	if err != nil {
		t.Fatal(err)
	}
	// el baneo se evalúa antes que el token: el link nuevo tampoco sirve
	wantErr(t, svc.JoinGroup(ctx, g.ID, fresh, uMember), ErrBanned)
}

func TestBannedUserCannotRejoinWithGarbageToken(t *testing.T) {
	svc, _, g, _ := setupBanned(t)
	// un token mal formado responde 404 sin llegar a la base; sigue sin entrar
	err := svc.JoinGroup(ctx, g.ID, "basura", uMember)
	wantErr(t, err, ErrInvalidInviteToken)
}

func TestBanIsScopedToTheGroup(t *testing.T) {
	svc, store, _, _ := setupBanned(t)
	other := newGroup(t, svc, uAdmin2)
	if err := svc.JoinGroup(ctx, other.ID, tokenOf(t, store, other.ID), uMember); err != nil {
		t.Fatalf("el baneo en un grupo no debe bloquear otro: %v", err)
	}
}

func TestBannedUserLosesAccessToGroupData(t *testing.T) {
	svc, _, g, _ := setupBanned(t)

	_, err := svc.GetGroup(ctx, g.ID, uMember)
	wantErr(t, err, ErrForbidden)

	_, err = svc.ListMembers(ctx, g.ID, uMember)
	wantErr(t, err, ErrForbidden)

	_, err = svc.MemberRole(ctx, g.ID, uMember)
	wantErr(t, err, ErrForbidden)

	_, err = svc.RegenerateInvite(ctx, g.ID, uMember)
	wantErr(t, err, ErrForbidden)
}

// Kick no es ban: con el token vigente el expulsado puede volver; con un token
// rotado, no.
func TestKickedUserCanRejoinOnlyWithCurrentToken(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	oldToken := tokenOf(t, store, g.ID)
	joinAs(t, svc, store, g.ID, uMember)
	if err := svc.KickMember(ctx, g.ID, uAdmin, uMember); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.RegenerateInvite(ctx, g.ID, uAdmin); err != nil {
		t.Fatal(err)
	}
	wantErr(t, svc.JoinGroup(ctx, g.ID, oldToken, uMember), ErrInvalidInviteToken)

	if err := svc.JoinGroup(ctx, g.ID, tokenOf(t, store, g.ID), uMember); err != nil {
		t.Fatalf("con el token vigente el expulsado puede volver: %v", err)
	}
}

// El invite_token es la llave de entrada: nunca debe quedar al alcance de
// quien no es admin del grupo.
func TestInviteTokenNeverLeaksToNonAdmins(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	tok := tokenOf(t, store, g.ID)
	joinAs(t, svc, store, g.ID, uMember)

	if _, err := svc.GetGroup(ctx, g.ID, uStrang); err == nil {
		t.Fatal("un extraño no debe poder leer el grupo (y con él el token)")
	}
	v, err := svc.GetGroup(ctx, g.ID, uMember)
	if err != nil {
		t.Fatal(err)
	}
	if v.InviteToken != "" || v.InviteToken == tok {
		t.Fatal("un miembro común no debe recibir el token")
	}
}

// Un admin degradado pierde de inmediato el acceso al token y a las acciones.
func TestDemotedAdminLosesAdminAccess(t *testing.T) {
	svc, store := newTestSvc()
	g := newGroup(t, svc, uAdmin)
	joinAs(t, svc, store, g.ID, uMember)
	if err := svc.TransferAdmin(ctx, g.ID, uAdmin, uMember); err != nil {
		t.Fatal(err)
	}

	v, err := svc.GetGroup(ctx, g.ID, uAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if v.InviteToken != "" {
		t.Fatal("el admin degradado no debe ver el token")
	}
	_, err = svc.RegenerateInvite(ctx, g.ID, uAdmin)
	wantErr(t, err, ErrForbidden)
	wantErr(t, svc.BanMember(ctx, g.ID, uAdmin, uMember), ErrForbidden)
}
