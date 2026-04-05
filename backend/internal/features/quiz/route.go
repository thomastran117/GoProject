package quiz

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountQuizRoutes registers all quiz-related routes on the given router group.
func MountQuizRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	// Course-scoped
	course := rg.Group("/courses/:courseId/quizzes")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleListByCourse)
	}

	// Quiz resource
	quizzes := rg.Group("/quizzes/:id")
	{
		quizzes.GET("", auth, h.handleGet)
		quizzes.PUT("", auth, h.handleUpdate)
		quizzes.DELETE("", auth, h.handleDelete)
		quizzes.POST("/publish", auth, h.handlePublish)
		quizzes.POST("/close", auth, h.handleClose)

		// Questions
		quizzes.GET("/questions", auth, h.handleListQuestions)
		quizzes.POST("/questions", auth, h.handleCreateQuestion)
		quizzes.PUT("/questions/:questionId", auth, h.handleUpdateQuestion)
		quizzes.DELETE("/questions/:questionId", auth, h.handleDeleteQuestion)

		// Choices
		quizzes.POST("/questions/:questionId/choices", auth, h.handleCreateChoice)
		quizzes.PUT("/questions/:questionId/choices/:choiceId", auth, h.handleUpdateChoice)
		quizzes.DELETE("/questions/:questionId/choices/:choiceId", auth, h.handleDeleteChoice)

		// Student workflow
		quizzes.POST("/start", auth, h.handleStart)
		quizzes.PUT("/answers/:questionId", auth, h.handleSaveAnswer)
		quizzes.POST("/submit", auth, h.handleSubmit)
		quizzes.GET("/my-submission", auth, h.handleGetMySubmission)

		// Submission management (teacher/admin)
		quizzes.GET("/submissions", auth, h.handleListSubmissions)
		quizzes.GET("/submissions/:submissionId", auth, h.handleGetSubmission)
		quizzes.PUT("/submissions/:submissionId/answers/:answerId", auth, h.handleGradeAnswer)
	}
}
