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

func TestLegacySprintAcceptsDatesWithoutReplacingSheet(t *testing.T) {
	gid, admin, member, _, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()
	// Legacy default creation has NULL period_start/period_end.
	task, err := svc.CreateSprintTask(ctx, gid, admin, "", "Legacy task", member, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	sheetID := uuidStr(task.Task.SheetID)
	hours := NewHoursService(store)
	if _, _, err := hours.LogHours(ctx, admin, uuidStr(task.Task.ID), "2026-10-12", 3); err != nil {
		t.Fatal(err)
	}
	for _, end := range []string{"2026-10-01", "2026-10-05", "2026-11-04"} {
		sheet, err := svc.UpdateSprintSheet(ctx, gid, admin, sheetID, SheetPatch{Name: strptr("Sprint 1"), PeriodStart: strptr("2026-10-01"), PeriodEnd: &end})
		if err != nil || uuidStr(sheet.ID) != sheetID || !sheet.PeriodStart.Valid || !sheet.PeriodEnd.Valid {
			t.Fatalf("legacy dates: %v", err)
		}
		tasks, err := svc.ListSprintTasks(ctx, gid, admin, "", "", sheetID)
		if err != nil || len(tasks) != 1 {
			t.Fatal("tasks lost", err)
		}
	}
	_, total, err := hours.ListHours(ctx, admin, uuidStr(task.Task.ID), "", "")
	if err != nil || total != 3 {
		t.Fatal("hours lost", err)
	}
}

func TestDeleteSprintSheet(t *testing.T) {
	gid, admin, member, stranger, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()
	patch := SheetPatch{Name: strptr("Sprint 1"), PeriodStart: strptr("2026-09-30"), PeriodEnd: strptr("2026-10-15")}
	// 9. No borrar el último sprint (backend también lo protege).
	only, err := svc.CreateSprintSheet(ctx, gid, admin, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSprintSheet(ctx, gid, admin, uuidStr(only.ID)); err != ErrLastSheet {
		t.Fatalf("último sprint debe protegerse, got %v", err)
	}
	// 8. Permisos: extraño no puede borrar (misma política que crear/editar).
	if _, err := svc.DeleteSprintSheet(ctx, gid, stranger, uuidStr(only.ID)); err != ErrForbidden {
		t.Fatalf("stranger debe ser forbidden, got %v", err)
	}
	// 7. No borrar sheet de otro grupo.
	other := uuid.NewString()
	store.AddGroup(other, admin)
	if _, err := svc.DeleteSprintSheet(ctx, other, admin, uuidStr(only.ID)); err != ErrSheetNotFound {
		t.Fatalf("otro grupo debe ser not_found, got %v", err)
	}
	patch.Name = strptr("Sprint 2")
	second, err := svc.CreateSprintSheet(ctx, gid, member, patch)
	if err != nil {
		t.Fatal(err)
	}
	secondID := uuidStr(second.ID)
	// 2. Sprint con tareas + horas.
	task, err := svc.CreateSprintTask(ctx, gid, admin, secondID, "Tarea B", member, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	tid := uuidStr(task.Task.ID)
	hours := NewHoursService(store)
	if _, _, err := hours.LogHours(ctx, admin, tid, "2026-10-12", 2); err != nil {
		t.Fatal(err)
	}
	// 11. Sprint 1 se puede borrar habiendo otro (sin trato especial).
	deleted, err := svc.DeleteSprintSheet(ctx, gid, admin, uuidStr(only.ID))
	if err != nil || uuidStr(deleted.ID) != uuidStr(only.ID) {
		t.Fatalf("borrar Sprint 1: %v", err)
	}
	// 5-6. El otro sprint y sus tareas permanecen.
	list, err := svc.ListSprintSheets(ctx, gid, admin)
	if err != nil || len(list) != 1 || uuidStr(list[0].ID) != secondID {
		t.Fatal("otro sprint perdido", err)
	}
	kept, err := svc.ListSprintTasks(ctx, gid, admin, "", "", secondID)
	if err != nil || len(kept) != 1 {
		t.Fatal("tareas del otro perdidas", err)
	}
	// 1. Borrar sprint vacío (con otro presente para no violar la regla).
	patch.Name = strptr("Sprint 3")
	empty, err := svc.CreateSprintSheet(ctx, gid, admin, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSprintSheet(ctx, gid, admin, uuidStr(empty.ID)); err != nil {
		t.Fatal(err)
	}
	// 2-4. Borrar el sprint con tareas: tareas y hours desaparecen por cascada.
	// Se crea un cuarto sprint para que el borrado no deje al grupo con uno solo.
	patch.Name = strptr("Sprint 4")
	fourth, err := svc.CreateSprintSheet(ctx, gid, admin, patch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.DeleteSprintSheet(ctx, gid, admin, secondID); err != nil {
		t.Fatal(err)
	}
	// 3. Tareas del sprint desaparecen (quedan 0 en el grupo).
	all, err := svc.ListSprintTasks(ctx, gid, admin, "", "", "")
	if err != nil || len(all) != 0 {
		t.Fatalf("tareas huérfanas: %v %v", len(all), err)
	}
	// 4. Daily hours asociadas desaparecen.
	if _, total, err := hours.ListHours(ctx, admin, tid, "", ""); err == nil && total != 0 {
		t.Fatalf("hours huérfanas: total=%v err=%v", total, err)
	}
	// 5. El cuarto sprint permanece como único.
	list, err = svc.ListSprintSheets(ctx, gid, admin)
	if err != nil || len(list) != 1 || uuidStr(list[0].ID) != uuidStr(fourth.ID) {
		t.Fatal("sprint restante perdido", err)
	}
}
