package blob

import (
	"context"
	"net/http"

	"backend/internal/application/request"

	"github.com/gin-gonic/gin"
)

// blobService is the interface the Handler depends on.
type blobService interface {
	GenerateUploadURL(ctx context.Context, folder string) (*UploadURLResponse, error)
	ConfirmUpload(ctx context.Context, blobKey string) error
}

// uploadURLRequest is the optional JSON body for POST /blob/upload-url.
type uploadURLRequest struct {
	Folder string `json:"folder" binding:"omitempty,max=100"`
}

// confirmUploadRequest is the JSON body for POST /blob/confirm.
type confirmUploadRequest struct {
	BlobKey string `json:"blob_key" binding:"required"`
}

// Handler holds the HTTP handlers for blob operations.
type Handler struct {
	service blobService
}

// NewHandler creates a Handler wired to the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// handleGenerateUploadURL handles POST /blob/upload-url.
// Returns a short-lived SAS URL for direct client upload and the blob key.
// The blob key should not be stored in the database until confirmed via
// POST /blob/confirm.
func (h *Handler) handleGenerateUploadURL(c *gin.Context) {
	var req uploadURLRequest
	if !request.BindJSON(c, &req) {
		return
	}

	resp, err := h.service.GenerateUploadURL(c.Request.Context(), req.Folder)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

// handleConfirmUpload handles POST /blob/confirm.
// Verifies that the blob was actually uploaded to Azure before the client
// stores the blob key in the database.
func (h *Handler) handleConfirmUpload(c *gin.Context) {
	var req confirmUploadRequest
	if !request.BindJSON(c, &req) {
		return
	}

	if err := h.service.ConfirmUpload(c.Request.Context(), req.BlobKey); err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"blob_key": req.BlobKey}})
}
