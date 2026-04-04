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
	"backend/internal/features/exam"
	"backend/internal/features/grade"
	"backend/internal/features/health"
	"backend/internal/features/lecture"
	"backend/internal/features/profile"
	"backend/internal/features/quiz"
	"backend/internal/features/school"
	"backend/internal/features/submission"
	"backend/internal/features/test"
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

	if err := database.DB.AutoMigrate(
		&profile.Profile{},
		&school.School{},
		&course.Course{},
		&device.Device{},
		&announcement.Announcement{},
		&assignment.Assignment{},
		&assignment.AssignmentView{},
		&enrollment.Enrollment{},
		&lecture.Lecture{},
		&lecture.LectureView{},
		&submission.AssignmentSubmission{},
		&quiz.Quiz{},
		&test.Test{},
		&exam.Exam{},
		&grade.Grade{},
	); err != nil {
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

	isEnrolledFn := func(ctx context.Context, courseID, userID uint64) (bool, error) {
		e, err := enrollmentRepo.FindEnrollment(courseID, userID)
		if err != nil {
			return false, err
		}
		return e != nil && e.Status == "active", nil
	}

	findCourseFn := func(ctx context.Context, id uint64) (*announcement.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &announcement.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	announcementRepo := announcement.NewRepository(database.DB)
	announcementService := announcement.NewService(announcementRepo, findCourseFn, isEnrolledFn)
	announcementHandler := announcement.NewHandler(announcementService)

	findCourseForAssignmentFn := func(ctx context.Context, id uint64) (*assignment.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &assignment.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	assignmentRepo := assignment.NewRepository(database.DB)
	assignmentService := assignment.NewService(assignmentRepo, findCourseForAssignmentFn, isEnrolledFn)
	assignmentHandler := assignment.NewHandler(assignmentService)

	findAssignmentForSubmissionFn := func(ctx context.Context, id uint64) (*submission.AssignmentInfo, error) {
		a, err := assignmentRepo.FindByID(ctx, id)
		if err != nil || a == nil {
			return nil, err
		}
		return &submission.AssignmentInfo{
			CourseID: a.CourseID,
			AuthorID: a.AuthorID,
			DueAt:    a.DueAt,
			Points:   a.Points,
			Status:   a.Status,
		}, nil
	}

	findCourseForSubmissionFn := func(ctx context.Context, id uint64) (*submission.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &submission.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	submissionRepo := submission.NewRepository(database.DB)
	submissionService := submission.NewService(submissionRepo, findAssignmentForSubmissionFn, findCourseForSubmissionFn, isEnrolledFn)
	submissionHandler := submission.NewHandler(submissionService)

	quizRepo := quiz.NewRepository(database.DB)
	testRepo := test.NewRepository(database.DB)
	examRepo := exam.NewRepository(database.DB)

	findCourseForGradeFn := func(ctx context.Context, id uint64) (*grade.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &grade.CourseInfo{TeacherID: c.TeacherID}, nil
	}
	findAssignmentForGradeFn := func(ctx context.Context, id uint64) (*grade.ItemInfo, error) {
		a, err := assignmentRepo.FindByID(ctx, id)
		if err != nil || a == nil {
			return nil, err
		}
		return &grade.ItemInfo{CourseID: a.CourseID}, nil
	}
	findQuizForGradeFn := func(ctx context.Context, id uint64) (*grade.ItemInfo, error) {
		q, err := quizRepo.FindByID(ctx, id)
		if err != nil || q == nil {
			return nil, err
		}
		return &grade.ItemInfo{CourseID: q.CourseID}, nil
	}
	findTestForGradeFn := func(ctx context.Context, id uint64) (*grade.ItemInfo, error) {
		t, err := testRepo.FindByID(ctx, id)
		if err != nil || t == nil {
			return nil, err
		}
		return &grade.ItemInfo{CourseID: t.CourseID}, nil
	}
	findExamForGradeFn := func(ctx context.Context, id uint64) (*grade.ItemInfo, error) {
		e, err := examRepo.FindByID(ctx, id)
		if err != nil || e == nil {
			return nil, err
		}
		return &grade.ItemInfo{CourseID: e.CourseID}, nil
	}

	gradeRepo := grade.NewRepository(database.DB)
	gradeService := grade.NewService(gradeRepo, findCourseForGradeFn, isEnrolledFn,
		findAssignmentForGradeFn, findQuizForGradeFn, findTestForGradeFn, findExamForGradeFn)
	gradeHandler := grade.NewHandler(gradeService)

	findCourseForLectureFn := func(ctx context.Context, id uint64) (*lecture.CourseInfo, error) {
		c, err := courseRepo.FindByID(id)
		if err != nil || c == nil {
			return nil, err
		}
		return &lecture.CourseInfo{TeacherID: c.TeacherID}, nil
	}

	lectureRepo := lecture.NewRepository(database.DB)
	lectureService := lecture.NewService(lectureRepo, findCourseForLectureFn, isEnrolledFn)
	lectureHandler := lecture.NewHandler(lectureService)

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
	submission.MountSubmissionRoutes(api, submissionHandler)
	lecture.MountLectureRoutes(api, lectureHandler)
	enrollment.MountEnrollmentRoutes(api, enrollmentHandler)
	grade.MountGradeRoutes(api, gradeHandler)

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
