package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/model"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

type fakeUserDirectory struct {
	out []model.PublicUser
	err error
}

func (f *fakeUserDirectory) GetPublicByIDs(ctx context.Context, ids []string) ([]model.PublicUser, error) {
	return f.out, f.err
}

func TestInternalUsers_Lookup_200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	name := "Agustín Vega"
	h := NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{
		out: []model.PublicUser{{UserID: "u1", Email: "u1@example.com", DisplayName: &name}},
	}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/users/lookup?ids=u1,u2", nil)
	h.Lookup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []model.PublicUser
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("respuesta no es array: %v", err)
	}
	if len(got) != 1 || got[0].Email != "u1@example.com" || got[0].DisplayName == nil {
		t.Fatalf("payload inesperado: %+v", got)
	}
}

func TestInternalUsers_Lookup_SinIDs_Vacio(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewInternalUsersHandler(service.NewUsersService(&fakeUserDirectory{}))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/internal/users/lookup", nil)
	h.Lookup(c)
	if w.Code != http.StatusOK {
		t.Fatalf("esperado 200, got %d", w.Code)
	}
	if w.Body.String() != "[]" {
		t.Fatalf("sin ids debe dar [], got %s", w.Body.String())
	}
}
