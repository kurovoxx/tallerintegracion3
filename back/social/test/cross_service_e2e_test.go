//go:build crossservice

// La suite arranca los binarios reales, sin importar otros módulos ni simular
// Social/Identity/Notes. Solo Drive, Calendar y Stream usan el modo mock.
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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	socialv1 "github.com/kurovoxx/tallerintegracion3/back/social/internal/gen/socialv1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const internalKey = "cross-service-test-key"

type testUser struct{ id, token, email, name string }

type stack struct {
	pool                          *pgxpool.Pool
	auth, social, notes, academic string
	rpc                           socialv1.SocialServiceClient
	client                        *http.Client
	processes                     []*stackProcess
}

type stackProcess struct {
	name, binary, dir, base string
	port, grpcPort          string
	grpcEnvVar              string
	env                     []string
	cmd                     *exec.Cmd
	logFile                 *os.File
	logPaths                []string
}

func (p *stackProcess) start(t *testing.T, client *http.Client) {
	t.Helper()
	logPath := filepath.Join(p.dir, fmt.Sprintf("server-%d.log", len(p.logPaths)+1))
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(p.binary)
	cmd.Dir = p.dir // impide cargar los .env del repositorio y sus credenciales
	cmd.Env = append(append([]string{}, p.env...), "PORT="+p.port, p.grpcEnvVar+"="+p.grpcPort)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	p.cmd, p.logFile = cmd, logFile
	p.logPaths = append(p.logPaths, logPath)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		p.cmd, p.logFile = nil, nil
		t.Fatalf("iniciar %s: %v", p.name, err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		res, err := client.Get(p.base + "/health")
		if err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s no inició: %v", p.name, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (p *stackProcess) stop() {
	if p.cmd != nil && p.cmd.Process != nil && p.cmd.ProcessState == nil {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	}
	p.cmd = nil
	if p.logFile != nil {
		_ = p.logFile.Close()
		p.logFile = nil
	}
}

func (p *stackProcess) dumpLogs(t *testing.T) {
	t.Helper()
	for i, logPath := range p.logPaths {
		output, err := os.ReadFile(logPath)
		if err == nil {
			t.Logf("%s generación %d:\n%s", p.name, i+1, output)
		}
	}
}

func (s *stack) restartService(t *testing.T, name string) {
	t.Helper()
	for _, process := range s.processes {
		if process.name == name {
			process.stop()
			process.start(t, s.client)
			return
		}
	}
	t.Fatalf("servicio %q no encontrado para reiniciar", name)
}

func TestCrossServiceMembershipLifecycle(t *testing.T) {
	s := startStack(t)
	owner := s.register(t, "Administrador")
	writer := s.register(t, "Autor")
	outsider := s.register(t, "Externo")
	t.Run("social_http_routes", func(t *testing.T) { testSocialRoutes(t, s) })

	for _, action := range []string{"leave", "kick", "ban"} {
		t.Run(action, func(t *testing.T) {
			group, invite := s.group(t, owner)
			other, otherInvite := s.group(t, owner)
			s.join(t, writer, group, invite, http.StatusCreated)
			s.join(t, writer, other, otherInvite, http.StatusCreated)

			// Social obtiene nombres/correos del Identity gRPC real.
			var members []struct {
				UserID string `json:"user_id"`
				Name   string `json:"display_name"`
				Email  string `json:"email"`
			}
			s.request(t, "GET", s.social+"/groups/"+group+"/members", owner.token, nil, 200, &members)
			found := false
			for _, m := range members {
				if m.UserID == writer.id {
					found = m.Name == writer.name && m.Email == writer.email
				}
			}
			if !found {
				t.Fatalf("Identity no enriqueció al autor: %+v", members)
			}

			ctx, cancel := rpcContext()
			got, err := s.rpc.GetMemberEmails(ctx, &socialv1.GetMemberEmailsRequest{GroupId: group})
			cancel()
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Emails) != 2 {
				t.Fatalf("correos reales de Identity: %v", got.Emails)
			}

			note := s.note(t, writer)
			kept := s.note(t, owner)
			s.share(t, writer, note, group, 201)
			s.share(t, writer, note, other, 201)
			s.share(t, owner, kept, group, 201)
			s.request(t, "GET", s.notes+"/groups/"+group+"/notes", outsider.token, nil, 403, nil)
			s.share(t, outsider, note, group, 403)

			path, actor := "/groups/"+group+"/leave", writer
			if action != "leave" {
				path, actor = "/groups/"+group+"/members/"+writer.id+"/"+action, owner
				// Un miembro no puede expulsar/bloquear al administrador.
				s.request(t, "POST", s.social+"/groups/"+group+"/members/"+owner.id+"/"+action, writer.token, nil, 403, nil)
			}
			s.request(t, "POST", s.social+path, actor.token, map[string]bool{"cleanup_shared_notes": true}, 204, nil)
			s.awaitShares(t, note, group, 0)
			s.awaitShares(t, note, other, 1)
			s.awaitShares(t, kept, group, 1)
			s.request(t, "GET", s.social+"/groups/"+group, writer.token, nil, 403, nil)
			s.request(t, "GET", s.notes+"/groups/"+group+"/notes", writer.token, nil, 403, nil)
			s.request(t, "GET", s.notes+"/groups/"+other+"/notes", writer.token, nil, 200, nil)
			want := http.StatusCreated
			if action == "ban" {
				want = http.StatusForbidden
			}
			s.join(t, writer, group, invite, want)
		})
	}

	t.Run("leave_without_cleanup_and_admin_succession", func(t *testing.T) {
		group, invite := s.group(t, owner)
		s.join(t, writer, group, invite, 201)
		note := s.note(t, owner)
		s.share(t, owner, note, group, 201)
		s.request(t, "POST", s.social+"/groups/"+group+"/leave", owner.token, nil, 204, nil)
		s.awaitShares(t, note, group, 1)
		var view struct {
			Role string `json:"role"`
		}
		s.request(t, "GET", s.social+"/groups/"+group, writer.token, nil, 200, &view)
		if view.Role != "admin" {
			t.Fatalf("sucesor = %q", view.Role)
		}
	})

	t.Run("account_cleanup_rpc_is_authenticated_and_idempotent", func(t *testing.T) {
		deleted := s.register(t, "Cuenta a limpiar")
		group, invite := s.group(t, deleted)
		s.join(t, writer, group, invite, 201)
		note := s.note(t, deleted)
		s.share(t, deleted, note, group, 201)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: deleted.id})
		cancel()
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("sin clave: %v", err)
		}
		ctx, cancel = rpcContext()
		res, err := s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: deleted.id, UserToken: deleted.token})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Groups) != 1 || res.Groups[0].GetPromotedUserId() != writer.id {
			t.Fatalf("sucesión: %v", res)
		}
		s.awaitShares(t, note, group, 0)
		ctx, cancel = rpcContext()
		res, err = s.rpc.AccountDeletionCleanup(ctx, &socialv1.AccountDeletionCleanupRequest{UserId: deleted.id, UserToken: deleted.token})
		cancel()
		if err != nil || len(res.GetGroups()) != 0 {
			t.Fatalf("reintento: %v, %v", res, err)
		}
	})
}

func TestCrossServiceAccountDeletionRetriesPartialDriveCleanup(t *testing.T) {
	// Notes retries each Drive operation three times; fail exactly those
	// attempts so this request remains pending and the next DELETE can recover.
	s := startStackWithEnv(t, []string{"NOTES_TEST_DRIVE_REVOKE_FAILURES=3"})
	target := s.register(t, "Cuenta de prueba")
	successor := s.register(t, "Sucesor")
	author := s.register(t, "Autor externo")
	for _, user := range []testUser{target, successor, author} {
		if _, err := s.pool.Exec(context.Background(), `
			INSERT INTO identity.oauth_connections(user_id,provider,access_token,refresh_token,external_account_email)
			VALUES($1::uuid,'google_drive','synthetic-access','synthetic-refresh',$2)
		`, user.id, user.email); err != nil {
			t.Fatalf("create synthetic OAuth row: %v", err)
		}
	}
	sharedGroup, sharedInvite := s.group(t, target)
	lastGroup, _ := s.group(t, target)
	s.join(t, successor, sharedGroup, sharedInvite, http.StatusCreated)
	s.join(t, author, sharedGroup, sharedInvite, http.StatusCreated)

	foreignNote := s.note(t, author)
	s.share(t, author, foreignNote, sharedGroup, http.StatusCreated)
	lastMemberNote := s.note(t, target)
	s.share(t, target, lastMemberNote, lastGroup, http.StatusCreated)

	var pending struct {
		Status string `json:"status"`
	}
	s.request(t, http.MethodDelete, s.auth+"/auth/account", target.token, nil, http.StatusAccepted, &pending)
	if pending.Status != "pending" {
		t.Fatalf("first deletion status=%q, want pending after injected Drive failure", pending.Status)
	}
	var oauthCount, identityCount int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1::uuid`, target.id).Scan(&oauthCount); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users WHERE id=$1::uuid`, target.id).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if oauthCount != 1 || identityCount != 1 {
		t.Fatalf("OAuth/user rows removed before remote cleanup: oauth=%d identity=%d", oauthCount, identityCount)
	}

	// Old JWTs are fenced as soon as cleanup starts; another account also
	// cannot change the deleting user's membership/admin state.
	s.request(t, http.MethodGet, s.auth+"/auth/me", target.token, nil, http.StatusLocked, nil)
	s.request(t, http.MethodPost, s.social+"/groups", target.token, map[string]string{"name": "must be blocked"}, http.StatusLocked, nil)
	s.request(t, http.MethodPatch, s.social+"/groups/"+sharedGroup+"/members/"+target.id+"/role", successor.token, map[string]string{"role": "member"}, http.StatusForbidden, nil)

	// Identity must recover the pending saga from PostgreSQL after its process
	// restarts; OAuth remains available to the still-running Notes cleanup.
	s.restartService(t, "auth")
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1::uuid`, target.id).Scan(&oauthCount); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users WHERE id=$1::uuid`, target.id).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if oauthCount != 1 || identityCount != 1 {
		t.Fatalf("restart lost pending Identity/OAuth state: oauth=%d identity=%d", oauthCount, identityCount)
	}
	var persistedStatus string
	if err := s.pool.QueryRow(context.Background(), `SELECT status FROM identity.account_deletions WHERE user_id=$1::uuid`, target.id).Scan(&persistedStatus); err != nil {
		t.Fatal(err)
	}
	if persistedStatus != "pending" {
		t.Fatalf("persisted deletion status after restart=%q, want pending", persistedStatus)
	}

	var completed struct {
		Status string `json:"status"`
	}
	s.request(t, http.MethodDelete, s.auth+"/auth/account", target.token, nil, http.StatusOK, &completed)
	if completed.Status != "complete" {
		t.Fatalf("retry deletion status=%q, want complete", completed.Status)
	}
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1::uuid`, target.id).Scan(&oauthCount); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users WHERE id=$1::uuid`, target.id).Scan(&identityCount); err != nil {
		t.Fatal(err)
	}
	if oauthCount != 0 || identityCount != 0 {
		t.Fatalf("Identity rows remain after confirmed cleanup: oauth=%d identity=%d", oauthCount, identityCount)
	}
	s.request(t, http.MethodGet, s.auth+"/auth/me", target.token, nil, http.StatusLocked, nil)
	s.request(t, http.MethodGet, s.social+"/groups/"+sharedGroup, target.token, nil, http.StatusLocked, nil)
	s.request(t, http.MethodGet, s.notes+"/groups/"+sharedGroup+"/notes", target.token, nil, http.StatusLocked, nil)
	var foreignShares, lastShares, targetACL, pendingACL, lastGroupCount, pendingGroupTasks, pendingNoteTasks int
	queries := []struct {
		query string
		args  []any
		to    *int
	}{
		{`SELECT count(*) FROM notes.shared_notes WHERE note_id=$1::uuid AND group_id=$2::uuid`, []any{foreignNote, sharedGroup}, &foreignShares},
		{`SELECT count(*) FROM notes.shared_notes WHERE note_id=$1::uuid AND group_id=$2::uuid`, []any{lastMemberNote, lastGroup}, &lastShares},
		{`SELECT count(*) FROM notes.drive_managed_permissions WHERE note_id=$1::uuid AND principal_type='email' AND lower(principal_key)=lower($2)`, []any{foreignNote, target.email}, &targetACL},
		{`SELECT count(*) FROM notes.drive_managed_permissions WHERE note_id=$1::uuid AND sync_status<>'in_sync'`, []any{foreignNote}, &pendingACL},
		{`SELECT count(*) FROM social.groups WHERE id=$1::uuid`, []any{lastGroup}, &lastGroupCount},
		{`SELECT count(*) FROM social.account_deletion_groups WHERE user_id=$1::uuid AND status<>'complete'`, []any{target.id}, &pendingGroupTasks},
		{`SELECT count(*) FROM notes.account_deletion_group_notes WHERE user_id=$1::uuid`, []any{target.id}, &pendingNoteTasks},
	}
	for _, q := range queries {
		if err := s.pool.QueryRow(context.Background(), q.query, q.args...).Scan(q.to); err != nil {
			t.Fatal(err)
		}
	}
	if foreignShares != 1 || lastShares != 0 || targetACL != 0 || pendingACL != 0 || lastGroupCount != 0 || pendingGroupTasks != 0 || pendingNoteTasks != 0 {
		t.Fatalf("post-cleanup state foreignShares=%d lastShares=%d targetACL=%d pendingACL=%d lastGroup=%d pendingSocialTasks=%d pendingNotesTasks=%d", foreignShares, lastShares, targetACL, pendingACL, lastGroupCount, pendingGroupTasks, pendingNoteTasks)
	}
	var successorView struct {
		Role string `json:"role"`
	}
	s.request(t, http.MethodGet, s.social+"/groups/"+sharedGroup, successor.token, nil, http.StatusOK, &successorView)
	if successorView.Role != "admin" {
		t.Fatalf("successor role=%q, want admin", successorView.Role)
	}
	var sharedNotes struct {
		Notes []struct {
			ID string `json:"id"`
		} `json:"notes"`
	}
	s.request(t, http.MethodGet, s.notes+"/groups/"+sharedGroup+"/notes", successor.token, nil, http.StatusOK, &sharedNotes)
	foundForeign := false
	for _, note := range sharedNotes.Notes {
		if note.ID == foreignNote {
			foundForeign = true
		}
	}
	if !foundForeign {
		t.Fatalf("foreign author's shared note disappeared from surviving group: %+v", sharedNotes.Notes)
	}
}

func TestCrossServiceAccountDeletionCompletesNormallyAndCanRepeat(t *testing.T) {
	s := startStack(t)
	target := s.register(t, "Cuenta a eliminar")
	survivor := s.register(t, "Miembro que conserva acceso")
	for _, user := range []testUser{target, survivor} {
		if _, err := s.pool.Exec(context.Background(), `
			INSERT INTO identity.oauth_connections(user_id,provider,access_token,refresh_token,external_account_email)
			VALUES($1::uuid,'google_drive','synthetic-access','synthetic-refresh',$2)
		`, user.id, user.email); err != nil {
			t.Fatalf("create synthetic OAuth row: %v", err)
		}
	}
	groupID, invite := s.group(t, target)
	s.join(t, survivor, groupID, invite, http.StatusCreated)
	foreignNote := s.note(t, survivor)
	s.share(t, survivor, foreignNote, groupID, http.StatusCreated)

	var result struct {
		Status string `json:"status"`
	}
	s.request(t, http.MethodDelete, s.auth+"/auth/account", target.token, nil, http.StatusOK, &result)
	if result.Status != "complete" {
		t.Fatalf("first deletion status=%q, want complete", result.Status)
	}
	assertPostDeletionState := func() {
		t.Helper()
		var oauthCount, identityCount, groupCount, shareCount, targetPermissionCount, survivorPermissionCount int
		queries := []struct {
			query string
			args  []any
			to    *int
		}{
			{`SELECT count(*) FROM identity.oauth_connections WHERE user_id=$1::uuid`, []any{target.id}, &oauthCount},
			{`SELECT count(*) FROM identity.users WHERE id=$1::uuid`, []any{target.id}, &identityCount},
			{`SELECT count(*) FROM social.groups WHERE id=$1::uuid`, []any{groupID}, &groupCount},
			{`SELECT count(*) FROM notes.shared_notes WHERE note_id=$1::uuid AND group_id=$2::uuid`, []any{foreignNote, groupID}, &shareCount},
			{`SELECT count(*) FROM notes.drive_managed_permissions WHERE note_id=$1::uuid AND principal_type='email' AND lower(principal_key)=lower($2)`, []any{foreignNote, target.email}, &targetPermissionCount},
			{`SELECT count(*) FROM notes.drive_managed_permissions WHERE note_id=$1::uuid AND principal_type='email' AND lower(principal_key)=lower($2) AND sync_status='in_sync'`, []any{foreignNote, survivor.email}, &survivorPermissionCount},
		}
		for _, q := range queries {
			if err := s.pool.QueryRow(context.Background(), q.query, q.args...).Scan(q.to); err != nil {
				t.Fatal(err)
			}
		}
		if oauthCount != 0 || identityCount != 0 || groupCount != 1 || shareCount != 1 || targetPermissionCount != 0 || survivorPermissionCount != 1 {
			t.Fatalf("post-deletion state oauth=%d identity=%d group=%d share=%d target_permission=%d survivor_permission=%d", oauthCount, identityCount, groupCount, shareCount, targetPermissionCount, survivorPermissionCount)
		}
	}
	assertPostDeletionState()

	// Retrying after completion is safe and does not duplicate cleanup records
	// or remove the surviving group's valid share and Drive permission.
	s.request(t, http.MethodDelete, s.auth+"/auth/account", target.token, nil, http.StatusOK, &result)
	if result.Status != "complete" {
		t.Fatalf("repeated deletion status=%q, want complete", result.Status)
	}
	assertPostDeletionState()
}

func rpcContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", internalKey), cancel
}

func startStack(t *testing.T) *stack {
	return startStackWithEnv(t, nil)
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	if v == "" {
		t.Fatalf("variable de entorno %s no definida (requerida en modo remoto)", key)
	}
	return v
}

func startStackWithEnv(t *testing.T, extraEnv []string) *stack {
	t.Helper()
	dsn := os.Getenv("CROSS_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("CROSS_SERVICE_TEST_DATABASE_URL debe apuntar a una BD dedicada terminada en _test; la suite crea y elimina una BD hija")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("se requiere una base dedicada terminada en _test")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg.Copy())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	database := "social_cross_e2e_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pgx.Identifier{database}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP DATABASE "+identifier+" WITH (FORCE)"); err != nil {
			t.Errorf("eliminar BD temporal: %v", err)
		}
	})
	cfg.ConnConfig.Database = database
	databaseURL, err := url.Parse(dsn)
	if err != nil || (databaseURL.Scheme != "postgres" && databaseURL.Scheme != "postgresql") {
		t.Fatal("se requiere una URL postgres:// o postgresql://")
	}
	databaseURL.Path = "/" + database
	query := databaseURL.Query()
	query.Set("dbname", database)
	databaseURL.RawQuery = query.Encode()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile(filepath.Join(root, "docker", "postgres", "init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(schema)); err != nil {
		t.Fatal(err)
	}

	mode := os.Getenv("CROSS_SERVICE_MODE")
	s := &stack{pool: pool, client: &http.Client{Timeout: 8 * time.Second}}

	if mode == "remote" {
		// Modo remoto: conectar a servicios ya desplegados (ej. Kubernetes).
		// No se buildan ni se ejecutan procesos locales.
		s.auth = requireEnv(t, "AUTH_BASE_URL")
		s.notes = requireEnv(t, "NOTES_BASE_URL")
		s.social = requireEnv(t, "SOCIAL_BASE_URL")
		s.academic = requireEnv(t, "ACADEMIC_BASE_URL")

		// Esperar a que los 4 servicios estén listos.
		for _, u := range []string{s.auth, s.notes, s.social, s.academic} {
			deadline := time.Now().Add(60 * time.Second)
			for {
				res, err := s.client.Get(u + "/health")
				if err == nil {
					res.Body.Close()
					if res.StatusCode == http.StatusOK {
						break
					}
				}
				if time.Now().After(deadline) {
					t.Fatalf("servicio remoto no respondió: %s", u)
				}
				time.Sleep(1 * time.Second)
			}
		}

		// gRPC de Social vía DNS/addr remoto.
		grpcAddr := requireEnv(t, "SOCIAL_GRPC_ADDR")
		conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close() })
		s.rpc = socialv1.NewSocialServiceClient(conn)
		return s
	}

	// ---------- Modo local: buildar y ejecutar binarios ----------
	ports := make([]string, 8)
	listeners := make([]net.Listener, 0, len(ports))
	for i := range ports {
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		ports[i] = fmt.Sprint(lis.Addr().(*net.TCPAddr).Port)
		listeners = append(listeners, lis)
	}
	for _, lis := range listeners {
		lis.Close()
	}
	s = &stack{pool: pool, auth: "http://127.0.0.1:" + ports[0], notes: "http://127.0.0.1:" + ports[2], social: "http://127.0.0.1:" + ports[4], academic: "http://127.0.0.1:" + ports[6], client: &http.Client{Timeout: 8 * time.Second}}
	env := []string{
		"DATABASE_URL=" + databaseURL.String(), "JWT_SECRET=cross-service-test-jwt-secret",
		"INTERNAL_API_KEY=" + internalKey, "GIN_MODE=release", "STORAGE_MODE=mock", "CALENDAR_MODE=mock", "STREAM_MODE=mock",
		"STREAM_API_SECRET=cross-service-test-stream-secret", "IDENTITY_GRPC_ADDR=127.0.0.1:" + ports[1],
		"SOCIAL_GRPC_ADDR=127.0.0.1:" + ports[5], "SOCIAL_SERVICE_URL=" + s.social,
		"NOTES_GRPC_ADDR=127.0.0.1:" + ports[3],
		// Si gRPC falla, el fallback HTTP no puede ocultarlo en esta prueba.
		"NOTES_GRPC_URL=127.0.0.1:" + ports[3], "NOTES_URL=" + s.notes + "/legacy-disabled",
	}
	env = append(env, extraEnv...)
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	buildDir := map[string]string{
		"auth":     "auth",
		"notes":    "notes",
		"social":   "social",
		"academic": "infacadem",
	}
	for i, name := range []string{"auth", "notes", "social", "academic"} {
		dir := t.TempDir()
		binary := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		buildCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/server")
		build.Dir = filepath.Join(root, "back", buildDir[name])
		output, buildErr := build.CombinedOutput()
		cancel()
		if buildErr != nil {
			t.Fatalf("build %s: %v\n%s", name, buildErr, output)
		}
		base := []string{s.auth, s.notes, s.social, s.academic}[i]
		// Nombre de la variable de entorno para el puerto gRPC de cada servicio.
		grpcEnv := "GRPC_PORT"
		switch name {
		case "auth":
			grpcEnv = "AUTH_GRPC_PORT"
		case "notes":
			grpcEnv = "NOTES_GRPC_PORT"
		case "social":
			grpcEnv = "SOCIAL_GRPC_PORT"
		}
		process := &stackProcess{
			name: name, binary: binary, dir: dir, base: base,
			port: ports[2*i], grpcPort: ports[2*i+1], grpcEnvVar: grpcEnv,
			env: append([]string{}, env...),
		}
		s.processes = append(s.processes, process)
		t.Cleanup(func() {
			process.stop()
			if t.Failed() {
				process.dumpLogs(t)
			}
		})
		process.start(t, s.client)
	}
	conn, err := grpc.NewClient("127.0.0.1:"+ports[5], grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	s.rpc = socialv1.NewSocialServiceClient(conn)
	return s
}

func (s *stack) request(t *testing.T, method, url, token string, body any, want int, out any, headers ...map[string]string) {
	t.Helper()
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, set := range headers {
		for key, value := range set {
			req.Header.Set(key, value)
		}
	}
	res, err := s.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err = io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Fatalf("%s %s: status %d, esperado %d: %s", method, url, res.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatal(err)
		}
	}
}

func (s *stack) register(t *testing.T, name string) testUser {
	t.Helper()
	u := testUser{email: uuid.NewString() + "@example.com", name: name}
	body := map[string]string{"email": u.email, "password": "CrossService123!", "display_name": name}
	var registered struct {
		ID string `json:"id"`
	}
	s.request(t, "POST", s.auth+"/auth/register", "", body, 201, &registered)
	u.id = registered.ID
	var login struct {
		Token string `json:"access_token"`
	}
	s.request(t, "POST", s.auth+"/auth/login", "", body, 200, &login)
	u.token = login.Token
	if u.id == "" || u.token == "" {
		t.Fatal("Identity no emitió usuario/token")
	}
	return u
}

func (s *stack) group(t *testing.T, owner testUser) (string, string) {
	t.Helper()
	var created struct {
		ID string `json:"group_id"`
	}
	s.request(t, "POST", s.social+"/groups", owner.token, map[string]string{"name": "Grupo E2E"}, 201, &created)
	var view struct {
		Invite string `json:"invite_token"`
	}
	s.request(t, "GET", s.social+"/groups/"+created.ID, owner.token, nil, 200, &view)
	if created.ID == "" || view.Invite == "" {
		t.Fatal("grupo/invitación vacíos")
	}
	return created.ID, view.Invite
}

func (s *stack) join(t *testing.T, user testUser, group, invite string, status int) {
	t.Helper()
	s.request(t, "POST", s.social+"/groups/"+group+"/join", user.token, map[string]string{"invite_token": invite}, status, nil)
}

func (s *stack) note(t *testing.T, user testUser) string {
	t.Helper()
	var note struct {
		ID string `json:"note_id"`
	}
	s.request(t, "POST", s.notes+"/notes", user.token, map[string]string{"title": "Apunte E2E", "content": "# Prueba", "visibility": "private"}, 201, &note)
	if note.ID == "" {
		t.Fatal("note_id vacío")
	}
	return note.ID
}

func (s *stack) share(t *testing.T, user testUser, note, group string, status int) {
	t.Helper()
	s.request(t, "POST", s.notes+"/notes/"+note+"/share", user.token, map[string]string{"group_id": group, "access_mode": "restricted"}, status, nil)
}

func (s *stack) awaitShares(t *testing.T, note, group string, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for {
		var count int
		if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM notes.shared_notes WHERE note_id=$1 AND group_id=$2", note, group).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count == want {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("shares nota %s grupo %s: %d, esperado %d", note, group, count, want)
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func (s *stack) curriculum(t *testing.T, user testUser) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	s.request(t, "POST", s.academic+"/curricula", user.token, map[string]string{"name": "Malla E2E"}, 201, &created)
	if created.ID == "" {
		t.Fatal("curriculum_id vacío")
	}
	return created.ID
}

func (s *stack) subject(t *testing.T, user testUser, curriculumID string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	s.request(t, "POST", s.academic+"/curricula/"+curriculumID+"/subjects", user.token,
		map[string]any{"name": "Matemática E2E", "code": "MAT101"}, 201, &created)
	if created.ID == "" {
		t.Fatal("subject_id vacío")
	}
	return created.ID
}

func TestCrossService_FourServiceJourney(t *testing.T) {
	s := startStack(t)

	// 1) Auth: register + login → JWT real.
	owner := s.register(t, "Propietario académico")
	member := s.register(t, "Miembro del grupo")

	// 2) Social: crear grupo + unirse.
	groupID, invite := s.group(t, owner)
	s.join(t, member, groupID, invite, http.StatusCreated)

	// 3) Academic (infacadem): crear currículo y materia con el JWT de auth.
	curriculumID := s.curriculum(t, owner)
	subjectID := s.subject(t, owner, curriculumID)
	var gotSubject struct {
		ID string `json:"id"`
	}
	s.request(t, "GET", s.academic+"/subjects/"+subjectID, owner.token, nil, 200, &gotSubject)
	if gotSubject.ID != subjectID {
		t.Fatalf("materia no recuperada: %+v", gotSubject)
	}

	// 4) Notes: crear apunte y compartir al grupo (restricted).
	noteID := s.note(t, owner)
	s.share(t, owner, noteID, groupID, http.StatusCreated)

	// 5) Miembro del grupo ve la nota compartida.
	var shared struct {
		Notes []struct {
			ID string `json:"id"`
		} `json:"notes"`
	}
	s.request(t, "GET", s.notes+"/groups/"+groupID+"/notes", member.token, nil, 200, &shared)
	found := false
	for _, n := range shared.Notes {
		if n.ID == noteID {
			found = true
		}
	}
	if !found {
		t.Fatalf("miembro no ve la nota compartida %s en el grupo: %+v", noteID, shared.Notes)
	}

	// 6) Extrano no puede ver las notas del grupo.
	outsider := s.register(t, "Externo")
	s.request(t, "GET", s.notes+"/groups/"+groupID+"/notes", outsider.token, nil, 403, nil)
}

func TestCrossService_AcademicHealth(t *testing.T) {
	s := startStack(t)
	var health struct {
		Status  string `json:"status"`
		Service string `json:"service"`
	}
	s.request(t, "GET", s.academic+"/health", "", nil, 200, &health)
	if health.Status != "UP" || health.Service != "academic-service" {
		t.Fatalf("academic /health: %+v", health)
	}
	var db struct {
		Status   string `json:"status"`
		Database string `json:"database"`
		User     string `json:"user"`
	}
	s.request(t, "GET", s.academic+"/health/db", "", nil, 200, &db)
	if db.Status != "UP" || !strings.HasPrefix(db.Database, "social_cross_e2e_") {
		t.Fatalf("academic /health/db: %+v", db)
	}
}
