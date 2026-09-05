package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string
	DirectURL     string
	SupabaseURL   string
	SupabaseKey   string // SQL_API_KEY / SERVICE_ROLE
	JWT           string // kid for ES256 (discovery) or HS256 secret — ver .env
	DiscoveryURL  string
	Port          string
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
		DatabaseURL:  os.Getenv("DATABASE_URL"),
		DirectURL:    os.Getenv("DIRECT_URL"),
		SupabaseURL:  os.Getenv("SUPABASE_URL"),
		SupabaseKey:  os.Getenv("SQL_API_KEY"),
		JWT:          os.Getenv("JWT"),
		DiscoveryURL: os.Getenv("DISCOVERY_URL"),
		Port:         os.Getenv("PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	return cfg
}
