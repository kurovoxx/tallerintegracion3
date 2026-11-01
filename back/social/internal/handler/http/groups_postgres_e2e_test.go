package http

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"

	"github.com/google/uuid"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	grpchandler "github.com/kurovoxx/tallerintegracion3/back/social/internal/handler/grpc"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/interceptor"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/repository"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// Reutiliza HTTP real, JWT y PostgreSQL de Martín. No invoca Identity ni
// proveedores externos; leave usa cleanup_shared_notes=false explícitamente.
// El borrado de cuenta es gRPC (Social.AccountDeletionCleanup) desde 69acb9c:
// se verifica por bufconn contra el mismo PostgreSQL. La cobertura de esa
// operación con store en memoria ya existe en TestSocialServer; aquí se
// comprueba la persistencia real (borrado en cascada e idempotencia).
func TestGroupsPostgresE2E_MembershipLifecycle(t *testing.T) {
	e := newMartinE2EEnv(t)
	owner, member, visitor := uuid.NewString(), uuid.NewString(), uuid.NewString()
	gid, ownerToken, memberToken := e.setupGroup(owner, member)
	visitorToken := e.mintToken(visitor)
	base := "/groups/" + gid
	ctx := context.Background()

	role := func(user, want string) {
		t.Helper()
		var got string
		err := e.pool.QueryRow(ctx, `SELECT role FROM social.group_memberships WHERE group_id=$1 AND user_id=$2`, gid, user).Scan(&got)
		if err != nil || got != want {
			t.Fatalf("rol persistido: obtenido %q, esperado %q, error %v", got, want, err)
		}
	}
	count := func(query string, want int, args ...any) {
		t.Helper()
		var got int
		if err := e.pool.QueryRow(ctx, query, args...).Scan(&got); err != nil || got != want {
			t.Fatalf("conteo persistido: obtenido %d, esperado %d, error %v", got, want, err)
		}
	}
	list := func(path, token string) []map[string]any {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, e.server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := e.client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d", path, resp.StatusCode)
		}
		var rows []map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}

	role(owner, "admin")
	role(member, "member")
	if rows := list("/groups/me", memberToken); len(rows) != 1 || rows[0]["group_id"] != gid {
		t.Fatal("el miembro debe listar exactamente su grupo")
	}
	if rows := list(base+"/members", memberToken); len(rows) != 2 {
		t.Fatal("deben existir dos miembros")
	}
	e.mustStatus(http.MethodGet, base, "", nil, http.StatusUnauthorized)
	e.mustStatus(http.MethodGet, base, visitorToken, nil, http.StatusForbidden)
	oldInvite := e.mustStatus(http.MethodGet, base, ownerToken, nil, http.StatusOK)["invite_token"]
	if got := e.mustStatus(http.MethodGet, base, memberToken, nil, http.StatusOK)["invite_token"]; got != nil {
		t.Fatal("un miembro no debe recibir la invitación de administrador")
	}
	e.mustStatus(http.MethodPost, base+"/invite/regenerate", memberToken, nil, http.StatusForbidden)
	invite := e.mustStatus(http.MethodPost, base+"/invite/regenerate", ownerToken, nil, http.StatusOK)["new_invite_token"]
	if invite == nil || invite == oldInvite {
		t.Fatal("la invitación debe rotar")
	}
	e.mustStatus(http.MethodPost, base+"/join", visitorToken, map[string]any{"invite_token": oldInvite}, http.StatusNotFound)
	e.mustStatus(http.MethodPost, base+"/join", visitorToken, map[string]any{"invite_token": invite}, http.StatusCreated)
	e.mustStatus(http.MethodPost, base+"/members/"+visitor+"/kick", memberToken, nil, http.StatusForbidden)
	e.mustStatus(http.MethodPost, base+"/members/"+visitor+"/kick", ownerToken, nil, http.StatusNoContent)
	count(`SELECT count(*) FROM social.group_memberships WHERE group_id=$1 AND user_id=$2`, 0, gid, visitor)
	e.mustStatus(http.MethodPost, base+"/join", visitorToken, map[string]any{"invite_token": invite}, http.StatusCreated)
	e.mustStatus(http.MethodPost, base+"/members/"+visitor+"/ban", ownerToken, nil, http.StatusNoContent)
	count(`SELECT count(*) FROM social.banned_users WHERE group_id=$1 AND user_id=$2 AND banned_by_user_id=$3`, 1, gid, visitor, owner)
	e.mustStatus(http.MethodPost, base+"/join", visitorToken, map[string]any{"invite_token": invite}, http.StatusForbidden)
	e.mustStatus(http.MethodPatch, base+"/members/"+member+"/role", ownerToken, map[string]any{"role": "admin"}, http.StatusOK)
	role(member, "admin")
	e.mustStatus(http.MethodPatch, base+"/members/"+member+"/role", ownerToken, map[string]any{"role": "member"}, http.StatusOK)
	e.mustStatus(http.MethodPost, base+"/transfer-admin", ownerToken, map[string]any{"new_admin_user_id": member}, http.StatusOK)
	role(owner, "member")
	role(member, "admin")
	e.mustStatus(http.MethodPost, base+"/leave", memberToken, map[string]any{"cleanup_shared_notes": false}, http.StatusNoContent)
	role(owner, "admin") // sucesión al único miembro restante
	count(`SELECT count(*) FROM social.group_memberships WHERE group_id=$1 AND user_id=$2`, 0, gid, member)
	// Borrado de cuenta por gRPC: mismo GroupService con PostgreSQL servido
	// por bufconn, con la cadena de interceptores de main (logging + clave).
	const grpcKey = "test-internal-key-grupos"
	grpcSvc := service.NewGroupService(repository.NewGroupRepository(e.pool), nil)
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.ChainUnaryInterceptor(
		interceptor.NewLogging(nil),
		interceptor.NewInternalAuth(grpcKey),
	))
	socialv1.RegisterSocialServiceServer(gs, grpchandler.NewSocialServer(grpcSvc))
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(dialCtx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(dialCtx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("conectar gRPC de prueba: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	gc := socialv1.NewSocialServiceClient(conn)
	gctx := metadata.AppendToOutgoingContext(ctx, interceptor.MetadataInternalKey, grpcKey)

	resp, err := gc.AccountDeletionCleanup(gctx, &socialv1.AccountDeletionCleanupRequest{UserId: owner})
	if err != nil {
		t.Fatalf("AccountDeletionCleanup: %v", err)
	}
	if len(resp.Groups) != 1 || resp.Groups[0].GetGroupId() != gid || !resp.Groups[0].GetGroupDeleted() {
		t.Fatalf("cleanup debe borrar el grupo %s: %v", gid, resp.Groups)
	}
	count(`SELECT count(*) FROM social.groups WHERE id=$1`, 0, gid)
	count(`SELECT count(*) FROM social.banned_users WHERE group_id=$1`, 0, gid)
	again, err := gc.AccountDeletionCleanup(gctx, &socialv1.AccountDeletionCleanupRequest{UserId: owner})
	if err != nil || len(again.Groups) != 0 {
		t.Fatalf("cleanup idempotente: %v, %v", again, err)
	}
}
