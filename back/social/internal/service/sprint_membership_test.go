package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func sprintSeamSetup() (gid, admin, member, stranger string, store *MemorySprintStore) {
	gid, admin, member, stranger = uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	store = NewMemorySprintStore()
	store.AddGroup(gid, admin, member)
	return gid, admin, member, stranger, store
}

func TestSprintMembership_Outsider403(t *testing.T) {
	gid, admin, _, stranger, store := sprintSeamSetup()
	svc := NewSprintService(store)

	if _, err := svc.CreateSprintTask(context.Background(), gid, stranger, "", "T", admin, "", "", nil); err != ErrForbidden {
		t.Fatalf("crear extraño debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.ListSprintTasks(context.Background(), gid, stranger, "", "", ""); err != ErrForbidden {
		t.Fatalf("listar extraño debe dar ErrForbidden, got %v", err)
	}
	created, err := svc.CreateSprintTask(context.Background(), gid, admin, "", "T", admin, "", "", nil)
	if err != nil {
		t.Fatalf("crear admin: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	title := "X"
	if _, err := svc.UpdateSprintTask(context.Background(), gid, stranger, tid.String(), &title, nil, nil, nil, nil); err != ErrForbidden {
		t.Fatalf("update extraño debe dar ErrForbidden, got %v", err)
	}
	if _, err := svc.DeleteSprintTask(context.Background(), gid, stranger, tid.String()); err != ErrForbidden {
		t.Fatalf("delete extraño debe dar ErrForbidden, got %v", err)
	}
}

func TestSprintAssignee_DebeSerMiembro(t *testing.T) {
	gid, admin, member, stranger, store := sprintSeamSetup()
	svc := NewSprintService(store)

	if _, err := svc.CreateSprintTask(context.Background(), gid, admin, "", "T", stranger, "", "", nil); err != ErrAssigneeNotMember {
		t.Fatalf("asignado extraño debe dar ErrAssigneeNotMember, got %v", err)
	}
	created, err := svc.CreateSprintTask(context.Background(), gid, member, "", "T", member, "", "", nil)
	if err != nil {
		t.Fatalf("asignado miembro no debe fallar: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	if _, err := svc.UpdateSprintTask(context.Background(), gid, admin, tid.String(), nil, &stranger, nil, nil, nil); err != ErrAssigneeNotMember {
		t.Fatalf("reasignar a extraño debe dar ErrAssigneeNotMember, got %v", err)
	}
}

func TestSprintCrossGroup_404(t *testing.T) {
	gid, admin, _, _, store := sprintSeamSetup()
	other := uuid.NewString()
	store.AddGroup(other, admin)
	svc := NewSprintService(store)

	created, err := svc.CreateSprintTask(context.Background(), gid, admin, "", "T", admin, "", "", nil)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])
	title := "X"
	if _, err := svc.UpdateSprintTask(context.Background(), other, admin, tid.String(), &title, nil, nil, nil, nil); err != ErrSprintTaskNotFound {
		t.Fatalf("tarea de otro grupo debe dar ErrSprintTaskNotFound, got %v", err)
	}
	if _, err := svc.DeleteSprintTask(context.Background(), other, admin, tid.String()); err != ErrSprintTaskNotFound {
		t.Fatalf("delete de otro grupo debe dar ErrSprintTaskNotFound, got %v", err)
	}
}

func TestHoursMembership_Outsider403(t *testing.T) {
	gid, admin, member, stranger, store := sprintSeamSetup()
	svc := NewHoursService(store)
	sts := NewSprintService(store)

	created, err := sts.CreateSprintTask(context.Background(), gid, admin, "", "T", member, "", "", nil)
	if err != nil {
		t.Fatalf("crear: %v", err)
	}
	tid, _ := uuid.FromBytes(created.Task.ID.Bytes[:])

	if _, _, err := svc.LogHours(context.Background(), stranger, tid.String(), "2026-09-25", 2); err != ErrForbidden {
		t.Fatalf("log extraño debe dar ErrForbidden, got %v", err)
	}
	if _, _, err := svc.ListHours(context.Background(), stranger, tid.String(), "", ""); err != ErrForbidden {
		t.Fatalf("list extraño debe dar ErrForbidden, got %v", err)
	}
	entry, createdNew, err := svc.LogHours(context.Background(), member, tid.String(), "2026-09-25", 2)
	if err != nil || !createdNew || !entry.Hours.Valid {
		t.Fatalf("log miembro debe crear: %v %+v", err, entry)
	}
}
