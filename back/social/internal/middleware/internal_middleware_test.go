package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequireInternalKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	call := func(expected, got string) int {
		r := gin.New()
		r.Use(RequireInternalKey(expected))
		r.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if got != "" {
			req.Header.Set("X-Internal-Key", got)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := call("k", "k"); code != http.StatusNoContent {
		t.Fatalf("key válida debe pasar, got %d", code)
	}
	if code := call("k", "otra"); code != http.StatusUnauthorized {
		t.Fatalf("key mala debe dar 401, got %d", code)
	}
	if code := call("k", ""); code != http.StatusUnauthorized {
		t.Fatalf("sin key debe dar 401, got %d", code)
	}
	if code := call("", "k"); code != http.StatusServiceUnavailable {
		t.Fatalf("sin secreto configurado debe dar 503 (fail closed), got %d", code)
	}
}
