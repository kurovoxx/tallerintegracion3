package repository

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

// Opt-in: use a dedicated empty PostgreSQL database, never a live database.
func TestPasswordResetRepository(t *testing.T) {
	url := os.Getenv("PASSWORD_RESET_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("PASSWORD_RESET_TEST_DATABASE_URL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// CREATE fails if identity already exists, leaving existing data untouched.
	if _, err := pool.Exec(ctx, `CREATE SCHEMA identity`); err != nil {
		t.Fatalf("requires an empty test database: %v", err)
	}
	defer pool.Exec(ctx, `DROP SCHEMA identity CASCADE`)
	if _, err := pool.Exec(ctx, `
 CREATE TABLE identity.users (id uuid PRIMARY KEY, email text UNIQUE, password_hash text);
 CREATE TABLE identity.refresh_tokens (id uuid PRIMARY KEY, user_id uuid REFERENCES identity.users(id), token_hash text, expires_at timestamptz, revoked boolean DEFAULT false, created_at timestamptz DEFAULT now());
 INSERT INTO identity.users VALUES ('00000000-0000-0000-0000-000000000001', 'user@example.com', 'old');
 INSERT INTO identity.refresh_tokens (id, user_id) VALUES ('00000000-0000-0000-0000-000000000002', '00000000-0000-0000-0000-000000000001');`); err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/001_password_resets.sql")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := pool.Exec(ctx, string(migration)); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPasswordResetRepository(pool)
	const uid = "00000000-0000-0000-0000-000000000001"
	hash, _ := bcrypt.GenerateFromPassword([]byte("12345678"), bcrypt.MinCost)
	issue := func() {
		t.Helper()
		_, err := pool.Exec(ctx, `UPDATE identity.password_resets SET requested_at=now()-interval '61 seconds'`)
		if err != nil {
			t.Fatal(err)
		}
		ok, err := repo.Issue(ctx, uid, string(hash))
		if err != nil || !ok {
			t.Fatalf("issue: %v %v", ok, err)
		}
	}
	issue()
	if ok, err := repo.Issue(ctx, uid, string(hash)); err != nil || ok {
		t.Fatalf("cooldown: %v %v", ok, err)
	}
	for i := 0; i < 5; i++ {
		if ok, err := repo.Complete(ctx, "user@example.com", "00000000", "wrong"); err != nil || ok {
			t.Fatalf("wrong code: %v %v", ok, err)
		}
	}
	if ok, err := repo.Complete(ctx, "user@example.com", "12345678", "wrong"); err != nil || ok {
		t.Fatalf("attempt limit: %v %v", ok, err)
	}
	issue()
	if _, err := pool.Exec(ctx, `UPDATE identity.password_resets SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if ok, err := repo.Complete(ctx, "user@example.com", "12345678", "wrong"); err != nil || ok {
		t.Fatalf("expiration: %v %v", ok, err)
	}
	issue()
	newCode, _ := bcrypt.GenerateFromPassword([]byte("87654321"), bcrypt.MinCost)
	hash = newCode
	issue()
	if ok, err := repo.Complete(ctx, "user@example.com", "12345678", "wrong"); err != nil || ok {
		t.Fatalf("old code: %v %v", ok, err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := repo.Complete(ctx, "user@example.com", "87654321", "new-hash")
			if err != nil {
				t.Error(err)
			}
			if ok {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal("code must be redeemable exactly once")
	}
	var password string
	var revoked bool
	if err := pool.QueryRow(ctx, `SELECT password_hash FROM identity.users WHERE id=$1`, uid).Scan(&password); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT revoked FROM identity.refresh_tokens WHERE user_id=$1`, uid).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if password != "new-hash" || !revoked {
		t.Fatal("password update and session revocation failed")
	}
}
