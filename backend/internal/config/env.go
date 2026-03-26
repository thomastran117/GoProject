package config

import (
	"log"
	"os"
	"strconv"
	"strings"
)

type Env struct {
	// Runtime environment: "development" or "production"
	AppEnv string

	// Server
	Port string

	// Database
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// Auth
	JWTSecret          string
	GoogleClientID     string
	MicrosoftClientID  string
	TurnstileSecretKey string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Azure Blob Storage
	AzureStorageAccountName   string
	AzureStorageAccountKey    string
	AzureStorageContainerName string
}

// IsProd returns true when APP_ENV=production.
func (e *Env) IsProd() bool { return e.AppEnv == "production" }

// HasTurnstile reports whether Turnstile captcha is configured.
func (e *Env) HasTurnstile() bool { return e.TurnstileSecretKey != "" }

// HasOAuth reports whether both OAuth providers are configured.
func (e *Env) HasOAuth() bool { return e.GoogleClientID != "" && e.MicrosoftClientID != "" }

// HasAzureBlob reports whether Azure Blob Storage credentials are configured.
func (e *Env) HasAzureBlob() bool {
	return e.AzureStorageAccountName != "" && e.AzureStorageAccountKey != ""
}

// cfg is the package-level singleton populated by Load.
var cfg *Env

// Load reads environment variables once and stores them. Call this at startup
// before any other package uses Cfg().
func Load() {
	cfg = &Env{
		AppEnv:     getenv("APP_ENV", "development"),
		Port:       getenv("PORT", "8080"),
		DBHost:     getenv("DB_HOST", "localhost"),
		DBPort:     getenv("DB_PORT", "3306"),
		DBUser:     getenv("DB_USER", "root"),
		DBPassword: getenv("DB_PASSWORD", "password123"),
		DBName:     getenv("DB_NAME", "goapp"),

		JWTSecret:          getenv("JWT_SECRET", "changeme-set-JWT_SECRET-env-var"),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		MicrosoftClientID:  getenv("MICROSOFT_CLIENT_ID", ""),
		TurnstileSecretKey: getenv("TURNSTILE_SECRET_KEY", ""),

		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       getenvInt("REDIS_DB", 0),

		AzureStorageAccountName:   getenv("AZURE_STORAGE_ACCOUNT_NAME", ""),
		AzureStorageAccountKey:    getenv("AZURE_STORAGE_ACCOUNT_KEY", ""),
		AzureStorageContainerName: getenv("AZURE_STORAGE_CONTAINER_NAME", "uploads"),
	}
	cfg.validate()
}

// validate enforces required variables in production and logs warnings in development.
func (e *Env) validate() {
	type check struct {
		name    string
		value   string
		feature string // human-readable feature name for dev warnings
	}

	required := []check{
		{"JWT_SECRET", e.JWTSecret, ""},
		{"GOOGLE_CLIENT_ID", e.GoogleClientID, "Google OAuth"},
		{"MICROSOFT_CLIENT_ID", e.MicrosoftClientID, "Microsoft OAuth"},
		{"TURNSTILE_SECRET_KEY", e.TurnstileSecretKey, "Turnstile captcha"},
		{"AZURE_STORAGE_ACCOUNT_NAME", e.AzureStorageAccountName, "Azure Blob Storage"},
		{"AZURE_STORAGE_ACCOUNT_KEY", e.AzureStorageAccountKey, "Azure Blob Storage"},
	}

	// JWT_SECRET has an insecure default — treat the default as missing in prod.
	if e.JWTSecret == "changeme-set-JWT_SECRET-env-var" {
		e.JWTSecret = "" // zero it so the missing check below catches it
		required[0].value = ""
	}

	if e.IsProd() {
		var missing []string
		for _, c := range required {
			if c.value == "" {
				missing = append(missing, c.name)
			}
		}
		if len(missing) > 0 {
			log.Fatalf("config: missing required environment variables in production: %s", strings.Join(missing, ", "))
		}
		return
	}

	// Development: warn about missing optional vars and note which features are disabled.
	seen := map[string]bool{}
	for _, c := range required {
		if c.value == "" && c.feature != "" && !seen[c.feature] {
			seen[c.feature] = true
			log.Printf("config: %s not configured (%s disabled)", c.name, c.feature)
		}
	}
	if e.JWTSecret == "" {
		log.Printf("config: JWT_SECRET not set, using insecure default (development only)")
		e.JWTSecret = "changeme-set-JWT_SECRET-env-var"
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

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
