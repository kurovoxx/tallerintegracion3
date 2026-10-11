package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config concentra las variables de entorno del servicio Academic.
// Sin secretos hardcodeados: todo llega por entorno (o .env local).
type Config struct {
	DatabaseURL string
	Port        string
}

// Load lee .env (si existe) y el entorno, aplicando defaults.
// Nunca falla: sin .env ni vars, devuelve defaults sanos.
func Load() *Config {
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
	_ = godotenv.Load()

	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Port:        os.Getenv("PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = "8081"
	}
	return cfg
}
