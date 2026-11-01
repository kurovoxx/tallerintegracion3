package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

type cleanupStagesFake struct {
	begin, beginSocial, social, notes AccountCleanupResult
	calls                             []string
}

func (f *cleanupStagesFake) BeginNotes(context.Context, string) AccountCleanupResult {
	f.calls = append(f.calls, "begin_notes")
	return f.begin
}
func (f *cleanupStagesFake) BeginSocial(context.Context, string) AccountCleanupResult {
	f.calls = append(f.calls, "begin_social")
	return f.beginSocial
}
func (f *cleanupStagesFake) CleanupSocial(context.Context, string) AccountCleanupResult {
	f.calls = append(f.calls, "social")
	return f.social
}
func (f *cleanupStagesFake) CleanupNotes(context.Context, string) AccountCleanupResult {
	f.calls = append(f.calls, "notes")
	return f.notes
}

type deletionStoreFake struct {
	mu            sync.Mutex
	state         map[string]repository.AccountDeletionState
	completeErr   error
	completeCalls int
	deleted       map[string]bool
	lastError     string
}

func newDeletionStoreFake() *deletionStoreFake {
	return &deletionStoreFake{state: map[string]repository.AccountDeletionState{}, deleted: map[string]bool{}}
}
func (f *deletionStoreFake) WithExclusiveUserLock(_ context.Context, _ string, fn func() error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return fn()
}
func (f *deletionStoreFake) Begin(_ context.Context, id string) error {
	if _, ok := f.state[id]; !ok {
		f.state[id] = repository.AccountDeletionState{Status: "pending"}
	}
	return nil
}
func (f *deletionStoreFake) State(_ context.Context, id string) (repository.AccountDeletionState, bool, error) {
	state, ok := f.state[id]
	return state, ok, nil
}
func (f *deletionStoreFake) RecordError(_ context.Context, _ string, err error) error {
	if err != nil {
		f.lastError = err.Error()
	}
	return nil
}
func (f *deletionStoreFake) CompleteAndDelete(_ context.Context, id string) error {
	f.completeCalls++
	if f.completeErr != nil {
		return f.completeErr
	}
	f.deleted[id] = true
	state := f.state[id]
	state.Status = "complete"
	f.state[id] = state
	return nil
}
func (f *deletionStoreFake) PendingUsers(context.Context, int) ([]string, error) { return nil, nil }

func completeStages() *cleanupStagesFake {
	ok := AccountCleanupResult{Status: AccountCleanupComplete}
	return &cleanupStagesFake{begin: ok, beginSocial: ok, social: ok, notes: ok}
}

func TestAccountDeletionRequiresExplicitCompletionAtEveryStage(t *testing.T) {
	tests := []struct {
		name      string
		cleanup   *cleanupStagesFake
		wantErr   error
		wantCalls []string
	}{
		{"notes prepare ambiguous", &cleanupStagesFake{begin: AccountCleanupResult{Status: AccountCleanupUnspecified}}, ErrInvalidCleanupResult, []string{"begin_notes"}},
		{"pending gate then social prepare ambiguous", &cleanupStagesFake{begin: AccountCleanupResult{Status: AccountCleanupPending}, beginSocial: AccountCleanupResult{Status: AccountCleanupUnspecified}}, ErrInvalidCleanupResult, []string{"begin_notes", "begin_social"}},
		{"social ambiguous legacy response", &cleanupStagesFake{begin: AccountCleanupResult{Status: AccountCleanupPending}, beginSocial: AccountCleanupResult{Status: AccountCleanupPending}, social: AccountCleanupResult{Status: AccountCleanupUnspecified}}, ErrInvalidCleanupResult, []string{"begin_notes", "begin_social", "social"}},
		{"social pending", &cleanupStagesFake{begin: AccountCleanupResult{Status: AccountCleanupPending}, beginSocial: AccountCleanupResult{Status: AccountCleanupPending}, social: AccountCleanupResult{Status: AccountCleanupPending}}, ErrAccountCleanupPending, []string{"begin_notes", "begin_social", "social"}},
		{"notes error", &cleanupStagesFake{begin: AccountCleanupResult{Status: AccountCleanupComplete}, beginSocial: AccountCleanupResult{Status: AccountCleanupComplete}, social: AccountCleanupResult{Status: AccountCleanupComplete}, notes: AccountCleanupResult{Status: AccountCleanupError, Err: errors.New("drive pending")}}, ErrAccountCleanupFailed, []string{"begin_notes", "begin_social", "social", "notes"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newDeletionStoreFake()
			err := NewAccountDeletionCoordinator(tt.cleanup, store).DeleteAccount(context.Background(), "user-1")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DeleteAccount error=%v want=%v", err, tt.wantErr)
			}
			if store.deleted["user-1"] || store.completeCalls != 0 {
				t.Fatal("local Identity deletion occurred without complete results from every stage")
			}
			if len(tt.cleanup.calls) != len(tt.wantCalls) {
				t.Fatalf("calls=%v want=%v", tt.cleanup.calls, tt.wantCalls)
			}
			for i := range tt.wantCalls {
				if tt.cleanup.calls[i] != tt.wantCalls[i] {
					t.Fatalf("calls=%v want=%v", tt.cleanup.calls, tt.wantCalls)
				}
			}
			if got := store.lastError; got == "" {
				t.Fatal("expected durable safe stage code for retry")
			}
		})
	}
}

func TestAccountDeletionRetriesAfterLocalCommitFailure(t *testing.T) {
	cleanup := completeStages()
	store := newDeletionStoreFake()
	store.completeErr = errors.New("synthetic transaction failure")
	coordinator := NewAccountDeletionCoordinator(cleanup, store)
	if err := coordinator.DeleteAccount(context.Background(), "user-1"); err == nil {
		t.Fatal("expected local failure")
	}
	if store.deleted["user-1"] {
		t.Fatal("user was deleted despite failed local transaction")
	}
	store.completeErr = nil
	if err := coordinator.DeleteAccount(context.Background(), "user-1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !store.deleted["user-1"] || store.completeCalls != 2 {
		t.Fatalf("retry did not complete local deletion: deleted=%v calls=%d", store.deleted["user-1"], store.completeCalls)
	}
}

func TestAccountCleanupCompatibilityNeverTreatsUnknownAsComplete(t *testing.T) {
	if err := requireCleanupComplete(AccountCleanupResult{Status: AccountCleanupUnspecified}); !errors.Is(err, ErrInvalidCleanupResult) {
		t.Fatalf("unknown result error=%v", err)
	}
	if got := mapNotesCleanupStatus(0); got.Status != AccountCleanupUnspecified {
		t.Fatalf("legacy Notes response mapped to %v", got.Status)
	}
}
