package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	testPNGBytes  = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	testJPEGBytes = []byte("\xff\xd8\xff\xe0\x00\x10JFIF")
)

// TestHandlerUploadDirectToDrive verifica POST /notes/upload: sube el binario
// al Drive del usuario autenticado, preserva el MIME declarado y devuelve el
// external_file_id con su enlace web.
func TestHandlerUploadDirectToDrive(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")

	w := doMultipart(t, r, "/notes/upload", token, "captura.png", "application/octet-stream", testPNGBytes)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload directo esperaba 201, got %d %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json inválido: %v", err)
	}
	extID, _ := resp["external_file_id"].(string)
	if extID == "" {
		t.Fatal("external_file_id vacío")
	}
	if got, _ := resp["file_type"].(string); got != "image/png" {
		t.Fatalf("file_type detectado = %q, want image/png", got)
	}
	url, _ := resp["file_url"].(string)
	if !strings.Contains(url, extID) {
		t.Fatalf("file_url %q no contiene el file id %q", url, extID)
	}
	if size, _ := resp["file_size_bytes"].(float64); int(size) != len(testPNGBytes) {
		t.Fatalf("file_size_bytes = %v, want %d", size, len(testPNGBytes))
	}
	if mt, ok := driveMock.FileMimeType(extID); !ok || mt != "image/png" {
		t.Fatalf("mime en Drive = %q (ok=%v), want image/png", mt, ok)
	}
}

// TestHandlerUploadMarkdownThenLink reproduce el flujo del mandato: subida
// directa a Drive y posterior vinculación a la nota vía external_file_id.
func TestHandlerUploadMarkdownThenLink(t *testing.T) {
	r, svc, driveMock, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")

	md := []byte("# Apunte\n\ncontenido markdown")
	w := doMultipart(t, r, "/notes/upload", token, "apunte.md", "application/octet-stream", md)
	if w.Code != http.StatusCreated {
		t.Fatalf("upload md esperaba 201, got %d %s", w.Code, w.Body.String())
	}
	var up map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &up)
	extID, _ := up["external_file_id"].(string)
	if extID == "" {
		t.Fatal("external_file_id vacío")
	}
	if got, _ := up["file_type"].(string); got != "text/markdown" {
		t.Fatalf("file_type = %q, want text/markdown", got)
	}
	if mt, _ := driveMock.FileMimeType(extID); mt != "text/markdown" {
		t.Fatalf("mime en Drive = %q, want text/markdown", mt)
	}

	nid := createNoteHTTP(t, r, token, "Nota con adjunto", "private", "c")
	body := fmt.Sprintf(`{"external_file_id":%q,"file_name":"apunte.md","file_type":"application/octet-stream","is_inline":true}`, extID)
	w2 := doReq(r, http.MethodPost, "/notes/"+nid+"/attachments", token, body)
	if w2.Code != http.StatusCreated {
		t.Fatalf("vincular adjunto esperaba 201, got %d %s", w2.Code, w2.Body.String())
	}
	var att map[string]interface{}
	json.Unmarshal(w2.Body.Bytes(), &att)
	if att["attachment_id"] == nil || att["attachment_id"] == "" {
		t.Fatal("attachment_id vacío")
	}
	if att["external_file_id"] != extID {
		t.Fatalf("external_file_id = %v, want %q", att["external_file_id"], extID)
	}
	if att["is_inline"] != true {
		t.Fatalf("is_inline = %v, want true", att["is_inline"])
	}
	list, err := svc.ListAttachments(context.Background(), nid)
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("adjuntos = %d, want 1", len(list))
	}
	if list[0].FileType != "text/markdown" {
		t.Fatalf("file_type persistido = %q, want text/markdown (detección por extensión)", list[0].FileType)
	}
}

// TestHandlerUploadRequiresAuth: sin JWT => 401.
func TestHandlerUploadRequiresAuth(t *testing.T) {
	r, _, _, _ := setupRouter()
	w := doMultipart(t, r, "/notes/upload", "", "f.png", "application/octet-stream", testPNGBytes)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sin token esperaba 401, got %d %s", w.Code, w.Body.String())
	}
}

// TestHandlerUploadEmptyFile: binario de 0 bytes => 400.
func TestHandlerUploadEmptyFile(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	w := doMultipart(t, r, "/notes/upload", token, "vacio.png", "image/png", []byte{})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("archivo vacío esperaba 400, got %d %s", w.Code, w.Body.String())
	}
}

// TestHandlerUploadTooLarge: > 10MB => 413.
func TestHandlerUploadTooLarge(t *testing.T) {
	r, _, _, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	big := make([]byte, 10*1024*1024+1)
	w := doMultipart(t, r, "/notes/upload", token, "grande.pdf", "application/pdf", big)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("10MB+1 esperaba 413, got %d %s", w.Code, w.Body.String())
	}
}

// TestHandlerUploadOAuthDisconnected: usuario sin conexión Drive => 403 con
// mensaje de reconexión, sin filtrar detalles internos.
func TestHandlerUploadOAuthDisconnected(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	driveMock.Disconnect(userID)
	defer driveMock.Reconnect(userID)
	w := doMultipart(t, r, "/notes/upload", token, "f.png", "image/png", testPNGBytes)
	if w.Code != http.StatusForbidden {
		t.Fatalf("OAuth ausente esperaba 403, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Google Drive") {
		t.Fatalf("mensaje debe pedir reconectar Drive: %s", w.Body.String())
	}
}

// TestHandlerAttachmentPreservesDeclaredMime: el MIME declarado por el cliente
// en la parte multipart se preserva al registrar el adjunto de la nota.
func TestHandlerAttachmentPreservesDeclaredMime(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	nid := createNoteHTTP(t, r, token, "MIME declarado", "private", "c")

	w := doMultipart(t, r, "/notes/"+nid+"/attachments", token, "foto.jpg", "image/jpeg", testJPEGBytes)
	if w.Code != http.StatusCreated {
		t.Fatalf("adjunto esperaba 201, got %d %s", w.Code, w.Body.String())
	}
	var att map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &att)
	extID, _ := att["external_file_id"].(string)
	if extID == "" {
		t.Fatal("external_file_id vacío")
	}
	if mt, _ := driveMock.FileMimeType(extID); mt != "image/jpeg" {
		t.Fatalf("mime en Drive = %q, want image/jpeg", mt)
	}
}

// TestHandlerAttachmentDetectsMimeByExtension: sin MIME específico, la
// extensión resuelve el tipo (application/pdf).
func TestHandlerAttachmentDetectsMimeByExtension(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	userID := uuid.NewString()
	token := genToken(userID, "student")
	nid := createNoteHTTP(t, r, token, "MIME por extensión", "private", "c")

	w := doMultipart(t, r, "/notes/"+nid+"/attachments", token, "informe.pdf", "application/octet-stream", testPNGBytes)
	if w.Code != http.StatusCreated {
		t.Fatalf("adjunto esperaba 201, got %d %s", w.Code, w.Body.String())
	}
	var att map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &att)
	extID, _ := att["external_file_id"].(string)
	if mt, _ := driveMock.FileMimeType(extID); mt != "application/pdf" {
		t.Fatalf("mime en Drive = %q, want application/pdf (extensión .pdf)", mt)
	}
}

// doMultipart ejecuta la petición multipart contra el router dado.
func doMultipart(t *testing.T, r *gin.Engine, path, token, filename, partType string, data []byte) *httptest.ResponseRecorder {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	if partType != "" {
		h.Set("Content-Type", partType)
	}
	part, err := writer.CreatePart(h)
	if err != nil {
		t.Fatalf("CreatePart: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}
