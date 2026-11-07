package config

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// Config concentra las variables de entorno del microservicio IA.
// Sin secretos hardcodeados: todo llega por entorno (o .env local).
type Config struct {
	// Puerto HTTP del servidor Gin.
	Port string
	// Puerto alternativo (compatibilidad con otros servicios).
	AIPort string
	// Clave de API de Gemini (se usa en la tarea 4_5_6 para PDF→Markdown).
	GeminiAPIKey string
	// Modelo de Gemini a usar (ej: gemini-1.5-flash).
	GeminiModel string
	// Tamaño máximo de archivo PDF en MB.
	GeminiMaxFileSizeMB int
	// Límite de conversiones por usuario por hora.
	GeminiRateLimit int
	// Clave secreta compartida para endpoints internos servicio-a-servicio.
	InternalAPIKey string
	// DATABASE_URL opcional: si se provee, el servicio puede persistir
	// conversaciones/resultados en Postgres. Sin ella, funciona en modo
	// solo-memoria (scaffold).
	DatabaseURL string
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
		Port:               os.Getenv("PORT"),
		AIPort:             os.Getenv("AI_PORT"),
		GeminiAPIKey:       os.Getenv("GEMINI_API_KEY"),
		GeminiModel:        os.Getenv("GEMINI_MODEL"),
		GeminiMaxFileSizeMB: 10,
		GeminiRateLimit:    5,
		InternalAPIKey:     os.Getenv("INTERNAL_API_KEY"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
	}
	if cfg.Port == "" {
		cfg.Port = cfg.AIPort
	}
	if cfg.Port == "" {
		cfg.Port = "8084"
	}
	if cfg.GeminiModel == "" {
		cfg.GeminiModel = "gemini-1.5-flash"
	}
	if cfg.InternalAPIKey == "" {
		cfg.InternalAPIKey = "dev-internal-key-change-me"
		log.Print("config: INTERNAL_API_KEY no seteado, usando fallback (no usar en prod)")
	}
	return cfg
}
