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
	pool                *pgxpool.Pool
	auth, social, notes string
	rpc                 socialv1.SocialServiceClient
	client              *http.Client
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

func rpcContext() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	return metadata.AppendToOutgoingContext(ctx, "x-internal-key", internalKey), cancel
}

func startStack(t *testing.T) *stack {
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

	ports := make([]string, 6)
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
	s := &stack{pool: pool, auth: "http://127.0.0.1:" + ports[0], notes: "http://127.0.0.1:" + ports[2], social: "http://127.0.0.1:" + ports[4], client: &http.Client{Timeout: 8 * time.Second}}
	env := []string{
		"DATABASE_URL=" + databaseURL.String(), "JWT_SECRET=cross-service-test-jwt-secret",
		"INTERNAL_API_KEY=" + internalKey, "GIN_MODE=release", "STORAGE_MODE=mock", "CALENDAR_MODE=mock", "STREAM_MODE=mock",
		"STREAM_API_SECRET=cross-service-test-stream-secret", "IDENTITY_GRPC_ADDR=127.0.0.1:" + ports[1],
		"SOCIAL_GRPC_ADDR=127.0.0.1:" + ports[5], "SOCIAL_SERVICE_URL=" + s.social,
		// Si gRPC falla, el fallback HTTP no puede ocultarlo en esta prueba.
		"NOTES_GRPC_URL=127.0.0.1:" + ports[3], "NOTES_URL=" + s.notes + "/legacy-disabled",
	}
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP"} {
		if value := os.Getenv(key); value != "" {
			env = append(env, key+"="+value)
		}
	}
	for i, name := range []string{"auth", "notes", "social"} {
		dir := t.TempDir()
		binary := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		buildCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		build := exec.CommandContext(buildCtx, "go", "build", "-o", binary, "./cmd/server")
		build.Dir = filepath.Join(root, "back", name)
		output, buildErr := build.CombinedOutput()
		cancel()
		if buildErr != nil {
			t.Fatalf("build %s: %v\n%s", name, buildErr, output)
		}
		logPath := filepath.Join(dir, "server.log")
		logFile, err := os.Create(logPath)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary)
		cmd.Dir = dir // impide cargar los .env del repositorio y sus credenciales
		cmd.Env = append(append([]string{}, env...), "PORT="+ports[2*i], strings.ToUpper(name)+"_GRPC_PORT="+ports[2*i+1])
		cmd.Stdout, cmd.Stderr = logFile, logFile
		if err := cmd.Start(); err != nil {
			logFile.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			logFile.Close()
			if t.Failed() {
				output, _ := os.ReadFile(logPath)
				t.Logf("%s:\n%s", name, output)
			}
		})
		base := []string{s.auth, s.notes, s.social}[i]
		deadline := time.Now().Add(20 * time.Second)
		for {
			res, err := s.client.Get(base + "/health")
			if err == nil {
				res.Body.Close()
				if res.StatusCode == 200 {
					break
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s no inició: %v", name, err)
			}
			time.Sleep(50 * time.Millisecond)
		}
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
