package repository

// Ramas de error SQL no cubiertas por el CRUD feliz: DSN inválidos,
// pool cerrado (todo método propaga error envuelto, nunca nil) y UUID
// con formato inválido (22P02 → error, no NoRows). Opt-in con
// ACADEMIC_TEST_DATABASE_URL salvo TestRepository_NewPool_Bordes, que
// es unitario puro y siempre corre.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func repoStrPtr(s string) *string   { return &s }
func repoIntPtr(i int) *int         { return &i }
func repoF64Ptr(f float64) *float64 { return &f }

func TestRepository_NewPool_Bordes(t *testing.T) {
	ctx := context.Background()
	if _, err := NewPool(ctx, ""); err == nil {
		t.Fatal("dsn vacío debe fallar")
	} else if !strings.Contains(err.Error(), "vacío") {
		t.Fatalf("msg: %v", err)
	}
	if _, err := NewPool(ctx, "://malformed"); err == nil {
		t.Fatal("parse debe fallar")
	}
	if _, err := NewPool(ctx, "postgres://postgres:postgres@127.0.0.1:1/acad_test?sslmode=disable"); err == nil {
		t.Fatal("ping a puerto cerrado debe fallar")
	}
	if url := os.Getenv("ACADEMIC_TEST_DATABASE_URL"); url == "" {
		t.Skip("ACADEMIC_TEST_DATABASE_URL not configured")
	} else if pool, err := NewPool(ctx, url); err != nil {
		t.Fatalf("pool válido: %v", err)
	} else {
		pool.Close()
	}
}

func TestRepository_ClosedPool_Errores(t *testing.T) {
	repoTestPool(t) // schema vivo para este test; el pool cerrado va aparte
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, os.Getenv("ACADEMIC_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	pool.Close() // todo acceso posterior falla con error envuelto

	cur := NewCurriculumRepository(pool)
	if _, err := cur.Create(ctx, repoUserA, "X"); err == nil {
		t.Fatal("cur.Create")
	}
	if _, err := cur.GetByID(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("cur.GetByID")
	}
	if _, err := cur.ListByUser(ctx, repoUserA); err == nil {
		t.Fatal("cur.ListByUser")
	}
	if _, err := cur.UpdateName(ctx, repoUserA, repoUserA, "X"); err == nil {
		t.Fatal("cur.UpdateName")
	}
	if _, err := cur.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("cur.Delete")
	}

	sub := NewSubjectRepository(pool)
	if _, err := sub.Create(ctx, repoUserA, repoUserA, SubjectCreate{Name: "X", Status: "in_progress"}); err == nil {
		t.Fatal("sub.Create")
	}
	if _, err := sub.GetByID(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("sub.GetByID")
	}
	if _, _, err := sub.ListByCurriculum(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("sub.ListByCurriculum")
	}
	if _, err := sub.CountsByCurricula(ctx, repoUserA); err == nil {
		t.Fatal("sub.CountsByCurricula")
	}
	if _, err := sub.Update(ctx, repoUserA, repoUserA, SubjectPatch{Name: repoStrPtr("X")}); err == nil {
		t.Fatal("sub.Update")
	}
	if _, err := sub.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("sub.Delete")
	}

	sch := NewScheduleRepository(pool)
	st, _ := timeParseHM("08:00")
	en, _ := timeParseHM("09:00")
	if _, err := sch.Create(ctx, repoUserA, repoUserA, 1, st, en, nil); err == nil {
		t.Fatal("sch.Create")
	}
	if _, _, err := sch.ListBySubject(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("sch.ListBySubject")
	}
	if _, err := sch.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("sch.Delete")
	}

	gr := NewGradeRepository(pool)
	if _, err := gr.Create(ctx, repoUserA, repoUserA, GradeCreate{Name: "X", Weight: 0.5, Status: "pending"}); err == nil {
		t.Fatal("gr.Create")
	}
	if _, err := gr.GetByID(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("gr.GetByID")
	}
	if _, _, err := gr.ListBySubject(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("gr.ListBySubject")
	}
	if _, err := gr.Update(ctx, repoUserA, repoUserA, GradePatch{Name: repoStrPtr("X")}); err == nil {
		t.Fatal("gr.Update")
	}
	if _, err := gr.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("gr.Delete")
	}
	if _, err := gr.CountFinalExams(ctx, repoUserA, repoUserA, ""); err == nil {
		t.Fatal("gr.CountFinalExams")
	}

	qr := NewRequirementRepository(pool)
	if _, err := qr.Create(ctx, repoUserA, repoUserA, nil, true, ">=", 4.0, "force_exam"); err == nil {
		t.Fatal("qr.Create")
	}
	if _, _, err := qr.ListBySubject(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("qr.ListBySubject")
	}
	if _, err := qr.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("qr.Delete")
	}
	if _, err := qr.GradeSubjectOf(ctx, repoUserA, repoUserA, repoUserA); err == nil {
		t.Fatal("qr.GradeSubjectOf")
	}

	ar := NewAbsenceRepository(pool)
	d := timeDate(2026, 1, 1)
	if _, err := ar.Create(ctx, repoUserA, repoUserA, d, false); err == nil {
		t.Fatal("ar.Create")
	}
	if _, err := ar.ExistsOnDate(ctx, repoUserA, repoUserA, d); err == nil {
		t.Fatal("ar.ExistsOnDate")
	}
	if _, err := ar.GetByID(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("ar.GetByID")
	}
	if _, _, err := ar.ListBySubject(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("ar.ListBySubject")
	}
	if _, err := ar.UpdateJustified(ctx, repoUserA, repoUserA, true); err == nil {
		t.Fatal("ar.UpdateJustified")
	}
	if _, err := ar.Delete(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("ar.Delete")
	}
	if _, _, _, err := ar.CountsBySubject(ctx, repoUserA, repoUserA); err == nil {
		t.Fatal("ar.CountsBySubject")
	}
}

func TestRepository_UUIDInvalido_Errores(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	bad := "not-a-uuid"

	cur := NewCurriculumRepository(pool)
	if _, err := cur.Create(ctx, bad, "X"); err == nil {
		t.Fatal("cur.Create user malo")
	}
	if _, err := cur.GetByID(ctx, bad, repoUserA); err == nil {
		t.Fatal("cur.GetByID")
	}
	if _, err := cur.UpdateName(ctx, bad, repoUserA, "X"); err == nil {
		t.Fatal("cur.UpdateName")
	}
	if _, err := cur.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("cur.Delete")
	}
	if _, err := cur.ListByUser(ctx, bad); err == nil {
		t.Fatal("cur.ListByUser user malo")
	}

	sub := NewSubjectRepository(pool)
	if _, err := sub.Create(ctx, repoUserA, bad, SubjectCreate{Name: "X", Status: "in_progress"}); err == nil {
		t.Fatal("sub.Create cur malo")
	}
	if _, err := sub.GetByID(ctx, bad, repoUserA); err == nil {
		t.Fatal("sub.GetByID")
	}
	if _, _, err := sub.ListByCurriculum(ctx, bad, repoUserA); err == nil {
		t.Fatal("sub.ListByCurriculum")
	}
	if _, err := sub.Update(ctx, bad, repoUserA, SubjectPatch{Name: repoStrPtr("X")}); err == nil {
		t.Fatal("sub.Update")
	}
	if _, err := sub.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("sub.Delete")
	}
	if _, err := sub.CountsByCurricula(ctx, bad); err == nil {
		t.Fatal("sub.CountsByCurricula user malo")
	}

	sch := NewScheduleRepository(pool)
	if _, _, err := sch.ListBySubject(ctx, bad, repoUserA); err == nil {
		t.Fatal("sch.ListBySubject")
	}
	if _, err := sch.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("sch.Delete")
	}

	gr := NewGradeRepository(pool)
	if _, err := gr.Create(ctx, repoUserA, bad, GradeCreate{Name: "X", Weight: 0.5, Status: "pending"}); err == nil {
		t.Fatal("gr.Create materia mala")
	}
	if _, err := gr.GetByID(ctx, bad, repoUserA); err == nil {
		t.Fatal("gr.GetByID")
	}
	if _, _, err := gr.ListBySubject(ctx, bad, repoUserA); err == nil {
		t.Fatal("gr.ListBySubject")
	}
	if _, err := gr.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("gr.Delete")
	}
	if _, err := gr.CountFinalExams(ctx, bad, repoUserA, ""); err == nil {
		t.Fatal("gr.CountFinalExams")
	}

	qr := NewRequirementRepository(pool)
	if _, err := qr.Create(ctx, repoUserA, bad, nil, true, ">=", 4.0, "force_exam"); err == nil {
		t.Fatal("qr.Create materia mala")
	}
	if _, _, err := qr.ListBySubject(ctx, bad, repoUserA); err == nil {
		t.Fatal("qr.ListBySubject")
	}
	if _, err := qr.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("qr.Delete")
	}
	if _, err := qr.GradeSubjectOf(ctx, bad, bad, repoUserA); err == nil {
		t.Fatal("qr.GradeSubjectOf")
	}

	ar := NewAbsenceRepository(pool)
	d := timeDate(2026, 1, 1)
	if _, err := ar.Create(ctx, repoUserA, bad, d, false); err == nil {
		t.Fatal("ar.Create materia mala")
	}
	if _, err := ar.ExistsOnDate(ctx, repoUserA, bad, d); err == nil {
		t.Fatal("ar.ExistsOnDate")
	}
	if _, err := ar.GetByID(ctx, bad, repoUserA); err == nil {
		t.Fatal("ar.GetByID")
	}
	if _, _, err := ar.ListBySubject(ctx, bad, repoUserA); err == nil {
		t.Fatal("ar.ListBySubject")
	}
	if _, err := ar.UpdateJustified(ctx, bad, repoUserA, true); err == nil {
		t.Fatal("ar.UpdateJustified")
	}
	if _, err := ar.Delete(ctx, bad, repoUserA); err == nil {
		t.Fatal("ar.Delete")
	}
	if _, _, _, err := ar.CountsBySubject(ctx, bad, repoUserA); err == nil {
		t.Fatal("ar.CountsBySubject")
	}
}

// Patches completos: cubren todas las ramas set/add de Update que el
// CRUD feliz (patch parcial) no toca.
func TestRepository_Updates_Completos(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	curA := mustCurriculum(t, pool, repoUserA, "A")
	subR := NewSubjectRepository(pool)
	sem, cred, tot, att := 1, 5, 20, 75.5
	dec := 2
	status := "in_progress"
	s, err := subR.Create(ctx, repoUserA, curA, SubjectCreate{
		Name: "S", Semester: &sem, Credits: &cred, TotalClasses: &tot,
		MinAttendancePct: &att, AverageDecimalPlaces: dec, Status: status,
	})
	if err != nil || s == nil {
		t.Fatalf("seed: %v", err)
	}
	newSem, newCred, newTot, newAtt, newDec := 2, 10, 30, 80.0, 1
	newStatus := "approved"
	upd, err := subR.Update(ctx, s.ID, repoUserA, SubjectPatch{
		Name: repoStrPtr("S2"), Semester: &newSem, Credits: &newCred,
		TotalClasses: &newTot, MinAttendancePct: &newAtt,
		AverageDecimalPlaces: &newDec, Status: &newStatus,
	})
	if err != nil || upd == nil || upd.Name != "S2" || *upd.Semester != 2 || *upd.Credits != 10 {
		t.Fatalf("full patch: %+v %v", upd, err)
	}
	upd, err = subR.Update(ctx, s.ID, repoUserA, SubjectPatch{
		ClearSemester: true, ClearCredits: true, ClearTotalClasses: true, ClearAttendance: true,
	})
	if err != nil || upd == nil || upd.Semester != nil || upd.Credits != nil ||
		upd.TotalClasses != nil || upd.MinAttendancePct != nil {
		t.Fatalf("clear patch: %+v %v", upd, err)
	}

	grR := NewGradeRepository(pool)
	score := 60
	topics, evt := "T", "evt-1"
	sched := timeDate(2026, 5, 1)
	notify := true
	g, err := grR.Create(ctx, repoUserA, s.ID, GradeCreate{
		Name: "G", Score: &score, Weight: 0.5, Status: "graded",
		ScheduledDate: &sched, Topics: &topics, NotifyDiscord: notify,
		GoogleCalendarEventID: &evt,
	})
	if err != nil || g == nil {
		t.Fatalf("seed grade: %v", err)
	}
	newScore := 65
	newWeight := 0.7
	newStatusG := "excused"
	newTopics := "T2"
	newEvt := "evt-2"
	newSched := timeDate(2026, 6, 1)
	notifyOff := false
	final := true
	updG, err := grR.Update(ctx, g.ID, repoUserA, GradePatch{
		Name: repoStrPtr("G2"), Score: &newScore, Weight: &newWeight, Status: &newStatusG,
		ScheduledDate: &newSched, Topics: &newTopics, NotifyDiscord: &notifyOff,
		GoogleCalendarEventID: &newEvt, IsFinalExam: &final,
	})
	if err != nil || updG == nil || updG.Name != "G2" || *updG.Score != 65 || !updG.IsFinalExam {
		t.Fatalf("full grade patch: %+v %v", updG, err)
	}
	updG, err = grR.Update(ctx, g.ID, repoUserA, GradePatch{
		ClearScore: true, ClearScheduledAt: true, ClearTopics: true, ClearCalendarEvt: true,
	})
	if err != nil || updG == nil || updG.Score != nil || updG.ScheduledDate != nil ||
		updG.Topics != nil || updG.GoogleCalendarEventID != nil {
		t.Fatalf("clear grade patch: %+v %v", updG, err)
	}

	// Requirement con fuente (rama source non-nil).
	qr := NewRequirementRepository(pool)
	req, err := qr.Create(ctx, repoUserA, s.ID, &g.ID, false, ">=", 4.0, "force_fail")
	if err != nil || req == nil || req.SourceGradeID == nil {
		t.Fatalf("requirement con fuente: %+v %v", req, err)
	}
}
