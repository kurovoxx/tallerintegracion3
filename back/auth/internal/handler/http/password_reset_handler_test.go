package http

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

type resetHandlerStore struct{}

func (resetHandlerStore) Issue(context.Context, string, string) (bool, error) { return false, nil }
func (resetHandlerStore) Complete(context.Context, string, string, string) (bool, error) {
	return false, nil
}

type resetHandlerMailer struct{}

func (resetHandlerMailer) Ready() bool                                         { return true }
func (resetHandlerMailer) SendResetCode(context.Context, string, string) error { return nil }

func TestPasswordRecoveryHTTP(t *testing.T) {
	users := newMockUserRepoH()
	_, _ = users.CreateUser(context.Background(), "user@example.com", "hash", "User", nil, nil, nil, nil, nil)
	h := NewPasswordResetHandler(service.NewPasswordResetService(users, resetHandlerStore{}, resetHandlerMailer{}))
	r := gin.New()
	r.POST("/auth/forgot-password", h.Forgot)
	r.POST("/auth/reset-password", h.Reset)
	var generic string
	for _, email := range []string{"user@example.com", "unknown@example.com"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/auth/forgot-password", strings.NewReader(`{"email":"`+email+`"}`)))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("unexpected response: %d", w.Code)
		}
		if generic != "" && generic != w.Body.String() {
			t.Fatal("response leaks account existence")
		}
		generic = w.Body.String()
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"forgot-password", `{}`, 400}, {"forgot-password", `{"email":"bad"}`, 400},
		{"forgot-password", `{"email":"` + strings.Repeat("a", 5000) + `"}`, 400},
		{"reset-password", `{}`, 400},
		{"reset-password", `{"email":"user@example.com","code":"12345678","password":"Password9"}`, 400},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/auth/"+tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: got %d want %d", tc.path, w.Code, tc.status)
		}
	}
}
