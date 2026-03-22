package config

import (
	"os"
)

type Env struct {
	// Server
	Port string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Auth
	JWTSecret string
}

// cfg is the package-level singleton populated by Load.
var cfg *Env

// Load reads environment variables once and stores them. Call this at startup
// before any other package uses Cfg().
func Load() {
	cfg = &Env{
		Port:       getenv("PORT", "8080"),
		DBHost:     getenv("DB_HOST", "localhost"),
		DBPort:     getenv("DB_PORT", "3306"),
		DBUser:     getenv("DB_USER", "root"),
		DBPassword: getenv("DB_PASSWORD", ""),
		DBName:     getenv("DB_NAME", "app"),
		JWTSecret:  getenv("JWT_SECRET", "changeme-set-JWT_SECRET-env-var"),
	}
}

// Cfg returns the loaded configuration. Panics if Load has not been called.
func Cfg() *Env {
	if cfg == nil {
		panic("config: Cfg() called before Load()")
	}
	return cfg
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
