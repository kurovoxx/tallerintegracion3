package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func todoSeamSetup() (gid, admin, member, stranger string, store *MemoryTodoStore) {
	gid, admin, member, stranger = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	store = NewMemoryTodoStore()
	store.AddGroup(gid, admin, member)
	return gid, admin, member, stranger, store
}

func TestTodoMembership_Outsider403(t *testing.T) {
	gid, admin, _, stranger, store := todoSeamSetup()
	svc := NewTodoService(store)

	if _, err := svc.CreateTodo(context.Background(), gid, stranger, "", "T", "", "", ""); err != ErrForbidden {
		t.Fatalf("crear extraño debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.ListTodos(context.Background(), gid, stranger, "", ""); err != ErrForbidden {
		t.Fatalf("listar extraño debe dar ErrForbidden, got %v", err)
	}
	// Crea una como admin para probar update/delete.
	created, err := svc.CreateTodo(context.Background(), gid, admin, "", "T", "", "", "")
	if err != nil {
		t.Fatalf("crear admin: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	title := "X"
	if _, err := svc.UpdateTodo(context.Background(), gid, stranger, tid.String(), &title, nil, nil, nil); err != ErrForbidden {
		t.Fatalf("update extraño debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.DeleteTodo(context.Background(), gid, stranger, tid.String()); err != ErrForbidden {
		t.Fatalf("delete extraño debe dar ErrForbidden, got %v", err)
	}
}

func TestTodoAssignee_DebeSerMiembro(t *testing.T) {
	gid, admin, member, stranger, store := todoSeamSetup()
	svc := NewTodoService(store)

	if _, err := svc.CreateTodo(context.Background(), gid, admin, "", "T", "", stranger, ""); err != ErrAssigneeNotMember {
		t.Fatalf("asignado extraño debe dar ErrAssigneeNotMember, got %v", err)
	}
	created, err := svc.CreateTodo(context.Background(), gid, admin, "", "T", "", member, "")
	if err != nil {
		t.Fatalf("asignado miembro no debe fallar: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	if _, err := svc.UpdateTodo(context.Background(), gid, admin, tid.String(), nil, nil, &stranger, nil); err != ErrAssigneeNotMember {
		t.Fatalf("reasignar a extraño debe dar ErrAssigneeNotMember, got %v", err)
	}
	// Sin asignado sigue válido.
	if _, err := svc.CreateTodo(context.Background(), gid, member, "", "Libre", "", "", ""); err != nil {
		t.Fatalf("sin asignado no debe fallar: %v", err)
	}
}

func TestTodoCrossGroup_404(t *testing.T) {
	gid, admin, _, _, store := todoSeamSetup()
	other := uuid.NewString()
	store.AddGroup(other, admin)
	svc := NewTodoService(store)

	created, err := svc.CreateTodo(context.Background(), gid, admin, "", "T", "", "", "")
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	title := "X"
	if _, err := svc.UpdateTodo(context.Background(), other, admin, tid.String(), &title, nil, nil, nil); err != ErrTodoNotFound {
		t.Fatalf("tarea de otro grupo debe dar ErrTodoNotFound, got %v", err)
	}
	if _, err := svc.DeleteTodo(context.Background(), other, admin, tid.String()); err != ErrTodoNotFound {
		t.Fatalf("delete de otro grupo debe dar ErrTodoNotFound, got %v", err)
	}
}
