package http

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
	"net/http/httptest"
	"testing"
)

func TestSprintSheetRoutes(t *testing.T) {
	gid, user, outsider := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := service.NewMemorySprintStore()
	store.AddGroup(gid, user)
	h := NewSprintHandler(service.NewSprintService(store))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("user_id", u)
		}
	})
	r.GET("/groups/:id/sprint-sheets", h.ListSheets)
	r.POST("/groups/:id/sprint-sheets", h.CreateSheet)
	r.PATCH("/groups/:id/sprint-sheets/:sheetId", h.UpdateSheet)
	r.DELETE("/groups/:id/sprint-sheets/:sheetId", h.DeleteSheet)
	call := func(method, path, uid, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", uid)
		r.ServeHTTP(w, req)
		return w
	}
	path := "/groups/" + gid + "/sprint-sheets"
	if w := call("GET", path, "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := call("GET", path, outsider, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w := call("POST", path, user, `{"name":""}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	w := call("POST", path, user, `{"name":"Sprint 2","period_start":"2026-09-30","period_end":"2026-10-15"}`)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var body struct {
		Data struct {
			ID    string `json:"id"`
			Start string `json:"period_start"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.Start != "2026-09-30" || body.Data.ID == "" {
		t.Fatal(w.Body.String())
	}
	if w := call("PATCH", path+"/"+body.Data.ID, user, `{"name":"Sprint actualizado"}`); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := call("PATCH", path+"/"+body.Data.ID, user, `{"period_end":"2020-01-01"}`); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := call("GET", path, user, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
	// DELETE: último sprint protegido con mensaje humano.
	if w := call("DELETE", path+"/"+body.Data.ID, user, ""); w.Code != 400 {
		t.Fatalf("último sprint: esperado 400, got %d %s", w.Code, w.Body.String())
	} else if got := w.Body.String(); !bytes.Contains([]byte(got), []byte("único sprint")) {
		t.Fatalf("mensaje humano esperado, got %s", got)
	}
	if w := call("DELETE", path+"/"+body.Data.ID, outsider, ""); w.Code != 403 {
		t.Fatal(w.Code)
	}
	w2 := call("POST", path, user, `{"name":"Sprint 3","period_start":"2026-09-30","period_end":"2026-10-15"}`)
	if w2.Code != 201 {
		t.Fatal(w2.Code, w2.Body.String())
	}
	var second struct {
		Data struct {
			ID string `json:"id"`
		}
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if w := call("DELETE", path+"/"+second.Data.ID, user, ""); w.Code != 200 {
		t.Fatalf("borrar segundo: %d %s", w.Code, w.Body.String())
	}
	if w := call("GET", path, user, ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}
