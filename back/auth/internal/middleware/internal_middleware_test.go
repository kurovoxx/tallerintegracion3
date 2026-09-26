package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() { gin.SetMode(gin.TestMode) }

func runInternalKey(t *testing.T, expected, header string) int {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/internal/x", nil)
	if header != "" {
		c.Request.Header.Set("X-Internal-Key", header)
	}
	RequireInternalKey(expected)(c)
	return w.Code
}

func TestRequireInternalKey_OK(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/internal/x", nil)
	c.Request.Header.Set("X-Internal-Key", "secreto")
	RequireInternalKey("secreto")(c)
	if c.IsAborted() {
		t.Fatalf("con clave correcta no debe abortar, got %d %s", w.Code, w.Body.String())
	}
}

func TestRequireInternalKey_ClaveMala_401(t *testing.T) {
	if code := runInternalKey(t, "secreto", "otra"); code != 401 {
		t.Fatalf("esperado 401, got %d", code)
	}
}

func TestRequireInternalKey_SinHeader_401(t *testing.T) {
	if code := runInternalKey(t, "secreto", ""); code != 401 {
		t.Fatalf("esperado 401, got %d", code)
	}
}

func TestRequireInternalKey_NoConfigurada_503(t *testing.T) {
	if code := runInternalKey(t, "", "cualquiera"); code != 503 {
		t.Fatalf("esperado 503 si no hay clave configurada, got %d", code)
	}
	if code := runInternalKey(t, "   ", "cualquiera"); code != 503 {
		t.Fatalf("esperado 503 si la clave es vacía, got %d", code)
	}
}
