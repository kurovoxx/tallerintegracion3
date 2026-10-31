package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/social/internal/utils"
)

func TestRequireAuthWithSignedJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := []byte("social-test-signing-secret")
	const userID = "aaaaaaaa-0000-4000-8000-000000000001"
	validClaims := func() ClaimsPersonalizadas {
		return ClaimsPersonalizadas{
			UserID: userID,
			Role:   "member",
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer:    "identity",
				Audience:  jwt.ClaimStrings{"student-platform"},
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		}
	}
	sign := func(claims ClaimsPersonalizadas, method jwt.SigningMethod, key []byte) string {
		t.Helper()
		token, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return token
	}

	tests := []struct {
		name   string
		header string
		change func(*ClaimsPersonalizadas)
		method jwt.SigningMethod
		key    []byte
		code   string
	}{
		{name: "sin Authorization", code: utils.ErrUnauthorized},
		{name: "esquema incorrecto", header: "Basic secret", code: utils.ErrUnauthorized},
		{name: "Bearer vacío", header: "Bearer   ", code: utils.ErrUnauthorized},
		{name: "JWT malformado", header: "Bearer not-a-jwt", code: utils.ErrInvalidToken},
		{name: "firma de otra clave", key: []byte("another-secret"), code: utils.ErrInvalidToken},
		{name: "otro algoritmo HMAC", method: jwt.SigningMethodHS384, code: utils.ErrInvalidToken},
		{name: "expirado", change: func(c *ClaimsPersonalizadas) {
			c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
		}, code: utils.ErrTokenExpired},
		{name: "todavía no válido", change: func(c *ClaimsPersonalizadas) {
			c.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
		}, code: utils.ErrInvalidToken},
		{name: "issuer incorrecto", change: func(c *ClaimsPersonalizadas) { c.Issuer = "other" }, code: utils.ErrInvalidToken},
		{name: "audience incorrecta", change: func(c *ClaimsPersonalizadas) { c.Audience = jwt.ClaimStrings{"other"} }, code: utils.ErrInvalidToken},
		{name: "sin user_id", change: func(c *ClaimsPersonalizadas) { c.UserID = "  " }, code: utils.ErrInvalidToken},
		{name: "JWT válido"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header := tc.header
			if header == "" && tc.name != "sin Authorization" {
				claims := validClaims()
				if tc.change != nil {
					tc.change(&claims)
				}
				method, key := tc.method, tc.key
				if method == nil {
					method = jwt.SigningMethodHS256
				}
				if key == nil {
					key = secret
				}
				header = "Bearer " + sign(claims, method, key)
			}

			called := false
			router := gin.New()
			router.Use(NewAuthMiddleware(&SimpleHS256Validator{
				Secret: secret, Issuer: "identity", Audience: "student-platform",
			}).RequireAuth())
			router.GET("/protected", func(c *gin.Context) {
				called = true
				if id, ok := GetUserID(c); !ok || id != userID {
					t.Errorf("user_id de Gin = %q, %v", id, ok)
				}
				if id, ok := GetUserIDFromContext(c.Request.Context()); !ok || id != userID {
					t.Errorf("user_id de contexto = %q, %v", id, ok)
				}
				if role, ok := GetRole(c); !ok || role != "member" {
					t.Errorf("rol = %q, %v", role, ok)
				}
				c.Status(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/protected?user_id=attacker&role=admin", nil)
			req.Header.Set("Authorization", header)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if tc.code == "" {
				if w.Code != http.StatusNoContent || !called {
					t.Fatalf("JWT válido: status=%d llamado=%v body=%s", w.Code, called, w.Body)
				}
				return
			}
			var body utils.ErrorResponse
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != http.StatusUnauthorized || called || body.Error.Code != tc.code {
				t.Fatalf("status=%d llamado=%v error=%q; esperado 401, false, %q", w.Code, called, body.Error.Code, tc.code)
			}
		})
	}
}
