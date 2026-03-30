package school

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/application/middleware"
	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)



type schoolService interface {
	GetByID(ctx context.Context, id uint64) (*SchoolResponse, error)
	GetAll(ctx context.Context) ([]*SchoolResponse, error)
	GetByIDs(ctx context.Context, ids []uint64) ([]*SchoolResponse, error)
	Search(ctx context.Context, f SearchFilter) ([]*SchoolResponse, error)
	Create(ctx context.Context, callerUserID uint64, callerRole, name, address, city, country, phone, email, website string) (*SchoolResponse, error)
	Update(ctx context.Context, id, callerUserID uint64, callerRole, name, address, city, country, phone, email, website string) (*SchoolResponse, error)
	Delete(ctx context.Context, id, callerUserID uint64, callerRole string) error
}

type createSchoolRequest struct {
	Name    string `json:"name"    binding:"required,min=1,max=200"`
	Address string `json:"address" binding:"omitempty,max=500"`
	City    string `json:"city"    binding:"required,max=100"`
	Country string `json:"country" binding:"required,max=100"`
	Phone   string `json:"phone"   binding:"omitempty,max=30"`
	Email   string `json:"email"   binding:"omitempty,email,max=254"`
	Website string `json:"website" binding:"omitempty,url,max=2048"`
}

type batchSchoolRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

type updateSchoolRequest struct {
	Name    string `json:"name"    binding:"required,min=1,max=200"`
	Address string `json:"address" binding:"omitempty,max=500"`
	City    string `json:"city"    binding:"required,max=100"`
	Country string `json:"country" binding:"required,max=100"`
	Phone   string `json:"phone"   binding:"omitempty,max=30"`
	Email   string `json:"email"   binding:"omitempty,email,max=254"`
	Website string `json:"website" binding:"omitempty,url,max=2048"`
}

// Handler holds the HTTP handlers for the school resource.
type Handler struct {
	service schoolService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) handleGetAll(c *gin.Context) {
	schools, err := h.service.GetAll(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schools})
}

func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid school ID"},
		})
		return
	}
	school, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": school})
}

func (h *Handler) handleCreate(c *gin.Context) {
	var req createSchoolRequest
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
	school, err := h.service.Create(c.Request.Context(), claims.UserID, claims.Role, req.Name, req.Address, req.City, req.Country, req.Phone, req.Email, req.Website)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": school})
}

func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid school ID"},
		})
		return
	}
	var req updateSchoolRequest
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
	school, err := h.service.Update(c.Request.Context(), id, claims.UserID, claims.Role, req.Name, req.Address, req.City, req.Country, req.Phone, req.Email, req.Website)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": school})
}

func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid school ID"},
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

// handleBatch handles POST /schools/batch. Accepts a JSON body with an "ids"
// array and returns all matching schools in a single response.
func (h *Handler) handleBatch(c *gin.Context) {
	var req batchSchoolRequest
	if !request.BindJSON(c, &req) {
		return
	}
	schools, err := h.service.GetByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schools})
}

// handleSearch handles GET /schools/search. Accepts optional query parameters:
//
//	name        – case-insensitive substring match on school name
//	city        – exact match on city
//	country     – exact match on country
//	principal_id – exact match on owning principal
func (h *Handler) handleSearch(c *gin.Context) {
	var f SearchFilter
	f.Name = c.Query("name")
	f.City = c.Query("city")
	f.Country = c.Query("country")
	if raw := c.Query("principal_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
				"success": false,
				"error":   gin.H{"code": "INVALID_PRINCIPAL_ID", "message": "Invalid principal_id"},
			})
			return
		}
		f.PrincipalID = id
	}
	schools, err := h.service.Search(c.Request.Context(), f)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": schools})
}

func parseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
