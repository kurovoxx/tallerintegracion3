package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// fakeDriveEmails implementa DriveEmailDirectory para tests: correos Google por
// grupo + conteo de miembros sin Drive; permite inyectar error.
type fakeDriveEmails struct {
	byGroup map[string][]string
	without map[string]int
	err     error
}

func (f *fakeDriveEmails) ListMemberDriveEmails(_ context.Context, groupID string) ([]string, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.byGroup[groupID], f.without[groupID], nil
}

// TestRestrictedUsesGoogleEmails_NotAppEmails: los permisos Drive restricted se
// conceden a la cuenta Google conectada, NO al correo de registro.
func TestRestrictedUsesGoogleEmailsNotAppEmails(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	// Correos de registro (app) distintos de los de Google.
	md.SetEmails(group, []string{"app-owner@sigma.dev", "app-member@sigma.dev"})
	svc.SetDriveEmailDirectory(&fakeDriveEmails{
		byGroup: map[string][]string{group: {"google-owner@gmail.com", "google-member@gmail.com"}},
	})

	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := svc.Share(ctx, owner, n.ID, group, "restricted")
	if err != nil {
		t.Fatalf("share: %v", err)
	}
	got := map[string]bool{}
	for _, gc := range d.GrantCalls {
		got[gc.Email] = true
	}
	if got["app-owner@sigma.dev"] || got["app-member@sigma.dev"] {
		t.Fatalf("no debe conceder al correo de registro: %v", d.GrantCalls)
	}
	if !got["google-owner@gmail.com"] || !got["google-member@gmail.com"] {
		t.Fatalf("debe conceder a los correos Google: %v", d.GrantCalls)
	}
	if sh.PermissionSyncStatus != model.PermissionSyncInSync {
		t.Fatalf("share debe quedar in_sync: %+v", sh)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 2 {
		t.Fatalf("esperaba 2 filas (una por correo Google): %+v", rows)
	}
	for _, r := range rows {
		if r.PrincipalKey != "google-owner@gmail.com" && r.PrincipalKey != "google-member@gmail.com" {
			t.Fatalf("fila con correo no-Google: %+v", r)
		}
	}
}

// TestRestrictedMemberWithoutDriveIsSkipped: un miembro sin Drive no recibe
// permiso y NO se sustituye su correo; la nota converge para el resto.
func TestRestrictedMemberWithoutDriveIsSkipped(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	md.SetEmails(group, []string{"app-owner@sigma.dev", "app-member@sigma.dev"})
	svc.SetDriveEmailDirectory(&fakeDriveEmails{
		byGroup: map[string][]string{group: {"google-owner@gmail.com"}},
		without: map[string]int{group: 1},
	})

	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "restricted"); err != nil {
		t.Fatalf("share: %v", err)
	}
	if len(d.GrantCalls) != 1 || d.GrantCalls[0].Email != "google-owner@gmail.com" {
		t.Fatalf("solo debe conceder al miembro con Drive: %v", d.GrantCalls)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].PrincipalKey != "google-owner@gmail.com" {
		t.Fatalf("una sola fila (miembro con Drive): %+v", rows)
	}
}

// TestRestrictedDedupGoogleEmails: dos miembros con el mismo correo Google -> 1 grant.
func TestRestrictedDedupGoogleEmails(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	svc.SetDriveEmailDirectory(&fakeDriveEmails{
		byGroup: map[string][]string{group: {"same@gmail.com", "SAME@gmail.com", " "}},
	})
	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	if len(d.GrantCalls) != 1 || d.GrantCalls[0].Email != "same@gmail.com" {
		t.Fatalf("dedup/normalización falló: %v", d.GrantCalls)
	}
}

// TestLinkThenRestrictedKeepsAnyoneAndGrantsGoogle: LINK → RESTRICTED conserva el
// permiso anyone y añade los correos Google (shares mixtos).
func TestLinkThenRestrictedKeepsAnyoneAndGrantsGoogle(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	svc.SetDriveEmailDirectory(&fakeDriveEmails{
		byGroup: map[string][]string{group: {"google-member@gmail.com"}},
	})
	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "link"); err != nil {
		t.Fatalf("share link: %v", err)
	}
	if len(d.LinkGrantCalls) != 1 {
		t.Fatalf("link no se otorgó: %v", d.LinkGrantCalls)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "restricted"); err != nil {
		t.Fatalf("share restricted: %v", err)
	}
	// El link sigue vigente y NO se re-otorgó (anyone ya en in_sync).
	if len(d.LinkGrantCalls) != 1 {
		t.Fatalf("link se re-otorgó: %v", d.LinkGrantCalls)
	}
	got := map[string]bool{}
	for _, gc := range d.GrantCalls {
		got[gc.Email] = true
	}
	if !got["google-member@gmail.com"] {
		t.Fatalf("no se concedió al correo Google del miembro: %v", d.GrantCalls)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	types := map[string]bool{}
	for _, r := range rows {
		types[r.PrincipalType] = true
	}
	if !types[model.PermissionPrincipalAnyone] || !types[model.PermissionPrincipalEmail] {
		t.Fatalf("deben coexistir anyone + email: %+v", rows)
	}
}

// TestRestrictedUnshareRevokesGoogleEmail: al dejar de compartir se revoca el
// correo Google.
func TestRestrictedUnshareRevokesGoogleEmail(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	svc.SetDriveEmailDirectory(&fakeDriveEmails{
		byGroup: map[string][]string{group: {"google-member@gmail.com"}},
	})
	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := svc.Share(ctx, owner, n.ID, group, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Unshare(ctx, owner, sh.ID); err != nil {
		t.Fatalf("unshare: %v", err)
	}
	if len(d.RevokeCalls) != 1 || d.RevokeCalls[0].Email != "google-member@gmail.com" {
		t.Fatalf("no se revocó el correo Google: %v", d.RevokeCalls)
	}
	if rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID); len(rows) != 0 {
		t.Fatalf("fila revocada debe eliminarse: %+v", rows)
	}
}

// TestRestrictedDirectoryErrorFailsClosed: si el resolvedor falla, Share falla
// sin persistir share ni tocar Drive.
func TestRestrictedDirectoryErrorFailsClosed(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	svc.SetDriveEmailDirectory(&fakeDriveEmails{err: errors.New("identity down")})
	n, err := svc.Create(ctx, owner, "nota", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "restricted"); !errors.Is(err, ErrInternalDatabase) {
		t.Fatalf("debe fallar cerrado: %v", err)
	}
	if len(d.GrantCalls) != 0 {
		t.Fatalf("no debe tocar Drive: %v", d.GrantCalls)
	}
	if shares, _ := svc.shared.ListByNote(ctx, n.ID); len(shares) != 0 {
		t.Fatalf("no debe persistir share: %+v", shares)
	}
}

// --- Unidad: directorio compuesto (Social user_id + store de correos Google) ---

type fakeMemberIDs struct {
	ids []string
	err error
}

func (f fakeMemberIDs) ListMemberUserIDs(context.Context, string) ([]string, error) {
	return f.ids, f.err
}

type fakeEmailStore struct {
	m   map[string]string
	err error
}

func (f fakeEmailStore) GoogleDriveEmails(context.Context, []string) (map[string]string, error) {
	return f.m, f.err
}

func TestDriveEmailDirectoryComposite(t *testing.T) {
	ctx := context.Background()
	dir := NewDriveEmailDirectory(
		fakeMemberIDs{ids: []string{"u1", "u2", "u3"}},
		fakeEmailStore{m: map[string]string{"u1": "a@gmail.com", "u2": "b@gmail.com"}},
	)
	emails, without, err := dir.ListMemberDriveEmails(ctx, "g")
	if err != nil {
		t.Fatal(err)
	}
	if without != 1 {
		t.Fatalf("miembros sin Drive = %d, want 1", without)
	}
	got := map[string]bool{}
	for _, e := range emails {
		got[e] = true
	}
	if !got["a@gmail.com"] || !got["b@gmail.com"] || len(emails) != 2 {
		t.Fatalf("emails inesperados: %v", emails)
	}

	// Error del store se propaga (fail-closed).
	dirErr := NewDriveEmailDirectory(
		fakeMemberIDs{ids: []string{"u1"}},
		fakeEmailStore{err: errors.New("db down")},
	)
	if _, _, err := dirErr.ListMemberDriveEmails(ctx, "g"); err == nil {
		t.Fatal("debía propagar el error del store")
	}
}
