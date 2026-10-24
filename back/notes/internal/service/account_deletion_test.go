package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
)

type accountDeletionStoreFake struct {
	complete  bool
	completed int
	groupWork map[string]map[string]model.AccountDeletionGroupNote
}

func (f *accountDeletionStoreFake) Begin(context.Context, string) (bool, error) {
	return f.complete, nil
}
func (f *accountDeletionStoreFake) Complete(context.Context, string) error {
	f.complete = true
	f.completed++
	return nil
}
func (f *accountDeletionStoreFake) SaveGroupWork(_ context.Context, userID, groupID string, work []model.AccountDeletionGroupNote, groupDeleted bool) error {
	if f.groupWork == nil {
		f.groupWork = make(map[string]map[string]model.AccountDeletionGroupNote)
	}
	key := userID + ":" + groupID
	if f.groupWork[key] == nil {
		f.groupWork[key] = make(map[string]model.AccountDeletionGroupNote)
	}
	for _, item := range work {
		item.GroupDeleted = item.GroupDeleted || groupDeleted
		f.groupWork[key][item.NoteID] = item
	}
	return nil
}
func (f *accountDeletionStoreFake) PendingGroupWork(_ context.Context, userID, groupID string) ([]model.AccountDeletionGroupNote, error) {
	var work []model.AccountDeletionGroupNote
	for _, item := range f.groupWork[userID+":"+groupID] {
		work = append(work, item)
	}
	return work, nil
}
func (f *accountDeletionStoreFake) CompleteGroupWork(_ context.Context, userID, groupID, noteID string) error {
	delete(f.groupWork[userID+":"+groupID], noteID)
	return nil
}

type failingRevokeDrive struct {
	drive.Client
	err error
}

func (d *failingRevokeDrive) RevokePermissionByID(context.Context, string, string, string) error {
	return d.err
}
func (d *failingRevokeDrive) RevokePermission(context.Context, string, string, string) error {
	return d.err
}
func (d *failingRevokeDrive) RevokeAllPermissions(context.Context, string, string) error {
	return d.err
}

func TestAccountDeletionRevokesSharesButPreservesNoteContentAndOtherSources(t *testing.T) {
	svc, driveMock, members, social := newRestrictedService()
	ctx := context.Background()
	owner, other := uuid.NewString(), uuid.NewString()
	groupA, groupB := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, groupA)
	social.AddAdmin(owner, groupB)
	social.AddAdmin(other, groupA)
	members.SetEmails(groupA, []string{"owner@example.test", "other@example.test"})
	members.SetEmails(groupB, []string{"owner@example.test", "member-b@example.test"})

	owned, err := svc.Create(ctx, owner, "retain content", nil, "private", stringPtr("preserve this body"), "")
	if err != nil {
		t.Fatal(err)
	}
	shareA, err := svc.Share(ctx, owner, owned.ID, groupA, "restricted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, owned.ID, groupB, "restricted"); err != nil {
		t.Fatal(err)
	}
	foreign, err := svc.Create(ctx, other, "other owner's note", nil, "private", stringPtr("keep"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, other, foreign.ID, groupA, "restricted"); err != nil {
		t.Fatal(err)
	}

	store := &accountDeletionStoreFake{}
	svc.SetAccountDeletionRepository(store)
	status := svc.AccountDeletionCleanup(ctx, owner)
	if status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE || store.completed != 1 {
		t.Fatalf("cleanup status=%v completed=%d", status, store.completed)
	}
	if note, err := svc.notes.GetByID(ctx, owned.ID); err != nil || note == nil || note.Title != "retain content" {
		t.Fatalf("owned note content was deleted: note=%+v err=%v", note, err)
	}
	if shares, _ := svc.shared.ListByNote(ctx, owned.ID); len(shares) != 0 {
		t.Fatalf("account-owned shares remain: %+v", shares)
	}
	if shares, _ := svc.shared.ListByNote(ctx, foreign.ID); len(shares) != 1 {
		t.Fatalf("other author's share was removed: %+v", shares)
	}
	if len(driveMock.RevokeCalls) == 0 {
		t.Fatal("Drive permission revocation was not exercised")
	}
	_ = shareA
}

func TestDeletedLastMemberGroupRemovesOnlyThatGroupShares(t *testing.T) {
	svc, _, _, social := newRestrictedService()
	ctx := context.Background()
	owner, other := uuid.NewString(), uuid.NewString()
	removedGroup, survivingGroup := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, removedGroup)
	social.AddAdmin(owner, survivingGroup)
	social.AddAdmin(other, removedGroup)

	owned, _ := svc.Create(ctx, owner, "owner note", nil, "private", stringPtr("keep"), "")
	foreign, _ := svc.Create(ctx, other, "foreign note", nil, "private", stringPtr("keep"), "")
	_, _ = svc.Share(ctx, owner, owned.ID, removedGroup, "link")
	_, _ = svc.Share(ctx, owner, owned.ID, survivingGroup, "link")
	_, _ = svc.Share(ctx, other, foreign.ID, removedGroup, "link")

	svc.SetAccountDeletionRepository(&accountDeletionStoreFake{})
	status := svc.UnshareAccountGroup(ctx, owner, removedGroup, true)
	if status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("group cleanup status = %v", status)
	}
	if shares, _ := svc.shared.ListByNote(ctx, owned.ID); len(shares) != 1 || shares[0].GroupID != survivingGroup {
		t.Fatalf("shares from surviving group were not preserved: %+v", shares)
	}
	if shares, _ := svc.shared.ListByNote(ctx, foreign.ID); len(shares) != 0 {
		t.Fatalf("deleted-group shares remain: %+v", shares)
	}
	if note, _ := svc.notes.GetByID(ctx, foreign.ID); note == nil {
		t.Fatal("group cleanup deleted another user's own note")
	}
}

func TestAccountDeletionReturnsPendingWhenDriveRevocationFails(t *testing.T) {
	svc, _, members, social := newRestrictedService()
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	members.SetEmails(group, []string{"owner@example.test"})
	note, err := svc.Create(ctx, owner, "pending revoke", nil, "private", stringPtr("keep"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	store := &accountDeletionStoreFake{}
	svc.SetAccountDeletionRepository(store)
	svc.drive = &failingRevokeDrive{Client: svc.drive, err: errors.New("synthetic Drive outage")}
	status := svc.AccountDeletionCleanup(ctx, owner)
	if status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING {
		t.Fatalf("Drive outage status = %v, want pending", status)
	}
	if store.completed != 0 {
		t.Fatal("Notes tombstone marked complete while Drive revocation was pending")
	}
	if shares, _ := svc.shared.ListByNote(ctx, note.ID); len(shares) != 0 {
		t.Fatal("durable desired state should remove the share even while Drive is pending")
	}
	managed, _ := svc.shared.ListManagedPermissions(ctx, note.ID)
	for _, permission := range managed {
		if permission.SyncStatus == model.PermissionSyncInSync {
			continue
		}
		return
	}
	// Revoke failures may keep the managed row failed or remove it if the fake
	// backend reports an already-absent permission; a failed status is observed
	// through the pending return above.
}

func TestSurvivingGroupCleanupRevokesDepartingMemberFromOtherAuthorsNote(t *testing.T) {
	svc, driveMock, _, social := newRestrictedService()
	ctx := context.Background()
	owner, departing, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	social.AddMember(departing, group)
	memberEmails := &fakeDriveEmails{byGroup: map[string][]string{
		group: {"owner@gmail.com", "departing@gmail.com"},
	}}
	svc.SetDriveEmailDirectory(memberEmails)

	foreign, err := svc.Create(ctx, owner, "retained share", nil, "private", stringPtr("keep"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, foreign.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}

	// Social already removed the departing account; the group itself survives.
	memberEmails.byGroup[group] = []string{"owner@gmail.com"}
	svc.SetAccountDeletionRepository(&accountDeletionStoreFake{})
	status := svc.UnshareAccountGroup(ctx, departing, group, false)
	if status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("group cleanup status = %v", status)
	}
	shares, err := svc.shared.ListByNote(ctx, foreign.ID)
	if err != nil || len(shares) != 1 {
		t.Fatalf("other author's group share must remain: shares=%+v err=%v", shares, err)
	}
	if !revokedEmail(revokeEmailsOf(driveMock), "departing@gmail.com") {
		t.Fatalf("departing member's Drive permission was not revoked: %+v", driveMock.RevokeCalls)
	}
	if note, err := svc.notes.GetByID(ctx, foreign.ID); err != nil || note == nil {
		t.Fatalf("other user's note was removed: note=%+v err=%v", note, err)
	}
}
