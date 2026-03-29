package profile

import (
	"context"
	"net/http"
	"strconv"

	"backend/internal/app/utilities/request"
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

// profileService is the interface the Handler depends on, allowing the
// concrete Service to be swapped with a test double.
type profileService interface {
	GetByID(ctx context.Context, id uint64) (*ProfileResponse, error)
	GetAll(ctx context.Context) ([]*ProfileResponse, error)
	GetByIDs(ctx context.Context, ids []uint64) ([]*ProfileResponse, error)
	Create(ctx context.Context, userID uint64, username, avatarURL string) (*ProfileResponse, error)
	Update(ctx context.Context, id uint64, username, avatarURL string) (*ProfileResponse, error)
	Delete(ctx context.Context, id uint64) error
}

// createProfileRequest is the expected JSON body for POST /profiles.
type createProfileRequest struct {
	Username  string `json:"username"   binding:"required,min=3,max=100"`
	AvatarURL string `json:"avatar_url" binding:"omitempty,url"`
}

// updateProfileRequest is the expected JSON body for PUT /profiles/:id.
type updateProfileRequest struct {
	Username  string `json:"username"   binding:"required,min=3,max=100"`
	AvatarURL string `json:"avatar_url" binding:"omitempty,url"`
}

// batchProfileRequest is the expected JSON body for POST /profiles/batch.
type batchProfileRequest struct {
	IDs []uint64 `json:"ids" binding:"required,min=1"`
}

// Handler holds the HTTP handlers for the profile resource.
type Handler struct {
	service profileService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleCreate handles POST /profiles. It reads the authenticated user's ID
// from the JWT claims and creates a new profile owned by that user.
func (h *Handler) handleCreate(c *gin.Context) {
	var req createProfileRequest
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

	profile, err := h.service.Create(c.Request.Context(), claims.UserID, req.Username, req.AvatarURL)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": profile})
}

// handleUpdate handles PUT /profiles/:id. Replaces the username and avatar URL
// of the profile identified by the URL parameter.
func (h *Handler) handleUpdate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid profile ID"},
		})
		return
	}

	var req updateProfileRequest
	if !request.BindJSON(c, &req) {
		return
	}

	profile, err := h.service.Update(c.Request.Context(), id, req.Username, req.AvatarURL)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

// handleDelete handles DELETE /profiles/:id. Removes the profile identified by
// the URL parameter.
func (h *Handler) handleDelete(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid profile ID"},
		})
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// handleGet handles GET /profiles/:id. Returns the single profile identified
// by the URL parameter.
func (h *Handler) handleGet(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   gin.H{"code": "INVALID_ID", "message": "Invalid profile ID"},
		})
		return
	}

	profile, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

// handleGetAll handles GET /profiles. Returns all profiles in the system.
func (h *Handler) handleGetAll(c *gin.Context) {
	profiles, err := h.service.GetAll(c.Request.Context())
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

// handleBatch handles POST /profiles/batch. Accepts a JSON body with an "ids"
// array and returns all matching profiles in a single response.
func (h *Handler) handleBatch(c *gin.Context) {
	var req batchProfileRequest
	if !request.BindJSON(c, &req) {
		return
	}

	profiles, err := h.service.GetByIDs(c.Request.Context(), req.IDs)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

// parseID extracts and parses the ":id" URL parameter as a uint64.
func parseID(c *gin.Context) (uint64, error) {
	return strconv.ParseUint(c.Param("id"), 10, 64)
}
