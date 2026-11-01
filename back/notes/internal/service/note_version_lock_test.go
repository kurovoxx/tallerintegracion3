package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/utils"
)

// TestPermissionOptimisticVersionConflict verifica el versionado optimista de
// PATCH: un update con la versión vigente incrementa Version; uno con una
// versión stale responde 409 conflict y no muta la fila.
func TestPermissionOptimisticVersionConflict(t *testing.T) {
	svc, _, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Versionada", nil, "private", nil, "")
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if note.Version != 1 {
		t.Fatalf("una nota nueva debe nacer en version 1, got %d", note.Version)
	}
	updated, err := svc.UpdateWithExpectedVersion(ctx, author, note.ID, stringPtr("V2"), nil, nil, note.Version, "")
	if err != nil {
		t.Fatalf("update con versión vigente falló: %v", err)
	}
	if updated.Version != note.Version+1 {
		t.Fatalf("version esperada %d, got %d", note.Version+1, updated.Version)
	}
	_, err = svc.UpdateWithExpectedVersion(ctx, author, note.ID, stringPtr("stale"), nil, nil, note.Version, "")
	var se *ServiceError
	if !errors.As(err, &se) || se.Code != utils.ErrConflict {
		t.Fatalf("esperaba conflict 409, got %v", err)
	}
	stored, err := notes.GetByID(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Title != "V2" {
		t.Fatalf("un conflicto de versión no debe mutar la nota, got %q", stored.Title)
	}
	if stored.Version != note.Version+1 {
		t.Fatalf("la versión no debe cambiar tras el conflicto, got %d", stored.Version)
	}
}

// TestPermissionConcurrentUpdatesSingleWinner lanza dos PATCH concurrentes que
// parten de la misma versión: el lock por nota más el versionado garantizan que
// exactamente uno gane y el otro reciba 409, sin lost updates.
func TestPermissionConcurrentUpdatesSingleWinner(t *testing.T) {
	svc, _, notes, _, _, _, _, _ := newTestService()
	ctx := context.Background()
	author := uuid.NewString()
	note, err := svc.Create(ctx, author, "Carrera", nil, "private", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			title := fmt.Sprintf("writer-%d", i)
			_, errs[i] = svc.UpdateWithExpectedVersion(ctx, author, note.ID, &title, nil, nil, note.Version, "")
		}(i)
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		var se *ServiceError
		if errors.As(err, &se) && se.Code == utils.ErrConflict {
			conflicts++
			continue
		}
		t.Fatalf("error inesperado en la carrera: %v", err)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("esperaba exactamente 1 ganador y 1 conflicto, got %d/%d", successes, conflicts)
	}
	stored, err := notes.GetByID(ctx, note.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != note.Version+1 {
		t.Fatalf("una única escritura debe haber incrementado la versión a %d, got %d", note.Version+1, stored.Version)
	}
}

// TestPermissionWithNoteLockSerializes comprueba la exclusión mutua por nota del
// store en memoria (equivalente al pg_advisory_xact_lock de PG): ningún callback
// de la misma nota se ejecuta en paralelo.
func TestPermissionWithNoteLockSerializes(t *testing.T) {
	shared := NewMemorySharedStore()
	ctx := context.Background()
	var active, maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := shared.WithNoteLock(ctx, "note-lock", func(context.Context) error {
				cur := atomic.AddInt32(&active, 1)
				for {
					prev := atomic.LoadInt32(&maxActive)
					if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			}); err != nil {
				t.Errorf("WithNoteLock falló: %v", err)
			}
		}()
	}
	wg.Wait()
	if maxActive != 1 {
		t.Fatalf("el lock por nota debe serializar la sección crítica (max concurrencia %d)", maxActive)
	}

	if err := shared.WithNoteLock(ctx, "note-lock", nil); err == nil {
		t.Fatal("un callback nil debe devolver error")
	}
	sentinel := errors.New("boom")
	if err := shared.WithNoteLock(ctx, "note-lock", func(context.Context) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("el error del callback debe propagarse, got %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := shared.WithNoteLock(cancelled, "note-lock", func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("un contexto cancelado debe rechazar la adquisición, got %v", err)
	}
}

// TestPermissionMemoryStoreVersionConflict replica la semántica del UPDATE ...
// WHERE id = $1 AND version = $expected de PG en el store en memoria.
func TestPermissionMemoryStoreVersionConflict(t *testing.T) {
	store := NewMemoryNoteStore()
	ctx := context.Background()
	note, err := store.Create(ctx, "", "user-1", nil, "Base", nil, "private", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if note.Version != 1 {
		t.Fatalf("create debe fijar version 1, got %d", note.Version)
	}
	updated, err := store.Update(ctx, note.ID, stringPtr("Uno"), nil, note.Version)
	if err != nil {
		t.Fatalf("update con versión vigente falló: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version esperada 2, got %d", updated.Version)
	}
	if _, err := store.Update(ctx, note.ID, stringPtr("Dos"), nil, note.Version); !errors.Is(err, repository.ErrConflict) {
		t.Fatalf("esperaba repository.ErrConflict con versión stale, got %v", err)
	}
	missing, err := store.Update(ctx, uuid.NewString(), stringPtr("x"), nil, 1)
	if err != nil || missing != nil {
		t.Fatalf("nota inexistente debe devolver (nil, nil), got (%v, %v)", missing, err)
	}
}
