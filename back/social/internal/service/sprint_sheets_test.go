package service

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func strptr(s string) *string { return &s }
func TestSprintSheetsLifecycleAndIsolation(t *testing.T) {
	gid, admin, member, stranger, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()
	patch := SheetPatch{Name: strptr("Sprint 1"), PeriodStart: strptr("2026-09-30"), PeriodEnd: strptr("2026-10-15")}
	if _, err := svc.CreateSprintSheet(ctx, gid, stranger, patch); err != ErrForbidden {
		t.Fatal(err)
	}
	a, err := svc.CreateSprintSheet(ctx, gid, member, patch)
	if err != nil {
		t.Fatal(err)
	}
	patch.Name = strptr("Sprint 2")
	b, err := svc.CreateSprintSheet(ctx, gid, admin, patch)
	if err != nil {
		t.Fatal(err)
	}
	aid, bid := uuidStr(a.ID), uuidStr(b.ID)
	task, err := svc.CreateSprintTask(ctx, gid, admin, aid, "Solo A", member, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := svc.ListSprintTasks(ctx, gid, admin, "", "", bid)
	if err != nil || len(tasks) != 0 {
		t.Fatal("leaked tasks", err)
	}
	if _, err := svc.UpdateSprintSheet(ctx, gid, admin, aid, SheetPatch{Name: strptr("Renombrado"), PeriodEnd: strptr("2026-09-29")}); err != ErrInvalidSheet {
		t.Fatal(err)
	}
	for _, value := range []string{"", "2026-02-30", "not-date"} {
		bad := patch
		bad.PeriodStart = &value
		if _, err := svc.CreateSprintSheet(ctx, gid, admin, bad); err != ErrInvalidSheet {
			t.Fatal("invalid date accepted", err)
		}
	}
	hours := NewHoursService(store)
	tid := uuidStr(task.Task.ID)
	if _, _, err := hours.LogHours(ctx, admin, tid, "2026-10-12", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateSprintSheet(ctx, gid, admin, aid, SheetPatch{Name: strptr("Renombrado"), PeriodEnd: strptr("2026-10-01")}); err != nil {
		t.Fatal(err)
	}
	entries, total, err := hours.ListHours(ctx, admin, tid, "", "")
	if err != nil || len(entries) != 1 || total != 3 {
		t.Fatal("hours lost", err)
	}
	other := uuid.NewString()
	store.AddGroup(other, admin)
	if _, err := svc.UpdateSprintSheet(ctx, other, admin, aid, SheetPatch{Name: strptr("wrong")}); err != ErrSheetNotFound {
		t.Fatal(err)
	}
	if _, err := svc.CreateSprintTask(ctx, other, admin, aid, "wrong", admin, "", "", nil); err != ErrSheetNotFound {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSprintTask(ctx, other, admin, tid); err != ErrSprintTaskNotFound {
		t.Fatal(err)
	}
	list, err := svc.ListSprintSheets(ctx, gid, admin)
	if err != nil || len(list) != 2 {
		t.Fatal(err)
	}
	if _, err := svc.ListSprintSheets(ctx, gid, stranger); err != ErrForbidden {
		t.Fatal(err)
	}
}
