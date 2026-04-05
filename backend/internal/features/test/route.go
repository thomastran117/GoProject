package test

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountTestRoutes registers all test-related routes on the given router group.
func MountTestRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	// Course-scoped
	course := rg.Group("/courses/:courseId/tests")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleListByCourse)
	}

	// Test resource
	tests := rg.Group("/tests/:id")
	{
		tests.GET("", auth, h.handleGet)
		tests.PUT("", auth, h.handleUpdate)
		tests.DELETE("", auth, h.handleDelete)
		tests.POST("/publish", auth, h.handlePublish)
		tests.POST("/close", auth, h.handleClose)

		// Questions
		tests.GET("/questions", auth, h.handleListQuestions)
		tests.POST("/questions", auth, h.handleCreateQuestion)
		tests.PUT("/questions/:questionId", auth, h.handleUpdateQuestion)
		tests.DELETE("/questions/:questionId", auth, h.handleDeleteQuestion)

		// Choices
		tests.POST("/questions/:questionId/choices", auth, h.handleCreateChoice)
		tests.PUT("/questions/:questionId/choices/:choiceId", auth, h.handleUpdateChoice)
		tests.DELETE("/questions/:questionId/choices/:choiceId", auth, h.handleDeleteChoice)

		// Student workflow
		tests.POST("/start", auth, h.handleStart)
		tests.PUT("/answers/:questionId", auth, h.handleSaveAnswer)
		tests.POST("/submit", auth, h.handleSubmit)
		tests.GET("/my-submission", auth, h.handleGetMySubmission)

		// Submission management (teacher/admin)
		tests.GET("/submissions", auth, h.handleListSubmissions)
		tests.GET("/submissions/:submissionId", auth, h.handleGetSubmission)
		tests.PUT("/submissions/:submissionId/answers/:answerId", auth, h.handleGradeAnswer)
	}
}
