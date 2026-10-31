package service

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/google/uuid"
	notesv1 "github.com/kurovoxx/tallerintegracion3/back/notes/internal/gen/notesv1/v1"
)

// missingTableStore imita la base local sin la migración 003: Begin falla
// mientras beginErr esté definido; al "migrar" (beginErr = nil) se comporta
// como el repositorio real (INSERT ... ON CONFLICT DO NOTHING).
type missingTableStore struct {
	accountDeletionStoreFake
	beginErr   error
	beginCalls int
}

func (s *missingTableStore) Begin(ctx context.Context, userID string) (bool, error) {
	s.beginCalls++
	if s.beginErr != nil {
		return false, s.beginErr
	}
	return s.accountDeletionStoreFake.Begin(ctx, userID)
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	})
	return &buf
}

// Regresión de la prueba manual en Docker local (2026-10-24): Identity quedaba
// en pending con notes_begin_incomplete y Notes solo mostraba code=Internal.
func TestBeginAccountDeletionLogsCauseAndRetryCompletesAfterMigration(t *testing.T) {
	svc, _, _, _ := newRestrictedService()
	ctx := context.Background()
	user := uuid.NewString()
	store := &missingTableStore{beginErr: errors.New(`ERROR: relation "notes.account_deletion_users" does not exist (SQLSTATE 42P01)`)}
	svc.SetAccountDeletionRepository(store)
	logs := captureLog(t)

	// Reintentos del worker de Identity mientras falta la tabla.
	for i := 0; i < 2; i++ {
		if got := svc.BeginAccountDeletion(ctx, user); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR {
			t.Fatalf("Begin sin tabla = %v, want ERROR", got)
		}
	}
	out := logs.String()
	for _, want := range []string{"[begin]", user, `relation "notes.account_deletion_users" does not exist`} {
		if !strings.Contains(out, want) {
			t.Fatalf("el log debe incluir %q:\n%s", want, out)
		}
	}
	if store.completed != 0 {
		t.Fatal("no debe marcarse completo sin tombstone")
	}

	// Se aplica la migración: el mismo reintento avanza sin intervención manual.
	store.beginErr = nil
	if got := svc.BeginAccountDeletion(ctx, user); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_PENDING {
		t.Fatalf("Begin tras migrar = %v, want PENDING", got)
	}
	if got := svc.AccountDeletionCleanup(ctx, user); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("Cleanup = %v, want COMPLETE", got)
	}
	// Idempotente: repetir Begin y Cleanup (respuesta perdida, otro tick del
	// worker) no vuelve a limpiar ni falla.
	if got := svc.BeginAccountDeletion(ctx, user); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("Begin repetido = %v, want COMPLETE", got)
	}
	if got := svc.AccountDeletionCleanup(ctx, user); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_COMPLETE {
		t.Fatalf("Cleanup repetido = %v, want COMPLETE", got)
	}
	if store.completed != 1 {
		t.Fatalf("Complete ejecutado %d veces, want 1", store.completed)
	}
}

func TestAccountDeletionLogsInvalidInputWithoutPersonalData(t *testing.T) {
	svc, _, _, _ := newRestrictedService()
	logs := captureLog(t)
	if got := svc.BeginAccountDeletion(context.Background(), "no-uuid"); got != notesv1.AccountCleanupStatus_ACCOUNT_CLEANUP_STATUS_ERROR {
		t.Fatalf("Begin con id inválido = %v", got)
	}
	if !strings.Contains(logs.String(), "user_id inválido") {
		t.Fatalf("log = %q", logs.String())
	}
}
