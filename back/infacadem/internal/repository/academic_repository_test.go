package repository

// Tests propios de repository contra Postgres real (SQL + ownership + cascadas).
//
// Opt-in con ACADEMIC_TEST_DATABASE_URL (skip limpio sin env, patrón de los
// integration de service/). BD dedicada y vacía, nunca la de desarrollo:
//
//   docker run -d --name academic-test-db -e POSTGRES_PASSWORD=postgres \
//     -e POSTGRES_USER=postgres -e POSTGRES_DB=acad_test \
//     -p 5435:5432 postgres:16-alpine
//   export ACADEMIC_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5435/acad_test?sslmode=disable'
//   go test ./internal/repository/ -count=1 -v
//   docker rm -f academic-test-db
//
// Crea SCHEMA academic + tablas (subset de docker/postgres/init.sql) y limpia
// con DROP SCHEMA. Nota: service y handler/grpc usan la misma BD y schema;
// con la env fijada, correr el suite completo con -p 1 (misma solución que
// back/auth) para no crear el schema en paralelo con 42P06.

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var errBadTime = errors.New("bad time")

func timeParseHM(s string) (time.Time, error) {
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errBadTime
}

func timeDate(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func boolPtr(b bool) *bool { return &b }

const (
	repoUserA = "11111111-1111-4111-8111-111111111111"
	repoUserB = "22222222-2222-4222-8222-222222222222"
)

const repoSchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE academic.curricula (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL,
    name varchar(200) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE academic.subjects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    curriculum_id uuid NOT NULL REFERENCES academic.curricula(id) ON DELETE CASCADE,
    name varchar(200) NOT NULL,
    semester int,
    credits int,
    total_classes int,
    min_attendance_pct decimal(5,2),
    average_decimal_places smallint NOT NULL DEFAULT 2 CHECK (average_decimal_places BETWEEN 0 AND 2),
    status varchar(20) NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'approved', 'failed', 'pending'))
);
CREATE TABLE academic.schedules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    day_of_week int NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time time NOT NULL,
    CHECK (end_time > start_time)
);
CREATE TABLE academic.grades (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    parent_grade_id uuid REFERENCES academic.grades(id) ON DELETE CASCADE,
    is_final_exam boolean NOT NULL DEFAULT false,
    status varchar(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('graded', 'pending', 'excused', 'missing')),
    name varchar(200) NOT NULL,
    score smallint CHECK (score BETWEEN 10 AND 70),
    weight decimal(4,3) NOT NULL,
    scheduled_date date,
    topics text,
    notify_discord boolean NOT NULL DEFAULT false,
    google_calendar_event_id varchar(255),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE academic.grade_requirements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    source_grade_id uuid REFERENCES academic.grades(id) ON DELETE CASCADE,
    checks_final_average boolean NOT NULL DEFAULT false,
    comparator varchar(2) NOT NULL CHECK (comparator IN ('>', '>=', '<', '<=', '=')),
    threshold_value decimal(5,2) NOT NULL,
    action varchar(20) NOT NULL CHECK (action IN ('force_exam', 'force_fail', 'force_pass')),
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (checks_final_average = true OR source_grade_id IS NOT NULL)
);
CREATE TABLE academic.absences (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    date date NOT NULL,
    justified boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
`

func repoTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("ACADEMIC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ACADEMIC_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA academic`); err != nil {
		pool.Close()
		t.Fatalf("requires an empty test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA academic CASCADE`)
		pool.Close()
	})
	if _, err := pool.Exec(ctx, repoSchemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	return pool
}

func mustCurriculum(t *testing.T, pool *pgxpool.Pool, user, name string) string {
	t.Helper()
	r := NewCurriculumRepository(pool)
	c, err := r.Create(context.Background(), user, name)
	if err != nil || c.ID == "" {
		t.Fatalf("seed curriculum: %v", err)
	}
	return c.ID
}

func mustSubject(t *testing.T, pool *pgxpool.Pool, user, curID, name string) string {
	t.Helper()
	r := NewSubjectRepository(pool)
	s, err := r.Create(context.Background(), user, curID, SubjectCreate{Name: name, AverageDecimalPlaces: 2, Status: "in_progress"})
	if err != nil || s == nil {
		t.Fatalf("seed subject: %v", err)
	}
	return s.ID
}

func TestRepository_Curricula(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	r := NewCurriculumRepository(pool)

	c, err := r.Create(ctx, repoUserA, "Malla")
	if err != nil || c.UserID != repoUserA || c.Name != "Malla" {
		t.Fatalf("create: %+v %v", c, err)
	}
	if got, _ := r.GetByID(ctx, c.ID, repoUserA); got == nil || got.Name != "Malla" {
		t.Fatalf("get: %+v", got)
	}
	if got, _ := r.GetByID(ctx, c.ID, repoUserB); got != nil {
		t.Fatalf("ajena invisible: %+v", got)
	}
	if got, _ := r.GetByID(ctx, "11111111-1111-4111-8111-111111111111", repoUserA); got != nil {
		t.Fatal("inexistente nil")
	}
	list, err := r.ListByUser(ctx, repoUserA)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %d", err, len(list))
	}
	if listB, _ := r.ListByUser(ctx, repoUserB); len(listB) != 0 {
		t.Fatal("ajenas fuera")
	}
	upd, err := r.UpdateName(ctx, c.ID, repoUserA, "Malla 2")
	if err != nil || upd == nil || upd.Name != "Malla 2" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if upd, _ := r.UpdateName(ctx, c.ID, repoUserB, "Hack"); upd != nil {
		t.Fatal("update ajeno nil")
	}
	deleted, err := r.Delete(ctx, c.ID, repoUserB)
	if err != nil || deleted {
		t.Fatalf("delete ajeno false: %v %v", deleted, err)
	}
	deleted, err = r.Delete(ctx, c.ID, repoUserA)
	if err != nil || !deleted {
		t.Fatalf("delete: %v %v", deleted, err)
	}
	if got, _ := r.GetByID(ctx, c.ID, repoUserA); got != nil {
		t.Fatal("tras delete nil")
	}
}

func TestRepository_Subjects(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	r := NewSubjectRepository(pool)
	curA := mustCurriculum(t, pool, repoUserA, "A")
	curB := mustCurriculum(t, pool, repoUserB, "B")

	sem, cred := 3, 10
	s, err := r.Create(ctx, repoUserA, curA, SubjectCreate{Name: "Álgebra", Semester: &sem, Credits: &cred, AverageDecimalPlaces: 1, Status: "approved"})
	if err != nil || s == nil || s.Semester == nil || *s.Semester != 3 || s.Status != "approved" {
		t.Fatalf("create: %+v %v", s, err)
	}
	if s2, _ := r.Create(ctx, repoUserB, curA, SubjectCreate{Name: "X", Status: "in_progress"}); s2 != nil {
		t.Fatal("malla ajena nil")
	}
	if got, _ := r.GetByID(ctx, s.ID, repoUserA); got == nil {
		t.Fatal("get propia")
	}
	if got, _ := r.GetByID(ctx, s.ID, repoUserB); got != nil {
		t.Fatal("ajena nil")
	}
	list, owned, err := r.ListByCurriculum(ctx, curA, repoUserA)
	if err != nil || !owned || len(list) != 1 {
		t.Fatalf("list: %v %v %d", list, owned, err)
	}
	if _, owned, _ := r.ListByCurriculum(ctx, curB, repoUserA); owned {
		t.Fatal("malla ajena owned=false")
	}
	if _, owned, _ := r.ListByCurriculum(ctx, "11111111-1111-4111-8111-111111111111", repoUserA); owned {
		t.Fatal("inexistente owned=false")
	}
	newName := "Álgebra II"
	upd, err := r.Update(ctx, s.ID, repoUserA, SubjectPatch{Name: &newName})
	if err != nil || upd == nil || upd.Name != "Álgebra II" || *upd.Semester != 3 {
		t.Fatalf("patch parcial conserva resto: %+v %v", upd, err)
	}
	if _, err := r.Update(ctx, s.ID, repoUserA, SubjectPatch{}); err == nil {
		t.Fatal("patch vacío debe fallar")
	} else if err.Error() != "no fields to update" {
		t.Fatalf("msg: %v", err)
	}
	if upd, _ := r.Update(ctx, s.ID, repoUserB, SubjectPatch{Name: &newName}); upd != nil {
		t.Fatal("update ajeno nil")
	}
	deleted, err := r.Delete(ctx, s.ID, repoUserB)
	if err != nil || deleted {
		t.Fatalf("delete ajeno: %v", err)
	}
	deleted, err = r.Delete(ctx, s.ID, repoUserA)
	if err != nil || !deleted {
		t.Fatalf("delete: %v", err)
	}
}

func TestRepository_Schedules(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	r := NewScheduleRepository(pool)
	curA := mustCurriculum(t, pool, repoUserA, "A")
	mat := mustSubject(t, pool, repoUserA, curA, "S")

	start, _ := timeParseHM("08:00")
	end, _ := timeParseHM("09:30")
	h, err := r.Create(ctx, repoUserA, mat, 1, start, end)
	if err != nil || h == nil || h.DayOfWeek != 1 {
		t.Fatalf("create: %+v %v", h, err)
	}
	if h2, _ := r.Create(ctx, repoUserB, mat, 1, start, end); h2 != nil {
		t.Fatal("materia ajena nil")
	}
	list, owned, err := r.ListBySubject(ctx, mat, repoUserA)
	if err != nil || !owned || len(list) != 1 {
		t.Fatalf("list: %v %v %d", list, owned, err)
	}
	if _, owned, _ := r.ListBySubject(ctx, mat, repoUserB); owned {
		t.Fatal("ajena owned=false")
	}
	deleted, err := r.Delete(ctx, h.ID, repoUserB)
	if err != nil || deleted {
		t.Fatalf("delete ajeno: %v", err)
	}
	deleted, err = r.Delete(ctx, h.ID, repoUserA)
	if err != nil || !deleted {
		t.Fatalf("delete: %v", err)
	}
}

func TestRepository_Grades_Requirements(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	gr := NewGradeRepository(pool)
	qr := NewRequirementRepository(pool)
	curA := mustCurriculum(t, pool, repoUserA, "A")
	mat := mustSubject(t, pool, repoUserA, curA, "S")

	score := 60
	g, err := gr.Create(ctx, repoUserA, mat, GradeCreate{Name: "P1", Score: &score, Weight: 0.5, Status: "graded"})
	if err != nil || g == nil || g.Score == nil || *g.Score != 60 {
		t.Fatalf("create: %+v %v", g, err)
	}
	if g2, _ := gr.Create(ctx, repoUserB, mat, GradeCreate{Name: "X", Weight: 0.5, Status: "pending"}); g2 != nil {
		t.Fatal("materia ajena nil")
	}
	child, err := gr.Create(ctx, repoUserA, mat, GradeCreate{Name: "H", Weight: 1, Status: "pending"})
	if err != nil || child == nil {
		t.Fatal(err)
	}
	// Hija vía parent: UPDATE de parent no existe; crear con parent directo
	// requiere GradeCreate.ParentGradeID (se prueba abajo con otro insert).
	_ = child
	padre, err := gr.Create(ctx, repoUserA, mat, GradeCreate{Name: "Padre", Weight: 0.5, Status: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	hija, err := gr.Create(ctx, repoUserA, mat, GradeCreate{ParentGradeID: &padre.ID, Name: "Hija", Weight: 1, Status: "pending"})
	if err != nil || hija == nil || hija.ParentGradeID == nil {
		t.Fatalf("hija: %+v %v", hija, err)
	}
	if got, _ := gr.GetByID(ctx, g.ID, repoUserB); got != nil {
		t.Fatal("get ajeno nil")
	}
	list, owned, err := gr.ListBySubject(ctx, mat, repoUserA)
	if err != nil || !owned || len(list) != 4 {
		t.Fatalf("list: %v %v %d", list, owned, err)
	}
	if _, owned, _ := gr.ListBySubject(ctx, mat, repoUserB); owned {
		t.Fatal("ajena owned=false")
	}
	newName := "P1b"
	upd, err := gr.Update(ctx, g.ID, repoUserA, GradePatch{Name: &newName})
	if err != nil || upd == nil || upd.Name != "P1b" {
		t.Fatalf("update: %+v %v", upd, err)
	}
	if _, err := gr.Update(ctx, g.ID, repoUserA, GradePatch{}); err == nil {
		t.Fatal("patch vacío falla")
	}
	if upd, _ := gr.Update(ctx, g.ID, repoUserB, GradePatch{Name: &newName}); upd != nil {
		t.Fatal("update ajeno nil")
	}
	if n, err := gr.CountFinalExams(ctx, mat, repoUserA, ""); err != nil || n != 0 {
		t.Fatalf("count sin finales: %d %v", n, err)
	}
	if _, err := gr.Update(ctx, g.ID, repoUserA, GradePatch{IsFinalExam: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	if n, err := gr.CountFinalExams(ctx, mat, repoUserA, ""); err != nil || n != 1 {
		t.Fatalf("count 1: %d %v", n, err)
	}
	if n, err := gr.CountFinalExams(ctx, mat, repoUserA, g.ID); err != nil || n != 0 {
		t.Fatalf("count excluyendo: %d %v", n, err)
	}
	// Requirements: crear, GradeSubjectOf, listar, borrar.
	req, err := qr.Create(ctx, repoUserA, mat, nil, true, ">=", 4.0, "force_exam")
	if err != nil || req == nil {
		t.Fatalf("requirement: %+v %v", req, err)
	}
	if ok, _ := qr.GradeSubjectOf(ctx, g.ID, mat, repoUserA); !ok {
		t.Fatal("fuente propia true")
	}
	if ok, _ := qr.GradeSubjectOf(ctx, g.ID, mat, repoUserB); ok {
		t.Fatal("fuente ajena false")
	}
	if ok, _ := qr.GradeSubjectOf(ctx, "11111111-1111-4111-8111-111111111111", mat, repoUserA); ok {
		t.Fatal("fuente inexistente false")
	}
	if reqs, owned, _ := qr.ListBySubject(ctx, mat, repoUserA); !owned || len(reqs) != 1 {
		t.Fatalf("list req: %v %v", reqs, owned)
	}
	if _, owned, _ := qr.ListBySubject(ctx, mat, repoUserB); owned {
		t.Fatal("req ajenos owned=false")
	}
	deleted, err := qr.Delete(ctx, req.ID, repoUserB)
	if err != nil || deleted {
		t.Fatalf("delete req ajeno: %v", err)
	}
	deleted, err = qr.Delete(ctx, req.ID, repoUserA)
	if err != nil || !deleted {
		t.Fatalf("delete req: %v", err)
	}
	// Borrar padre arrastra hijas (CASCADE); borrar nota propia ok.
	deletedG, err := gr.Delete(ctx, padre.ID, repoUserA)
	if err != nil || !deletedG {
		t.Fatalf("delete padre: %v", err)
	}
	var nHijos int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.grades WHERE parent_grade_id IS NOT NULL`).Scan(&nHijos); err != nil {
		t.Fatal(err)
	}
	if nHijos != 0 {
		t.Fatalf("hijas huérfanas: %d", nHijos)
	}
	if deletedG, _ := gr.Delete(ctx, g.ID, repoUserB); deletedG {
		t.Fatal("delete ajeno false")
	}
}

func TestRepository_Absences_Counts(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	r := NewAbsenceRepository(pool)
	curA := mustCurriculum(t, pool, repoUserA, "A")
	mat := mustSubject(t, pool, repoUserA, curA, "S")

	d1 := timeDate(2026, 3, 1)
	a, err := r.Create(ctx, repoUserA, mat, d1, false)
	if err != nil || a == nil || a.Justified {
		t.Fatalf("create: %+v %v", a, err)
	}
	if a2, _ := r.Create(ctx, repoUserB, mat, d1, false); a2 != nil {
		t.Fatal("materia ajena nil")
	}
	exists, err := r.ExistsOnDate(ctx, repoUserA, mat, d1)
	if err != nil || !exists {
		t.Fatalf("exists: %v %v", exists, err)
	}
	if exists, _ := r.ExistsOnDate(ctx, repoUserA, mat, timeDate(2026, 3, 2)); exists {
		t.Fatal("otra fecha false")
	}
	if got, _ := r.GetByID(ctx, a.ID, repoUserB); got != nil {
		t.Fatal("get ajeno nil")
	}
	list, owned, err := r.ListBySubject(ctx, mat, repoUserA)
	if err != nil || !owned || len(list) != 1 {
		t.Fatalf("list: %v %v %d", list, owned, err)
	}
	if _, owned, _ := r.ListBySubject(ctx, mat, repoUserB); owned {
		t.Fatal("ajena owned=false")
	}
	upd, err := r.UpdateJustified(ctx, a.ID, repoUserA, true)
	if err != nil || upd == nil || !upd.Justified {
		t.Fatalf("toggle: %+v %v", upd, err)
	}
	if upd, _ := r.UpdateJustified(ctx, a.ID, repoUserB, false); upd != nil {
		t.Fatal("toggle ajeno nil")
	}
	un, just, owned, err := r.CountsBySubject(ctx, mat, repoUserA)
	if err != nil || !owned || un != 0 || just != 1 {
		t.Fatalf("counts: %d %d %v %v", un, just, owned, err)
	}
	if _, _, owned, _ := r.CountsBySubject(ctx, mat, repoUserB); owned {
		t.Fatal("counts ajenos owned=false")
	}
	deleted, err := r.Delete(ctx, a.ID, repoUserB)
	if err != nil || deleted {
		t.Fatalf("delete ajeno: %v", err)
	}
	deleted, err = r.Delete(ctx, a.ID, repoUserA)
	if err != nil || !deleted {
		t.Fatalf("delete: %v", err)
	}
}

func TestRepository_CountsByCurricula(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	r := NewSubjectRepository(pool)
	m1 := mustCurriculum(t, pool, repoUserA, "M1")
	m2 := mustCurriculum(t, pool, repoUserA, "M2")
	mustSubject(t, pool, repoUserA, m1, "A")
	status := "approved"
	s, err := r.Create(ctx, repoUserA, m1, SubjectCreate{Name: "B", AverageDecimalPlaces: 2, Status: status})
	if err != nil || s == nil {
		t.Fatal(err)
	}
	mustSubject(t, pool, repoUserA, m2, "C")
	counts, err := r.CountsByCurricula(ctx, repoUserA)
	if err != nil {
		t.Fatal(err)
	}
	if counts[m1].Total != 2 || counts[m1].Approved != 1 || counts[m1].InProgress != 1 {
		t.Fatalf("m1: %+v", counts[m1])
	}
	if counts[m2].Total != 1 {
		t.Fatalf("m2: %+v", counts[m2])
	}
	if countsB, _ := r.CountsByCurricula(ctx, repoUserB); len(countsB) != 0 {
		t.Fatalf("B vacío: %+v", countsB)
	}
}

func TestRepository_Cascadas(t *testing.T) {
	pool := repoTestPool(t)
	ctx := context.Background()
	curR := NewCurriculumRepository(pool)
	subR := NewSubjectRepository(pool)
	grR := NewGradeRepository(pool)
	qr := NewRequirementRepository(pool)
	ar := NewAbsenceRepository(pool)

	curA := mustCurriculum(t, pool, repoUserA, "A")
	mat := mustSubject(t, pool, repoUserA, curA, "S")
	start, _ := timeParseHM("08:00")
	end, _ := timeParseHM("09:00")
	if _, err := NewScheduleRepository(pool).Create(ctx, repoUserA, mat, 1, start, end); err != nil {
		t.Fatal(err)
	}
	score := 50
	g, err := grR.Create(ctx, repoUserA, mat, GradeCreate{Name: "G", Score: &score, Weight: 1, Status: "graded"})
	if err != nil || g == nil {
		t.Fatal(err)
	}
	if _, err := qr.Create(ctx, repoUserA, mat, &g.ID, false, ">=", 4.0, "force_fail"); err != nil {
		t.Fatal(err)
	}
	if _, err := ar.Create(ctx, repoUserA, mat, timeDate(2026, 1, 1), false); err != nil {
		t.Fatal(err)
	}
	// Borrar materia arrastra todo (schedules, grades, requirements, absences).
	if _, err := subR.Delete(ctx, mat, repoUserA); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"schedules", "grades", "grade_requirements", "absences"} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.`+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("%s huérfanos: %d", table, n)
		}
	}
	// Borrar malla arrastra subjects.
	mat2 := mustSubject(t, pool, repoUserA, curA, "S2")
	_ = mat2
	if _, err := curR.Delete(ctx, curA, repoUserA); err != nil {
		t.Fatal(err)
	}
	var nSub int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.subjects`).Scan(&nSub); err != nil {
		t.Fatal(err)
	}
	if nSub != 0 {
		t.Fatalf("subjects huérfanos: %d", nSub)
	}
}
