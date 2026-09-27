package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	DirectURL   string
	SupabaseURL string
	SupabaseKey string
	JWTSecret   string
	Port        string
	AccessTTL   int
	RefreshTTL  int
	StorageMode string // "mock" | "drive"
	// GoogleClientID/Secret permiten renovar el access_token de Drive cuando expiró.
	GoogleClientID     string
	GoogleClientSecret string
	ReconcileInterval  time.Duration
	// SocialServiceURL es la base HTTP del servicio Social (membership, admin,
	// seguidores y correos de miembros). Por defecto http://social:8083.
	SocialServiceURL string
	// SocialTimeout acota cada llamada HTTP a Social para no bloquear el request
	// ante un cuelgue del upstream (degradación controlada).
	SocialTimeout time.Duration
	// InternalAPIKey autentica llamadas servicio-a-servicio (header X-Internal-Key).
	InternalAPIKey string
}

func Load() *Config {
	candidates := []string{
		".env",
		"../../.env",
		"../../../.env",
		filepath.Join("..", "..", "..", ".env"),
		"../auth/.env",
		"../../auth/.env",
		filepath.Join("..", "auth", ".env"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Load(p); err == nil {
				log.Printf("config notes: cargado .env desde %s", p)
				break
			}
		}
	}
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DirectURL:          os.Getenv("DIRECT_URL"),
		SupabaseURL:        os.Getenv("SUPABASE_URL"),
		SupabaseKey:        os.Getenv("SQL_API_KEY"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		Port:               os.Getenv("NOTES_PORT"),
		StorageMode:        os.Getenv("STORAGE_MODE"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		ReconcileInterval:  30 * time.Second,
		SocialServiceURL:   strings.TrimSpace(os.Getenv("SOCIAL_SERVICE_URL")),
		SocialTimeout:      5 * time.Second,
		InternalAPIKey:     strings.TrimSpace(os.Getenv("INTERNAL_API_KEY")),
	}
	if cfg.SocialServiceURL == "" {
		cfg.SocialServiceURL = "http://social:8083"
	}
	if value := strings.TrimSpace(os.Getenv("SOCIAL_TIMEOUT")); value != "" {
		if timeout, err := time.ParseDuration(value); err == nil && timeout > 0 {
			cfg.SocialTimeout = timeout
		} else {
			log.Printf("config notes: SOCIAL_TIMEOUT inválido; usando 5s")
		}
	}
	if value := strings.TrimSpace(os.Getenv("NOTES_RECONCILE_INTERVAL")); value != "" {
		if interval, err := time.ParseDuration(value); err == nil && interval > 0 {
			cfg.ReconcileInterval = interval
		} else {
			log.Printf("config notes: NOTES_RECONCILE_INTERVAL inválido; usando 30s")
		}
	}
	if cfg.Port == "" {
		cfg.Port = os.Getenv("PORT")
	}
	if cfg.Port == "" {
		cfg.Port = "8082"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = os.Getenv("JWT")
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = "dev-jwt-secret-change-me"
		}
		log.Printf("config notes: JWT_SECRET fallback usado (no prod)")
	}
	if cfg.StorageMode == "" {
		cfg.StorageMode = "mock"
	}
	cfg.AccessTTL = 900
	if v := os.Getenv("ACCESS_TOKEN_EXPIRES_IN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.AccessTTL = n
		}
	}
	cfg.RefreshTTL = 604800
	if v := os.Getenv("REFRESH_TOKEN_EXPIRES_IN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RefreshTTL = n
		}
	}
	return cfg
}
