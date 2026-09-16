package service

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const clavePrueba = "clave-solo-para-pruebas-no-usar-en-produccion"

const uuidStudent = "550e8400-e29b-41d4-a716-446655440001"
const uuidTeacher = "550e8400-e29b-41d4-a716-446655440002"

func nuevoServicioPrueba(t *testing.T) *JWTService {
	t.Helper()

	servicio, err := NuevoJWTService(ConfiguracionJWT{
		ClaveSecreta: []byte(clavePrueba),
		Issuer:       "apuntes-auth",
		Audience:     "apuntes-client",
		Duracion:     15 * time.Minute,
	})
	if err != nil {
		t.Fatalf("no se pudo crear el servicio JWT de prueba: %v", err)
	}

	return servicio
}

func generarTokenPrueba(
	t *testing.T,
	clave []byte,
	issuer string,
	audience string,
	userID string,
	role string,
	expiraEn time.Time,
) string {
	t.Helper()

	claims := ClaimsPersonalizadas{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(expiraEn),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	tokenString, err := token.SignedString(clave)
	if err != nil {
		t.Fatalf("no se pudo generar token de prueba: %v", err)
	}

	return tokenString
}

func TestJWTServiceGenerarYValidarAccessToken(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	resultado, err := servicio.GenerarAccessToken(UsuarioAutenticado{
		ID:   uuidStudent,
		Role: RolStudent,
	})
	if err != nil {
		t.Fatalf("no se pudo generar el access token: %v", err)
	}

	if resultado.AccessToken == "" {
		t.Fatal("se esperaba un access token no vacío")
	}

	if !resultado.ExpiraEn.After(time.Now()) {
		t.Fatal("se esperaba una fecha de expiración futura")
	}

	usuario, err := servicio.ValidarAccessToken(resultado.AccessToken)
	if err != nil {
		t.Fatalf("se esperaba token válido, se recibió error: %v", err)
	}

	if usuario.ID != uuidStudent {
		t.Fatalf("user_id esperado: %s; recibido: %s", uuidStudent, usuario.ID)
	}

	if usuario.Role != RolStudent {
		t.Fatalf("role esperado: %s; recibido: %q", RolStudent, usuario.Role)
	}
}

func TestJWTServiceGeneraTokenParaTeacher(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	resultado, err := servicio.GenerarAccessToken(UsuarioAutenticado{
		ID:   uuidTeacher,
		Role: RolTeacher,
	})
	if err != nil {
		t.Fatalf("no se pudo generar el access token: %v", err)
	}

	usuario, err := servicio.ValidarAccessToken(resultado.AccessToken)
	if err != nil {
		t.Fatalf("se esperaba token válido, se recibió error: %v", err)
	}

	if usuario.ID != uuidTeacher || usuario.Role != RolTeacher {
		t.Fatalf(
			"usuario esperado: ID=%s, role=%s; recibido: ID=%s, role=%s",
			uuidTeacher,
			RolTeacher,
			usuario.ID,
			usuario.Role,
		)
	}
}

func TestJWTServiceRechazaUsuarioConIDInvalido(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	_, err := servicio.GenerarAccessToken(UsuarioAutenticado{
		ID:   "",
		Role: RolStudent,
	})

	if !errors.Is(err, ErrUsuarioIDInvalido) {
		t.Fatalf("se esperaba ErrUsuarioIDInvalido; recibido: %v", err)
	}
}

func TestJWTServiceRechazaRolVacio(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	_, err := servicio.GenerarAccessToken(UsuarioAutenticado{
		ID:   uuidStudent,
		Role: "   ",
	})

	if !errors.Is(err, ErrRolVacio) {
		t.Fatalf("se esperaba ErrRolVacio; recibido: %v", err)
	}
}

func TestJWTServiceRechazaRolGlobalInvalido(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	_, err := servicio.GenerarAccessToken(UsuarioAutenticado{
		ID:   uuidStudent,
		Role: "admin",
	})

	if !errors.Is(err, ErrRolInvalido) {
		t.Fatalf("se esperaba ErrRolInvalido; recibido: %v", err)
	}
}

func TestJWTServiceRechazaTokenVacio(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	_, err := servicio.ValidarAccessToken("   ")

	if !errors.Is(err, ErrTokenVacio) {
		t.Fatalf("se esperaba ErrTokenVacio; recibido: %v", err)
	}
}

func TestJWTServiceRechazaFirmaConClaveIncorrecta(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte("otra-clave-distinta"),
		"apuntes-auth",
		"apuntes-client",
		uuidStudent,
		RolStudent,
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)
	if err == nil {
		t.Fatal("se esperaba error para un token firmado con otra clave")
	}
}

func TestJWTServiceRechazaTokenExpirado(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"apuntes-auth",
		"apuntes-client",
		uuidStudent,
		RolStudent,
		time.Now().Add(-15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)
	if err == nil {
		t.Fatal("se esperaba error para un token expirado")
	}
}

func TestJWTServiceRechazaIssuerIncorrecto(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"otro-auth",
		"apuntes-client",
		uuidStudent,
		RolStudent,
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)
	if err == nil {
		t.Fatal("se esperaba error para issuer incorrecto")
	}
}

func TestJWTServiceRechazaAudienceIncorrecta(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"apuntes-auth",
		"otra-aplicacion",
		uuidStudent,
		RolStudent,
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)
	if err == nil {
		t.Fatal("se esperaba error para audience incorrecta")
	}
}

func TestJWTServiceRechazaUserIDInvalidoEnToken(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"apuntes-auth",
		"apuntes-client",
		"",
		RolStudent,
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)

	if !errors.Is(err, ErrUsuarioIDInvalido) {
		t.Fatalf("se esperaba ErrUsuarioIDInvalido; recibido: %v", err)
	}
}

func TestJWTServiceRechazaRolVacioEnToken(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"apuntes-auth",
		"apuntes-client",
		uuidStudent,
		"   ",
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)

	if !errors.Is(err, ErrRolVacio) {
		t.Fatalf("se esperaba ErrRolVacio; recibido: %v", err)
	}
}

func TestJWTServiceRechazaRolInvalidoEnToken(t *testing.T) {
	servicio := nuevoServicioPrueba(t)

	token := generarTokenPrueba(
		t,
		[]byte(clavePrueba),
		"apuntes-auth",
		"apuntes-client",
		uuidStudent,
		"admin",
		time.Now().Add(15*time.Minute),
	)

	_, err := servicio.ValidarAccessToken(token)

	if !errors.Is(err, ErrRolInvalido) {
		t.Fatalf("se esperaba ErrRolInvalido; recibido: %v", err)
	}
}

func TestNuevoJWTServiceRechazaConfiguracionInvalida(t *testing.T) {
	pruebas := []struct {
		nombre string
		config ConfiguracionJWT
		err    error
	}{
		{
			nombre: "clave secreta vacía",
			config: ConfiguracionJWT{
				ClaveSecreta: []byte(""),
				Issuer:       "apuntes-auth",
				Audience:     "apuntes-client",
				Duracion:     15 * time.Minute,
			},
			err: ErrClaveSecretaVacia,
		},
		{
			nombre: "issuer vacío",
			config: ConfiguracionJWT{
				ClaveSecreta: []byte("clave"),
				Issuer:       "   ",
				Audience:     "apuntes-client",
				Duracion:     15 * time.Minute,
			},
			err: ErrIssuerVacio,
		},
		{
			nombre: "audience vacía",
			config: ConfiguracionJWT{
				ClaveSecreta: []byte("clave"),
				Issuer:       "apuntes-auth",
				Audience:     "   ",
				Duracion:     15 * time.Minute,
			},
			err: ErrAudienceVacia,
		},
		{
			nombre: "duración inválida",
			config: ConfiguracionJWT{
				ClaveSecreta: []byte("clave"),
				Issuer:       "apuntes-auth",
				Audience:     "apuntes-client",
				Duracion:     0,
			},
			err: ErrDuracionInvalida,
		},
	}

	for _, prueba := range pruebas {
		t.Run(prueba.nombre, func(t *testing.T) {
			_, err := NuevoJWTService(prueba.config)

			if !errors.Is(err, prueba.err) {
				t.Fatalf("se esperaba %v; recibido: %v", prueba.err, err)
			}
		})
	}
}
