package repository

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type groupQueryGate struct {
	statement string
	reached   chan struct{}
	release   chan struct{}
}

type groupQueryGateKey struct{}

func (g *groupQueryGate) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, g.statement) {
		return context.WithValue(ctx, groupQueryGateKey{}, true)
	}
	return ctx
}

func (g *groupQueryGate) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if ctx.Value(groupQueryGateKey{}) == true {
		close(g.reached)
		select {
		case <-g.release:
		case <-ctx.Done():
		}
	}
}

func concurrentGroupRepo(t *testing.T, appName string, tracer pgx.QueryTracer) *GroupRepository {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("SOCIAL_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = appName
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewGroupRepository(pool)
}

func waitGroupQueryBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, appName string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var blocked bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE application_name = $1 AND wait_event_type = 'Lock'
		)`, appName).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("la operación concurrente no esperó el lock del grupo")
		case <-ticker.C:
		}
	}
}

func TestIntegrationConcurrentBanPreventsInviteReentry(t *testing.T) {
	r, pool := testRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, member := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, member)

	gate := &groupQueryGate{statement: "INSERT INTO social.banned_users", reached: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(gate.release) })
	defer release()
	banRepo := concurrentGroupRepo(t, "ban-"+uuid.NewString(), gate)
	joinApp := "join-" + uuid.NewString()
	joinRepo := concurrentGroupRepo(t, joinApp, nil)
	banned := make(chan error, 1)
	go func() { banned <- banRepo.BanMember(ctx, g.ID, admin, member) }()
	select {
	case <-gate.reached:
	case <-ctx.Done():
		t.Fatal("no se alcanzó el baneo pendiente de commit")
	}
	joined := make(chan error, 1)
	go func() { joined <- joinRepo.JoinWithInviteToken(ctx, g.ID, g.InviteToken, member) }()
	waitGroupQueryBlocked(t, ctx, pool, joinApp)
	release()
	if err := <-banned; err != nil {
		t.Fatalf("ban: %v", err)
	}
	wantStoreErr(t, <-joined, "user_banned")
	if roleIn(t, r, g.ID, member) != "" {
		t.Fatal("el baneo concurrente no debe dejar al usuario como miembro")
	}
}

func TestIntegrationConcurrentTransferRevokesInviteRegeneration(t *testing.T) {
	r, pool := testRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, member := uuid.NewString(), uuid.NewString()
	g := newTestGroup(t, r, admin)
	joinUser(t, r, g, member)

	gate := &groupQueryGate{statement: "UPDATE social.group_memberships SET role = 'member'", reached: make(chan struct{}), release: make(chan struct{})}
	release := sync.OnceFunc(func() { close(gate.release) })
	defer release()
	transferRepo := concurrentGroupRepo(t, "transfer-"+uuid.NewString(), gate)
	inviteApp := "invite-" + uuid.NewString()
	inviteRepo := concurrentGroupRepo(t, inviteApp, nil)
	transferred := make(chan error, 1)
	go func() { transferred <- transferRepo.TransferAdmin(ctx, g.ID, admin, member) }()
	select {
	case <-gate.reached:
	case <-ctx.Done():
		t.Fatal("no se alcanzó la transferencia pendiente de commit")
	}
	regenerated := make(chan error, 1)
	go func() {
		_, err := inviteRepo.RegenerateInviteToken(ctx, g.ID, admin)
		regenerated <- err
	}()
	waitGroupQueryBlocked(t, ctx, pool, inviteApp)
	release()
	if err := <-transferred; err != nil {
		t.Fatalf("transfer: %v", err)
	}
	wantStoreErr(t, <-regenerated, "forbidden")
	updated, err := r.GetByID(ctx, g.ID)
	if err != nil || updated.InviteToken != g.InviteToken {
		t.Fatalf("un ex-admin no debe modificar la invitación: %v", err)
	}
}
