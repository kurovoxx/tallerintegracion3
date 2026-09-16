package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	DirectURL   string
	SupabaseURL string
	SupabaseKey string
	JWTSecret   string
	Port        string
}

func Load() *Config {
	// Mismo patrón de notes/auth: intenta cargar .env desde varias ubicaciones
	// relativas para que `go run ./cmd/server` funcione parado en back/social/.
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
				log.Printf("config social: cargado .env desde %s", p)
				break
			}
		}
	}
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DirectURL:   os.Getenv("DIRECT_URL"),
		SupabaseURL: os.Getenv("SUPABASE_URL"),
		SupabaseKey: os.Getenv("SQL_API_KEY"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        os.Getenv("SOCIAL_PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = os.Getenv("PORT")
	}
	if cfg.Port == "" {
		cfg.Port = "8083"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = os.Getenv("JWT")
		if cfg.JWTSecret == "" {
			cfg.JWTSecret = "dev-jwt-secret-change-me"
		}
		log.Printf("config social: JWT_SECRET fallback usado (no prod)")
	}
	return cfg
}
