package config

import (
	"backend/internal/application/middleware"
	"backend/internal/application/validators"
	"backend/internal/config/database"
	"backend/internal/config/environment"
	configredis "backend/internal/config/redis"
	"backend/internal/external/blob"
	"backend/internal/external/email"
	"backend/internal/features/auth"
	"backend/internal/features/cache"
	"backend/internal/features/health"
	"backend/internal/features/profile"
	"backend/internal/features/school"
	"backend/internal/features/token"

	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func MountRoutes() *gin.Engine {
	environment.Load()

	env := environment.Cfg()
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

	if err := database.DB.AutoMigrate(&profile.Profile{}, &school.School{}); err != nil {
		log.Fatal("database: failed to migrate profile:", err)
	}

	var emailSender email.Sender
	if env.HasEmail() {
		emailSender = email.NewGmailService(env.EmailFrom, env.EmailAppPassword)
	} else {
		log.Println("config: GMAIL_FROM/GMAIL_APP_PASSWORD not configured (email verification disabled)")
	}

	authRepo := auth.NewRepository(database.DB)
	authService := auth.NewService(authRepo, env.GoogleClientID, env.MicrosoftClientID, env.TurnstileSecretKey, !env.HasTurnstile(), configredis.Client, emailSender, env.AppURL)
	authHandler := auth.NewHandler(authService)

	profileRepo := profile.NewRepository(database.DB)
	profileService := profile.NewService(profileRepo)
	profileHandler := profile.NewHandler(profileService)

	schoolRepo := school.NewRepository(database.DB)
	schoolService := school.NewService(schoolRepo)
	schoolHandler := school.NewHandler(schoolService)

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
	school.MountSchoolRoutes(api, schoolHandler)

	if env.HasAzureBlob() {
		blobService, err := blob.NewService(env.AzureStorageAccountName, env.AzureStorageAccountKey, env.AzureStorageContainerName)
		if err != nil {
			log.Fatal("blob: failed to initialize azure storage client:", err)
		}
		blobHandler := blob.NewHandler(blobService)
		blob.MountBlobRoutes(api, blobHandler)
	} else {
		log.Println("config: AZURE_STORAGE_ACCOUNT_NAME/KEY not configured (blob storage disabled)")
	}

	return r
}
