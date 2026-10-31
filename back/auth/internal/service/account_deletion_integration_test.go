package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

type fixedAccountCleanup struct {
	result AccountCleanupResult
	calls  int
}

func (f *fixedAccountCleanup) BeginNotes(context.Context, string) AccountCleanupResult {
	return f.result
}
func (f *fixedAccountCleanup) BeginSocial(context.Context, string) AccountCleanupResult {
	return f.result
}
func (f *fixedAccountCleanup) CleanupSocial(context.Context, string) AccountCleanupResult {
	return f.result
}
func (f *fixedAccountCleanup) CleanupNotes(context.Context, string) AccountCleanupResult {
	f.calls++
	return f.result
}

type timeoutAccountCleanup struct{}

func (timeoutAccountCleanup) BeginNotes(ctx context.Context, _ string) AccountCleanupResult {
	<-ctx.Done()
	return AccountCleanupResult{Status: AccountCleanupError, Err: ctx.Err()}
}
func (timeoutAccountCleanup) BeginSocial(context.Context, string) AccountCleanupResult {
	return AccountCleanupResult{Status: AccountCleanupComplete}
}
func (timeoutAccountCleanup) CleanupSocial(context.Context, string) AccountCleanupResult {
	return AccountCleanupResult{Status: AccountCleanupComplete}
}
func (timeoutAccountCleanup) CleanupNotes(context.Context, string) AccountCleanupResult {
	return AccountCleanupResult{Status: AccountCleanupComplete}
}

func TestAuthIntegration_AccountDeletionPreservesRowsUntilCompleteAndCascades(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	users := repository.NewUserRepository(pool)

	createFixture := func(email string) string {
		t.Helper()
		user, err := users.CreateUser(ctx, email, "synthetic-password-hash", "QA account", nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatalf("create identity fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity.refresh_tokens (user_id, token_hash, expires_at) VALUES ($1, 'synthetic-refresh-hash', now() + interval '1 day')`, user.ID); err != nil {
			t.Fatalf("create refresh fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO identity.password_resets (user_id, code_hash, expires_at) VALUES ($1, 'synthetic-reset-hash', now() + interval '1 hour')`, user.ID); err != nil {
			t.Fatalf("create password reset fixture: %v", err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO identity.oauth_connections (user_id, provider, access_token, refresh_token, external_account_email)
			VALUES ($1, 'google_drive', 'synthetic-drive-access', 'synthetic-drive-refresh', 'drive@test.invalid'),
			       ($1, 'google_calendar', 'synthetic-calendar-access', 'synthetic-calendar-refresh', 'calendar@test.invalid')
		`, user.ID); err != nil {
			t.Fatalf("create OAuth fixtures: %v", err)
		}
		return user.ID
	}
	assertRows := func(userID string, wantPresent bool) {
		t.Helper()
		for _, table := range []string{"users", "profiles", "refresh_tokens", "password_resets", "oauth_connections"} {
			column := "user_id"
			if table == "users" {
				column = "id"
			}
			var count int
			query := fmt.Sprintf("SELECT count(*) FROM identity.%s WHERE %s = $1", table, column)
			if err := pool.QueryRow(ctx, query, userID).Scan(&count); err != nil {
				t.Fatalf("count %s: %v", table, err)
			}
			if wantPresent && count == 0 {
				t.Errorf("expected %s rows to remain for pending account deletion", table)
			}
			if !wantPresent && count != 0 {
				t.Errorf("expected cascaded %s rows to be deleted, got %d", table, count)
			}
		}
		if wantPresent {
			var oauthCount int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1`, userID).Scan(&oauthCount); err != nil {
				t.Fatal(err)
			}
			if oauthCount != 2 {
				t.Errorf("expected both OAuth connections to remain, got %d", oauthCount)
			}
		}
	}

	// Pending cleanup must preserve both the Identity row and its OAuth tokens.
	pendingID := createFixture("pending-delete@test.invalid")
	pending := NewAccountDeletionCoordinator(
		&fixedAccountCleanup{result: AccountCleanupResult{Status: AccountCleanupPending}}, repository.NewAccountDeletionRepository(pool),
	)
	if err := pending.DeleteAccount(ctx, pendingID); !errors.Is(err, ErrAccountCleanupPending) {
		t.Fatalf("pending cleanup error = %v", err)
	}
	assertRows(pendingID, true)

	// A real context deadline from the cleanup interface must stop before local deletion.
	timeoutID := createFixture("timeout-delete@test.invalid")
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	err := NewAccountDeletionCoordinator(timeoutAccountCleanup{}, repository.NewAccountDeletionRepository(pool)).DeleteAccount(timeoutCtx, timeoutID)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout cleanup error = %v", err)
	}
	assertRows(timeoutID, true)

	// An explicit cleanup error has the same retention behavior.
	errorID := createFixture("error-delete@test.invalid")
	remoteErr := errors.New("synthetic cleanup failure")
	failed := NewAccountDeletionCoordinator(
		&fixedAccountCleanup{result: AccountCleanupResult{Status: AccountCleanupError, Err: remoteErr}}, repository.NewAccountDeletionRepository(pool),
	)
	if err := failed.DeleteAccount(ctx, errorID); !errors.Is(err, remoteErr) {
		t.Fatalf("cleanup error = %v", err)
	}
	assertRows(errorID, true)

	// Force a PostgreSQL error after cleanup completes. The failed DELETE is atomic,
	// leaving every dependent row available for a repeated coordinator call.
	retryID := createFixture("retry-delete@test.invalid")
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION identity.reject_test_user_delete() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic local delete failure'; END; $$;
		CREATE TRIGGER reject_test_user_delete BEFORE DELETE ON identity.users
		FOR EACH ROW EXECUTE FUNCTION identity.reject_test_user_delete();
	`); err != nil {
		t.Fatalf("install synthetic delete failure: %v", err)
	}
	cleanup := &fixedAccountCleanup{result: AccountCleanupResult{Status: AccountCleanupComplete}}
	coordinator := NewAccountDeletionCoordinator(cleanup, repository.NewAccountDeletionRepository(pool))
	if err := coordinator.DeleteAccount(ctx, retryID); err == nil {
		t.Fatal("expected local PostgreSQL delete failure after completed cleanup")
	}
	assertRows(retryID, true)
	if _, err := pool.Exec(ctx, `DROP TRIGGER reject_test_user_delete ON identity.users; DROP FUNCTION identity.reject_test_user_delete()`); err != nil {
		t.Fatalf("remove synthetic delete failure: %v", err)
	}
	if err := coordinator.DeleteAccount(ctx, retryID); err != nil {
		t.Fatalf("retry after local PostgreSQL failure: %v", err)
	}
	assertRows(retryID, false)
	if cleanup.calls != 2 {
		t.Fatalf("cleanup must be safe to repeat after local failure, got %d calls", cleanup.calls)
	}
}

func TestAuthIntegrationLoginAndRefreshAreBlockedAfterDeletionBegins(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	users := repository.NewUserRepository(pool)
	user, err := users.CreateUser(ctx, "deleting-account@example.test", "synthetic-hash", "Deleting account", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, hash, err := repository.GenerateRawToken()
	if err != nil {
		t.Fatal(err)
	}
	refreshRepo := repository.NewRefreshTokenRepository(pool)
	if _, err := refreshRepo.Create(ctx, user.ID, hash, repository.FingerprintRawToken(raw), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	deletion := repository.NewAccountDeletionRepository(pool)
	t.Cleanup(deletion.Close)
	auth := newIntegrationAuthServiceWithRefreshRepo(pool, refreshRepo)
	auth.SetAccountAccessGate(deletion)
	if err := deletion.Begin(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(ctx, user.Email, "anything"); err == nil {
		t.Fatal("login issued credentials after deletion tombstone")
	}
	if _, err := auth.Refresh(ctx, raw); err == nil {
		t.Fatal("refresh rotated credentials after deletion tombstone")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.refresh_tokens WHERE user_id=$1::uuid`, user.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("blocked refresh changed token rows: count=%d", count)
	}
}

func TestAuthIntegrationAccountGateDoesNotConsumeBusinessPool(t *testing.T) {
	pool := setupIntegrationPoolWithMaxConns(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	users := repository.NewUserRepository(pool)
	var userIDs []string
	for _, email := range []string{"gate-one@example.test", "gate-two@example.test"} {
		user, err := users.CreateUser(ctx, email, "synthetic-hash", "Pool gate", nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, user.ID)
	}
	deletion := repository.NewAccountDeletionRepository(pool)
	t.Cleanup(deletion.Close)
	var releases []func()
	for _, userID := range userIDs {
		release, active, err := deletion.AcquireRequest(ctx, userID)
		if err != nil || !active {
			t.Fatalf("gate active=%v err=%v", active, err)
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
		t.Fatalf("business pool blocked by two active gates: value=%d err=%v", one, err)
	}
}

func TestAuthIntegration_InFlightRequestFinishesBeforeDeleteAndOldJWTIsBlocked(t *testing.T) {
	pool := setupIntegrationPool(t)
	ctx := context.Background()
	users := repository.NewUserRepository(pool)
	user, err := users.CreateUser(ctx, "lock-delete@test.invalid", "synthetic-password-hash", "QA account", nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO identity.oauth_connections (user_id,provider,access_token,refresh_token,external_account_email) VALUES ($1,'google_drive','synthetic-access','synthetic-refresh','drive@test.invalid')`, user.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM identity.account_deletions WHERE user_id=$1`, user.ID) })

	store := repository.NewAccountDeletionRepository(pool)
	release, active, err := store.AcquireRequest(ctx, user.ID)
	if err != nil || !active {
		t.Fatalf("acquire in-flight account request: active=%v err=%v", active, err)
	}
	deleteDone := make(chan error, 1)
	go func() {
		deleteDone <- NewAccountDeletionCoordinator(completeStages(), store).DeleteAccount(ctx, user.ID)
	}()
	select {
	case err := <-deleteDone:
		t.Fatalf("delete passed the active request lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	var oauthCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1`, user.ID).Scan(&oauthCount); err != nil || oauthCount != 1 {
		t.Fatalf("OAuth must remain until the in-flight request drains: count=%d err=%v", oauthCount, err)
	}
	release()
	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatalf("delete after request drained: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("delete did not resume after the request lock was released")
	}
	_, active, err = store.AcquireRequest(ctx, user.ID)
	if err != nil || active {
		t.Fatalf("old JWT account gate after deletion: active=%v err=%v", active, err)
	}
}
