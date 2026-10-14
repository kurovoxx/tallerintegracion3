package identity

import (
	"context"
	"testing"
	"time"

	identityv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/identityv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// TestGRPCDirectory_LookupUsers verifica mapeo, vacíos y tolerancia a fallos.
func TestGRPCDirectory_LookupUsers(t *testing.T) {
	c := servefakeIdentityGRPC(t, &fakeIdentityGRPC{key: "k", users: testUsers()})
	d := NewGRPCUserDirectory(c)
	ctx := context.Background()

	m, err := d.LookupUsers(ctx, []string{"u-ada", "nadie", "u-solo", "", "  "})
	if err != nil || len(m) != 2 || m["u-ada"].Email != "ada@test.invalid" {
		t.Fatalf("LookupUsers = %v, %v", m, err)
	}
	if got, err := d.LookupUsers(ctx, nil); err != nil || len(got) != 0 {
		t.Fatalf("nil debe dar mapa vacío sin error: %v, %v", got, err)
	}

	caido := NewGRPCUserDirectory(&GRPCClient{client: c.client, key: "mala", timeout: 2 * time.Second})
	if got, err := caido.LookupUsers(ctx, []string{"u-ada"}); err != nil || len(got) != 0 {
		t.Fatalf("best-effort: ante fallo, mapa vacío sin error: %v, %v", got, err)
	}
	var _ service.UserDirectory = d
}

// TestGRPCDirectory_EmailsStrict verifica correos, duplicados, minúsculas,
// vacíos y fail-closed ante fallos.
func TestGRPCDirectory_EmailsStrict(t *testing.T) {
	c := servefakeIdentityGRPC(t, &fakeIdentityGRPC{key: "k", users: map[string]*identityv1.User{
		"a": {UserId: "a", Email: "A@test.invalid"},
		"b": {UserId: "b", Email: "DUPLI@test.invalid"},
		"c": {UserId: "c", Email: "dupli@test.invalid"},
		"d": {UserId: "d", Email: ""},
	}})
	d := NewGRPCUserDirectory(c)
	ctx := context.Background()

	emails, err := d.LookupEmailsStrict(ctx, []string{"a", "b", "c", "d", "nadie"})
	if err != nil {
		t.Fatalf("LookupEmailsStrict: %v", err)
	}
	want := []string{"a@test.invalid", "dupli@test.invalid"}
	if len(emails) != len(want) || emails[0] != want[0] || emails[1] != want[1] {
		t.Fatalf("emails = %v, se esperaba %v", emails, want)
	}
	empty, err := d.LookupEmailsStrict(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("nil debe dar lista vacía: %v, %v", empty, err)
	}

	caido := NewGRPCUserDirectory(&GRPCClient{client: c.client, key: "mala", timeout: 2 * time.Second})
	if _, err := caido.LookupEmailsStrict(ctx, []string{"a"}); err == nil {
		t.Fatal("fail-closed: ante fallo debe devolver error")
	}
}
