package grpc

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestPool conecta a una BD de prueba dedicada y vacía, crea el schema
// academic mínimo (curricula + subjects) y lo limpia al terminar.
// Falla si el schema ya existe: nunca tocar la BD de desarrollo.
// Nota: el paquete service usa la misma BD y el mismo schema en sus tests
// de integración; con ACADEMIC_TEST_DATABASE_URL fijada, correr el suite
// completo con -p 1 para no crear el schema en paralelo desde dos paquetes.
func newTestPool(ctx context.Context, t *testing.T, url string) (*pgxpool.Pool, error) {
	t.Helper()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if _, err := pool.Exec(ctx, `CREATE SCHEMA academic`); err != nil {
		pool.Close()
		t.Fatalf("requires an empty test database (CREATE SCHEMA academic failed): %v", err)
		return nil, err
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DROP SCHEMA academic CASCADE`)
		pool.Close()
	})
	_, err = pool.Exec(ctx, `
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
    average_decimal_places smallint NOT NULL DEFAULT 2,
    status varchar(20) NOT NULL DEFAULT 'in_progress'
);
`)
	if err != nil {
		return nil, err
	}
	return pool, nil
}
