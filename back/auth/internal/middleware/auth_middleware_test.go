package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/kurovoxx/tallerintegracion3/back/auth/internal/service"
)

// Helpers para tests

const clavePruebaMw = "clave-solo-para-pruebas-middleware-no-usar-en-produccion"

func nuevoJWTPrueba(t *testing.T) *service.JWTService {
	t.Helper()
	svc, err := service.NuevoJWTService(service.ConfiguracionJWT{
		ClaveSecreta: []byte(clavePruebaMw),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("no se pudo crear JWTService de prueba: %v", err)
	}
	return svc
}

func generarTokenPruebaMw(t *testing.T, clave []byte, issuer, audience, userID string, expiraEn time.Time) string {
	t.Helper()
	claims := service.ClaimsPersonalizadas{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiraEn),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := token.SignedString(clave)
	if err != nil {
		t.Fatalf("no se pudo firmar token: %v", err)
	}
	return s
}

func init() { gin.SetMode(gin.TestMode) }

// 1) Token válido -> 200 y contexto inyectado solo con user_id
func TestMiddlewareTokenValido(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	res, err := jwtSvc.GenerarAccessToken(service.UsuarioAutenticado{ID: "550e8400-e29b-41d4-a716-446655440001"})
	if err != nil {
		t.Fatalf("generar token: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c.Request.Header.Set("Authorization", "Bearer "+res.AccessToken)

	nextCalled := false
	var gotID string
	handler := func(c *gin.Context) {
		nextCalled = true
		if id, ok := GetUserID(c); ok {
			gotID = id
		}
		c.JSON(200, gin.H{"ok": true})
	}

	mw.RequireAuth()(c)
	if c.IsAborted() {
		t.Fatalf("middleware abortó con token válido, status %d body %s", w.Code, w.Body.String())
	}
	if !c.IsAborted() {
		handler(c)
	}

	if !nextCalled {
		t.Fatal("handler siguiente no fue llamado con token válido")
	}
	if gotID != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("user_id esperado 550e...0001, got %q", gotID)
	}
	if w.Code != 200 {
		t.Fatalf("status esperado 200, got %d", w.Code)
	}
	if id, ok := GetUserIDFromContext(c.Request.Context()); !ok || id != gotID {
		t.Fatalf("context.Context user_id no inyectado")
	}
	// Verificar que no se inyecta role
	if _, exists := c.Get("role"); exists {
		t.Fatal("role no debe existir en contexto")
	}
}

// 2) Token expirado -> 401
func TestMiddlewareTokenExpirado(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	token := generarTokenPruebaMw(t, []byte(clavePruebaMw), "apuntes-auth", "apuntes-client", "550e8400-e29b-41d4-a716-446655440001", time.Now().Add(-15*time.Minute))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	mw.RequireAuth()(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 para token expirado, got %d body %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body no es JSON: %v", err)
	}
	if _, ok := body["error"]; !ok {
		t.Fatalf("esperado {error:{code}} para token expirado, got %s", w.Body.String())
	}
	if c.IsAborted() == false {
		t.Fatal("middleware debería hacer Abort() en token expirado")
	}
}

// 3) Token malformado -> 401
func TestMiddlewareTokenMalformado(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	casos := []string{
		"malformado",
		"eyJhbGciOiJIUzI1NiJ9.eyJtYWwiOiJqc29uIn0.firma-invalida",
		"Bearer",
		"eyJhbGciOiJub25lIn0.eyJ1c2VyX2lkIjoiMTIzIn0.",
	}

	for i, token := range casos {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
		auth := "Bearer " + token
		if token == "Bearer" {
			auth = token
		}
		c.Request.Header.Set("Authorization", auth)

		mw.RequireAuth()(c)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("caso %d token malformado %q esperado 401 got %d body %s", i, token, w.Code, w.Body.String())
		}
	}
}

// 4) Sin header Authorization -> 401
func TestMiddlewareSinHeaderAuthorization(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)

	mw.RequireAuth()(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 sin header, got %d body %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if _, ok := body["error"]; !ok {
		t.Fatalf("esperado {error:{code}} sin header, got %s", w.Body.String())
	}

	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c2.Request.Header.Set("Authorization", "Bearer ")
	mw.RequireAuth()(c2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 Bearer vacío, got %d", w2.Code)
	}

	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c3.Request.Header.Set("Authorization", "Token abc123")
	mw.RequireAuth()(c3)
	if w3.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 formato Token, got %d", w3.Code)
	}
}

// 5) Firma inválida (otra clave) -> 401
func TestMiddlewareFirmaInvalida(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	token := generarTokenPruebaMw(t, []byte("otra-clave-distinta"), "apuntes-auth", "apuntes-client", "550e8400-e29b-41d4-a716-446655440001", time.Now().Add(15*time.Minute))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	mw.RequireAuth()(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 firma inválida, got %d", w.Code)
	}
}

// 6) Token sin user_id -> 401
func TestMiddlewareUserIDVacioEnToken(t *testing.T) {
	jwtSvc := nuevoJWTPrueba(t)
	mw := NewAuthMiddleware(jwtSvc)

	token := generarTokenPruebaMw(t, []byte(clavePruebaMw), "apuntes-auth", "apuntes-client", "", time.Now().Add(15*time.Minute))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)

	mw.RequireAuth()(c)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 para token sin user_id, got %d body %s", w.Code, w.Body.String())
	}
}
