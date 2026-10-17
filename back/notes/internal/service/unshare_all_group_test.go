package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

func revokedEmail(calls []string, email string) bool {
	for _, e := range calls {
		if e == email {
			return true
		}
	}
	return false
}

func revokeEmailsOf(d *drive.MockClient) []string {
	out := make([]string, 0, len(d.RevokeCalls))
	for _, c := range d.RevokeCalls {
		if c.Email != "" {
			out = append(out, c.Email)
		}
	}
	return out
}

// permKeys devuelve los principal_key de las filas de permisos de una nota.
func permKeys(t *testing.T, svc *NoteService, noteID string) map[string]string {
	t.Helper()
	rows, err := svc.shared.ListManagedPermissions(context.Background(), noteID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, r := range rows {
		out[r.PrincipalKey] = r.SyncStatus
	}
	return out
}

// TestUnshareAllRevokesLeaverOnOtherAuthorsNote: al salir B, se revoca el correo
// Google de B en la nota RESTRICTED de A (ya no es miembro) y se conserva el de A.
func TestUnshareAllRevokesLeaverOnOtherAuthorsNote(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, member, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddMember(member, group)

	fake := &fakeDriveEmails{byGroup: map[string][]string{
		group: {"owner@gmail.com", "member@gmail.com"},
	}}
	svc.SetDriveEmailDirectory(fake)

	// A comparte su nota RESTRICTED con el grupo.
	na, err := svc.Create(ctx, owner, "A note", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, na.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	// B comparte su propia nota RESTRICTED con el grupo.
	nb, err := svc.Create(ctx, member, "B note", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, member, nb.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}

	// B sale: Social ya actualizó la membresía (desired = solo owner).
	fake.byGroup[group] = []string{"owner@gmail.com"}

	if err := svc.UnshareAll(ctx, member, group); err != nil {
		t.Fatalf("unshare-all: %v", err)
	}

	// Nota de A: se conserva owner y se revoca member.
	keys := permKeys(t, svc, na.ID)
	if _, ok := keys["owner@gmail.com"]; !ok {
		t.Fatalf("owner debe conservar permiso en la nota de A: %v", keys)
	}
	if _, ok := keys["member@gmail.com"]; ok {
		t.Fatalf("member debe ser revocado en la nota de A: %v", keys)
	}
	if !revokedEmail(revokeEmailsOf(d), "member@gmail.com") {
		t.Fatalf("no se revocó el correo de member: %v", d.RevokeCalls)
	}
	// Nota de B: su share desaparece y no quedan permisos gestionados.
	shares, _ := svc.shared.ListByNote(ctx, nb.ID)
	if len(shares) != 0 {
		t.Fatalf("el share propio de B debe eliminarse: %+v", shares)
	}
	if rows, _ := svc.shared.ListManagedPermissions(ctx, nb.ID); len(rows) != 0 {
		t.Fatalf("nota de B sin permisos gestionados: %+v", rows)
	}
	// La nota personal de B NO se elimina.
	if n, _ := svc.observedNotesGetByID(ctx, nb.ID); n == nil {
		t.Fatal("la nota personal de B no debe eliminarse")
	}
	// La nota de A sigue compartida con el grupo.
	if shares, _ := svc.shared.ListByNote(ctx, na.ID); len(shares) != 1 {
		t.Fatalf("la nota de A debe seguir compartida: %+v", shares)
	}
}

// TestUnshareAllKeepsLeaverIfSharedByAnotherGroup: si B sigue en otro grupo que
// comparte la MISMA nota, su correo se conserva.
func TestUnshareAllKeepsLeaverIfSharedByAnotherGroup(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, member, g1, g2 := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, g1)
	social.AddAdmin(owner, g2)
	social.AddMember(member, g1)
	social.AddMember(member, g2)

	fake := &fakeDriveEmails{byGroup: map[string][]string{
		g1: {"owner@gmail.com", "member@gmail.com"},
		g2: {"owner@gmail.com", "member@gmail.com"},
	}}
	svc.SetDriveEmailDirectory(fake)

	na, err := svc.Create(ctx, owner, "shared", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, na.ID, g1, "restricted"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, na.ID, g2, "restricted"); err != nil {
		t.Fatal(err)
	}

	// B sale de g1 pero sigue en g2.
	fake.byGroup[g1] = []string{"owner@gmail.com"}

	if err := svc.UnshareAll(ctx, member, g1); err != nil {
		t.Fatal(err)
	}
	keys := permKeys(t, svc, na.ID)
	if _, ok := keys["member@gmail.com"]; !ok {
		t.Fatalf("member debe conservar permiso (sigue en g2): %v", keys)
	}
	if revokedEmail(revokeEmailsOf(d), "member@gmail.com") {
		t.Fatalf("no debe revocarse a member (acceso legítimo por g2): %v", d.RevokeCalls)
	}
}

// TestUnshareAllKeepsLinkAnyone: el modo LINK/anyone no se revoca por una salida.
func TestUnshareAllKeepsLinkAnyone(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, member, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddMember(member, group)

	n, err := svc.Create(ctx, owner, "link note", nil, "private", stringPtr("x"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "link"); err != nil {
		t.Fatal(err)
	}
	linkGrantsBefore := len(d.LinkGrantCalls)

	if err := svc.UnshareAll(ctx, member, group); err != nil {
		t.Fatal(err)
	}
	if len(d.LinkGrantCalls) != linkGrantsBefore {
		t.Fatalf("link se re-otorgó: %v", d.LinkGrantCalls)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	foundAnyone := false
	for _, r := range rows {
		if r.PrincipalType == model.PermissionPrincipalAnyone && r.SyncStatus == model.PermissionSyncInSync {
			foundAnyone = true
		}
	}
	if !foundAnyone {
		t.Fatalf("el permiso anyone debe conservarse: %+v", rows)
	}
}

// TestUnshareAllIdempotent: repetir UnshareAll no altera el resultado.
func TestUnshareAllIdempotent(t *testing.T) {
	svc, _, _, social := newRestrictedService()
	ctx := context.Background()
	owner, member, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddMember(member, group)
	na, _ := svc.Create(ctx, owner, "A note", nil, "private", stringPtr("x"), "")
	svc.Share(ctx, owner, na.ID, group, "restricted")
	if err := svc.UnshareAll(ctx, member, group); err != nil {
		t.Fatal(err)
	}
	before := permKeys(t, svc, na.ID)
	if err := svc.UnshareAll(ctx, member, group); err != nil {
		t.Fatalf("segunda llamada: %v", err)
	}
	after := permKeys(t, svc, na.ID)
	if len(before) != len(after) {
		t.Fatalf("idempotencia rota: before=%v after=%v", before, after)
	}
}

// TestUnshareAllDriveRevokeFailureDeferred: si Drive falla al revocar, no se
// rompe UnshareAll y la fila queda 'failed' (convergencia diferida).
func TestUnshareAllDriveRevokeFailureDeferred(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, member, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddMember(member, group)
	fake := &fakeDriveEmails{byGroup: map[string][]string{
		group: {"owner@gmail.com", "member@gmail.com"},
	}}
	svc.SetDriveEmailDirectory(fake)
	na, _ := svc.Create(ctx, owner, "A note", nil, "private", stringPtr("x"), "")
	svc.Share(ctx, owner, na.ID, group, "restricted")

	fake.byGroup[group] = []string{"owner@gmail.com"}
	d.RevokeErr = errors.New("drive down")
	if err := svc.UnshareAll(ctx, member, group); err != nil {
		t.Fatalf("best-effort no debe fallar: %v", err)
	}
	keys := permKeys(t, svc, na.ID)
	if keys["member@gmail.com"] != model.PermissionSyncFailed {
		t.Fatalf("la revocación fallida debe quedar 'failed': %v", keys)
	}
}
