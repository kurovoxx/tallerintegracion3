package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/middleware"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/service"
)

func init() { gin.SetMode(gin.TestMode) }

const testSecret = "test-jwt-secret-notes-handler"
const testIssuer = "apuntes-auth"
const testAudience = "apuntes-client"

func genToken(userID, role string) string {
	claims := middleware.ClaimsPersonalizadas{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := tok.SignedString([]byte(testSecret))
	return s
}

func setupRouter() (*gin.Engine, *service.NoteService, *drive.MockClient, *service.MemorySocialResolver) {
	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	validator := &middleware.SimpleHS256Validator{Secret: []byte(testSecret), Issuer: testIssuer, Audience: testAudience}
	authMw := middleware.NewAuthMiddleware(validator)
	h := NewNoteHandler(svc)
	r := gin.New()
	prot := r.Group("")
	prot.Use(authMw.RequireAuth())
	{
		prot.POST("/notes", h.Create)
		prot.GET("/notes/me", h.ListMy)
		prot.POST("/notes/unshare-all", h.UnshareAll)
		prot.GET("/notes/:id/access", h.GetAccess)
		prot.POST("/notes/:id/attachments", h.UploadAttachment)
		prot.DELETE("/notes/:id/attachments/:attachmentId", h.DeleteAttachment)
		prot.POST("/notes/:id/save", h.Save)
		prot.POST("/notes/:id/copy", h.Copy)
		prot.POST("/notes/:id/like", h.Like)
		prot.DELETE("/notes/:id/like", h.Unlike)
		prot.POST("/notes/:id/share", h.Share)
		prot.PATCH("/notes/:id", h.Patch)
		prot.DELETE("/notes/:id", h.Delete)
		prot.GET("/notes/:id", h.Get)
		prot.DELETE("/notes/shared/:sharedNoteId", h.Unshare)
		prot.GET("/groups/:id/notes", h.ListGroupNotes)
	}
	return r, svc, driveMock, social
}

func TestHandlerCreateSuccess(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	body := `{"title":"Mi apunte handler","visibility":"private"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if resp["note_id"] == "" {
		t.Fatal("note_id vacío")
	}
}

func TestHandlerCreateValidation400(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	cases := []struct {
		body string
	}{
		{`{"title":"","visibility":"private"}`},
		{`{"title":"ok","visibility":"invalid"}`},
		{`{"visibility":"private"}`},
	}
	for i, tc := range cases {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(tc.body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("case %d expected 400 got %d body %s", i, w.Code, w.Body.String())
		}
	}
}

func TestHandlerCreateUnauthorized401(t *testing.T) {
	r, _, _, _ := setupRouter()
	body := `{"title":"x","visibility":"private"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	// sin Authorization
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", w.Code)
	}
}

func TestHandlerGetNotFound404(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	fakeID := uuid.NewString()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/"+fakeID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetWithDriveResilienceAlternative(t *testing.T) {
	// Test más directo usando service layer y handler combinados
	r := gin.New()
	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	validator := &middleware.SimpleHS256Validator{Secret: []byte(testSecret), Issuer: testIssuer, Audience: testAudience}
	authMw := middleware.NewAuthMiddleware(validator)
	h := NewNoteHandler(svc)
	r.Use(authMw.RequireAuth())
	r.GET("/notes/:id", h.Get)
	r.POST("/notes", h.Create)

	author := uuid.NewString()
	token := genToken(author, "student")
	// crear
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"T","visibility":"public","content":"c"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	noteID := cre["note_id"]
	// obtener fileID via store directo
	n, _ := noteStore.GetByID(req.Context(), noteID) // need background
	// use background context
	_ = n
	// get fileID from noteStore with background
	// Note: GetByID no necesita context específico
	// Try with background
	note, _ := noteStore.GetByID(nilContext(), noteID)
	if note == nil || note.ExternalFileID == nil {
		t.Fatalf("note externalFileID nil")
	}
	fileID := *note.ExternalFileID
	driveMock.InjectGetError(fileID, &drive.DriveError{Code: 404, Message: "Nota no disponible en almacenamiento remoto"})
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notes/"+noteID, nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w2, req2)
	if w2.Code != 404 {
		t.Fatalf("expected 404, got %d %s", w2.Code, w2.Body.String())
	}
}

func nilContext() context.Context { return context.Background() }

func TestHandlerPrivateAccessForbidden(t *testing.T) {
	r, _, _, social := setupRouter()
	author := uuid.NewString()
	other := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	authorToken := genToken(author, "student")
	otherToken := genToken(other, "student")
	// crear privada
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"Priv","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authorToken)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	noteID := cre["note_id"]
	// other intenta leer
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/notes/"+noteID, nil)
	req2.Header.Set("Authorization", "Bearer "+otherToken)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d %s", w2.Code, w2.Body.String())
	}
	// other intenta editar
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPatch, "/notes/"+noteID, bytes.NewBufferString(`{"title":"hack"}`))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+otherToken)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on patch, got %d %s", w3.Code, w3.Body.String())
	}
}

func TestHandlerLikeAndCopy(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	other := uuid.NewString()
	authorTok := genToken(author, "student")
	otherTok := genToken(other, "student")
	// crear public
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"Likeable","visibility":"public","content":"data"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+authorTok)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	noteID := cre["note_id"]
	// like
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+noteID+"/like", nil)
	req2.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusCreated {
		t.Fatalf("like expected 201 got %d %s", w2.Code, w2.Body.String())
	}
	// duplicate like 409
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/notes/"+noteID+"/like", nil)
	req3.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusConflict {
		t.Fatalf("duplicate like expected 409 got %d %s", w3.Code, w3.Body.String())
	}
	// unlike
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodDelete, "/notes/"+noteID+"/like", nil)
	req4.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusNoContent {
		t.Fatalf("unlike expected 204 got %d %s", w4.Code, w4.Body.String())
	}
	// copy
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodPost, "/notes/"+noteID+"/copy", nil)
	req5.Header.Set("Authorization", "Bearer "+otherTok)
	r.ServeHTTP(w5, req5)
	if w5.Code != http.StatusCreated {
		t.Fatalf("copy expected 201 got %d %s", w5.Code, w5.Body.String())
	}
	var copyResp map[string]string
	json.Unmarshal(w5.Body.Bytes(), &copyResp)
	if copyResp["note_id"] == "" {
		t.Fatal("copy note_id vacío")
	}
	if copyResp["forked_from_note_id"] != noteID {
		t.Fatalf("forked_from mismatch")
	}
}

func TestHandlerSaveConflict(t *testing.T) {
	r, _, _, _ := setupRouter()
	author := uuid.NewString()
	saver := uuid.NewString()
	at := genToken(author, "student")
	st := genToken(saver, "student")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"SaveTest","visibility":"public"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/save", nil)
	req2.Header.Set("Authorization", "Bearer "+st)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("save 201 got %d %s", w2.Code, w2.Body.String())
	}
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/save", nil)
	req3.Header.Set("Authorization", "Bearer "+st)
	r.ServeHTTP(w3, req3)
	if w3.Code != 409 {
		t.Fatalf("duplicate save 409 got %d %s", w3.Code, w3.Body.String())
	}
}

func TestHandlerAttachment(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"AttTest","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	// upload attachment multipart
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.png")
	part.Write([]byte("fake png data"))
	writer.Close()
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/attachments", body)
	req2.Header.Set("Content-Type", writer.FormDataContentType())
	req2.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("upload expected 201 got %d %s", w2.Code, w2.Body.String())
	}
	var attResp map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &attResp)
	attID, _ := attResp["attachment_id"].(string)
	if attID == "" {
		t.Fatal("attachment_id vacío")
	}
	// delete
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodDelete, "/notes/"+nid+"/attachments/"+attID, nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w3, req3)
	if w3.Code != 204 {
		t.Fatalf("delete att 204 got %d %s", w3.Code, w3.Body.String())
	}
}

func TestHandlerShareAndGroupNotes(t *testing.T) {
	r, _, _, social := setupRouter()
	author := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	at := genToken(author, "student")
	mt := genToken(member, "student")
	// create private
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"ShareHandler","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	// share
	w2 := httptest.NewRecorder()
	shareBody := fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/share", bytes.NewBufferString(shareBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("share 201 got %d %s", w2.Code, w2.Body.String())
	}
	var shareResp map[string]string
	json.Unmarshal(w2.Body.Bytes(), &shareResp)
	sharedID := shareResp["shared_note_id"]
	if sharedID == "" {
		t.Fatal("shared_note_id vacío")
	}
	// member can now get note
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notes/"+nid, nil)
	req3.Header.Set("Authorization", "Bearer "+mt)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("member get after share expected 200 got %d %s", w3.Code, w3.Body.String())
	}
	// group notes list
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodGet, "/groups/"+groupID+"/notes", nil)
	req4.Header.Set("Authorization", "Bearer "+mt)
	r.ServeHTTP(w4, req4)
	if w4.Code != 200 {
		t.Fatalf("group notes 200 got %d %s", w4.Code, w4.Body.String())
	}
	// unshare
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodDelete, "/notes/shared/"+sharedID, nil)
	req5.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w5, req5)
	if w5.Code != 204 {
		t.Fatalf("unshare 204 got %d %s", w5.Code, w5.Body.String())
	}
}

func TestHandlerUnshareAll(t *testing.T) {
	r, _, _, social := setupRouter()
	author := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	at := genToken(author, "student")
	// create and share
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"UnshareAll","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("share %d", w2.Code)
	}
	w3 := httptest.NewRecorder()
	body := fmt.Sprintf(`{"user_id":"%s","group_id":"%s"}`, author, groupID)
	req3 := httptest.NewRequest(http.MethodPost, "/notes/unshare-all", bytes.NewBufferString(body))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w3, req3)
	if w3.Code != 204 {
		t.Fatalf("unshare-all 204 got %d %s", w3.Code, w3.Body.String())
	}
}

func TestHandlerListMyPagination(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	// create 3 notes
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"title":"Pag %d","visibility":"private"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		r.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("create pag %d failed %d %s", i, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/notes/me?limit=2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list me 200 got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json %v", err)
	}
	notes, ok := resp["notes"].([]interface{})
	if !ok || len(notes) != 2 {
		t.Fatalf("expected 2 notes, got %v", resp["notes"])
	}
	if resp["next_cursor"] == nil || resp["next_cursor"] == "" {
		t.Fatalf("expected next_cursor")
	}
}

func TestHandlerExternalAttachment(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"ExtAtt","visibility":"private"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	extID := "drive_external_12345"
	body := fmt.Sprintf(`{"external_file_id":"%s","file_name":"doc.pdf","file_type":"application/pdf","is_inline":false}`, extID)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/attachments", bytes.NewBufferString(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("external att 201 got %d %s", w2.Code, w2.Body.String())
	}
	var attResp map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &attResp)
	if attResp["attachment_id"] == nil || attResp["attachment_id"] == "" {
		t.Fatal("attachment_id vacío external")
	}
}

func TestHandlerPatchSyncDrive(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"PatchDrive","visibility":"private","content":"old"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPatch, "/notes/"+nid, bytes.NewBufferString(`{"title":"Nuevo","content":"new content","visibility":"public"}`))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("patch 200 got %d %s", w2.Code, w2.Body.String())
	}
	// verify content updated
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notes/"+nid, nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("get after patch 200 got %d %s", w3.Code, w3.Body.String())
	}
	var getResp map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &getResp)
	if getResp["content"] != "new content" {
		t.Fatalf("content not synced, got %v", getResp["content"])
	}
	if getResp["title"] != "Nuevo" {
		t.Fatalf("title not updated got %v", getResp["title"])
	}
}

func TestConcurrentLikes(t *testing.T) {
	r, svc, _, _ := setupRouter()
	author := uuid.NewString()
	at := genToken(author, "student")
	// crear nota publica
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"ConcurrentH","visibility":"public","content":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	if w.Code != 201 {
		t.Fatalf("create %d %s", w.Code, w.Body.String())
	}
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nid := cre["note_id"]
	const workers = 10
	var wg sync.WaitGroup
	wg.Add(workers)
	codes := make([]int, workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			tok := genToken(uuid.NewString(), "student")
			w2 := httptest.NewRecorder()
			req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/like", nil)
			req2.Header.Set("Authorization", "Bearer "+tok)
			r.ServeHTTP(w2, req2)
			codes[idx] = w2.Code
		}(i)
	}
	wg.Wait()
	for i, c := range codes {
		if c != 201 {
			t.Fatalf("worker %d expected 201 got %d", i, c)
		}
	}
	// verificar likes_count via Get (author)
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notes/"+nid, nil)
	req3.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("get %d %s", w3.Code, w3.Body.String())
	}
	var getResp map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &getResp)
	if int(getResp["likes_count"].(float64)) != workers {
		t.Fatalf("likes_count esperado %d got %v", workers, getResp["likes_count"])
	}
	_ = svc
}

func TestAccessModeLinkVsRestricted(t *testing.T) {
	r, _, _, social := setupRouter()
	author := uuid.NewString()
	nonMember := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(author, groupID)
	social.AddMember(member, groupID)
	at := genToken(author, "student")
	nt := genToken(nonMember, "student")
	mt := genToken(member, "student")
	// link
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"LinkH","visibility":"private","content":"link"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w, req)
	var cre map[string]string
	json.Unmarshal(w.Body.Bytes(), &cre)
	nidLink := cre["note_id"]
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/notes/"+nidLink+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("share link %d %s", w2.Code, w2.Body.String())
	}
	// nonMember debe leer con link
	w3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/notes/"+nidLink, nil)
	req3.Header.Set("Authorization", "Bearer "+nt)
	r.ServeHTTP(w3, req3)
	if w3.Code != 200 {
		t.Fatalf("nonMember link expected 200 got %d %s", w3.Code, w3.Body.String())
	}
	// restricted
	w4 := httptest.NewRecorder()
	req4 := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(`{"title":"RestrH","visibility":"private","content":"restr"}`))
	req4.Header.Set("Content-Type", "application/json")
	req4.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w4, req4)
	var cre2 map[string]string
	json.Unmarshal(w4.Body.Bytes(), &cre2)
	nidRestr := cre2["note_id"]
	w5 := httptest.NewRecorder()
	req5 := httptest.NewRequest(http.MethodPost, "/notes/"+nidRestr+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"restricted"}`, groupID)))
	req5.Header.Set("Content-Type", "application/json")
	req5.Header.Set("Authorization", "Bearer "+at)
	r.ServeHTTP(w5, req5)
	if w5.Code != 201 {
		t.Fatalf("share restricted %d %s", w5.Code, w5.Body.String())
	}
	// nonMember 403
	w6 := httptest.NewRecorder()
	req6 := httptest.NewRequest(http.MethodGet, "/notes/"+nidRestr, nil)
	req6.Header.Set("Authorization", "Bearer "+nt)
	r.ServeHTTP(w6, req6)
	if w6.Code != 403 {
		t.Fatalf("nonMember restricted expected 403 got %d %s", w6.Code, w6.Body.String())
	}
	// member 200
	w7 := httptest.NewRecorder()
	req7 := httptest.NewRequest(http.MethodGet, "/notes/"+nidRestr, nil)
	req7.Header.Set("Authorization", "Bearer "+mt)
	r.ServeHTTP(w7, req7)
	if w7.Code != 200 {
		t.Fatalf("member restricted expected 200 got %d %s", w7.Code, w7.Body.String())
	}
}

func TestListGroupNotesOrdering(t *testing.T) {
	// Usar stores explícitos para poder manipular shared_at de forma determinística
	driveMock := drive.NewMockClient()
	noteStore := service.NewMemoryNoteStore()
	attStore := service.NewMemoryAttachmentStore()
	savedStore := service.NewMemorySavedStore()
	likeStore := service.NewMemoryLikeStore()
	likeStore.SetNoteStore(noteStore)
	sharedStore := service.NewMemorySharedStore()
	social := service.NewMemorySocialResolver()
	svc := service.NewNoteService(noteStore, attStore, savedStore, likeStore, sharedStore, driveMock, social)
	validator := &middleware.SimpleHS256Validator{Secret: []byte(testSecret), Issuer: testIssuer, Audience: testAudience}
	authMw := middleware.NewAuthMiddleware(validator)
	h := NewNoteHandler(svc)
	r := gin.New()
	prot := r.Group("")
	prot.Use(authMw.RequireAuth())
	{
		prot.POST("/notes", h.Create)
		prot.POST("/notes/:id/share", h.Share)
		prot.GET("/groups/:id/notes", h.ListGroupNotes)
	}
	authorAdmin := uuid.NewString()
	authorMember := uuid.NewString()
	member := uuid.NewString()
	groupID := uuid.NewString()
	social.AddAdmin(authorAdmin, groupID)
	social.AddMember(authorMember, groupID)
	social.AddMember(member, groupID)
	atAdmin := genToken(authorAdmin, "student")
	atMember := genToken(authorMember, "student")
	mt := genToken(member, "student")
	// crear 4 notas via handler
	createNote := func(title, tok string) string {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes", bytes.NewBufferString(fmt.Sprintf(`{"title":"%s","visibility":"private"}`, title)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		var cre map[string]string
		json.Unmarshal(w.Body.Bytes(), &cre)
		return cre["note_id"]
	}
	nAdminHigh := createNote("AdminHighH", atAdmin)
	nAdminLow := createNote("AdminLowH", atAdmin)
	nMemberHigh := createNote("MemberHighH", atMember)
	nMemberLow := createNote("MemberLowH", atMember)
	share := func(nid, tok string) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/notes/"+nid+"/share", bytes.NewBufferString(fmt.Sprintf(`{"group_id":"%s","access_mode":"link"}`, groupID)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("share %s %d %s", nid, w.Code, w.Body.String())
		}
	}
	share(nAdminHigh, atAdmin)
	share(nAdminLow, atAdmin)
	share(nMemberHigh, atMember)
	share(nMemberLow, atMember)
	// likes: AdminHigh 5, AdminLow 1, MemberHigh 10, MemberLow 2
	for i := 0; i < 5; i++ {
		_ = svc.Like(context.Background(), uuid.NewString(), nAdminHigh)
	}
	for i := 0; i < 1; i++ {
		_ = svc.Like(context.Background(), uuid.NewString(), nAdminLow)
	}
	for i := 0; i < 10; i++ {
		_ = svc.Like(context.Background(), uuid.NewString(), nMemberHigh)
	}
	for i := 0; i < 2; i++ {
		_ = svc.Like(context.Background(), uuid.NewString(), nMemberLow)
	}
	// Ajustar shared_at manualmente para orden determinístico
	sharedStore.SetSharedAt(nAdminHigh, parseTimeHandler("2026-01-10T10:00:00Z"))
	sharedStore.SetIsAdminNote(nAdminHigh, true)
	sharedStore.SetSharedAt(nAdminLow, parseTimeHandler("2026-01-10T11:00:00Z"))
	sharedStore.SetIsAdminNote(nAdminLow, true)
	sharedStore.SetSharedAt(nMemberHigh, parseTimeHandler("2026-01-10T12:00:00Z"))
	sharedStore.SetIsAdminNote(nMemberHigh, false)
	sharedStore.SetSharedAt(nMemberLow, parseTimeHandler("2026-01-10T09:00:00Z"))
	sharedStore.SetIsAdminNote(nMemberLow, false)
	// list inicial 4
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/groups/"+groupID+"/notes", nil)
	req.Header.Set("Authorization", "Bearer "+mt)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("list group notes %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	arr, ok := resp["notes"].([]interface{})
	if !ok || len(arr) != 4 {
		t.Fatalf("expected 4 notes got %v", resp["notes"])
	}
	getTitle := func(m interface{}) string {
		return m.(map[string]interface{})["title"].(string)
	}
	if getTitle(arr[0]) != "AdminHighH" {
		t.Fatalf("pos0 esperado AdminHighH got %s", getTitle(arr[0]))
	}
	if getTitle(arr[1]) != "AdminLowH" {
		t.Fatalf("pos1 esperado AdminLowH got %s", getTitle(arr[1]))
	}
	if getTitle(arr[2]) != "MemberHighH" {
		t.Fatalf("pos2 esperado MemberHighH got %s", getTitle(arr[2]))
	}
	if getTitle(arr[3]) != "MemberLowH" {
		t.Fatalf("pos3 esperado MemberLowH got %s", getTitle(arr[3]))
	}
	// --- Desempate por fecha: mismo likes (3) y mismo is_admin_note, debe ordenar por shared_at DESC ---
	nTieRecent := createNote("TieRecentH", atMember)
	nTieOld := createNote("TieOldH", atMember)
	share(nTieRecent, atMember)
	share(nTieOld, atMember)
	for i := 0; i < 3; i++ {
		_ = svc.Like(context.Background(), uuid.NewString(), nTieRecent)
		_ = svc.Like(context.Background(), uuid.NewString(), nTieOld)
	}
	sharedStore.SetSharedAt(nTieRecent, parseTimeHandler("2026-01-10T13:00:00Z"))
	sharedStore.SetIsAdminNote(nTieRecent, false)
	sharedStore.SetSharedAt(nTieOld, parseTimeHandler("2026-01-10T10:30:00Z"))
	sharedStore.SetIsAdminNote(nTieOld, false)
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/groups/"+groupID+"/notes", nil)
	req2.Header.Set("Authorization", "Bearer "+mt)
	r.ServeHTTP(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("list tie group notes %d %s", w2.Code, w2.Body.String())
	}
	var resp2 map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	arr2, ok := resp2["notes"].([]interface{})
	if !ok || len(arr2) != 6 {
		t.Fatalf("expected 6 notes tie got %v", resp2["notes"])
	}
	// Orden esperado: AdminHigh(5), AdminLow(1) luego MemberHigh(10), TieRecent(3 reciente), TieOld(3 viejo), MemberLow(2)
	expected := []string{"AdminHighH", "AdminLowH", "MemberHighH", "TieRecentH", "TieOldH", "MemberLowH"}
	for i, exp := range expected {
		if getTitle(arr2[i]) != exp {
			t.Fatalf("orden tie pos %d esperado %s got %s", i, exp, getTitle(arr2[i]))
		}
	}
	// Verificación adicional de desempate
	var idxRecent, idxOld int = -1, -1
	for i, m := range arr2 {
		title := getTitle(m)
		if title == "TieRecentH" {
			idxRecent = i
		}
		if title == "TieOldH" {
			idxOld = i
		}
	}
	if idxRecent == -1 || idxOld == -1 {
		t.Fatalf("no se encontraron tie notes")
	}
	if idxRecent > idxOld {
		t.Fatalf("desempate handler falló: TieRecent debe antes que TieOld, idxRecent=%d idxOld=%d", idxRecent, idxOld)
	}
}

func parseTimeHandler(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
