//go:build clustersmoke

package test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// La suite usa los servicios ya desplegados mediante port-forward. Nunca inicia
// servidores, inicializa esquemas ni cambia el despliegue. Los fixtures evitan
// crear canales Stream y archivos Drive: no prueban los proveedores externos.
func TestClusterSmoke(t *testing.T) {
	s := newClusterSmoke(t)
	owner := s.register(t, "Administrador cluster")
	member := s.register(t, "Integrante cluster")
	outsider := s.register(t, "Externo cluster")

	t.Run("health_and_authentication", func(t *testing.T) {
		for _, base := range []string{s.auth, s.social, s.notes} {
			s.request(t, "GET", base+"/health", "", nil, 200, nil)
		}
		s.request(t, "GET", s.social+"/groups/me", "", nil, 401, nil)
		// Estos rechazos cubren las rutas sin disparar integraciones externas.
		s.request(t, "POST", s.social+"/groups", owner.token, map[string]string{}, 400, nil)
		f := s.fixture(t)
		group, _ := f.group(t, owner)
		s.request(t, "GET", s.social+"/groups/"+group+"/stream-token", outsider.token, nil, 403, nil)
	})

	t.Run("identity_enrichment_and_membership_rpc", func(t *testing.T) {
		f := s.fixture(t)
		group, invite := f.group(t, owner)
		s.join(t, member, group, invite, 201)
		var members []struct {
			UserID string `json:"user_id"`
			Name   string `json:"display_name"`
			Email  string `json:"email"`
		}
		s.request(t, "GET", s.social+"/groups/"+group+"/members", owner.token, nil, 200, &members)
		identities := map[string]string{}
		for _, m := range members {
			identities[m.UserID] = m.Name + "|" + m.Email
		}
		if len(members) != 2 || identities[owner.id] != owner.name+"|"+owner.email || identities[member.id] != member.name+"|"+member.email {
			t.Errorf("Social no enriqueció los miembros desde Identity; comprobar Service auth:9080: %+v", members)
		}
		ctx, cancel := s.rpcContext()
		membership, err := s.rpc.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: group, UserId: member.id})
		cancel()
		if err != nil || !membership.GetIsMember() || membership.GetRole() != socialv1.Role_ROLE_MEMBER {
			t.Errorf("IsMember: %v, %v", membership, err)
		}
		ctx, cancel = s.rpcContext()
		admin, err := s.rpc.IsAdmin(ctx, &socialv1.IsAdminRequest{GroupId: group, UserId: owner.id})
		cancel()
		if err != nil || !admin.GetIsAdmin() {
			t.Errorf("IsAdmin: %v, %v", admin, err)
		}
		ctx, cancel = s.rpcContext()
		rpcMembers, err := s.rpc.GetMembers(ctx, &socialv1.GetMembersRequest{GroupId: group})
		cancel()
		if err != nil || len(rpcMembers.GetMembers()) != 2 {
			t.Errorf("GetMembers: %v, %v", rpcMembers, err)
		} else {
			identities = map[string]string{}
			for _, m := range rpcMembers.Members {
				identities[m.GetUserId()] = m.GetDisplayName() + "|" + m.GetEmail()
			}
			if identities[owner.id] != owner.name+"|"+owner.email || identities[member.id] != member.name+"|"+member.email {
				t.Errorf("GetMembers no resolvió identidades; comprobar Service auth:9080: %v", rpcMembers)
			}
		}
		ctx, cancel = s.rpcContext()
		emails, err := s.rpc.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: group})
		cancel()
		if err != nil || len(emails.GetEmails()) != 2 || !clusterContains(emails.GetEmails(), owner.email) || !clusterContains(emails.GetEmails(), member.email) {
			t.Errorf("GetMemberEmails falló contra Identity; comprobar Service auth:9080: %v, %v", emails, err)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Second)
		_, err = s.rpc.IsMember(ctx, &socialv1.IsMemberRequest{GroupId: group, UserId: owner.id})
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("IsMember sin clave interna: %v", err)
		}
	})

	t.Run("invitations_roles_and_transfer", func(t *testing.T) {
		f := s.fixture(t)
		group, oldInvite := f.group(t, owner)
		base := s.social + "/groups/" + group
		s.join(t, member, group, oldInvite, 201)
		s.request(t, "GET", base, outsider.token, nil, 403, nil)
		s.request(t, "POST", base+"/invite/regenerate", member.token, nil, 403, nil)
		var invite struct {
			Token string `json:"new_invite_token"`
		}
		s.request(t, "POST", base+"/invite/regenerate", owner.token, nil, 200, &invite)
		if invite.Token == "" || invite.Token == oldInvite {
			t.Fatal("la invitación no rotó")
		}
		s.join(t, outsider, group, oldInvite, 404)
		s.join(t, outsider, group, invite.Token, 201)
		s.request(t, "PATCH", base+"/members/"+owner.id+"/role", member.token, map[string]string{"role": "member"}, 403, nil)
		roleURL := base + "/members/" + member.id + "/role"
		s.request(t, "PATCH", roleURL, owner.token, map[string]string{"role": "admin"}, 200, nil)
		s.assertRole(t, group, member, "admin", true)
		s.request(t, "PATCH", roleURL, owner.token, map[string]string{"role": "member"}, 200, nil)
		s.request(t, "POST", base+"/transfer-admin", owner.token, map[string]string{"new_admin_user_id": member.id}, 200, nil)
		s.assertRole(t, group, member, "admin", true)
		s.assertRole(t, group, owner, "member", false)
		s.request(t, "POST", base+"/invite/regenerate", owner.token, nil, 403, nil)
	})

	for _, action := range []string{"leave", "kick", "ban"} {
		t.Run("notes_cleanup_on_"+action, func(t *testing.T) {
			f := s.fixture(t)
			group, invite := f.group(t, owner)
			other, otherInvite := f.group(t, owner)
			s.join(t, member, group, invite, 201)
			s.join(t, member, other, otherInvite, 201)
			note := f.note(t, member)
			kept := f.note(t, owner)
			s.share(t, member, note, group, 201)
			s.share(t, member, note, other, 201)
			s.share(t, owner, kept, group, 201)
			s.assertGroupNotes(t, owner, group, note, kept)
			s.request(t, "GET", s.notes+"/groups/"+group+"/notes", outsider.token, nil, 403, nil)
			s.share(t, outsider, note, group, 403)
			actor, path := member, "/groups/"+group+"/leave"
			if action != "leave" {
				actor, path = owner, "/groups/"+group+"/members/"+member.id+"/"+action
				s.request(t, "POST", s.social+"/groups/"+group+"/members/"+owner.id+"/"+action, member.token, nil, 403, nil)
			}
			s.request(t, "POST", s.social+path, actor.token, map[string]bool{"cleanup_shared_notes": true}, 204, nil)
			// No se llama manualmente al fallback: debe resolverlo el Social desplegado.
			s.awaitShares(t, note, group, 0)
			s.awaitShares(t, note, other, 1)
			s.awaitShares(t, kept, group, 1)
			s.assertGroupNotes(t, owner, group, kept)
			s.request(t, "GET", s.social+"/groups/"+group, member.token, nil, 403, nil)
			s.request(t, "GET", s.notes+"/groups/"+group+"/notes", member.token, nil, 403, nil)
			s.assertGroupNotes(t, member, other, note)
			want := 201
			if action == "ban" {
				want = 403
			}
			s.join(t, member, group, invite, want)
		})
	}

	t.Run("leave_preserves_notes_and_promotes_successor", func(t *testing.T) {
		f := s.fixture(t)
		group, invite := f.group(t, owner)
		s.join(t, member, group, invite, 201)
		note := f.note(t, owner)
		s.share(t, owner, note, group, 201)
		s.request(t, "POST", s.social+"/groups/"+group+"/leave", owner.token, nil, 204, nil)
		s.awaitShares(t, note, group, 1)
		s.assertRole(t, group, member, "admin", true)
		s.assertGroupNotes(t, member, group, note)
	})

	t.Run("account_cleanup_rpc_authentication_succession_and_retry", func(t *testing.T) {
		f := s.fixture(t)
		group, invite := f.group(t, owner)
		solo, _ := f.group(t, owner)
		s.join(t, member, group, invite, 201)
		note := f.note(t, owner)
		s.share(t, owner, note, group, 201)
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		_, err := s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: owner.id})
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("AccountDeletionCleanup sin clave: %v", err)
		}
		ctx, cancel = s.rpcContext()
		res, err := s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: owner.id, UserToken: owner.token})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if len(res.GetGroups()) != 2 {
			t.Errorf("cleanup debe procesar sólo los dos grupos temporales: %v", res)
		}
		promoted, deleted := false, false
		for _, g := range res.GetGroups() {
			promoted = promoted || (g.GetGroupId() == group && g.GetPromotedUserId() == member.id && !g.GetGroupDeleted())
			deleted = deleted || (g.GetGroupId() == solo && g.GetGroupDeleted())
		}
		if !promoted || !deleted {
			t.Errorf("cleanup no promovió al sucesor o no borró el grupo vacío: %v", res)
		}
		s.awaitShares(t, note, group, 0)
		s.assertRole(t, group, member, "admin", true)
		s.request(t, "GET", s.social+"/groups/"+solo, owner.token, nil, 404, nil)
		ctx, cancel = s.rpcContext()
		res, err = s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: owner.id, UserToken: owner.token})
		cancel()
		if err != nil || len(res.GetGroups()) != 0 {
			t.Fatalf("cleanup no es idempotente: %v, %v", res, err)
		}
	})

	t.Run("todo_sprint_hours_discord_and_views", func(t *testing.T) {
		f := s.fixture(t)
		group, invite := f.group(t, owner)
		s.join(t, member, group, invite, 201)
		base := s.social + "/groups/" + group
		type item struct {
			ID     string  `json:"id"`
			Title  string  `json:"title"`
			Name   string  `json:"name"`
			Status string  `json:"status"`
			Hours  float64 `json:"hours"`
		}
		var todo struct{ Data item }
		s.request(t, "POST", base+"/todo", member.token, map[string]string{"title": "Cluster temporal", "assigned_to": member.id, "status": "todo"}, 201, &todo)
		if todo.Data.ID == "" {
			t.Fatal("TODO sin id")
		}
		todoURL := base + "/todo/" + todo.Data.ID
		s.request(t, "PATCH", todoURL, member.token, map[string]string{"title": "Cluster verificado", "status": "done"}, 200, &todo)
		var todos struct{ Data []item }
		s.request(t, "GET", base+"/todo?status=done", member.token, nil, 200, &todos)
		if len(todos.Data) != 1 || todos.Data[0].ID != todo.Data.ID || todos.Data[0].Title != "Cluster verificado" {
			t.Fatal("TODO no persistió sus cambios")
		}
		var sheet, task struct{ Data item }
		periodStart := time.Now().UTC().Format("2006-01-02")
		periodEnd := time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02")
		sheetBody := map[string]string{"name": "Cluster temporal", "period_start": periodStart, "period_end": periodEnd}
		s.request(t, "POST", base+"/sprint-sheets", owner.token, sheetBody, 201, &sheet)
		if sheet.Data.ID == "" {
			t.Fatal("hoja sin id")
		}
		sheetURL := base + "/sprint-sheets/" + sheet.Data.ID
		s.request(t, "PATCH", sheetURL, member.token, map[string]string{"name": "Cluster verificado"}, 200, &sheet)
		var sheets struct{ Data []item }
		s.request(t, "GET", base+"/sprint-sheets", member.token, nil, 200, &sheets)
		if len(sheets.Data) != 1 || sheets.Data[0].ID != sheet.Data.ID || sheets.Data[0].Name != "Cluster verificado" {
			t.Fatal("hoja no persistió sus cambios")
		}
		s.request(t, "POST", base+"/sprint-sheet", member.token, map[string]any{"title": "Prueba temporal", "sheet_id": sheet.Data.ID, "assigned_to": member.id, "priority": "media", "status": "sin_empezar", "estimated_hours": 2}, 201, &task)
		if task.Data.ID == "" {
			t.Fatal("tarea sprint sin id")
		}
		taskURL := base + "/sprint-sheet/" + task.Data.ID
		s.request(t, "PATCH", taskURL, member.token, map[string]string{"status": "en_proceso"}, 200, &task)
		var tasks struct{ Data []item }
		s.request(t, "GET", base+"/sprint-sheet?sheet_id="+sheet.Data.ID, member.token, nil, 200, &tasks)
		if len(tasks.Data) != 1 || tasks.Data[0].ID != task.Data.ID || tasks.Data[0].Status != "en_proceso" {
			t.Fatal("tarea sprint no persistió sus cambios")
		}
		hoursURL := s.social + "/sprint-sheet/" + task.Data.ID + "/hours"
		date := time.Now().UTC().Format("2006-01-02")
		s.request(t, "POST", hoursURL, member.token, map[string]any{"log_date": date, "hours": 1}, 201, nil)
		s.request(t, "POST", hoursURL, member.token, map[string]any{"log_date": date, "hours": 2.5}, 200, nil)
		var hours struct {
			Data  []item  `json:"data"`
			Total float64 `json:"total_hours"`
		}
		s.request(t, "GET", hoursURL, member.token, nil, 200, &hours)
		if len(hours.Data) != 1 || hours.Total != 2.5 || hours.Data[0].Hours != 2.5 {
			t.Fatal("upsert de horas duplicó filas o perdió el total")
		}
		discordBody := map[string]string{"server_name": "Prueba cluster", "invite_url": "https://discord.gg/cluster-smoke"}
		s.request(t, "PUT", base+"/discord-config", member.token, discordBody, 403, nil)
		s.request(t, "PUT", base+"/discord-config", owner.token, discordBody, 200, nil)
		var discord struct {
			Server  string  `json:"server_name"`
			Invite  string  `json:"invite_url"`
			Webhook *string `json:"webhook_url"`
		}
		s.request(t, "GET", base+"/discord-config", member.token, nil, 200, &discord)
		if discord.Server != discordBody["server_name"] || discord.Invite != discordBody["invite_url"] || discord.Webhook != nil {
			t.Fatal("Discord no persistió configuración sin webhook")
		}
		var workspace model.Workspace
		s.request(t, "GET", base+"/workspace", member.token, nil, 200, &workspace)
		if workspace.Group.ID != group || workspace.Group.Role != "member" || len(workspace.Kanban.Done) != 1 || len(workspace.SprintSheet.Tasks) != 1 {
			t.Fatal("workspace no refleja TODO y sprint")
		}
		var overview model.MyOverview
		s.request(t, "GET", s.social+"/me/overview", member.token, nil, 200, &overview)
		if overview.UserID != member.id || len(overview.Groups) != 1 || overview.Groups[0].GroupID != group || overview.Groups[0].MemberCount != 2 {
			t.Fatal("overview no refleja el grupo temporal")
		}
		for _, path := range []string{"/todo", "/sprint-sheets", "/sprint-sheet", "/workspace", "/discord-config"} {
			s.request(t, "GET", base+path, outsider.token, nil, 403, nil)
		}
		s.request(t, "GET", hoursURL, outsider.token, nil, 403, nil)
		s.request(t, "DELETE", todoURL, member.token, nil, 200, nil)
		s.request(t, "DELETE", taskURL, member.token, nil, 200, nil)
		s.request(t, "GET", hoursURL, member.token, nil, 404, nil)
		// La regla de negocio impide eliminar la última hoja del grupo.
		s.request(t, "DELETE", sheetURL, member.token, nil, 400, nil)
		var retained struct{ Data item }
		sheetBody["name"] = "Hoja restante"
		s.request(t, "POST", base+"/sprint-sheets", owner.token, sheetBody, 201, &retained)
		if retained.Data.ID == "" {
			t.Fatal("segunda hoja sin id")
		}
		s.request(t, "DELETE", sheetURL, member.token, nil, 200, nil)
		s.request(t, "GET", base+"/todo", member.token, nil, 200, &todos)
		s.request(t, "GET", base+"/sprint-sheet", member.token, nil, 200, &tasks)
		s.request(t, "GET", base+"/sprint-sheets", member.token, nil, 200, &sheets)
		if len(todos.Data)+len(tasks.Data) != 0 || len(sheets.Data) != 1 || sheets.Data[0].ID != retained.Data.ID || sheets.Data[0].Name != "Hoja restante" {
			t.Fatal("quedaron recursos después de DELETE")
		}
	})
}

type clusterSmokeUser struct{ id, email, name, token string }

type clusterSmoke struct {
	pool                *pgxpool.Pool
	auth, social, notes string
	key                 string
	rpc                 socialv1.SocialServiceClient
	client              *http.Client
	users               []*clusterSmokeUser
}

type clusterSmokeFixture struct {
	s             *clusterSmoke
	groups, notes []string
}

func newClusterSmoke(t *testing.T) *clusterSmoke {
	t.Helper()
	if os.Getenv("CLUSTER_TEST_CONFIRM") != "student-brojas" {
		t.Fatal("se requiere CLUSTER_TEST_CONFIRM=student-brojas para probar el despliegue existente")
	}
	dsn := os.Getenv("CLUSTER_TEST_DATABASE_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if dsn == "" || err != nil {
		t.Fatal("CLUSTER_TEST_DATABASE_URL debe contener el DSN del port-forward a Postgres")
	}
	if !clusterLoopback(cfg.ConnConfig.Host) {
		t.Fatal("Postgres debe usar un host loopback mediante port-forward")
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if !clusterLoopback(fallback.Host) {
			t.Fatal("ningún host fallback de Postgres puede salir de loopback")
		}
	}
	front := strings.TrimRight(os.Getenv("CLUSTER_TEST_FRONT_URL"), "/")
	if front == "" {
		front = "http://127.0.0.1:18086"
	}
	u, err := url.Parse(front)
	if err != nil || u.Scheme != "http" || !clusterLoopback(u.Hostname()) || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		t.Fatal("CLUSTER_TEST_FRONT_URL debe ser el origen HTTP loopback del port-forward, sin credenciales ni ruta")
	}
	addr := os.Getenv("CLUSTER_TEST_SOCIAL_GRPC_ADDR")
	if addr == "" {
		addr = "127.0.0.1:19083"
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || !clusterLoopback(host) {
		t.Fatal("CLUSTER_TEST_SOCIAL_GRPC_ADDR debe ser host:puerto loopback")
	}
	key := os.Getenv("CLUSTER_TEST_INTERNAL_KEY")
	if key == "" {
		t.Fatal("se requiere CLUSTER_TEST_INTERNAL_KEY; no escribir su valor en logs")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("no se pudo abrir el port-forward de Postgres")
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("Postgres no responde por el port-forward")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal("no se pudo abrir el cliente gRPC de Social")
	}
	t.Cleanup(func() { _ = conn.Close() })
	s := &clusterSmoke{pool: pool, auth: front + "/api-auth", social: front + "/api-social", notes: front + "/api-notes", key: key, rpc: socialv1.NewSocialServiceClient(conn), client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	t.Cleanup(func() { s.cleanupUsers(t) })
	return s
}

func clusterLoopback(host string) bool {
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func (s *clusterSmoke) rpcContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", s.key), cancel
}

func (s *clusterSmoke) request(t *testing.T, method, target, token string, body any, want int, out any) {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, target, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, target, err)
	}
	defer res.Body.Close()
	data, err = io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d, esperado %d: %s", method, target, res.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("%s %s: respuesta JSON inválida", method, target)
		}
	}
}

func (s *clusterSmoke) register(t *testing.T, name string) *clusterSmokeUser {
	t.Helper()
	u := &clusterSmokeUser{email: uuid.NewString() + "@example.invalid", name: name}
	s.users = append(s.users, u)
	body := map[string]string{"email": u.email, "password": uuid.NewString() + "Aa1!", "display_name": name}
	var registered struct{ ID string }
	s.request(t, "POST", s.auth+"/auth/register", "", body, 201, &registered)
	if _, err := uuid.Parse(registered.ID); err != nil {
		t.Fatal("Identity no devolvió UUID de registro")
	}
	u.id = registered.ID
	var login struct {
		Token string `json:"access_token"`
	}
	s.request(t, "POST", s.auth+"/auth/login", "", body, 200, &login)
	if login.Token == "" {
		t.Fatal("Identity no emitió JWT")
	}
	u.token = login.Token
	return u
}

func (s *clusterSmoke) fixture(t *testing.T) *clusterSmokeFixture {
	t.Helper()
	f := &clusterSmokeFixture{s: s}
	t.Cleanup(func() { f.cleanup(t) })
	return f
}

func (f *clusterSmokeFixture) group(t *testing.T, owner *clusterSmokeUser) (string, string) {
	t.Helper()
	id, invite := uuid.NewString(), uuid.NewString()
	f.groups = append(f.groups, id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := f.s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, "INSERT INTO social.groups(id,name,owner_user_id,invite_token) VALUES($1,$2,$3,$4)", id, "Cluster smoke "+id, owner.id, invite); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "INSERT INTO social.group_memberships(group_id,user_id,role) VALUES($1,$2,'admin')", id, owner.id); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return id, invite
}

func (f *clusterSmokeFixture) note(t *testing.T, owner *clusterSmokeUser) string {
	t.Helper()
	id := uuid.NewString()
	f.notes = append(f.notes, id)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.s.pool.Exec(ctx, "INSERT INTO notes.notes(id,user_id,title,visibility,external_file_id,sync_status) VALUES($1,$2,$3,'private',NULL,'synced')", id, owner.id, "Cluster smoke "+id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (s *clusterSmoke) join(t *testing.T, user *clusterSmokeUser, group, invite string, want int) {
	t.Helper()
	s.request(t, "POST", s.social+"/groups/"+group+"/join", user.token, map[string]string{"invite_token": invite}, want, nil)
}

func (s *clusterSmoke) share(t *testing.T, user *clusterSmokeUser, note, group string, want int) {
	t.Helper()
	s.request(t, "POST", s.notes+"/notes/"+note+"/share", user.token, map[string]string{"group_id": group, "access_mode": "restricted"}, want, nil)
}

func (s *clusterSmoke) assertRole(t *testing.T, group string, user *clusterSmokeUser, want string, invite bool) {
	t.Helper()
	var view struct {
		Role   string `json:"role"`
		Invite string `json:"invite_token"`
	}
	s.request(t, "GET", s.social+"/groups/"+group, user.token, nil, 200, &view)
	if view.Role != want || (view.Invite != "") != invite {
		t.Fatalf("rol/invitación incorrectos: rol=%q invitación_visible=%t", view.Role, view.Invite != "")
	}
}

func (s *clusterSmoke) assertGroupNotes(t *testing.T, user *clusterSmokeUser, group string, want ...string) {
	t.Helper()
	var list struct{ Notes []struct{ ID string } }
	s.request(t, "GET", s.notes+"/groups/"+group+"/notes", user.token, nil, 200, &list)
	ids := []string{}
	for _, n := range list.Notes {
		ids = append(ids, n.ID)
	}
	if len(ids) != len(want) {
		t.Fatalf("notas de grupo: %v, esperadas %v", ids, want)
	}
	for _, id := range want {
		if !clusterContains(ids, id) {
			t.Fatalf("nota temporal %s ausente de su grupo", id)
		}
	}
}

func (s *clusterSmoke) awaitShares(t *testing.T, note, group string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	for {
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM notes.shared_notes WHERE note_id=$1 AND group_id=$2", note, group).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("Social no retiró los shares del fixture: count=%d, esperado=%d; revisar Notes gRPC y fallback HTTP del despliegue", count, want)
		case <-time.After(150 * time.Millisecond):
		}
	}
}

func (f *clusterSmokeFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	steps := []struct {
		query string
		ids   []string
	}{
		{"DELETE FROM notes.drive_reconciliation_queue WHERE note_id=ANY($1::uuid[])", f.notes},
		{"DELETE FROM notes.drive_managed_permissions WHERE note_id=ANY($1::uuid[])", f.notes},
		{"DELETE FROM notes.idempotency_keys WHERE resource_id=ANY($1::uuid[])", f.notes},
		{"DELETE FROM notes.notes WHERE id=ANY($1::uuid[])", f.notes},
		{"DELETE FROM notes.shared_notes WHERE group_id=ANY($1::uuid[])", f.groups},
		{"DELETE FROM social.groups WHERE id=ANY($1::uuid[])", f.groups},
	}
	for _, step := range steps {
		if len(step.ids) == 0 {
			continue
		}
		if _, err := f.s.pool.Exec(ctx, step.query, step.ids); err != nil {
			t.Errorf("cleanup acotado falló: %s: %v", step.query, err)
		}
	}
	checks := []struct {
		query string
		ids   []string
	}{
		{"SELECT count(*) FROM notes.notes WHERE id=ANY($1::uuid[])", f.notes},
		{"SELECT count(*) FROM notes.shared_notes WHERE note_id=ANY($1::uuid[])", f.notes},
		{"SELECT count(*) FROM notes.drive_reconciliation_queue WHERE note_id=ANY($1::uuid[])", f.notes},
		{"SELECT count(*) FROM notes.drive_managed_permissions WHERE note_id=ANY($1::uuid[])", f.notes},
		{"SELECT count(*) FROM notes.idempotency_keys WHERE resource_id=ANY($1::uuid[])", f.notes},
		{"SELECT count(*) FROM social.groups WHERE id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.group_memberships WHERE group_id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.banned_users WHERE group_id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.todo_boards WHERE group_id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.sprint_sheets WHERE group_id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.discord_integrations WHERE group_id=ANY($1::uuid[])", f.groups},
		{"SELECT count(*) FROM social.stream_channels WHERE group_id=ANY($1::uuid[])", f.groups},
	}
	for _, check := range checks {
		if len(check.ids) == 0 {
			continue
		}
		var count int
		if err := f.s.pool.QueryRow(ctx, check.query, check.ids).Scan(&count); err != nil || count != 0 {
			t.Errorf("restos de fixture: %s: count=%d err=%v", check.query, count, err)
		}
	}
}

func (s *clusterSmoke) cleanupUsers(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, u := range s.users {
		// Recupera sólo el correo aleatorio de este run si HTTP falló después de
		// persistir el registro; no se inspeccionan cuentas preexistentes.
		var ids []string
		rows, err := s.pool.Query(ctx, "SELECT id::text FROM identity.users WHERE email=$1", u.email)
		if err != nil {
			t.Errorf("cleanup: no se pudo localizar la cuenta temporal %s: %v", u.email, err)
			continue
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				t.Error(err)
				continue
			}
			ids = append(ids, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Error(err)
			continue
		}
		for _, id := range ids {
			if u.id != "" && u.id != id {
				t.Errorf("UUID de cuenta temporal no corresponde al registro: %s", u.email)
				continue
			}
			if _, err := s.pool.Exec(ctx, "DELETE FROM identity.users WHERE id=$1 AND email=$2", id, u.email); err != nil {
				t.Errorf("cleanup de cuenta temporal: %v", err)
			}
			for _, table := range []string{"identity.users", "identity.profiles", "identity.refresh_tokens", "identity.oauth_connections", "identity.password_resets"} {
				column := "user_id"
				if table == "identity.users" {
					column = "id"
				}
				var count int
				query := fmt.Sprintf("SELECT count(*) FROM %s WHERE %s=$1", table, column)
				if err := s.pool.QueryRow(ctx, query, id).Scan(&count); err != nil || count != 0 {
					t.Errorf("restos de cuenta temporal en %s: count=%d err=%v", table, count, err)
				}
			}
		}
	}
}

func clusterContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
