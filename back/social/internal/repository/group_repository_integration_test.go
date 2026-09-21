package repository

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
)

// Tests de integración contra Postgres real. Se omiten salvo que
// SOCIAL_TEST_DATABASE_URL apunte a una base con el schema de
// docker/postgres/init.sql (docker compose up -d postgres). Cada test crea sus
// propios grupos con usuarios uuid aleatorios y los borra al terminar (el
// resto cae por ON DELETE CASCADE). Nunca apuntar a una base compartida.

func testRepo(t *testing.T) (*GroupRepository, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("SOCIAL_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SOCIAL_TEST_DATABASE_URL no definida: se omite el test de integración")
	}
	pool, err := NewPool(context.Background(), dsn)
	if err != nil {
		t.Fatalf("conectar a la base de pruebas: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewGroupRepository(pool), pool
}

// newTestGroup crea un grupo y lo elimina al final del test.
func newTestGroup(t *testing.T, r *GroupRepository, owner string) *model.Group {
	t.Helper()
	g, err := r.CreateWithOwner(context.Background(), "it-"+uuid.NewString()[:8], nil, owner)
	if err != nil {
		t.Fatalf("crear grupo: %v", err)
	}
	t.Cleanup(func() { _ = r.DeleteGroup(context.Background(), g.ID) })
	return g
}

func joinUser(t *testing.T, r *GroupRepository, g *model.Group, user string) {
	t.Helper()
	if err := r.JoinWithInviteToken(context.Background(), g.ID, g.InviteToken, user); err != nil {
		t.Fatalf("join: %v", err)
	}
}

func roleIn(t *testing.T, r *GroupRepository, groupID, user string) string {
	t.Helper()
	role, err := r.GetMemberRole(context.Background(), groupID, user)
	if err != nil {
		return ""
	}
	return role
}

func wantStoreErr(t *testing.T, got error, want string) {
	t.Helper()
	if got == nil || got.Error() != want {
		t.Fatalf("error = %v, se esperaba %q", got, want)
	}
}

// Regresión F1: el INSERT en banned_users omitía banned_by_user_id (NOT NULL).
func TestIntegrationBanRecordsBannedBy(t *testing.T) {
	r, pool := testRepo(t)
	ctx := context.Background()
	admin, member := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, member)

	if err := r.BanMember(ctx, g.ID, admin, member); err != nil {
		t.Fatalf("ban: %v", err)
	}
	var by string
	if err := pool.QueryRow(ctx, `SELECT banned_by_user_id FROM social.banned_users WHERE group_id = $1 AND user_id = $2`, g.ID, member).Scan(&by); err != nil {
		t.Fatalf("no quedó fila de baneo: %v", err)
	}
	if by != admin {
		t.Fatalf("banned_by_user_id = %s, se esperaba %s", by, admin)
	}
	if roleIn(t, r, g.ID, member) != "" {
		t.Fatal("el baneado debe salir del grupo")
	}
	// ni con el token vigente puede volver
	wantStoreErr(t, r.JoinWithInviteToken(ctx, g.ID, g.InviteToken, member), "user_banned")
}

func TestIntegrationBanAndKickTargetRules(t *testing.T) {
	r, pool := testRepo(t)
	ctx := context.Background()
	admin, admin2, member, stranger := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, admin2)
	joinUser(t, r, g, member)
	if err := r.ChangeMemberRole(ctx, g.ID, admin, admin2, "admin"); err != nil {
		t.Fatal(err)
	}

	wantStoreErr(t, r.BanMember(ctx, g.ID, admin, admin2), "target_is_admin")
	wantStoreErr(t, r.KickMember(ctx, g.ID, admin, admin2), "target_is_admin")
	wantStoreErr(t, r.BanMember(ctx, g.ID, admin, stranger), "target_not_found")
	wantStoreErr(t, r.KickMember(ctx, g.ID, admin, stranger), "target_not_found")
	wantStoreErr(t, r.BanMember(ctx, g.ID, admin, admin), "cannot_modify_self")
	wantStoreErr(t, r.KickMember(ctx, g.ID, member, admin2), "forbidden")

	var bans int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM social.banned_users WHERE group_id = $1`, g.ID).Scan(&bans)
	if bans != 0 {
		t.Fatalf("las acciones rechazadas no deben dejar baneos: %d", bans)
	}
	if err := r.KickMember(ctx, g.ID, admin, member); err != nil {
		t.Fatalf("kick válido: %v", err)
	}
}

// La autorización va antes que la validación de rol: un no-admin ve 403, no 400.
func TestIntegrationAuthorizationBeforeValidation(t *testing.T) {
	r, _ := testRepo(t)
	ctx := context.Background()
	admin, member := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, member)

	wantStoreErr(t, r.ChangeMemberRole(ctx, g.ID, member, admin, "teacher"), "forbidden")
	wantStoreErr(t, r.ChangeMemberRole(ctx, g.ID, member, member, "admin"), "forbidden")
	wantStoreErr(t, r.TransferAdmin(ctx, g.ID, member, member), "forbidden")
	wantStoreErr(t, r.ChangeMemberRole(ctx, g.ID, admin, member, "teacher"), "invalid_role")
}

func TestIntegrationSuccessionPromotesOldestMember(t *testing.T) {
	r, pool := testRepo(t)
	ctx := context.Background()
	admin, oldest, newer := uuid.NewString(), uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, oldest)
	joinUser(t, r, g, newer)
	// joined_at explícito para no depender de la resolución del reloj
	_, _ = pool.Exec(ctx, `UPDATE social.group_memberships SET joined_at = now() - interval '2 days' WHERE group_id = $1 AND user_id = $2`, g.ID, oldest)
	_, _ = pool.Exec(ctx, `UPDATE social.group_memberships SET joined_at = now() - interval '1 day' WHERE group_id = $1 AND user_id = $2`, g.ID, newer)

	res, err := r.HandleAccountDeletion(ctx, admin)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].PromotedUserID == nil || *res[0].PromotedUserID != oldest {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if roleIn(t, r, g.ID, oldest) != model.RoleAdmin || roleIn(t, r, g.ID, newer) != model.RoleMember {
		t.Fatal("solo el miembro más antiguo debe ser admin")
	}
	if roleIn(t, r, g.ID, admin) != "" {
		t.Fatal("el usuario eliminado debe salir del grupo")
	}
	got, _ := r.GetByID(ctx, g.ID)
	if got.OwnerUserID != oldest {
		t.Fatalf("owner_user_id = %s, se esperaba el sucesor %s", got.OwnerUserID, oldest)
	}

	// idempotente
	res, err = r.HandleAccountDeletion(ctx, admin)
	if err != nil || len(res) != 0 {
		t.Fatalf("segunda llamada debe ser no-op: %v %v", res, err)
	}
}

func TestIntegrationSuccessionWithCoAdminAndLastMember(t *testing.T) {
	r, _ := testRepo(t)
	ctx := context.Background()

	// con co-admin: no se promueve a nadie y el dueño pasa al admin restante
	admin, admin2, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, admin2)
	joinUser(t, r, g, member)
	if err := r.ChangeMemberRole(ctx, g.ID, admin, admin2, "admin"); err != nil {
		t.Fatal(err)
	}
	res, err := r.HandleAccountDeletion(ctx, admin)
	if err != nil || len(res) != 1 || res[0].PromotedUserID != nil || res[0].GroupDeleted {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	if roleIn(t, r, g.ID, member) != model.RoleMember {
		t.Fatal("no debe ascender a nadie si queda otro admin")
	}
	if got, _ := r.GetByID(ctx, g.ID); got.OwnerUserID != admin2 {
		t.Fatalf("owner_user_id = %s, se esperaba %s", got.OwnerUserID, admin2)
	}

	// último miembro: el grupo se elimina
	solo := uuid.NewString()
	g2 := newTestGroup(t, r, solo)
	res, err = r.HandleAccountDeletion(ctx, solo)
	if err != nil || len(res) != 1 || !res[0].GroupDeleted {
		t.Fatalf("resultado inesperado: %+v %v", res, err)
	}
	if got, _ := r.GetByID(ctx, g2.ID); got != nil {
		t.Fatal("el grupo debe haberse eliminado")
	}
}

func TestIntegrationAccountDeletionClearsBans(t *testing.T) {
	r, pool := testRepo(t)
	ctx := context.Background()
	admin, victim := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, victim)
	if err := r.BanMember(ctx, g.ID, admin, victim); err != nil {
		t.Fatal(err)
	}
	if _, err := r.HandleAccountDeletion(ctx, victim); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM social.banned_users WHERE user_id = $1`, victim).Scan(&n)
	if n != 0 {
		t.Fatalf("los baneos del usuario eliminado deben limpiarse: %d", n)
	}
}

// Dos admins que eliminan su cuenta a la vez no deben dejar el grupo sin admin
// (FOR UPDATE serializa las sucesiones).
func TestIntegrationConcurrentAdminDeletionsKeepAnAdmin(t *testing.T) {
	r, _ := testRepo(t)
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		a1, a2, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
		g := newTestGroup(t, r, a1)
		joinUser(t, r, g, a2)
		joinUser(t, r, g, member)
		if err := r.ChangeMemberRole(ctx, g.ID, a1, a2, "admin"); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for _, u := range []string{a1, a2} {
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				if _, err := r.HandleAccountDeletion(ctx, u); err != nil {
					t.Errorf("HandleAccountDeletion(%s): %v", u, err)
				}
			}(u)
		}
		wg.Wait()

		if roleIn(t, r, g.ID, member) != model.RoleAdmin {
			t.Fatalf("iteración %d: el miembro restante debe ser admin", i)
		}
	}
}

func TestIntegrationListMyGroupsDetailed(t *testing.T) {
	r, _ := testRepo(t)
	ctx := context.Background()
	admin, member := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, member)

	cards, err := r.ListMyGroupsDetailed(ctx, member)
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards = %v err=%v", cards, err)
	}
	if c := cards[0]; c.GroupID != g.ID || c.Role != model.RoleMember || c.MemberCount != 2 || c.JoinedAt.IsZero() {
		t.Fatalf("tarjeta inesperada: %+v", c)
	}
	if empty, err := r.ListMyGroupsDetailed(ctx, uuid.NewString()); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("sin grupos debe ser slice vacío no nil: %v %v", empty, err)
	}
}
