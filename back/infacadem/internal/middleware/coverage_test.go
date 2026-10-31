package middleware

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Ramas no cubiertas: GetUserID por request-context, GetUserIDFromContext,
// ValidarAccessToken con claims/user_id vacío y emisor/audiencia ajenos.

func TestCoverage_GetUserID_Contexto(t *testing.T) {
	// Solo request-context (sin gin-key).
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest("GET", "/x", nil)
	c.Request = req.WithContext(context.WithValue(req.Context(), contextKeyUserID, "u-ctx"))
	if uid, ok := GetUserID(c); !ok || uid != "u-ctx" {
		t.Fatalf("ctx fallback: %q %v", uid, ok)
	}
	// Gin-key manda sobre contexto.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/x", nil)
	c2.Set(ContextUserIDKey, "u-gin")
	if uid, ok := GetUserID(c2); !ok || uid != "u-gin" {
		t.Fatalf("gin key: %q %v", uid, ok)
	}
	// Gin-key vacía + contexto válido → contexto.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	req3 := httptest.NewRequest("GET", "/x", nil)
	c3.Request = req3.WithContext(context.WithValue(req3.Context(), contextKeyUserID, "u-ctx3"))
	c3.Set(ContextUserIDKey, "")
	if uid, ok := GetUserID(c3); !ok || uid != "u-ctx3" {
		t.Fatalf("gin vacía → ctx: %q %v", uid, ok)
	}
	// GetUserIDFromContext directa.
	ctx := context.WithValue(context.Background(), contextKeyUserID, "u-4")
	if uid, ok := GetUserIDFromContext(ctx); !ok || uid != "u-4" {
		t.Fatalf("from ctx: %q %v", uid, ok)
	}
	if _, ok := GetUserIDFromContext(context.Background()); ok {
		t.Fatal("vacío false")
	}
	if _, ok := GetUserIDFromContext(context.WithValue(context.Background(), contextKeyUserID, "")); ok {
		t.Fatal("vacío false")
	}
}

func mintWith(userID, secret, iss, aud string, dur time.Duration) string {
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: iss, Audience: jwt.ClaimStrings{aud},
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(dur)),
		},
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		panic(err)
	}
	return s
}

func TestCoverage_ValidarAccessToken_Bordes(t *testing.T) {
	v := &SimpleHS256Validator{Secret: []byte(testSecret), Issuer: TokenIssuer, Audience: TokenAudience}
	if _, err := v.ValidarAccessToken("  "); err == nil {
		t.Fatal("vacío debe fallar")
	}
	if _, err := v.ValidarAccessToken(mintWith("", testSecret, TokenIssuer, TokenAudience, time.Hour)); err == nil {
		t.Fatal("user_id vacío debe fallar")
	} else if !strings.Contains(err.Error(), "user_id") {
		t.Fatalf("user_id: %v", err)
	}
	if _, err := v.ValidarAccessToken(mintWith("u", testSecret, "otro-issuer", TokenAudience, time.Hour)); err == nil {
		t.Fatal("issuer ajeno debe fallar")
	}
	if _, err := v.ValidarAccessToken(mintWith("u", testSecret, TokenIssuer, "otra-audience", time.Hour)); err == nil {
		t.Fatal("audience ajena debe fallar")
	}
	if _, err := v.ValidarAccessToken("abc.def.ghi"); err == nil {
		t.Fatal("malformado debe fallar")
	}
	if uid, err := v.ValidarAccessToken(mintWith("u-9", testSecret, TokenIssuer, TokenAudience, time.Hour)); err != nil || uid != "u-9" {
		t.Fatalf("válido: %v %q", err, uid)
	}
}
