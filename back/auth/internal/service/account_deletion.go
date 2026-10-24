package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/repository"
)

type AccountCleanupStatus uint8

const (
	AccountCleanupUnspecified AccountCleanupStatus = iota
	AccountCleanupComplete
	AccountCleanupPending
	AccountCleanupError
)

type AccountCleanupResult struct {
	Status AccountCleanupStatus
	Err    error
}

// AccountCleanup is the Identity-owned boundary for the Social and Notes
// services. BeginNotes must durably gate Notes writes before Social snapshots
// memberships. CleanupSocial and CleanupNotes are safe to repeat after a crash.
type AccountCleanup interface {
	BeginNotes(context.Context, string) AccountCleanupResult
	BeginSocial(context.Context, string) AccountCleanupResult
	CleanupSocial(context.Context, string) AccountCleanupResult
	CleanupNotes(context.Context, string) AccountCleanupResult
}

type AccountDeletionStore interface {
	WithExclusiveUserLock(context.Context, string, func() error) error
	Begin(context.Context, string) error
	State(context.Context, string) (repository.AccountDeletionState, bool, error)
	RecordError(context.Context, string, error) error
	CompleteAndDelete(context.Context, string) error
	PendingUsers(context.Context, int) ([]string, error)
}

var (
	ErrAccountCleanupPending = errors.New("account cleanup pending")
	ErrAccountCleanupFailed  = errors.New("account cleanup failed")
	ErrInvalidCleanupResult  = errors.New("invalid account cleanup result")
)

type AccountDeletionCoordinator struct {
	cleanup AccountCleanup
	store   AccountDeletionStore
}

func NewAccountDeletionCoordinator(cleanup AccountCleanup, store AccountDeletionStore) *AccountDeletionCoordinator {
	return &AccountDeletionCoordinator{cleanup: cleanup, store: store}
}

// DeleteAccount durably gates Identity first, then gates Notes, drains Social
// membership work, confirms Notes permissions, and only then deletes local
// rows. The durable state and per-service tombstones make every stage retryable.
func (c *AccountDeletionCoordinator) DeleteAccount(ctx context.Context, userID string) error {
	userID = strings.TrimSpace(userID)
	if userID == "" || c.cleanup == nil || c.store == nil {
		return ErrInvalidCleanupResult
	}
	return c.store.WithExclusiveUserLock(ctx, userID, func() error {
		state, exists, err := c.store.State(ctx, userID)
		if err != nil {
			return fmt.Errorf("read account deletion state: %w", err)
		}
		if exists && state.Status == "complete" {
			return nil
		}
		if !exists {
			if err := c.store.Begin(ctx, userID); err != nil {
				return err
			}
		}

		if err := requireCleanupAccepted(c.cleanup.BeginNotes(ctx, userID)); err != nil {
			_ = c.store.RecordError(ctx, userID, errors.New("notes_begin_incomplete"))
			return err
		}
		if err := requireCleanupAccepted(c.cleanup.BeginSocial(ctx, userID)); err != nil {
			_ = c.store.RecordError(ctx, userID, errors.New("social_begin_incomplete"))
			return err
		}
		if err := requireCleanupComplete(c.cleanup.CleanupSocial(ctx, userID)); err != nil {
			_ = c.store.RecordError(ctx, userID, errors.New("social_cleanup_incomplete"))
			return err
		}
		if err := requireCleanupComplete(c.cleanup.CleanupNotes(ctx, userID)); err != nil {
			_ = c.store.RecordError(ctx, userID, errors.New("notes_cleanup_incomplete"))
			return err
		}
		if err := c.store.CompleteAndDelete(ctx, userID); err != nil {
			_ = c.store.RecordError(ctx, userID, errors.New("identity_delete_incomplete"))
			return fmt.Errorf("delete identity account after remote cleanup: %w", err)
		}
		return nil
	})
}

func requireCleanupComplete(result AccountCleanupResult) error {
	switch result.Status {
	case AccountCleanupComplete:
		if result.Err != nil {
			return fmt.Errorf("%w: complete result has an error", ErrInvalidCleanupResult)
		}
		return nil
	case AccountCleanupPending:
		if result.Err != nil {
			return fmt.Errorf("%w: %w", ErrAccountCleanupPending, result.Err)
		}
		return ErrAccountCleanupPending
	case AccountCleanupError:
		if result.Err != nil {
			return fmt.Errorf("%w: %w", ErrAccountCleanupFailed, result.Err)
		}
		return ErrAccountCleanupFailed
	default:
		return fmt.Errorf("%w: status %d", ErrInvalidCleanupResult, result.Status)
	}
}

func requireCleanupAccepted(result AccountCleanupResult) error {
	switch result.Status {
	case AccountCleanupComplete:
		if result.Err != nil {
			return fmt.Errorf("%w: complete begin result has an error", ErrInvalidCleanupResult)
		}
		return nil
	case AccountCleanupPending:
		if result.Err != nil {
			return fmt.Errorf("%w: %w", ErrAccountCleanupFailed, result.Err)
		}
		return nil
	case AccountCleanupError:
		if result.Err != nil {
			return fmt.Errorf("%w: %w", ErrAccountCleanupFailed, result.Err)
		}
		return ErrAccountCleanupFailed
	default:
		return fmt.Errorf("%w: begin status %d", ErrInvalidCleanupResult, result.Status)
	}
}

func (c *AccountDeletionCoordinator) State(ctx context.Context, userID string) (repository.AccountDeletionState, bool, error) {
	if c.store == nil {
		return repository.AccountDeletionState{}, false, ErrInvalidCleanupResult
	}
	return c.store.State(ctx, strings.TrimSpace(userID))
}

// RunPendingOnce is called by the bounded worker. It intentionally continues
// across independent accounts and stores only stage codes, never RPC details.
func (c *AccountDeletionCoordinator) RunPendingOnce(ctx context.Context, limit int) error {
	users, err := c.store.PendingUsers(ctx, limit)
	if err != nil {
		return err
	}
	var failures []error
	for _, userID := range users {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if err := c.DeleteAccount(ctx, userID); err != nil && !errors.Is(err, ErrAccountCleanupPending) && !errors.Is(err, ErrAccountCleanupFailed) {
			failures = append(failures, fmt.Errorf("resume account deletion: %w", err))
		}
	}
	return errors.Join(failures...)
}

func (c *AccountDeletionCoordinator) RunWorker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			workerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			_ = c.RunPendingOnce(workerCtx, 50)
			cancel()
		}
	}
}
