package config

import (
	"backend/internal/app/core/auth"
	"backend/internal/app/core/health"
	"backend/internal/app/core/token"
	"backend/internal/config/database"
	"backend/internal/config/middleware"

	"github.com/gin-gonic/gin"
)

func MountRoutes() *gin.Engine {
	Load()

	env := Cfg()
	token.Init(env.JWTSecret)
	database.Init(database.Config{
		Host:     env.DBHost,
		Port:     env.DBPort,
		User:     env.DBUser,
		Password: env.DBPassword,
		Name:     env.DBName,
	})

	authRepo := auth.NewRepository(database.DB)
	authService := auth.NewService(authRepo)
	authHandler := auth.NewHandler(authService)

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())

	health.MountHealthRoutes(r.Group("/"))

	api := r.Group("/api")
	auth.MountAuthRoutes(api, authHandler)

	return r
}
