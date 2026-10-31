package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() { gin.SetMode(gin.TestMode) }

const testSecret = "middleware-test-secret-32-chars!!"

func mint(userID, secret string, dur time.Duration) string {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    TokenIssuer,
			Audience:  jwt.ClaimStrings{TokenAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(dur)),
		},
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		panic(err)
	}
	return s
}

func runMw(token string) int {
	mw := NewAuthMiddleware(&SimpleHS256Validator{Secret: []byte(testSecret), Issuer: TokenIssuer, Audience: TokenAudience})
	r := gin.New()
	r.GET("/x", mw.RequireAuth(), func(c *gin.Context) {
		uid, _ := GetUserID(c)
		c.JSON(http.StatusOK, gin.H{"user_id": uid})
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRequireAuth_OK(t *testing.T) {
	mw := NewAuthMiddleware(&SimpleHS256Validator{Secret: []byte(testSecret), Issuer: TokenIssuer, Audience: TokenAudience})
	r := gin.New()
	r.GET("/x", mw.RequireAuth(), func(c *gin.Context) {
		uid, ok := GetUserID(c)
		if !ok || uid != "u-1" {
			t.Fatalf("user_id inyectado: %q %v", uid, ok)
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+mint("u-1", testSecret, time.Hour))
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("esperado 200, got %d", w.Code)
	}
}

func TestRequireAuth_401(t *testing.T) {
	if code := runMw(""); code != 401 {
		t.Fatalf("sin header: 401, got %d", code)
	}
	// Sin "Bearer ".
	mw := NewAuthMiddleware(&SimpleHS256Validator{Secret: []byte(testSecret), Issuer: TokenIssuer, Audience: TokenAudience})
	r := gin.New()
	r.GET("/x", mw.RequireAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Token abc")
	r.ServeHTTP(w, req)
	if w.Code != 401 {
		t.Fatalf("formato malo: 401, got %d", w.Code)
	}
	if code := runMw("malformado"); code != 401 {
		t.Fatalf("malformado: 401, got %d", code)
	}
	if code := runMw(mint("u-1", "otro-secreto-32-chars-long-xxxxx", time.Hour)); code != 401 {
		t.Fatalf("otro secreto: 401, got %d", code)
	}
	if code := runMw(mint("u-1", testSecret, -time.Hour)); code != 401 {
		t.Fatalf("expirado: 401, got %d", code)
	}
}
