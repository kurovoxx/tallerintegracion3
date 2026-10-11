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
	hor, err := schSvc.Create(ctx, intUserA, mat.ID, 3, "14:00", "15:30")
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
