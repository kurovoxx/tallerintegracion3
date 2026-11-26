package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
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

	if _, err := svc.CreateMeeting(ctx, gid, member, "Daily", nil, nil, when, nil, nil); err != ErrForbidden {
		t.Fatalf("miembro común no puede agendar: got %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, gid, stranger, "Daily", nil, nil, when, nil, nil); err != ErrForbidden {
		t.Fatalf("extraño no puede agendar: got %v", err)
	}
	if _, err := svc.CreateMeeting(ctx, gid, admin, "Daily", nil, nil, when, nil, nil); err != nil {
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

// Permisos delegados: un miembro con can_create_meetings agenda y uno con
// can_edit_kanban modifica el sprint; el permiso de uno no vale para el otro.
func TestMeetingCreate_MiembroConPermisoDelegado(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := NewMemoryMeetingStore()
	store.AddGroup(gid, admin, member)
	svc := NewMeetingService(store, NoopMeetingCreatedNotifier{})
	ctx := context.Background()
	when := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	store.SetPermissions(gid, member, model.MemberPermissions{CanEditKanban: true})
	if _, err := svc.CreateMeeting(ctx, gid, member, "Daily", nil, nil, when, nil, nil); err != ErrForbidden {
		t.Fatalf("can_edit_kanban no habilita reuniones: got %v", err)
	}
	store.SetPermissions(gid, member, model.MemberPermissions{CanCreateMeetings: true})
	if _, err := svc.CreateMeeting(ctx, gid, member, "Daily", nil, nil, when, nil, nil); err != nil {
		t.Fatalf("miembro con can_create_meetings agenda: %v", err)
	}
}

func TestSprint_MiembroConPermisoDelegado(t *testing.T) {
	gid, admin, member, _, store := sprintSeamSetup()
	svc := NewSprintService(store)
	ctx := context.Background()

	store.SetPermissions(gid, member, model.MemberPermissions{CanCreateMeetings: true})
	if _, err := svc.CreateSprintTask(ctx, gid, member, "", "T", member, "", "", nil); err != ErrForbidden {
		t.Fatalf("can_create_meetings no habilita el kanban: got %v", err)
	}
	store.SetPermissions(gid, member, model.MemberPermissions{CanEditKanban: true})
	created, err := svc.CreateSprintTask(ctx, gid, member, "", "T", member, "", "", nil)
	if err != nil {
		t.Fatalf("miembro con can_edit_kanban crea: %v", err)
	}
	title := "Nuevo"
	if _, err := svc.UpdateSprintTask(ctx, gid, member, uuidStr(created.Task.ID), &title, nil, nil, nil, nil); err != nil {
		t.Fatalf("edita: %v", err)
	}
	if _, err := svc.DeleteSprintTask(ctx, gid, member, uuidStr(created.Task.ID)); err != nil {
		t.Fatalf("borra: %v", err)
	}
	_ = admin
}

func TestGroupService_SetMemberPermissions(t *testing.T) {
	ctx := context.Background()
	svc := NewGroupService(NewMemoryGroupStore(), nil)
	admin, member, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	g, err := svc.Create(ctx, admin, "G", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{member, other} {
		if err := svc.JoinGroup(ctx, g.ID, g.InviteToken, u); err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	upd := model.MemberPermissionsUpdate{CanShareNotes: &yes}

	if _, err := svc.SetMemberPermissions(ctx, g.ID, member, other, upd); err != ErrForbidden {
		t.Fatalf("un miembro no delega permisos: got %v", err)
	}
	if _, err := svc.SetMemberPermissions(ctx, g.ID, admin, admin, upd); err != ErrCannotModifySelf {
		t.Fatalf("el admin no se auto-asigna: got %v", err)
	}
	if _, err := svc.SetMemberPermissions(ctx, g.ID, admin, member, model.MemberPermissionsUpdate{}); err == nil {
		t.Fatal("sin ningún permiso debe fallar")
	}
	got, err := svc.SetMemberPermissions(ctx, g.ID, admin, member, upd)
	if err != nil || !got.CanShareNotes || got.CanCreateMeetings || got.CanEditKanban {
		t.Fatalf("got %+v err=%v", got, err)
	}
	// Parcial: no pisa lo ya concedido.
	got, err = svc.SetMemberPermissions(ctx, g.ID, admin, member, model.MemberPermissionsUpdate{CanEditKanban: &yes})
	if err != nil || !got.CanShareNotes || !got.CanEditKanban {
		t.Fatalf("parcial: got %+v err=%v", got, err)
	}
	members, _ := svc.ListMembers(ctx, g.ID, admin)
	for _, m := range members {
		if m.UserID == member && (!m.CanShareNotes || !m.CanEditKanban) {
			t.Fatalf("ListMembers debe exponer permisos: %+v", m)
		}
	}
}

// Permisos "para todo el grupo": solo un admin los cambia, cualquier miembro
// los consulta, las actualizaciones parciales no pisan lo ya habilitado y un
// no miembro no puede ni verlos.
func TestGroupService_MemberDefaults(t *testing.T) {
	ctx := context.Background()
	svc := NewGroupService(NewMemoryGroupStore(), nil)
	admin, member, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	g, err := svc.Create(ctx, admin, "G", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.JoinGroup(ctx, g.ID, g.InviteToken, member); err != nil {
		t.Fatal(err)
	}
	yes, no := true, false

	if got, err := svc.GetMemberDefaults(ctx, g.ID, member); err != nil || got != (model.MemberPermissions{}) {
		t.Fatalf("por defecto todo apagado: got %+v err=%v", got, err)
	}
	if _, err := svc.SetMemberDefaults(ctx, g.ID, member, model.MemberPermissionsUpdate{CanCreateMeetings: &yes}); err != ErrForbidden {
		t.Fatalf("un miembro no cambia permisos del grupo: got %v", err)
	}
	if _, err := svc.SetMemberDefaults(ctx, g.ID, admin, model.MemberPermissionsUpdate{}); err == nil {
		t.Fatal("sin ningún permiso debe fallar")
	}
	got, err := svc.SetMemberDefaults(ctx, g.ID, admin, model.MemberPermissionsUpdate{CanCreateMeetings: &yes})
	if err != nil || !got.CanCreateMeetings || got.CanShareNotes || got.CanEditKanban {
		t.Fatalf("got %+v err=%v", got, err)
	}
	got, err = svc.SetMemberDefaults(ctx, g.ID, admin, model.MemberPermissionsUpdate{CanShareNotes: &yes})
	if err != nil || !got.CanCreateMeetings || !got.CanShareNotes {
		t.Fatalf("parcial no debe pisar lo anterior: got %+v err=%v", got, err)
	}
	got, err = svc.SetMemberDefaults(ctx, g.ID, admin, model.MemberPermissionsUpdate{CanCreateMeetings: &no})
	if err != nil || got.CanCreateMeetings || !got.CanShareNotes {
		t.Fatalf("retirar uno: got %+v err=%v", got, err)
	}
	if got, err := svc.GetMemberDefaults(ctx, g.ID, member); err != nil || !got.CanShareNotes {
		t.Fatalf("el miembro ve el estado: got %+v err=%v", got, err)
	}
	if _, err := svc.GetMemberDefaults(ctx, g.ID, outsider); err != ErrForbidden {
		t.Fatalf("un no miembro no consulta: got %v", err)
	}
}

// Un admin puede quitarle a un miembro concreto un permiso que el grupo tiene
// habilitado para todos, sin apagar el switch del grupo.
func TestGroupService_SetMemberPermissions_RevocaPermisoDeGrupo(t *testing.T) {
	ctx := context.Background()
	svc := NewGroupService(NewMemoryGroupStore(), nil)
	admin, member := uuid.NewString(), uuid.NewString()
	g, err := svc.Create(ctx, admin, "G", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.JoinGroup(ctx, g.ID, g.InviteToken, member); err != nil {
		t.Fatal(err)
	}
	yes, no := true, false
	if _, err := svc.SetMemberDefaults(ctx, g.ID, admin, model.MemberPermissionsUpdate{CanShareNotes: &yes}); err != nil {
		t.Fatal(err)
	}

	// Retirar el permiso (efectivo) solo a este miembro queda marcado como denegado.
	got, err := svc.SetMemberPermissions(ctx, g.ID, admin, member, model.MemberPermissionsUpdate{CanShareNotes: &no})
	if err != nil || got.CanShareNotes {
		t.Fatalf("el permiso sigue activo: got %+v err=%v", got, err)
	}
	members, err := svc.ListMembers(ctx, g.ID, admin)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if m.UserID == member && !m.DenyShareNotes {
			t.Fatalf("ListMembers debe exponer el retiro individual: %+v", m)
		}
	}

	// Volver a dárselo lo quita de la lista de denegados.
	got, err = svc.SetMemberPermissions(ctx, g.ID, admin, member, model.MemberPermissionsUpdate{CanShareNotes: &yes})
	if err != nil || !got.CanShareNotes {
		t.Fatalf("debe poder devolverse el permiso: got %+v err=%v", got, err)
	}
	members, _ = svc.ListMembers(ctx, g.ID, admin)
	for _, m := range members {
		if m.UserID == member && m.DenyShareNotes {
			t.Fatalf("el retiro individual debe limpiarse: %+v", m)
		}
	}
}
