package auth

import (
	"net/http"
	"unicode"

	"backend/internal/config/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterValidation("strong_password", func(fl validator.FieldLevel) bool {
			var hasUpper, hasLower, hasDigit, hasSpecial bool
			for _, ch := range fl.Field().String() {
				switch {
				case unicode.IsUpper(ch):
					hasUpper = true
				case unicode.IsLower(ch):
					hasLower = true
				case unicode.IsDigit(ch):
					hasDigit = true
				case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
					hasSpecial = true
				}
			}
			return hasUpper && hasLower && hasDigit && hasSpecial
		})
	}
}

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type signupRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,strong_password"`
}

type oauthRequest struct {
	Token string `json:"token" binding:"required"`
}

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func bindJSON(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		c.Error(&middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
		})
		return false
	}
	return true
}

func (h *Handler) HandleLogin(c *gin.Context) {
	var req loginRequest
	if !bindJSON(c, &req) {
		return
	}

	resp, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *Handler) HandleSignup(c *gin.Context) {
	var req signupRequest
	if !bindJSON(c, &req) {
		return
	}

	resp, err := h.service.Signup(req.Email, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

func HandleVerify(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "test"})
}

func HandleGoogle(c *gin.Context) {
	var req oauthRequest
	if !bindJSON(c, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleMicrosoft(c *gin.Context) {
	var req oauthRequest
	if !bindJSON(c, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleApple(c *gin.Context) {
	var req oauthRequest
	if !bindJSON(c, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleRefresh(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}