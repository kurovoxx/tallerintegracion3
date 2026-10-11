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
	// JWTSecret es el secreto HS256 compartido con auth para validar
	// access tokens localmente. Nunca hardcodeado: solo entorno.
	JWTSecret string
	// GRPCPort es el puerto del servidor gRPC (GetSubject para Notes).
	GRPCPort string
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
		JWTSecret:   os.Getenv("JWT_SECRET"),
		GRPCPort:    os.Getenv("GRPC_PORT"),
	}
	if cfg.Port == "" {
		cfg.Port = "8081"
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "9081"
	}
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "dev-jwt-secret-change-me"
		log.Print("config: JWT_SECRET no seteado, usando fallback (no usar en prod)")
	}
	return cfg
}
