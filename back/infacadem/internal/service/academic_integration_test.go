package service

// Integración CRUD + cascada contra Postgres real.
//
// Cómo levantarlo (BD de prueba dedicada y vacía, nunca la de desarrollo):
//
//   docker run -d --name academic-test-db -e POSTGRES_PASSWORD=postgres \
//     -e POSTGRES_USER=postgres -e POSTGRES_DB=acad_test \
//     -p 5435:5432 postgres:16-alpine
//   export ACADEMIC_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5435/acad_test?sslmode=disable'
//   go test ./internal/service/ -run TestAcademicIntegration -count=1 -v
//   docker rm -f academic-test-db
//
// Sin ACADEMIC_TEST_DATABASE_URL hace skip. Crea SCHEMA academic + tablas
// mínimas (subset de docker/postgres/init.sql) y limpia con DROP SCHEMA.
// Nota: el paquete handler/grpc usa la misma BD y el mismo schema en su test
// de integración; con la env fijada, correr el suite completo con -p 1
// (misma solución que back/auth: evita crear el schema en paralelo con 42P06):
//
//   go test ./... -p 1 -count=1

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/infacadem/internal/repository"
)

const integrationSchemaSQL = `
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
    code varchar(50),
    professor varchar(200),
    room varchar(100),
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
    modality varchar(50),
    CHECK (end_time > start_time)
);
CREATE TABLE academic.contents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id uuid NOT NULL REFERENCES academic.subjects(id) ON DELETE CASCADE,
    title varchar(200) NOT NULL,
    body text,
    position int NOT NULL DEFAULT 0
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
    pinned boolean NOT NULL DEFAULT false,
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
    source varchar(20),
    type varchar(50),
    created_at timestamptz NOT NULL DEFAULT now()
);
`

func setupIntegration(t *testing.T) (*CurriculumService, *SubjectService, *ScheduleService, *pgxpool.Pool) {
	t.Helper()
	url := os.Getenv("ACADEMIC_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ACADEMIC_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("connect test db: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA academic`); err != nil {
		pool.Close()
		t.Fatalf("requires an empty test database (CREATE SCHEMA academic failed): %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA academic CASCADE`)
		pool.Close()
	})
	if _, err := pool.Exec(ctx, integrationSchemaSQL); err != nil {
		t.Fatalf("apply schema: %v", err)
	}
	return NewCurriculumService(repository.NewCurriculumRepository(pool)),
		NewSubjectService(repository.NewSubjectRepository(pool)),
		NewScheduleService(repository.NewScheduleRepository(pool)),
		pool
}

const (
	intUserA = "11111111-1111-4111-8111-111111111111"
	intUserB = "22222222-2222-4222-8222-222222222222"
)

// CRUD completo + cascada real por FK + propiedad cruzada a nivel SQL.
func TestAcademicIntegration_CRUDCascada(t *testing.T) {
	curSvc, subSvc, schSvc, pool := setupIntegration(t)
	ctx := context.Background()

	malla, err := curSvc.Create(ctx, intUserA, "Malla real")
	if err != nil {
		t.Fatalf("create curriculum: %v", err)
	}
	name := "Álgebra"
	credits := 10
	mat, err := subSvc.Create(ctx, intUserA, malla.ID, SubjectInput{Name: &name, Credits: &credits})
	if err != nil {
		t.Fatalf("create subject: %v", err)
	}
	if mat.Credits == nil || *mat.Credits != 10 || mat.Status != "in_progress" {
		t.Fatalf("subject: %+v", mat)
	}
	hor, err := schSvc.Create(ctx, intUserA, mat.ID, 3, "14:00", "15:30", nil)
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if hor.StartTime.Format("15:04") != "14:00" {
		t.Fatalf("start: %v", hor.StartTime)
	}

	// Propiedad cruzada a nivel SQL: todo 404.
	if _, err := curSvc.Get(ctx, intUserB, malla.ID); err == nil {
		t.Fatal("get ajeno debe ser 404")
	} else if se := err.(*ServiceError); se.Code != "not_found" {
		t.Fatalf("code: %v", err)
	}
	if _, err := subSvc.Create(ctx, intUserB, malla.ID, SubjectInput{Name: &name}); err == nil {
		t.Fatal("create en malla ajena debe ser 404")
	} else if se := err.(*ServiceError); se.Code != "not_found" {
		t.Fatalf("code: %v", err)
	}
	if err := schSvc.Delete(ctx, intUserB, hor.ID); err == nil {
		t.Fatal("delete ajeno debe ser 404")
	} else if se := err.(*ServiceError); se.Code != "not_found" {
		t.Fatalf("code: %v", err)
	}

	// Borrado en cascada: subjects y schedules huérfanos cero.
	if err := curSvc.Delete(ctx, intUserA, malla.ID); err != nil {
		t.Fatalf("delete curriculum: %v", err)
	}
	var nSub, nSch int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.subjects`).Scan(&nSub); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.schedules`).Scan(&nSch); err != nil {
		t.Fatal(err)
	}
	if nSub != 0 || nSch != 0 {
		t.Fatalf("cascada incompleta: subjects=%d schedules=%d", nSub, nSch)
	}
}

// Notas + requisitos + promedio contra Postgres real, con cascada
// subject→grades/requirements verificada por query directa.
func TestAcademicIntegration_GradesCascada(t *testing.T) {
	curSvc, subSvc, _, pool := setupIntegration(t)
	ctx := context.Background()
	gradeRepo := repository.NewGradeRepository(pool)
	reqRepo := repository.NewRequirementRepository(pool)
	gradeSvc := NewGradeService(gradeRepo)
	reqSvc := NewRequirementService(reqRepo)
	avgSvc := NewAverageService(repository.NewSubjectRepository(pool), gradeRepo, reqRepo)

	malla, err := curSvc.Create(ctx, intUserA, "Malla notas")
	if err != nil {
		t.Fatal(err)
	}
	name := "Cálculo"
	mat, err := subSvc.Create(ctx, intUserA, malla.ID, SubjectInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}

	s70, s50 := 70, 50
	if _, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &name, Weight: ptrFloat(0.5), Score: &s70}); err != nil {
		t.Fatalf("grade A: %v", err)
	}
	padre, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &name, Weight: ptrFloat(0.5)})
	if err != nil {
		t.Fatalf("padre: %v", err)
	}
	if _, err := gradeSvc.CreateChild(ctx, intUserA, padre.ID, GradeInput{Name: &name, Weight: ptrFloat(1), Score: &s50}); err != nil {
		t.Fatalf("hija: %v", err)
	}
	// (70*.5 + 50*.5)/10 = 6.0 aprobado.
	res, err := avgSvc.Average(ctx, intUserA, mat.ID)
	if err != nil || res.Average == nil || *res.Average != 6.0 || res.Status != "approved" {
		t.Fatalf("average 6.0 approved: %+v %v", res, err)
	}
	// Final único real: segundo → 409.
	finalName := "Examen"
	if _, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &finalName, Weight: ptrFloat(0.2), IsFinalExam: ptrBool(true)}); err != nil {
		t.Fatalf("final: %v", err)
	}
	if _, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &finalName, Weight: ptrFloat(0.2), IsFinalExam: ptrBool(true)}); err == nil {
		t.Fatal("segundo final debe ser 409")
	} else if se := err.(*ServiceError); se.Code != "conflict" {
		t.Fatalf("code: %v", err)
	}
	// Requisito + borrado directo.
	tf, ge, fe, thr := true, ">=", "force_exam", 4.0
	req, err := reqSvc.Create(ctx, intUserA, mat.ID, RequirementInput{ChecksFinalAverage: &tf, Comparator: &ge, ThresholdValue: &thr, Action: &fe})
	if err != nil {
		t.Fatalf("requirement: %v", err)
	}
	if err := reqSvc.Delete(ctx, intUserA, req.ID); err != nil {
		t.Fatalf("delete requirement: %v", err)
	}

	// Borrar la materia: grades, hijas y requisitos huérfanos cero.
	if err := subSvc.Delete(ctx, intUserA, mat.ID); err != nil {
		t.Fatalf("delete subject: %v", err)
	}
	for _, tc := range []struct{ table string }{{"academic.grades"}, {"academic.grade_requirements"}} {
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM `+tc.table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("cascada incompleta en %s: %d", tc.table, n)
		}
	}
}

func ptrFloat(f float64) *float64 { return &f }
func ptrBool(b bool) *bool        { return &b }
func ptrInt(i int) *int           { return &i }

// Final único contra Postgres real (los fakes no cubren este SQL).
// Regresión: CountFinalExams con excludeID vacío rompía con 22P02
// (” casteado a uuid) y el error crudo subía como 500 en POST con
// is_final_exam:true. Cubre crear (201) + duplicado por POST y por
// PATCH (409 conflict) en ambas ramas del conteo.
func TestAcademicIntegration_FinalUnico(t *testing.T) {
	curSvc, subSvc, _, pool := setupIntegration(t)
	ctx := context.Background()
	gradeSvc := NewGradeService(repository.NewGradeRepository(pool))

	malla, err := curSvc.Create(ctx, intUserA, "Malla final")
	if err != nil {
		t.Fatal(err)
	}
	name := "Álgebra"
	mat, err := subSvc.Create(ctx, intUserA, malla.ID, SubjectInput{Name: &name})
	if err != nil {
		t.Fatal(err)
	}

	// Crear con is_final_exam:true (rama excludeID vacío) → éxito, no 500.
	finalName := "Examen Final"
	if _, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &finalName, Weight: ptrFloat(0.3), IsFinalExam: ptrBool(true)}); err != nil {
		t.Fatalf("crear final: %v", err)
	}
	// Duplicado por POST → 409 conflict (no 500).
	if _, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &finalName, Weight: ptrFloat(0.3), IsFinalExam: ptrBool(true)}); err == nil {
		t.Fatal("segundo final debe ser 409")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "conflict" {
		t.Fatalf("esperado conflict, got %v", err)
	}
	// Duplicado por PATCH en otra nota (rama excludeID con UUID) → 409.
	otra, err := gradeSvc.Create(ctx, intUserA, mat.ID, GradeInput{Name: &name, Weight: ptrFloat(0.2)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gradeSvc.Update(ctx, intUserA, otra.ID, GradeInput{IsFinalExam: ptrBool(true)}); err == nil {
		t.Fatal("patch a final duplicado debe ser 409")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "conflict" {
		t.Fatalf("esperado conflict, got %v", err)
	}
	// La propia final puede re-guardarse como final sin chocar consigo misma.
	final, err := gradeSvc.List(ctx, intUserA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	var finalID string
	for _, n := range final {
		if n.Grade.IsFinalExam {
			finalID = n.Grade.ID
		}
	}
	if finalID == "" {
		t.Fatal("final no encontrada")
	}
	if _, err := gradeSvc.Update(ctx, intUserA, finalID, GradeInput{IsFinalExam: ptrBool(true), Name: &finalName}); err != nil {
		t.Fatalf("re-guardar la propia final: %v", err)
	}
}

// Asistencia contra Postgres real: conteos, duplicado 409 y cascada
// subject→absences verificada por query directa.
func TestAcademicIntegration_Attendance(t *testing.T) {
	curSvc, subSvc, _, pool := setupIntegration(t)
	ctx := context.Background()
	absSvc := NewAbsenceService(repository.NewAbsenceRepository(pool))
	attSvc := NewAttendanceService(repository.NewSubjectRepository(pool), repository.NewAbsenceRepository(pool))

	malla, err := curSvc.Create(ctx, intUserA, "Malla asistencia")
	if err != nil {
		t.Fatal(err)
	}
	name := "Historia"
	total, min := 32, 75.0
	mat, err := subSvc.Create(ctx, intUserA, malla.ID, SubjectInput{Name: &name, TotalClasses: &total, MinAttendancePct: &min})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"2026-03-01", "2026-03-02", "2026-03-03", "2026-03-04", "2026-03-05"} {
		if _, err := absSvc.Create(ctx, intUserA, mat.ID, d, false, nil, nil); err != nil {
			t.Fatalf("falta %s: %v", d, err)
		}
	}
	for _, d := range []string{"2026-03-06", "2026-03-07"} {
		if _, err := absSvc.Create(ctx, intUserA, mat.ID, d, true, nil, nil); err != nil {
			t.Fatalf("justificada %s: %v", d, err)
		}
	}
	if _, err := absSvc.Create(ctx, intUserA, mat.ID, "2026-03-01", false, nil, nil); err == nil {
		t.Fatal("duplicada debe ser 409")
	} else if se, ok := err.(*ServiceError); !ok || se.Code != "conflict" {
		t.Fatalf("esperado conflict, got %v", err)
	}
	res, err := attSvc.Summary(ctx, intUserA, mat.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res.Percentage == nil || *res.Percentage != 0.84375 {
		t.Fatalf("percentage 0.84375: %+v", res)
	}
	if res.MeetsMinimum == nil || !*res.MeetsMinimum || res.Remaining == nil || *res.Remaining != 3 {
		t.Fatalf("meets true + remaining 3: %+v", res)
	}
	// total_classes null → nulos (materia sin configurar).
	name2 := "Sin total"
	mat2, err := subSvc.Create(ctx, intUserA, malla.ID, SubjectInput{Name: &name2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := absSvc.Create(ctx, intUserA, mat2.ID, "2026-03-01", false, nil, nil); err != nil {
		t.Fatal(err)
	}
	if res2, err := attSvc.Summary(ctx, intUserA, mat2.ID); err != nil {
		t.Fatal(err)
	} else if res2.Percentage != nil || res2.MeetsMinimum != nil || res2.Remaining != nil {
		t.Fatalf("sin total → nulos: %+v", res2)
	}
	// Borrar materia: absences huérfanas cero.
	if err := subSvc.Delete(ctx, intUserA, mat.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM academic.absences WHERE subject_id=$1`, mat.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("cascada incompleta: %d", n)
	}
}

// Overview agregado contra Postgres real (cubre el GROUP BY de conteos).
func TestAcademicIntegration_Overview(t *testing.T) {
	curSvc, subSvc, _, pool := setupIntegration(t)
	ctx := context.Background()
	ovSvc := NewOverviewService(curSvc, repository.NewSubjectRepository(pool))

	m1, err := curSvc.Create(ctx, intUserA, "M1")
	if err != nil {
		t.Fatal(err)
	}
	m2, err := curSvc.Create(ctx, intUserA, "M2")
	if err != nil {
		t.Fatal(err)
	}
	approved, inprog := "approved", "in_progress"
	name := "S"
	if _, err := subSvc.Create(ctx, intUserA, m1.ID, SubjectInput{Name: &name, Status: &approved}); err != nil {
		t.Fatal(err)
	}
	if _, err := subSvc.Create(ctx, intUserA, m1.ID, SubjectInput{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if _, err := subSvc.Create(ctx, intUserA, m2.ID, SubjectInput{Name: &name, Status: &inprog}); err != nil {
		t.Fatal(err)
	}
	out, err := ovSvc.Get(ctx, intUserA)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Curricula) != 2 || out.Stats.CurriculaCount != 2 || out.Stats.SubjectsCount != 3 {
		t.Fatalf("overview: %+v", out)
	}
	byID := map[string]CurriculumOverview{}
	for _, c := range out.Curricula {
		byID[c.ID] = c
	}
	if byID[m1.ID].SubjectsCount != 2 || byID[m1.ID].ApprovedCount != 1 || byID[m1.ID].InProgressCount != 1 {
		t.Fatalf("m1: %+v", byID[m1.ID])
	}
	if byID[m2.ID].SubjectsCount != 1 || byID[m2.ID].ApprovedCount != 0 {
		t.Fatalf("m2: %+v", byID[m2.ID])
	}
	// Solo propias.
	outB, err := ovSvc.Get(ctx, intUserB)
	if err != nil {
		t.Fatal(err)
	}
	if len(outB.Curricula) != 0 || outB.Stats.SubjectsCount != 0 {
		t.Fatalf("B vacío: %+v", outB)
	}
}
