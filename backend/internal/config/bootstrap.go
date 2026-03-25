package config

import (
	"backend/internal/app/core/auth"
	"backend/internal/app/core/blob"
	"backend/internal/app/core/cache"
	"backend/internal/app/core/health"
	"backend/internal/app/core/profile"
	"backend/internal/app/core/token"
	"backend/internal/app/utilities/validators"
	"backend/internal/config/database"
	"backend/internal/config/middleware"
	configredis "backend/internal/config/redis"

	"log"
	"time"

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
		MaxOpenConns: 25,
		MaxIdleConns: 10,
		ConnMaxLife:  time.Hour,
		ConnMaxIdle:  15 * time.Minute,
	})

	configredis.Init(configredis.Config{
		Addr:     env.RedisAddr,
		Password: env.RedisPassword,
		DB:       env.RedisDB,
	})

	cacheService := cache.NewService(configredis.Client)
	token.Init(env.JWTSecret, cacheService)

	if err := database.DB.AutoMigrate(&profile.Profile{}); err != nil {
		log.Fatal("database: failed to migrate profile:", err)
	}

	authRepo := auth.NewRepository(database.DB)
	authService := auth.NewService(authRepo, env.GoogleClientID, env.MicrosoftClientID, env.TurnstileSecretKey)
	authHandler := auth.NewHandler(authService)

	profileRepo := profile.NewRepository(database.DB)
	profileService := profile.NewService(profileRepo)
	profileHandler := profile.NewHandler(profileService)

	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		validators.Register(v)
	}

	r := gin.New()
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.ClientInfoMiddleware())
	
	health.MountHealthRoutes(r.Group("/"))

	api := r.Group("/api")
	auth.MountAuthRoutes(api, authHandler)

	profile.MountProfileRoutes(api, profileHandler)

	blobService, err := blob.NewService(env.AzureStorageAccountName, env.AzureStorageAccountKey, env.AzureStorageContainerName)
	if err != nil {
		log.Fatal("blob: failed to initialize azure storage client:", err)
	}
	blobHandler := blob.NewHandler(blobService)
	blob.MountBlobRoutes(api, blobHandler)

	return r
}
