package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL        string
	DirectURL          string
	SupabaseURL        string
	SupabaseKey        string // SQL_API_KEY / SERVICE_ROLE
	JWT                string // kid for ES256 (discovery)
	JWTSecret          string // HS256 secret para access tokens propios
	DiscoveryURL       string
	Port               string
	AccessExpiresIn    int // segundos, default 900
	RefreshExpiresIn   int // segundos, default 604800 (7d)
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string
	InternalAPIKey     string // secreto compartido para endpoints servicio-a-servicio (X-Internal-Key)
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
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DirectURL:          os.Getenv("DIRECT_URL"),
		SupabaseURL:        os.Getenv("SUPABASE_URL"),
		SupabaseKey:        os.Getenv("SQL_API_KEY"),
		JWT:                os.Getenv("JWT"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		DiscoveryURL:       os.Getenv("DISCOVERY_URL"),
		Port:               os.Getenv("PORT"),
		GoogleClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURI:  os.Getenv("GOOGLE_REDIRECT_URI"),
		InternalAPIKey:     os.Getenv("INTERNAL_API_KEY"),
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
