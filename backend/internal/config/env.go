package config

import (
	"os"
	"strconv"
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

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int
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
		DBPassword: getenv("DB_PASSWORD", "password123"),
		DBName:     getenv("DB_NAME", "goapp"),
		JWTSecret:     getenv("JWT_SECRET", "changeme-set-JWT_SECRET-env-var"),
		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),
	}
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
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
