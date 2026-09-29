package service

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

func TestComputeDesiredDrivePermissions(t *testing.T) {
	svc, _, md, _ := newRestrictedService()
	md.SetEmails("a", []string{" Ana@Example.com ", "", "ana@example.com", "ben@example.com"})
	md.SetEmails("b", []string{"ben@example.com", "cal@example.com"})
	for _, tc := range []struct {
		name, visibility string
		shares           []*model.SharedNote
		want             DesiredDrivePermissions
	}{
		{"private", "private", nil, DesiredDrivePermissions{Emails: []string{}}},
		{"public", "public", nil, DesiredDrivePermissions{Anyone: true, Emails: []string{}}},
		{"link", "private", []*model.SharedNote{{GroupID: "a", AccessMode: "link"}}, DesiredDrivePermissions{Anyone: true, Emails: []string{}}},
		{"restricted union", "private", []*model.SharedNote{{GroupID: "a", AccessMode: "restricted"}, {GroupID: "b", AccessMode: "restricted"}}, DesiredDrivePermissions{Emails: []string{"ana@example.com", "ben@example.com", "cal@example.com"}}},
		{"public restricted", "public", []*model.SharedNote{{GroupID: "a", AccessMode: "restricted"}}, DesiredDrivePermissions{Anyone: true, Emails: []string{"ana@example.com", "ben@example.com"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.ComputeDesiredDrivePermissions(context.Background(), tc.visibility, tc.shares)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
}

func TestUnsharePreservesOtherGroupPermissions(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, a, b := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, a)
	social.AddAdmin(owner, b)
	md.SetEmails(a, []string{"only-a@example.com", "shared@example.com"})
	md.SetEmails(b, []string{"shared@example.com", "only-b@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sa, err := svc.Share(ctx, owner, n.ID, a, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	sb, err := svc.Share(ctx, owner, n.ID, b, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.GrantCalls) != 3 {
		t.Fatalf("duplicate grants: %v", d.GrantCalls)
	}
	if err := svc.Unshare(ctx, owner, sa.ID); err != nil {
		t.Fatal(err)
	}
	if len(d.RevokeCalls) != 1 || d.RevokeCalls[0].Email != "only-a@example.com" {
		t.Fatalf("revoked another group's access: %v", d.RevokeCalls)
	}
	if err := svc.Unshare(ctx, owner, sb.ID); err != nil {
		t.Fatal(err)
	}
	if len(d.RevokeCalls) != 3 {
		t.Fatalf("remaining members were not revoked: %v", d.RevokeCalls)
	}
}

func TestLinkShareLifecyclePreservesPublicAndOtherLinks(t *testing.T) {
	for _, visibility := range []string{"private", "public"} {
		t.Run(visibility, func(t *testing.T) {
			svc, d, _, social := newRestrictedService()
			ctx := context.Background()
			owner, a, b := uuid.NewString(), uuid.NewString(), uuid.NewString()
			social.AddAdmin(owner, a)
			social.AddAdmin(owner, b)
			n, err := svc.Create(ctx, owner, "note", nil, visibility, stringPtr("body"), "")
			if err != nil {
				t.Fatal(err)
			}
			sa, err := svc.Share(ctx, owner, n.ID, a, "link")
			if err != nil {
				t.Fatal(err)
			}
			sb, err := svc.Share(ctx, owner, n.ID, b, "link")
			if err != nil {
				t.Fatal(err)
			}
			if len(d.LinkGrantCalls) != 1 {
				t.Fatalf("link must be granted once: %v", d.LinkGrantCalls)
			}
			if err := svc.Unshare(ctx, owner, sa.ID); err != nil {
				t.Fatal(err)
			}
			if len(d.RevokeIDCalls) != 0 {
				t.Fatal("another link still needs access")
			}
			if err := svc.Unshare(ctx, owner, sb.ID); err != nil {
				t.Fatal(err)
			}
			want := 1
			if visibility == "public" {
				want = 0
			}
			if len(d.RevokeIDCalls) != want {
				t.Fatalf("revocations = %v; expected %d", d.RevokeIDCalls, want)
			}
		})
	}
}

func TestUnshareAllRevokesOnlyOwnerGroupAcrossPages(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, other, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddAdmin(other, group)
	md.SetEmails(group, []string{"actual@example.com"})
	var notes []*model.Note
	for i := 0; i < 102; i++ {
		n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
		if err != nil {
			t.Fatal(err)
		}
		mode := "restricted"
		if i%2 == 0 {
			mode = "link"
		}
		if _, err := svc.Share(ctx, owner, n.ID, group, mode); err != nil {
			t.Fatal(err)
		}
		notes = append(notes, n)
	}
	foreign, err := svc.Create(ctx, other, "other", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, other, foreign.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	if err := svc.UnshareAll(ctx, owner, group); err != nil {
		t.Fatal(err)
	}
	for _, n := range notes {
		shares, _ := svc.shared.ListByNote(ctx, n.ID)
		if len(shares) != 0 {
			t.Fatalf("share left on note %s", n.ID)
		}
	}
	shares, _ := svc.shared.ListByNote(ctx, foreign.ID)
	if len(shares) != 1 {
		t.Fatal("removed another owner's share")
	}
	if len(d.RevokeCalls) != 51 || len(d.RevokeIDCalls) != 51 {
		t.Fatalf("not all Drive permissions revoked: users=%d links=%d", len(d.RevokeCalls), len(d.RevokeIDCalls))
	}
}

type failingPermissionSharedStore struct {
	SharedStore
	createErr, deleteErr error
}

func (s failingPermissionSharedStore) Create(ctx context.Context, noteID, groupID string, admin bool, mode string, followers int) (*model.SharedNote, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return s.SharedStore.Create(ctx, noteID, groupID, admin, mode, followers)
}

func (s failingPermissionSharedStore) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.SharedStore.Delete(ctx, id)
}

// DB-first: si la persistencia del share falla, Drive no debe tocarse (no hay
// intención durable que converger ni permiso que compensar).
func TestShareDatabaseFailureLeavesDriveUntouched(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	svc.shared = failingPermissionSharedStore{SharedStore: svc.shared, createErr: errors.New("database unavailable")}
	sh, err := svc.Share(ctx, owner, n.ID, group, "link")
	if !errors.Is(err, ErrInternalDatabase) || sh != nil {
		t.Fatalf("share: %v %v", sh, err)
	}
	if len(d.LinkGrantCalls) != 0 || len(d.GrantCalls) != 0 || len(d.RevokeIDCalls) != 0 {
		t.Fatalf("Drive mutó sin intención durable: link=%v grants=%v revokes=%v", d.LinkGrantCalls, d.GrantCalls, d.RevokeIDCalls)
	}
	shares, _ := svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 0 {
		t.Fatal("failed share persisted")
	}
}

func TestUnshareDatabaseFailurePreservesShare(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	md.SetEmails(group, []string{"actual@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := svc.Share(ctx, owner, n.ID, group, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	svc.shared = failingPermissionSharedStore{SharedStore: svc.shared, deleteErr: errors.New("db failed")}
	if err := svc.Unshare(ctx, owner, sh.ID); err == nil {
		t.Fatal("database failure must surface")
	}
	shares, _ := svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 1 {
		t.Fatal("share lost after failed database deletion")
	}
	if len(d.RevokeCalls) != 0 {
		t.Fatalf("Drive must stay untouched when DB delete failed: %v", d.RevokeCalls)
	}
}

// Convergencia ante fallos: la baja se persiste en BD, la revocación de Drive
// se difiere y el reconciliador la completa sin resucitar el share.
func TestUnshareDriveFailureConvergesLater(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	md.SetEmails(group, []string{"actual@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := svc.Share(ctx, owner, n.ID, group, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	d.RevokeErr = errors.New("Drive failed")
	if err := svc.Unshare(ctx, owner, sh.ID); err != nil {
		t.Fatalf("DB-first unshare must succeed even if Drive fails: %v", err)
	}
	shares, _ := svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 0 {
		t.Fatal("share must be deleted in database")
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].SyncStatus != model.PermissionSyncFailed {
		t.Fatalf("managed permission must persist the failed convergence: %+v", rows)
	}
	attemptsBefore := len(d.RevokeCalls)
	if attemptsBefore == 0 {
		t.Fatal("revoke must be attempted immediately")
	}
	d.RevokeErr = nil
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	if len(d.RevokeCalls) != attemptsBefore+1 || d.RevokeCalls[attemptsBefore].Email != "actual@example.com" {
		t.Fatalf("reconciler must retry the revoke: %v", d.RevokeCalls)
	}
	rows, _ = svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 0 {
		t.Fatalf("converged permission row must be removed: %+v", rows)
	}
}

func TestVisibilityUpdatesDriveLink(t *testing.T) {
	svc, d, _, _ := newRestrictedService()
	ctx := context.Background()
	owner := uuid.NewString()
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Update(ctx, owner, n.ID, nil, stringPtr("public"), nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(d.LinkGrantCalls) != 1 {
		t.Fatal("public visibility did not grant link")
	}
	if _, err := svc.Update(ctx, owner, n.ID, nil, stringPtr("private"), nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(d.RevokeIDCalls) != 1 {
		t.Fatal("private visibility did not revoke link")
	}
}

func TestPublicCreateAndLinkShareFailWhenGrantFails(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	d.LinkGrantErr = &drive.DriveError{Code: 403, Message: "policy denies link access"}
	if n, err := svc.Create(ctx, owner, "note", nil, "public", stringPtr("body"), ""); err == nil || n != nil {
		t.Fatal("public create must fail")
	}
	if d.FileCount() != 0 {
		t.Fatal("failed publication left orphan file")
	}
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	// DB-first: el share se persiste aunque el grant de link falle; queda
	// 'failed' y el reconciliador converge cuando Drive se recupera.
	sh, err := svc.Share(ctx, owner, n.ID, group, "link")
	if err != nil || sh == nil {
		t.Fatalf("DB-first share must persist: %v %v", sh, err)
	}
	if sh.PermissionSyncStatus != model.PermissionSyncFailed {
		t.Fatalf("share must expose failed convergence: %+v", sh)
	}
	shares, _ := svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 1 {
		t.Fatal("share with failed Drive sync must persist for convergence")
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].PrincipalType != model.PermissionPrincipalAnyone || rows[0].SyncStatus != model.PermissionSyncFailed {
		t.Fatalf("desired-state permission must record the failed link grant: %+v", rows)
	}
	d.LinkGrantErr = nil
	grantsBefore := 0
	for _, call := range d.LinkGrantCalls {
		if call == *n.ExternalFileID {
			grantsBefore++
		}
	}
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	grantsAfter := 0
	for _, call := range d.LinkGrantCalls {
		if call == *n.ExternalFileID {
			grantsAfter++
		}
	}
	if grantsAfter != grantsBefore+1 {
		t.Fatalf("reconciler must retry the link grant once: %v", d.LinkGrantCalls)
	}
	rows, _ = svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].SyncStatus != model.PermissionSyncInSync {
		t.Fatalf("link permission must converge to in_sync: %+v", rows)
	}
	shares, _ = svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 1 || shares[0].PermissionSyncStatus != model.PermissionSyncInSync {
		t.Fatalf("share must converge to in_sync: %+v", shares)
	}
}

// Notas públicas creadas antes del desired-state no tienen fila en
// drive_managed_permissions: el link debe asumirse existente (sin duplicar el
// grant) y revocarse al pasar a privada.
func TestLegacyPublicNoteRevokesLinkOnVisibilityChange(t *testing.T) {
	svc, d, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	fileID := "legacy-public-file"
	n, err := notes.Create(ctx, "", owner, nil, "legacy public", &fileID, "public", nil, "synced")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.GrantLinkPermission(ctx, owner, fileID); err != nil {
		t.Fatal(err)
	}
	grantsBefore := len(d.LinkGrantCalls)
	if _, err := svc.Update(ctx, owner, n.ID, nil, stringPtr("private"), nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(d.LinkGrantCalls) != grantsBefore {
		t.Fatalf("legacy link must not be re-granted: %v", d.LinkGrantCalls)
	}
	if len(d.RevokeIDCalls) != 1 {
		t.Fatalf("legacy link must be revoked: %v", d.RevokeIDCalls)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 0 {
		t.Fatalf("revoked legacy permission must be removed: %+v", rows)
	}
}

func TestRecreatedPublicFileGetsLinkPermission(t *testing.T) {
	svc, d, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	owner := uuid.NewString()
	n, err := notes.Create(ctx, "", owner, nil, "legacy public", nil, "public", nil, "synced")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Update(ctx, owner, n.ID, nil, nil, stringPtr("restored"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.LinkGrantCalls) != 1 || d.LinkGrantCalls[0] != *updated.ExternalFileID {
		t.Fatalf("new file is not publicly readable: %v", d.LinkGrantCalls)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].PrincipalType != model.PermissionPrincipalAnyone || rows[0].SyncStatus != model.PermissionSyncInSync {
		t.Fatalf("recreated file must persist the link permission: %+v", rows)
	}
}

type cancelGrantDrive struct {
	*drive.MockClient
	cancel context.CancelFunc
}

func (d *cancelGrantDrive) GrantPermission(ctx context.Context, owner, file, email, role string) error {
	if email == "fail@example.com" {
		d.cancel()
		return context.Canceled
	}
	return d.MockClient.GrantPermission(ctx, owner, file, email, role)
}

// Cancelación durante la convergencia: la intención ya persistida sobrevive, el
// acceso del grupo preexistente no se toca y el reconciliador completa el alta
// pendiente sin resucitar el share.
func TestShareCancellationPersistsIntentionAndPreservesExistingGroup(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	owner, a, b := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, a)
	social.AddAdmin(owner, b)
	md.SetEmails(a, []string{"existing@example.com"})
	md.SetEmails(b, []string{"existing@example.com", "new@example.com", "fail@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, a, "restricted"); err != nil {
		t.Fatal(err)
	}
	failing := &cancelGrantDrive{MockClient: d, cancel: cancel}
	svc.drive = failing
	sh, err := svc.Share(ctx, owner, n.ID, b, "restricted")
	if err != nil || sh == nil {
		t.Fatalf("share intention must survive cancellation: %v %v", sh, err)
	}
	if sh.PermissionSyncStatus != model.PermissionSyncFailed {
		t.Fatalf("share must be failed after canceled convergence: %+v", sh)
	}
	if len(d.RevokeCalls) != 0 {
		t.Fatalf("existing group access must not be revoked: %v", d.RevokeCalls)
	}
	// Recuperar Drive y converger: fail@example.com completa su alta.
	svc.drive = d
	if err := svc.ReconcilePendingNotes(context.Background()); err != nil {
		t.Fatal(err)
	}
	counts := map[string]string{}
	rows, _ := svc.shared.ListManagedPermissions(context.Background(), n.ID)
	for _, row := range rows {
		counts[row.PrincipalKey] = row.SyncStatus
	}
	if counts["existing@example.com"] != model.PermissionSyncInSync ||
		counts["new@example.com"] != model.PermissionSyncInSync ||
		counts["fail@example.com"] != model.PermissionSyncInSync {
		t.Fatalf("all group members must converge to in_sync: %+v", rows)
	}
	shares, _ := svc.shared.ListByNote(context.Background(), n.ID)
	if len(shares) != 2 {
		t.Fatalf("both shares must persist: %+v", shares)
	}
	for _, share := range shares {
		if share.PermissionSyncStatus != model.PermissionSyncInSync {
			t.Fatalf("shares must converge to in_sync: %+v", share)
		}
	}
}
