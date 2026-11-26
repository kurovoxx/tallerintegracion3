package http

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/service"
)

// Un miembro común recibe 403 al modificar el sprint; el admin puede, y ambos
// leen el tiempo restante.
func TestSprintRoutes_SoloAdminsModificanYRemaining(t *testing.T) {
	gid, admin, member := uuid.NewString(), uuid.NewString(), uuid.NewString()
	store := service.NewMemorySprintStore()
	store.AddGroup(gid, admin, member)
	h := NewSprintHandler(service.NewSprintService(store))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("user_id", u)
		}
	})
	r.GET("/groups/:id/sprint-sheets/remaining", h.SprintRemaining)
	r.POST("/groups/:id/sprint-sheets", h.CreateSheet)
	call := func(method, path, uid, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Test-User", uid)
		r.ServeHTTP(w, req)
		return w
	}
	path := "/groups/" + gid + "/sprint-sheets"
	today := time.Now().UTC()
	body := `{"name":"Sprint X","period_start":"` + today.AddDate(0, 0, -5).Format("2006-01-02") +
		`","period_end":"` + today.AddDate(0, 0, 2).Format("2006-01-02") + `"}`

	if w := call("POST", path, member, body); w.Code != 403 {
		t.Fatalf("miembro debe recibir 403, got %d", w.Code)
	}
	if w := call("POST", path, admin, body); w.Code != 201 {
		t.Fatalf("admin debe crear, got %d %s", w.Code, w.Body.String())
	}
	if w := call("GET", path+"/remaining", "", ""); w.Code != 401 {
		t.Fatalf("sin auth 401, got %d", w.Code)
	}
	w := call("GET", path+"/remaining", member, "")
	if w.Code != 200 {
		t.Fatalf("remaining: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data []struct {
			State         string `json:"state"`
			DaysRemaining int    `json:"days_remaining"`
			Notify        bool   `json:"notify"`
			Message       string `json:"message"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, d := range resp.Data {
		if d.State == "active" && d.DaysRemaining == 2 && d.Notify && d.Message != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("esperaba sprint activo con 2 días y notify: %s", w.Body.String())
	}
}
