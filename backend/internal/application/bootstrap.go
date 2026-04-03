package config

import (
	"context"

	"backend/internal/application/middleware"
	"backend/internal/application/validators"
	"backend/internal/config/database"
	"backend/internal/config/environment"
	configredis "backend/internal/config/redis"
	"backend/internal/external/blob"
	"backend/internal/external/email"
	"backend/internal/features/announcement"
	"backend/internal/features/assignment"
	"backend/internal/features/auth"
	"backend/internal/features/cache"
	"backend/internal/features/course"
	"backend/internal/features/device"
	"backend/internal/features/enrollment"
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

	if err := database.DB.AutoMigrate(&profile.Profile{}, &school.School{}, &course.Course{}, &device.Device{}, &announcement.Announcement{}, &assignment.Assignment{}, &enrollment.Enrollment{}); err != nil {
		log.Fatal("database: failed to migrate profile:", err)
	}

	var emailSender email.Sender
	if env.HasEmail() {
		emailSender = email.NewGmailService(env.EmailFrom, env.EmailAppPassword)
	} else {
		log.Println("config: GMAIL_FROM/GMAIL_APP_PASSWORD not configured (email verification disabled)")
	}

	authRepo := auth.NewRepository(database.DB)
	deviceRepo := device.NewRepository(database.DB)

	schoolRepo := school.NewRepository(database.DB)
	schoolExistsFn := func(ctx context.Context, id uint64) (bool, error) {
		s, err := schoolRepo.FindByID(id)
		return s != nil, err
	}

	authService := auth.NewService(authRepo, deviceRepo, env.GoogleClientID, env.MicrosoftClientID, env.TurnstileSecretKey, !env.HasTurnstile(), configredis.Client, emailSender, env.AppURL, schoolExistsFn)
	authHandler := auth.NewHandler(authService)

	profileRepo := profile.NewRepository(database.DB)
	profileService := profile.NewService(profileRepo)
	profileHandler := profile.NewHandler(profileService)
	schoolService := school.NewService(schoolRepo)
	schoolHandler := school.NewHandler(schoolService)

	teacherExistsFn := func(ctx context.Context, id uint64) (bool, error) {
		u, err := authRepo.FindByID(id)
		if err != nil {
			return false, err
		}
		return u != nil && u.Role == auth.RoleTeacher, nil
	}

	findSchoolFn := func(ctx context.Context, id uint64) (*course.SchoolInfo, error) {
		s, err := schoolRepo.FindByID(id)
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		return &course.SchoolInfo{PrincipalID: s.PrincipalID}, nil
	}

	courseRepo := course.NewRepository(database.DB)
	courseService := course.NewService(courseRepo, schoolExistsFn, teacherExistsFn, findSchoolFn)
	courseHandler := course.NewHandler(courseService)

	findCourseFn := func(ctx context.Context, id uint64) (*announcement.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &announcement.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	announcementRepo := announcement.NewRepository(database.DB)
	announcementService := announcement.NewService(announcementRepo, findCourseFn)
	announcementHandler := announcement.NewHandler(announcementService)

	findCourseForAssignmentFn := func(ctx context.Context, id uint64) (*assignment.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &assignment.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	assignmentRepo := assignment.NewRepository(database.DB)
	assignmentService := assignment.NewService(assignmentRepo, findCourseForAssignmentFn)
	assignmentHandler := assignment.NewHandler(assignmentService)

	findCourseForEnrollmentFn := func(ctx context.Context, id uint64) (*enrollment.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &enrollment.CourseInfo{TeacherID: c.TeacherID, Visibility: c.Visibility, MaxEnrollment: c.MaxEnrollment}, nil
	}

	userExistsFn := func(ctx context.Context, id uint64) (bool, error) {
		u, err := authRepo.FindByID(id)
		return u != nil, err
	}

	enrollmentRepo := enrollment.NewRepository(database.DB, configredis.Client)
	enrollmentService := enrollment.NewService(enrollmentRepo, findCourseForEnrollmentFn, userExistsFn)
	enrollmentHandler := enrollment.NewHandler(enrollmentService)

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
	course.MountCourseRoutes(api, courseHandler)
	announcement.MountAnnouncementRoutes(api, announcementHandler)
	assignment.MountAssignmentRoutes(api, assignmentHandler)
	enrollment.MountEnrollmentRoutes(api, enrollmentHandler)

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
