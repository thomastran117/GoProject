package exam

import (
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// MountExamRoutes registers all exam-related routes on the given router group.
func MountExamRoutes(rg *gin.RouterGroup, h *Handler) {
	auth := middleware.Authenticate()

	// Course-scoped
	course := rg.Group("/courses/:courseId/exams")
	{
		course.POST("", auth, h.handleCreate)
		course.GET("", auth, h.handleListByCourse)
	}

	// Exam resource
	exams := rg.Group("/exams/:id")
	{
		exams.GET("", auth, h.handleGet)
		exams.PUT("", auth, h.handleUpdate)
		exams.DELETE("", auth, h.handleDelete)
		exams.POST("/publish", auth, h.handlePublish)
		exams.POST("/close", auth, h.handleClose)

		// Questions
		exams.GET("/questions", auth, h.handleListQuestions)
		exams.POST("/questions", auth, h.handleCreateQuestion)
		exams.PUT("/questions/:questionId", auth, h.handleUpdateQuestion)
		exams.DELETE("/questions/:questionId", auth, h.handleDeleteQuestion)

		// Choices
		exams.POST("/questions/:questionId/choices", auth, h.handleCreateChoice)
		exams.PUT("/questions/:questionId/choices/:choiceId", auth, h.handleUpdateChoice)
		exams.DELETE("/questions/:questionId/choices/:choiceId", auth, h.handleDeleteChoice)

		// Student workflow
		exams.POST("/start", auth, h.handleStart)
		exams.PUT("/answers/:questionId", auth, h.handleSaveAnswer)
		exams.POST("/submit", auth, h.handleSubmit)
		exams.GET("/my-submission", auth, h.handleGetMySubmission)

		// Submission management (teacher/admin)
		exams.GET("/submissions", auth, h.handleListSubmissions)
		exams.GET("/submissions/:submissionId", auth, h.handleGetSubmission)
		exams.PUT("/submissions/:submissionId/answers/:answerId", auth, h.handleGradeAnswer)
	}
}
