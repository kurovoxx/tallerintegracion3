package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

// Share exitoso: el estado deseado queda materializado en
// notes.drive_managed_permissions y permission_sync_status en in_sync.
func TestSharePersistsDesiredStatePermissions(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	md.SetEmails(group, []string{"ana@example.com", "ben@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sh, err := svc.Share(ctx, owner, n.ID, group, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if sh.PermissionSyncStatus != model.PermissionSyncInSync {
		t.Fatalf("share must be in_sync: %+v", sh)
	}
	rows, err := svc.shared.ListManagedPermissions(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected one desired-state row per member: %+v", rows)
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if row.NoteID != n.ID || row.ExternalFileID != *n.ExternalFileID {
			t.Fatalf("row must reference the note file: %+v", row)
		}
		if row.PrincipalType != model.PermissionPrincipalEmail || row.Role != "reader" || row.SyncStatus != model.PermissionSyncInSync {
			t.Fatalf("unexpected managed permission: %+v", row)
		}
		seen[row.PrincipalKey] = true
	}
	if !seen["ana@example.com"] || !seen["ben@example.com"] {
		t.Fatalf("missing desired principals: %+v", seen)
	}
	if len(d.GrantCalls) != 2 {
		t.Fatalf("expected grants for both members: %v", d.GrantCalls)
	}
}

// Share link: el principal anyone se persiste con el ID de permiso devuelto por
// Drive, habilitando revocaciones posteriores sin re-listar el ACL.
func TestShareLinkPersistsAnyonePermissionID(t *testing.T) {
	svc, d, _, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, group, "link"); err != nil {
		t.Fatal(err)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].PrincipalType != model.PermissionPrincipalAnyone || rows[0].PrincipalKey != "" {
		t.Fatalf("expected a single anyone row: %+v", rows)
	}
	if rows[0].DrivePermissionID == nil || *rows[0].DrivePermissionID == "" {
		t.Fatalf("anyone row must persist the Drive permission ID: %+v", rows[0])
	}
	if len(d.RevokeIDCalls) != 0 {
		t.Fatal("grant must not revoke")
	}
}

// Caída de Social durante Unshare: la baja del share se persiste, el permiso
// queda pendiente y el reconciliador converge cuando Social/Drive se recuperan.
func TestUnshareWithoutDirectoryConvergesWhenSocialRecovers(t *testing.T) {
	svc, d, md, social := newRestrictedService()
	ctx := context.Background()
	owner, a, b := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, a)
	social.AddAdmin(owner, b)
	md.SetEmails(a, []string{"ana@example.com"})
	md.SetEmails(b, []string{"ben@example.com"})
	n, err := svc.Create(ctx, owner, "note", nil, "private", stringPtr("body"), "")
	if err != nil {
		t.Fatal(err)
	}
	sa, err := svc.Share(ctx, owner, n.ID, a, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, n.ID, b, "restricted"); err != nil {
		t.Fatal(err)
	}
	md.SetError(errors.New("social down"))
	if err := svc.Unshare(ctx, owner, sa.ID); err != nil {
		t.Fatalf("DB-first unshare must survive a Social outage: %v", err)
	}
	shares, _ := svc.shared.ListByNote(ctx, n.ID)
	if len(shares) != 1 || shares[0].GroupID != b {
		t.Fatalf("only the target share must be removed: %+v", shares)
	}
	rows, _ := svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 2 {
		t.Fatalf("pending state must be preserved for convergence: %+v", rows)
	}
	for _, row := range rows {
		if row.SyncStatus == model.PermissionSyncInSync {
			t.Fatalf("directory outage must not report in_sync: %+v", row)
		}
	}
	// Recuperación: el reconciliador revoca ana y conserva ben.
	md.SetError(nil)
	if err := svc.ReconcilePendingNotes(ctx); err != nil {
		t.Fatal(err)
	}
	rows, _ = svc.shared.ListManagedPermissions(ctx, n.ID)
	if len(rows) != 1 || rows[0].PrincipalKey != "ben@example.com" || rows[0].SyncStatus != model.PermissionSyncInSync {
		t.Fatalf("convergence must keep only the remaining group: %+v", rows)
	}
	revokedAna := false
	for _, call := range d.RevokeCalls {
		if call.Email == "ana@example.com" {
			revokedAna = true
		}
		if call.Email == "ben@example.com" {
			t.Fatalf("ben must keep access: %v", d.RevokeCalls)
		}
	}
	if !revokedAna {
		t.Fatalf("stale permission must be revoked: %v", d.RevokeCalls)
	}
}
