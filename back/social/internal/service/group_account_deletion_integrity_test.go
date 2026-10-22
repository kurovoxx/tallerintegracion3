package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

type partialAccountDeletionStore struct {
	GroupStore
	results []model.SuccessionResult
	err     error
}

func (s partialAccountDeletionStore) HandleAccountDeletion(context.Context, string) ([]model.SuccessionResult, error) {
	return s.results, s.err
}

type accountDeletionNotesRecorder struct{ calls chan unsharerCall }

func (r accountDeletionNotesRecorder) UnshareAll(_ context.Context, userID, groupID string) error {
	r.calls <- unsharerCall{userID: userID, groupID: groupID}
	return nil
}

// El repositorio confirma cada grupo por separado: un error posterior no debe
// perder el cleanup de los grupos que ya no aparecerán en el reintento.
func TestAccountDeletionCleansCommittedGroupsOnPartialFailure(t *testing.T) {
	storeErr := errors.New("falló el siguiente grupo")
	successor := uMember
	committed := []model.SuccessionResult{
		{GroupID: "grupo-con-sucesor", PromotedUserID: &successor},
		{GroupID: "grupo-eliminado", GroupDeleted: true},
	}
	svc := NewGroupService(partialAccountDeletionStore{results: committed, err: storeErr}, nil)
	calls := make(chan unsharerCall, len(committed))
	svc.SetNotesUnsharer(accountDeletionNotesRecorder{calls: calls})

	results, err := svc.HandleAccountDeletion(context.Background(), uAdmin, "jwt-del-usuario")
	if !errors.Is(err, storeErr) {
		t.Fatalf("debe conservarse el error para permitir reintentos: %v", err)
	}
	if len(results) != len(committed) {
		t.Fatalf("se perdieron las sucesiones ya confirmadas: got %v, want %v", results, committed)
	}
	seen := map[string]bool{}
	for range committed {
		select {
		case call := <-calls:
			if call.userID != uAdmin || seen[call.groupID] {
				t.Fatalf("cleanup inesperado: %+v", call)
			}
			seen[call.groupID] = true
		case <-time.After(2 * time.Second):
			t.Fatal("faltó limpiar Notes para los grupos ya confirmados")
		}
	}
	for i, result := range committed {
		if results[i] != result || !seen[result.GroupID] {
			t.Fatalf("sucesión o cleanup perdido para %s", result.GroupID)
		}
	}
}
