package course

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

type courseService interface {
	GetByID(ctx context.Context, id uint64) (*CourseResponse, error)
	GetByIDs(ctx context.Context, ids []uint64) ([]*CourseResponse, error)
	Search(ctx context.Context, f SearchFilter) ([]*CourseResponse, error)
	Create(ctx context.Context, callerUserID uint64, callerRole string, p CreateParams) (*CourseResponse, error)
	Update(ctx context.Context, id, callerUserID uint64, callerRole string, p UpdateParams) (*CourseResponse, error)
	Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error
}

type createCourseRequest struct {
	SchoolID      uint64     `json:"school_id"      binding:"required"`
	TeacherID     uint64     `json:"teacher_id"     binding:"required"`
	Name          string     `json:"name"           binding:"required,min=1,max=200"`
	Code          string     `json:"code"           binding:"required,min=1,max=20"`
	Description   string     `json:"description"    binding:"omitempty,max=2000"`
	Subject       string     `json:"subject"        binding:"omitempty,max=100"`
	GradeLevel    string     `json:"grade_level"    binding:"omitempty,max=50"`
	Language      string     `json:"language"       binding:"omitempty,max=50"`
	Room          string     `json:"room"           binding:"omitempty,max=100"`
	Schedule      string     `json:"schedule"       binding:"omitempty,max=500"`
	MaxEnrollment uint       `json:"max_enrollment"`
	Credits       uint       `json:"credits"`
	Status        string     `json:"status"         binding:"omitempty,max=20"`
	Visibility    string     `json:"visibility"     binding:"omitempty,max=10"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
}

type updateCourseRequest struct {
	TeacherID     uint64     `json:"teacher_id"     binding:"required"`
	Name          string     `json:"name"           binding:"required,min=1,max=200"`
	Code          string     `json:"code"           binding:"required,min=1,max=20"`
	Description   string     `json:"description"    binding:"omitempty,max=2000"`
	Subject       string     `json:"subject"        binding:"omitempty,max=100"`
	GradeLevel    string     `json:"grade_level"    binding:"omitempty,max=50"`
	Language      string     `json:"language"       binding:"omitempty,max=50"`
	Room          string     `json:"room"           binding:"omitempty,max=100"`
	Schedule      string     `json:"schedule"       binding:"omitempty,max=500"`
	MaxEnrollment uint       `json:"max_enrollment"`
	Credits       uint       `json:"credits"`
	Status        string     `json:"status"         binding:"required,max=20"`
	Visibility    string     `json:"visibility"     binding:"omitempty,max=10"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
}

type batchCourseRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

// Handler holds the HTTP handlers for the course resource.
type Handler struct {
	service courseService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseCourseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid course ID"},
		})
		return
	}
	course, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": course})
}

// handleBatch handles POST /courses/batch. Accepts a JSON body with an "ids"
// array and returns all matching courses in a single response.
func (h *Handler) handleBatch(c *gin.Context) {
	var req batchCourseRequest
	if !request.BindJSON(c, &req) {
		return
	}
	courses, err := h.service.GetByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": courses})
}

// handleSearch handles GET /courses/search. Accepts optional query parameters:
//
//	name        – case-insensitive substring match on course name
//	code        – case-insensitive substring match on course code
//	school_id   – exact match on school
//	teacher_id  – exact match on assigned teacher
//	subject     – exact match on subject
//	grade_level – exact match on grade level
//	status      – exact match on status (active/inactive/archived)
//	language    – exact match on language
func (h *Handler) handleSearch(c *gin.Context) {
	var f SearchFilter
	f.Name = c.Query("name")
	f.Code = c.Query("code")
	f.Subject = c.Query("subject")
	f.GradeLevel = c.Query("grade_level")
	f.Status = c.Query("status")
	f.Language = c.Query("language")

	schoolID, ok := parseQueryUint64(c, "school_id", "INVALID_SCHOOL_ID", "Invalid school_id")
	if !ok {
		return
	}
	f.SchoolID = schoolID

	teacherID, ok := parseQueryUint64(c, "teacher_id", "INVALID_TEACHER_ID", "Invalid teacher_id")
	if !ok {
		return
	}
	f.TeacherID = teacherID

	courses, err := h.service.Search(c.Request.Context(), f)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": courses})
}

func (h *Handler) handleCreate(c *gin.Context) {
	var req createCourseRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	params := CreateParams{
		SchoolID:      req.SchoolID,
		TeacherID:     req.TeacherID,
		Name:          req.Name,
		Code:          req.Code,
		Description:   req.Description,
		Subject:       req.Subject,
		GradeLevel:    req.GradeLevel,
		Language:      req.Language,
		Room:          req.Room,
		Schedule:      req.Schedule,
		MaxEnrollment: req.MaxEnrollment,
		Credits:       req.Credits,
		Status:        req.Status,
		Visibility:    req.Visibility,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
	}
	course, err := h.service.Create(c.Request.Context(), claims.UserID, claims.Role, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": course})
}

func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseCourseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid course ID"},
		})
		return
	}
	var req updateCourseRequest
	if !request.BindJSON(c, &req) {
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	params := UpdateParams{
		TeacherID:     req.TeacherID,
		Name:          req.Name,
		Code:          req.Code,
		Description:   req.Description,
		Subject:       req.Subject,
		GradeLevel:    req.GradeLevel,
		Language:      req.Language,
		Room:          req.Room,
		Schedule:      req.Schedule,
		MaxEnrollment: req.MaxEnrollment,
		Credits:       req.Credits,
		Status:        req.Status,
		Visibility:    req.Visibility,
		StartDate:     req.StartDate,
		EndDate:       req.EndDate,
	}
	course, err := h.service.Update(c.Request.Context(), id, claims.UserID, claims.Role, params)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": course})
}

func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseCourseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid course ID"},
		})
		return
	}
	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error":   gin.H{"code": "UNAUTHORIZED", "message": "Unauthorized"},
		})
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, claims.UserID, claims.Role); err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func parseCourseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}

// parseQueryUint64 reads a named query parameter and parses it as a uint64.
// Returns (0, true) when the parameter is absent, (id, true) on success, and
// (0, false) after writing a 400 response when the value is present but invalid.
func parseQueryUint64(c *gin.Context, param, code, message string) (uint64, bool) {
	raw := c.Query(param)
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": code, "message": message},
		})
		return 0, false
	}
	return id, true
}
