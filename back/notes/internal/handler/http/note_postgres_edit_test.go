package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

// Real PostgreSQL, real HTTP handler/service/repositories; only Drive is mocked.
// Each run creates and drops its own database. Never uses application credentials.
func TestPostgresConsecutiveNoteEdits(t *testing.T) {
	dsn := os.Getenv("NOTES_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("NOTES_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "notes_edit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP DATABASE "+quoted); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `CREATE SCHEMA notes;
 CREATE TABLE notes.notes (
 id uuid PRIMARY KEY, user_id uuid NOT NULL, subject_id uuid, title text NOT NULL,
 external_file_id text, visibility text NOT NULL, likes_count integer NOT NULL DEFAULT 0,
 forked_from_note_id uuid, sync_status text NOT NULL, version bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());`)
	if err != nil {
		t.Fatal(err)
	}
	// Regression proof: the former query fails before the callback / Drive mutation.
	if _, err := pool.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('test', 0))::bigint`); err == nil {
		t.Fatal("old query unexpectedly accepted")
	}
	store := service.NewPGNoteStore(pool)
	files := drive.NewMockClient()
	owner, id := uuid.NewString(), uuid.NewString()
	fileID, err := files.CreateFile(ctx, owner, id, "Prueba.md", "antes")
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(ctx, id, owner, nil, "Prueba", &fileID, "private", nil, "synced")
	if err != nil {
		t.Fatal(err)
	}
	svc := service.NewNoteService(store, service.NewMemoryAttachmentStore(), service.NewMemorySavedStore(), service.NewMemoryLikeStore(), service.NewPGSharedStore(pool), files, service.NewMemorySocialResolver())
	handler := NewNoteHandler(svc)
	router := gin.New()
	validator := &middleware.SimpleHS256Validator{Secret: []byte(testSecret), Issuer: testIssuer, Audience: testAudience}
	router.Use(middleware.NewAuthMiddleware(validator).RequireAuth())
	router.GET("/notes/me", handler.ListMy)
	router.GET("/notes/:id", handler.Get)
	router.PATCH("/notes/:id", handler.Patch)
	request := func(method, path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+genToken(owner, "student"))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	for version := 1; version <= 2; version++ {
		read := request("GET", "/notes/"+id, nil)
		var detail map[string]any
		_ = json.Unmarshal(read.Body.Bytes(), &detail)
		if read.Code != 200 || detail["version"] != float64(version) {
			t.Fatalf("GET status=%d version=%v", read.Code, detail["version"])
		}
		response := request("PATCH", "/notes/"+id, map[string]any{"title": "Editada", "content": "texto de prueba", "version": version})
		var result map[string]any
		_ = json.Unmarshal(response.Body.Bytes(), &result)
		t.Logf("PATCH note=%s owner=%s sentVersion=%d status=%d returnedVersion=%v", id, owner, version, response.Code, result["version"])
		if response.Code != 200 || result["version"] != float64(version+1) {
			t.Fatalf("PATCH status=%d body=%s", response.Code, response.Body.String())
		}
	}
	if stale := request("PATCH", "/notes/"+id, map[string]any{"title": "stale", "version": 1}); stale.Code != 409 {
		t.Fatalf("stale status=%d", stale.Code)
	}
	mine := request("GET", "/notes/me", nil)
	var listing struct {
		Notes []struct {
			Version int64 `json:"version"`
		} `json:"notes"`
	}
	_ = json.Unmarshal(mine.Body.Bytes(), &listing)
	if mine.Code != 200 || len(listing.Notes) != 1 || listing.Notes[0].Version != 3 {
		t.Fatal("list lost current version")
	}
}
