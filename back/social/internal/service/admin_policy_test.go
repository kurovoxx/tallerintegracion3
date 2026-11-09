package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository/sqlc"
)

// Política: solo los admins del grupo agendan reuniones y modifican el
// sprint/kanban. Los miembros comunes siguen pudiendo leer.

func TestMeetingCreate_SoloAdmins(t *testing.T) {
	gid, admin, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	// El notifier corre en una goroutine best-effort que lee bestEffortTimeout:
	// se espera su fin para no pisar tests que modifican esa variable global (-race).
	done := make(chan struct{})
	svc := NewMeetingService(store, notifierFunc(func() { close(done) }))
	ctx := context.Background()
	when := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	if _, err := svc.CreateMeeting(ctx, gid, member, "Daily", nil, when, nil, nil); err != ErrForbidden {
		t.Fatalf("miembro común no puede agendar: got %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, gid, stranger, "Daily", nil, when, nil, nil); err != ErrForbidden {
		t.Fatalf("extraño no puede agendar: got %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, gid, admin, "Daily", nil, when, nil, nil); err != nil {
		t.Fatalf("admin sí puede agendar: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("el notifier best-effort no terminó")
	}
	// Los miembros siguen viendo las reuniones próximas.
	got, err := svc.ListUpcomingMeetings(ctx, gid, member, time.Now())
	if err != nil || len(got) != 1 {
		t.Fatalf("miembro debe poder listar: %v len=%d", err, len(got))
	}
}

func TestSprint_SoloAdminsModifican(t *testing.T) {
	gid, admin, member, _, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()

	if _, err := svc.CreateSprintTask(ctx, gid, member, "", "T", member, "", "", nil); err != ErrForbidden {
		t.Fatalf("miembro no crea tareas: got %v", err)
	}
	created, err := svc.CreateSprintTask(ctx, gid, admin, "", "T", member, "", "", nil)
	if err != nil {
		t.Fatalf("admin crea: %v", err)
	}
	tid := uuidStr(created.Task.ID)
	title := "Nuevo"
	if _, err := svc.UpdateSprintTask(ctx, gid, member, tid, &title, nil, nil, nil, nil); err != ErrForbidden {
		t.Fatalf("miembro no edita: got %v", err)
	}
	if _, err := svc.UpdateSprintTask(ctx, gid, admin, tid, &title, nil, nil, nil, nil); err != nil {
		t.Fatalf("admin edita: %v", err)
	}
	if _, err := svc.DeleteSprintTask(ctx, gid, member, tid); err != ErrForbidden {
		t.Fatalf("miembro no borra: got %v", err)
	}
	// Lectura permitida a miembros.
	tasks, err := svc.ListSprintTasks(ctx, gid, member, "", "", "")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("miembro lista: %v len=%d", err, len(tasks))
	}
	// Hojas de sprint.
	patch := SheetPatch{Name: strptr("Sprint X"), PeriodStart: strptr("2026-09-30"), PeriodEnd: strptr("2026-10-15")}
	if _, err := svc.CreateSprintSheet(ctx, gid, member, patch); err != ErrForbidden {
		t.Fatalf("miembro no crea hoja: got %v", err)
	}
	sheet, err := svc.CreateSprintSheet(ctx, gid, admin, patch)
	if err != nil {
		t.Fatal(err)
	}
	sid := uuidStr(sheet.ID)
	if _, err := svc.UpdateSprintSheet(ctx, gid, member, sid, SheetPatch{Name: strptr("Otro")}); err != ErrForbidden {
		t.Fatalf("miembro no edita hoja: got %v", err)
	}
	if _, err := svc.DeleteSprintSheet(ctx, gid, member, sid); err != ErrForbidden {
		t.Fatalf("miembro no borra hoja: got %v", err)
	}
	if _, err := svc.ListSprintSheets(ctx, gid, member); err != nil {
		t.Fatalf("miembro lista hojas: %v", err)
	}
}

func TestSprintTimeRemaining(t *testing.T) {
	gid, admin, member, stranger, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()
	mk := func(name, start, end string) {
		if _, err := svc.CreateSprintSheet(ctx, gid, admin, SheetPatch{Name: strptr(name), PeriodStart: strptr(start), PeriodEnd: strptr(end)}); err != nil {
			t.Fatal(err)
		}
	}
	mk("Activo", "2026-11-01", "2026-11-11") // quedan 2 → notifica
	mk("Largo", "2026-11-01", "2026-11-30")  // quedan 21 → no notifica
	mk("Hoy", "2026-11-01", "2026-11-09")    // termina hoy
	mk("Viejo", "2026-10-01", "2026-10-15")  // terminó
	mk("Futuro", "2026-12-01", "2026-12-15") // no empezó
	now := time.Date(2026, 11, 9, 15, 0, 0, 0, time.UTC)

	got, err := svc.SprintTimeRemaining(ctx, gid, member, now)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SprintRemaining{}
	for _, r := range got {
		by[r.Sheet.Name] = r
	}
	check := func(name, state string, days int, notify bool) {
		r, ok := by[name]
		if !ok || r.State != state || r.DaysRemaining != days || r.Notify != notify {
			t.Fatalf("%s: got %+v", name, r)
		}
	}
	check("Activo", "active", 2, true)
	check("Largo", "active", 21, false)
	check("Hoy", "active", 0, true)
	check("Viejo", "ended", -25, false)
	check("Futuro", "not_started", 36, false)
	if by["Activo"].Message != "Al sprint \"Activo\" le quedan 2 días." {
		t.Fatalf("mensaje: %q", by["Activo"].Message)
	}
	if _, err := svc.SprintTimeRemaining(ctx, gid, stranger, now); err != ErrForbidden {
		t.Fatalf("extraño: got %v", err)
	}
}

// notifierFunc adapta una función a MeetingCreatedNotifier.
type notifierFunc func()

func (f notifierFunc) OnMeetingCreated(_ context.Context, _ sqlc.SocialMeeting) { f() }
