package config

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL  string
	DirectURL    string
	SupabaseURL  string
	SupabaseKey  string
	JWTSecret    string
	Port         string
	AccessTTL    int
	RefreshTTL   int
	StorageMode  string // "mock" | "drive"
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
		DatabaseURL: os.Getenv("DATABASE_URL"),
		DirectURL:   os.Getenv("DIRECT_URL"),
		SupabaseURL: os.Getenv("SUPABASE_URL"),
		SupabaseKey: os.Getenv("SQL_API_KEY"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
		Port:        os.Getenv("NOTES_PORT"),
		StorageMode: os.Getenv("STORAGE_MODE"),
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
