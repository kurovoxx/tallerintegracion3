package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrClaveSecretaVacia = errors.New("la clave secreta JWT no puede estar vacía")
	ErrIssuerVacio       = errors.New("el issuer JWT no puede estar vacío")
	ErrAudienceVacia     = errors.New("la audience JWT no puede estar vacía")
	ErrDuracionInvalida  = errors.New("la duración del access token debe ser mayor que cero")

	ErrUsuarioIDInvalido = errors.New("el user_id no puede estar vacío")

	ErrTokenVacio        = errors.New("el access token no puede estar vacío")
	ErrAlgoritmoInvalido = errors.New("el algoritmo de firma no está permitido")
	ErrTokenInvalido     = errors.New("el access token no es válido")
)

type ClaimsPersonalizadas struct {
	UserID string `json:"user_id"`
	jwt.RegisteredClaims
}

type ConfiguracionJWT struct {
	ClaveSecreta []byte
	Issuer       string
	Audience     string
	Duracion     time.Duration
}

type ResultadoToken struct {
	AccessToken string
	ExpiraEn    time.Time
}

type UsuarioAutenticado struct {
	ID string
}

type JWTService struct {
	config ConfiguracionJWT
}

func NuevoJWTService(config ConfiguracionJWT) (*JWTService, error) {
	if len(config.ClaveSecreta) == 0 {
		return nil, ErrClaveSecretaVacia
	}

	config.Issuer = strings.TrimSpace(config.Issuer)
	if config.Issuer == "" {
		return nil, ErrIssuerVacio
	}

	config.Audience = strings.TrimSpace(config.Audience)
	if config.Audience == "" {
		return nil, ErrAudienceVacia
	}

	if config.Duracion <= 0 {
		return nil, ErrDuracionInvalida
	}

	return &JWTService{
		config: config,
	}, nil
}

func (s *JWTService) GenerarAccessToken(usuario UsuarioAutenticado) (ResultadoToken, error) {
	if strings.TrimSpace(usuario.ID) == "" {
		return ResultadoToken{}, ErrUsuarioIDInvalido
	}

	ahora := time.Now()
	expiraEn := ahora.Add(s.config.Duracion)

	claims := ClaimsPersonalizadas{
		UserID: usuario.ID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.config.Issuer,
			Audience:  jwt.ClaimStrings{s.config.Audience},
			IssuedAt:  jwt.NewNumericDate(ahora),
			ExpiresAt: jwt.NewNumericDate(expiraEn),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	accessToken, err := token.SignedString(s.config.ClaveSecreta)
	if err != nil {
		return ResultadoToken{}, fmt.Errorf("firmar access token: %w", err)
	}

	return ResultadoToken{
		AccessToken: accessToken,
		ExpiraEn:    expiraEn,
	}, nil
}

func (s *JWTService) ValidarAccessToken(tokenString string) (UsuarioAutenticado, error) {
	if strings.TrimSpace(tokenString) == "" {
		return UsuarioAutenticado{}, ErrTokenVacio
	}

	claims := &ClaimsPersonalizadas{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
				return nil, ErrAlgoritmoInvalido
			}

			return s.config.ClaveSecreta, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.config.Issuer),
		jwt.WithAudience(s.config.Audience),
	)
	if err != nil {
		return UsuarioAutenticado{}, fmt.Errorf("validar access token: %w", err)
	}

	if !token.Valid {
		return UsuarioAutenticado{}, ErrTokenInvalido
	}

	if strings.TrimSpace(claims.UserID) == "" {
		return UsuarioAutenticado{}, ErrUsuarioIDInvalido
	}

	return UsuarioAutenticado{
		ID: claims.UserID,
	}, nil
}
