package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	notesrepo "github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
)

func accountDeletionPGService(t *testing.T) (*NoteService, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL not configured")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connect disposable notes database: %v", err)
	}
	t.Cleanup(pool.Close)
	driveMock := drive.NewMockClient()
	social := NewMemorySocialResolver()
	members := NewMemoryMemberDirectory()
	svc := NewNoteService(NewPGNoteStore(pool), NewPGAttachmentStore(pool), NewPGSavedStore(pool), NewPGLikeStore(pool), NewPGSharedStore(pool), driveMock, social)
	svc.SetMemberDirectory(members)
	deletionRepo := notesrepo.NewAccountDeletionRepository(pool)
	t.Cleanup(deletionRepo.Close)
	svc.SetAccountDeletionRepository(deletionRepo)
	return svc, pool
}

func TestNotesIntegrationDeletionFenceDrainsInflightRequestAndRejectsLaterWrites(t *testing.T) {
	_, pool := accountDeletionPGService(t)
	ctx := context.Background()
	userID := uuid.NewString()
	store := notesrepo.NewAccountDeletionRepository(pool)
	t.Cleanup(store.Close)
	release, active, err := store.AcquireRequest(ctx, userID)
	if err != nil || !active {
		t.Fatalf("acquire in-flight request: active=%v err=%v", active, err)
	}
	beginDone := make(chan error, 1)
	go func() {
		_, err := store.Begin(ctx, userID)
		beginDone <- err
	}()
	select {
	case err := <-beginDone:
		t.Fatalf("deletion fence passed an in-flight request: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	select {
	case err := <-beginDone:
		if err != nil {
			t.Fatalf("begin deletion after request drained: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("deletion fence did not continue after request release")
	}
	_, active, err = store.AcquireRequest(ctx, userID)
	if err != nil || active {
		t.Fatalf("new request accepted after deletion fence: active=%v err=%v", active, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM notes.account_deletion_users WHERE user_id=$1::uuid`, userID)
	})
}

func TestNotesIntegrationAccountGateDoesNotConsumeBusinessPool(t *testing.T) {
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL not configured")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	cfg.MinConns = 0
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := notesrepo.NewAccountDeletionRepository(pool)
	t.Cleanup(store.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var releases []func()
	for range 2 {
		release, active, err := store.AcquireRequest(ctx, uuid.NewString())
		if err != nil || !active {
			t.Fatalf("request gate active=%v err=%v", active, err)
		}
		releases = append(releases, release)
	}
	defer func() {
		for _, release := range releases {
			release()
		}
	}()
	var one int
	if err := pool.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("business pool blocked while two gate requests were in flight: value=%d err=%v", one, err)
	}
}

func TestNotesIntegrationDeletedGroupDriveFailureSurvivesRestartAndRetries(t *testing.T) {
	svc, pool := accountDeletionPGService(t)
	ctx := context.Background()
	departing, author, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
	driveMock := svc.drive.(*drive.MockClient)
	members := svc.members.(*MemoryMemberDirectory)
	social := svc.social.(*MemorySocialResolver)
	social.AddAdmin(departing, group)
	social.AddMember(author, group)
	members.SetEmails(group, []string{"departing@example.test", "survivor@example.test"})
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.notes WHERE user_id=$1::uuid`, author)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.account_deletion_group_notes WHERE user_id=$1::uuid`, departing)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.account_deletion_users WHERE user_id=$1::uuid`, departing)
	})

	note, err := svc.Create(ctx, author, "foreign author note", nil, "private", stringPtr("must remain"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, author, note.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	if !driveMock.HasPermission(*note.ExternalFileID, "departing@example.test") {
		t.Fatal("fixture did not grant the departing member's Drive permission")
	}
	baseDrive := svc.drive
	svc.drive = &failingRevokeDrive{Client: baseDrive, err: errors.New("synthetic Drive outage")}
	if status := svc.UnshareAccountGroup(ctx, departing, group, true); status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING {
		t.Fatalf("first cleanup status=%v, want PENDING", status)
	}
	var queued, shares int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.account_deletion_group_notes WHERE user_id=$1::uuid AND group_id=$2::uuid AND note_id=$3::uuid`, departing, group, note.ID).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.shared_notes WHERE note_id=$1::uuid AND group_id=$2::uuid`, note.ID, group).Scan(&shares); err != nil {
		t.Fatal(err)
	}
	if queued != 1 || shares != 0 {
		t.Fatalf("after failed Drive revoke: durable tasks=%d shares=%d; want 1 and 0", queued, shares)
	}
	if !driveMock.HasPermission(*note.ExternalFileID, "departing@example.test") {
		t.Fatal("failed Drive revoke unexpectedly removed access")
	}

	// Recreate the service and repository as if Notes restarted. The durable
	// task row must be enough to find the note after shared_notes is empty.
	restarted := NewNoteService(svc.notes, svc.attachments, svc.saved, svc.likes, svc.shared, baseDrive, svc.social)
	restarted.SetAccountDeletionRepository(notesrepo.NewAccountDeletionRepository(pool))
	deletionRepo := restarted.accountDeletion.(*notesrepo.AccountDeletionRepository)
	t.Cleanup(deletionRepo.Close)
	restarted.SetMemberDirectory(members)
	if status := restarted.UnshareAccountGroup(ctx, departing, group, true); status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("retry after restart status=%v, want COMPLETE", status)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notes.account_deletion_group_notes WHERE user_id=$1::uuid AND group_id=$2::uuid`, departing, group).Scan(&queued); err != nil {
		t.Fatal(err)
	}
	if queued != 0 {
		t.Fatalf("completed task rows=%d, want 0", queued)
	}
	if driveMock.HasPermission(*note.ExternalFileID, "departing@example.test") {
		t.Fatal("departing member's Drive permission reappeared after restart retry")
	}
	if retained, err := restarted.notes.GetByID(ctx, note.ID); err != nil || retained == nil || retained.UserID != author {
		t.Fatalf("foreign note was not preserved: note=%+v err=%v", retained, err)
	}
	managed, err := restarted.shared.ListManagedPermissions(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, permission := range managed {
		if permission.SyncStatus != model.PermissionSyncInSync {
			t.Fatalf("managed permission left pending after retry: %+v", permission)
		}
	}
}

func TestNotesIntegrationAccountDeletionRevokesSharesAndRetriesDriveFailure(t *testing.T) {
	svc, pool := accountDeletionPGService(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social := svc.social.(*MemorySocialResolver)
	members := svc.members.(*MemoryMemberDirectory)
	social.AddAdmin(owner, group)
	members.SetEmails(group, []string{"owner@example.test", "member@example.test"})
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.notes WHERE user_id=$1::uuid`, owner)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.account_deletion_users WHERE user_id=$1::uuid`, owner)
	})

	note, err := svc.Create(ctx, owner, "must remain", nil, "private", stringPtr("content retained"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	store := svc.accountDeletion
	status := svc.AccountDeletionCleanup(ctx, owner)
	if status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("complete Drive cleanup status=%v", status)
	}
	if retained, err := svc.notes.GetByID(ctx, note.ID); err != nil || retained == nil {
		t.Fatalf("account cleanup deleted own note: note=%+v err=%v", retained, err)
	}
	shares, err := svc.shared.ListByNote(ctx, note.ID)
	if err != nil || len(shares) != 0 {
		t.Fatalf("account share rows = %d, %v", len(shares), err)
	}
	complete, err := store.Begin(ctx, owner)
	if err != nil || !complete {
		t.Fatalf("tombstone complete=%v err=%v", complete, err)
	}
}

func TestNotesIntegrationAccountDeletionKeepsTombstonePendingUntilDriveRetry(t *testing.T) {
	svc, pool := accountDeletionPGService(t)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social := svc.social.(*MemorySocialResolver)
	members := svc.members.(*MemoryMemberDirectory)
	social.AddAdmin(owner, group)
	members.SetEmails(group, []string{"owner@example.test"})
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.notes WHERE user_id=$1::uuid`, owner)
		_, _ = pool.Exec(context.Background(), `DELETE FROM notes.account_deletion_users WHERE user_id=$1::uuid`, owner)
	})
	note, err := svc.Create(ctx, owner, "retry revoke", nil, "private", stringPtr("preserve"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
		t.Fatal(err)
	}
	baseDrive := svc.drive
	svc.drive = &failingRevokeDrive{Client: baseDrive, err: errors.New("synthetic Drive outage")}
	if status := svc.AccountDeletionCleanup(ctx, owner); status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING {
		t.Fatalf("failure status=%v, want pending", status)
	}
	complete, err := svc.accountDeletion.Begin(ctx, owner)
	if err != nil || complete {
		t.Fatalf("tombstone prematurely complete=%v err=%v", complete, err)
	}
	managed, err := svc.shared.ListManagedPermissions(ctx, note.ID)
	if err != nil || len(managed) == 0 {
		t.Fatalf("failed permission intent not retained: rows=%d err=%v", len(managed), err)
	}
	svc.drive = baseDrive
	if status := svc.AccountDeletionCleanup(ctx, owner); status != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("retry status=%v", status)
	}
}
