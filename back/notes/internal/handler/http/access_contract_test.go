package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

func TestAccessJSONContract(t *testing.T) {
	r, svc, _, _ := setupRouter()
	owner := uuid.NewString()
	note, err := svc.Create(context.Background(), owner, "Access contract", nil, "public", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		user, mode string
		write      bool
	}{
		{owner, "owner", true}, {uuid.NewString(), "public", false},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/notes/"+note.ID+"/access", nil)
		req.Header.Set("Authorization", "Bearer "+genToken(tc.user, "student"))
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"can_read", "can_write", "visibility", "access_mode", "drive_sync_status", "drive_access_verified", "drive_connection_required", "drive_url"} {
			if _, ok := fields[key]; !ok {
				t.Errorf("missing field %q: %s", key, w.Body.String())
			}
		}
		var access model.NoteAccessResponse
		if err := json.Unmarshal(w.Body.Bytes(), &access); err != nil {
			t.Fatal(err)
		}
		if !access.CanRead || access.CanWrite != tc.write || access.AccessMode != tc.mode || access.Visibility != "public" || access.DriveSyncStatus != "synced" || access.DriveAccessVerified || access.DriveConnectionRequired || access.DriveURL == nil {
			t.Fatalf("unexpected access: %+v", access)
		}
	}
}

func TestNoteListsEmptyArrays(t *testing.T) {
	r, _, _, social := setupRouter()
	user, group := uuid.NewString(), uuid.NewString()
	social.AddMember(user, group)
	for _, path := range []string{"/notes/me", "/groups/" + group + "/notes"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+genToken(user, "student"))
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if string(body["notes"]) != "[]" {
			t.Errorf("%s: notes=%s, want []", path, body["notes"])
		}
	}
}

// TestAccessJSONContractDriveSyncFailed verifica el contrato HTTP del estado
// real de convergencia: si una fila de notes.drive_managed_permissions quedó
// 'failed' (Drive rechazó el grant), GET /notes/:id/access lo refleja.
func TestAccessJSONContractDriveSyncFailed(t *testing.T) {
	r, svc, driveMock, social := setupRouter()
	md := service.NewMemoryMemberDirectory()
	svc.SetMemberDirectory(md)
	ctx := context.Background()
	owner, group := uuid.NewString(), uuid.NewString()
	social.AddAdmin(owner, group)
	md.SetEmails(group, []string{"rechazado@example.com"})
	note, err := svc.Create(ctx, owner, "Sync failed", nil, "private", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	driveMock.InjectGrantError("rechazado@example.com", &drive.DriveError{Code: 500, Message: "upstream caído"})
	if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
		t.Fatalf("el share debe persistirse aunque Drive falle: %v", err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+note.ID+"/access", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(owner, "student"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var access model.NoteAccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &access); err != nil {
		t.Fatal(err)
	}
	if access.DriveSyncStatus != "failed" {
		t.Fatalf("drive_sync_status = %q, want failed: %+v", access.DriveSyncStatus, access)
	}
}

// TestAccessJSONContractDriveConnectionRequired verifica el contrato HTTP de
// las banderas de Drive con la semántica estricta por modo de acceso:
// drive_connection_required=true sólo para el autor de una nota no pública sin
// conexión OAuth (respuesta 200, nunca 403) y false para el resto (link,
// restricted y public), donde el archivo se sirve con la autorización de la
// aplicación. drive_access_verified sólo lo declara el autor verificado por
// Drive. El drive_sync_status real de notes.drive_managed_permissions ('failed')
// no se altera en ningún caso.
func TestAccessJSONContractDriveConnectionRequired(t *testing.T) {
	t.Run("owner sin conexión: conexión requerida y no verificado", func(t *testing.T) {
		r, svc, driveMock, social := setupRouter()
		md := service.NewMemoryMemberDirectory()
		svc.SetMemberDirectory(md)
		ctx := context.Background()
		owner, group := uuid.NewString(), uuid.NewString()
		social.AddAdmin(owner, group)
		md.SetEmails(group, []string{"sin-drive@example.com"})
		note, err := svc.Create(ctx, owner, "Connection required", nil, "private", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		driveMock.InjectGrantError("sin-drive@example.com", &drive.DriveError{Code: 500, Message: "upstream caído"})
		if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
			t.Fatalf("el share debe persistirse aunque Drive falle: %v", err)
		}
		driveMock.Disconnect(owner)
		access := getAccessOverHTTP(t, r, owner, note.ID)
		if !access.CanRead || access.AccessMode != "owner" || !access.CanWrite {
			t.Fatalf("unexpected application access: %+v", access)
		}
		if access.DriveSyncStatus != "failed" {
			t.Fatalf("drive_sync_status = %q, want failed: %+v", access.DriveSyncStatus, access)
		}
		if !access.DriveConnectionRequired {
			t.Fatalf("drive_connection_required = false, want true: %+v", access)
		}
		if access.DriveAccessVerified {
			t.Fatalf("drive_access_verified = true sin verificación: %+v", access)
		}
	})

	t.Run("owner con conexión: verificado y sin conexión requerida", func(t *testing.T) {
		r, svc, _, _ := setupRouter()
		ctx := context.Background()
		owner := uuid.NewString()
		note, err := svc.Create(ctx, owner, "Owner verified", nil, "private", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		access := getAccessOverHTTP(t, r, owner, note.ID)
		if !access.DriveAccessVerified {
			t.Fatalf("drive_access_verified = false, want true: %+v", access)
		}
		if access.DriveConnectionRequired {
			t.Fatalf("drive_connection_required = true con OAuth vigente: %+v", access)
		}
	})

	t.Run("lector restricted sin conexión: sin banderas de Drive", func(t *testing.T) {
		r, svc, driveMock, social := setupRouter()
		md := service.NewMemoryMemberDirectory()
		svc.SetMemberDirectory(md)
		ctx := context.Background()
		owner, reader, group := uuid.NewString(), uuid.NewString(), uuid.NewString()
		social.AddAdmin(owner, group)
		social.AddMember(reader, group)
		md.SetEmails(group, []string{"sin-drive@example.com"})
		note, err := svc.Create(ctx, owner, "Connection required", nil, "private", nil, "")
		if err != nil {
			t.Fatal(err)
		}
		driveMock.InjectGrantError("sin-drive@example.com", &drive.DriveError{Code: 500, Message: "upstream caído"})
		if _, err := svc.Share(ctx, owner, note.ID, group, "restricted"); err != nil {
			t.Fatalf("el share debe persistirse aunque Drive falle: %v", err)
		}
		driveMock.Disconnect(reader)
		access := getAccessOverHTTP(t, r, reader, note.ID)
		if !access.CanRead || access.AccessMode != "restricted" || access.CanWrite {
			t.Fatalf("unexpected application access: %+v", access)
		}
		if access.DriveSyncStatus != "failed" {
			t.Fatalf("drive_sync_status = %q, want failed: %+v", access.DriveSyncStatus, access)
		}
		if access.DriveConnectionRequired || access.DriveAccessVerified {
			t.Fatalf("banderas de Drive inesperadas para un lector: %+v", access)
		}
	})
}

// getAccessOverHTTP ejecuta GET /notes/:id/access con el token del usuario y
// exige 200, devolviendo el contrato de acceso deserializado.
func getAccessOverHTTP(t *testing.T, r *gin.Engine, userID, noteID string) model.NoteAccessResponse {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+noteID+"/access", nil)
	req.Header.Set("Authorization", "Bearer "+genToken(userID, "student"))
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var access model.NoteAccessResponse
	if err := json.Unmarshal(w.Body.Bytes(), &access); err != nil {
		t.Fatal(err)
	}
	return access
}

type timezoneNoteStore struct {
	*service.MemoryNoteStore
	note *model.Note
}

func (s *timezoneNoteStore) ListByUser(context.Context, string, string, int) ([]*model.Note, string, error) {
	return []*model.Note{s.note}, "", nil
}

func (s *timezoneNoteStore) GetByID(context.Context, string) (*model.Note, error) {
	return s.note, nil
}

func TestNoteListsUTCRFC3339(t *testing.T) {
	ctx := context.Background()
	user, group := uuid.NewString(), uuid.NewString()
	created := time.Date(2026, 9, 26, 12, 34, 56, 123456789, time.FixedZone("Chile", -3*60*60))
	note := &model.Note{ID: uuid.NewString(), UserID: user, Title: "Timezone", Visibility: "private", CreatedAt: created, UpdatedAt: created.Add(time.Hour)}
	store := &timezoneNoteStore{MemoryNoteStore: service.NewMemoryNoteStore(), note: note}
	shared, social := service.NewMemorySharedStore(), service.NewMemorySocialResolver()
	social.AddMember(user, group)
	if _, err := shared.Create(ctx, note.ID, group, false, "restricted", 0); err != nil {
		t.Fatal(err)
	}
	svc := service.NewNoteService(store, service.NewMemoryAttachmentStore(), service.NewMemorySavedStore(), service.NewMemoryLikeStore(), shared, nil, social)
	h := NewNoteHandler(svc)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user_id", user); c.Next() })
	r.GET("/notes/me", h.ListMy)
	r.GET("/groups/:id/notes", h.ListGroupNotes)
	for _, path := range []string{"/notes/me", "/groups/" + group + "/notes"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		var body struct {
			Notes []struct {
				CreatedAt string `json:"created_at"`
				UpdatedAt string `json:"updated_at"`
			} `json:"notes"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Notes) != 1 {
			t.Fatalf("%s: expected one note: %s", path, w.Body.String())
		}
		if body.Notes[0].CreatedAt != "2026-09-26T15:34:56Z" || body.Notes[0].UpdatedAt != "2026-09-26T16:34:56Z" {
			t.Errorf("%s: non-UTC RFC3339 dates: %+v", path, body.Notes[0])
		}
	}
}
