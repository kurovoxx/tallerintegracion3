package http

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/notes/internal/drive"
)

// GET /notes/me debe exponer forked_from_note_id y sync_status: la app los usa
// para no mostrar como apunte una copia que todavía no llegó a Drive. Antes el
// DTO los omitía y el filtro del cliente nunca se activaba.
func TestListMyExposesCopyStateFields(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	author, copier := uuid.NewString(), uuid.NewString()
	at, ct := genToken(author, "student"), genToken(copier, "student")
	nid := createNoteHTTP(t, r, at, "Origen", "public", "# Hola")

	// Copia fallida de forma ambigua: queda un clon failed_sync sin archivo.
	driveMock.CopyErr = &drive.DriveError{Code: 503, Message: "no disponible"}
	w := doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	driveMock.CopyErr = nil
	if w.Code == http.StatusCreated {
		t.Fatalf("la copia debía fallar: %s", w.Body.String())
	}

	w = doReq(r, http.MethodGet, "/notes/me", ct, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /notes/me: %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Notes []map[string]any `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || len(body.Notes) != 1 {
		t.Fatalf("esperaba 1 clon: %v %s", err, w.Body.String())
	}
	n := body.Notes[0]
	if n["forked_from_note_id"] != nid || n["sync_status"] != "failed_sync" {
		t.Fatalf("faltan campos de estado de copia: %v", n)
	}
	if _, ok := n["external_file_id"]; ok {
		t.Fatalf("un clon sin archivo no debe traer external_file_id: %v", n)
	}

	// Al reintentar con Drive disponible se completa y trae su archivo.
	w = doReq(r, http.MethodPost, "/notes/"+nid+"/copy", ct, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("el reintento debe completar la copia: %d %s", w.Code, w.Body.String())
	}
	w = doReq(r, http.MethodGet, "/notes/me", ct, "")
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Notes) != 1 || body.Notes[0]["sync_status"] != "synced" || body.Notes[0]["external_file_id"] == nil {
		t.Fatalf("la copia completada debe verse sincronizada: %s", w.Body.String())
	}
}

// Sin Drive, POST /notes/reconcile responde 403 drive_not_connected (no un
// error genérico que la app contaba como «apunte con error»).
func TestReconcileWithoutDriveIsDriveNotConnected(t *testing.T) {
	r, _, driveMock, _ := setupRouter()
	user := uuid.NewString()
	driveMock.Disconnect(user)
	defer driveMock.Reconnect(user)
	w := doReq(r, http.MethodPost, "/notes/reconcile", genToken(user, "student"), "")
	if w.Code != http.StatusForbidden {
		t.Fatalf("esperaba 403, got %d %s", w.Code, w.Body.String())
	}
	var errBody map[string]map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &errBody)
	if errBody["error"]["code"] != "drive_not_connected" {
		t.Fatalf("code esperado drive_not_connected: %s", w.Body.String())
	}
}
