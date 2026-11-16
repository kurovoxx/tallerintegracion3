package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	SMTPHost           string
	SMTPPort           string
	SMTPUsername       string
	SMTPPassword       string
	DatabaseURL        string
	DirectURL          string
	JWT                string // kid for ES256 (discovery)
	JWTSecret          string // HS256 secret para access tokens propios
	Port               string
	AccessExpiresIn    int // segundos, default 900
	RefreshExpiresIn   int // segundos, default 604800 (7d)
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	InternalAPIKey     string // secreto compartido para endpoints servicio-a-servicio (X-Internal-Key)
	GRPCPort           string // servidor gRPC Identity (IdentityService)
	SocialGRPCAddr     string // cliente SocialService para el borrado de cuenta
	NotesGRPCAddr      string // cliente NotesService para el borrado de cuenta

	// SMTP opcional (los valores vacíos usan los defectos del mailer).
	SMTPFrom     string // SMTP_FROM; vacío = SMTP_USERNAME
	SMTPFromName string // SMTP_FROM_NAME; vacío = "Sigma Academy"
	SMTPTLSMode  string // SMTP_TLS_MODE: starttls (defecto, 587) o tls (465)

	// Confirmación de correo. EmailVerificationURL es la base opcional del
	// enlace del correo (se añade ?token=...). EmailVerificationTTL: 0 =
	// defecto del servicio (24 h); -1 = valor mal formado, rechazado al arrancar.
	EmailVerificationURL string
	EmailVerificationTTL time.Duration
	// EMAIL_VERIFICATION_REQUIRED: gate de login/refresh para cuentas sin
	// confirmar. Por defecto false. Un valor que no sea booleano deja
	// EmailVerificationRequiredErr y el arranque se detiene.
	EmailVerificationRequired    bool
	EmailVerificationRequiredErr error
}

func Load() *Config {
	// Intenta cargar .env desde varias ubicaciones (root del repo, cwd, etc.)
	candidates := []string{
		".env",
		"../../.env",
		"../../../.env",
		filepath.Join("..", "..", "..", ".env"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err == nil {
				log.Printf("config: cargado .env desde %s", p)
				break
			}
		}
	}
	// Fallback: intenta Load sin path (busca en cwd hacia arriba si godotenv lo soporta)
	_ = godotenv.Load()

	cfg := &Config{
		SMTPHost:           os.Getenv("SMTP_HOST"),
		SMTPPort:           os.Getenv("SMTP_PORT"),
		SMTPUsername:       os.Getenv("SMTP_USERNAME"),
		SMTPPassword:       os.Getenv("SMTP_PASSWORD"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DirectURL:          os.Getenv("DIRECT_URL"),
		JWT:                os.Getenv("JWT"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		Port:               os.Getenv("PORT"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI:  os.Getenv("GOOGLE_REDIRECT_URI"),
		InternalAPIKey:     os.Getenv("INTERNAL_API_KEY"),
		GRPCPort:           os.Getenv("AUTH_GRPC_PORT"),
		SocialGRPCAddr:     os.Getenv("SOCIAL_GRPC_ADDR"),
		NotesGRPCAddr:      os.Getenv("NOTES_GRPC_ADDR"),
	}
	cfg.SMTPFrom = os.Getenv("SMTP_FROM")
	cfg.SMTPFromName = os.Getenv("SMTP_FROM_NAME")
	cfg.SMTPTLSMode = os.Getenv("SMTP_TLS_MODE")
	cfg.EmailVerificationURL = os.Getenv("EMAIL_VERIFICATION_URL")
	cfg.EmailVerificationRequired, cfg.EmailVerificationRequiredErr = parseEmailVerificationRequired(os.Getenv("EMAIL_VERIFICATION_REQUIRED"))
	if v := os.Getenv("EMAIL_VERIFICATION_TTL"); v != "" {
		cfg.EmailVerificationTTL = -1
		if d, err := time.ParseDuration(v); err == nil {
			cfg.EmailVerificationTTL = d
		}
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = os.Getenv("GRPC_PORT")
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9080"
	}
	if cfg.SocialGRPCAddr == "" {
		cfg.SocialGRPCAddr = "social:9083"
	}
	if cfg.NotesGRPCAddr == "" {
		cfg.NotesGRPCAddr = "notes:9082"
	}
	if cfg.SMTPHost == "" {
		cfg.SMTPHost = "smtp.gmail.com"
	}
	if cfg.SMTPPort == "" {
		cfg.SMTPPort = "587"
	}
	if cfg.SMTPUsername == "" {
		cfg.SMTPUsername = "sigmaacademy.noreply@gmail.com"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = cfg.JWT
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = "dev-jwt-secret-change-me"
		}
		log.Printf("config: JWT_SECRET no seteado, usando fallback (no usar en prod)")
	}
	cfg.AccessExpiresIn = 900
	if v := os.Getenv("ACCESS_TOKEN_EXPIRES_IN"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			cfg.AccessExpiresIn = n
		}
	}
	cfg.RefreshExpiresIn = 604800
	if v := os.Getenv("REFRESH_TOKEN_EXPIRES_IN"); v != "" {
		if n, err := parsePositiveInt(v); err == nil {
			cfg.RefreshExpiresIn = n
		}
	}
	return cfg
}

// parseEmailVerificationRequired: vacío = false (gate apagado); cualquier
// valor no booleano es un error de configuración, nunca un false silencioso.
func parseEmailVerificationRequired(v string) (bool, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, nil
	}
	required, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("EMAIL_VERIFICATION_REQUIRED debe ser true o false")
	}
	return required, nil
}

func parsePositiveInt(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not int")
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return 0, fmt.Errorf("must >0")
	}
	return n, nil
}
