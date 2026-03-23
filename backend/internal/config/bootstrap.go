package config

import (
	"backend/internal/app/core/auth"
	"backend/internal/app/core/cache"
	"backend/internal/app/core/health"
	"backend/internal/app/core/token"
	"backend/internal/app/utilities/validators"
	"backend/internal/config/database"
	"backend/internal/config/middleware"
	configredis "backend/internal/config/redis"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func MountRoutes() *gin.Engine {
	Load()

	env := Cfg()
	database.Init(database.Config{
		Host:     env.DBHost,
		Port:     env.DBPort,
		User:     env.DBUser,
		Password: env.DBPassword,
		Name:     env.DBName,
	})

	configredis.Init(configredis.Config{
		Addr:     env.RedisAddr,
		Password: env.RedisPassword,
		DB:       env.RedisDB,
	})

	cacheService := cache.NewService(configredis.Client)
	token.Init(env.JWTSecret, cacheService)

	authRepo := auth.NewRepository(database.DB)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService)

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		validators.Register(v)
	}

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	health.MountHealthRoutes(r.Group("/"))

	api := r.Group("/api")
	auth.MountAuthRoutes(api, authHandler)

	return r
}
