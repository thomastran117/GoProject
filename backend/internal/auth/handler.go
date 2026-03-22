package auth

import (
	"errors"
	"net/http"
	"unicode"

	"backend/internal/middleware"

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

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) HandleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(&middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
		})
		return
	}

	resp, err := h.service.Login(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) || errors.Is(err, ErrInvalidPassword) {
			c.Error(&middleware.APIError{
				Status:  http.StatusUnauthorized,
				Code:    "INVALID_CREDENTIALS",
				Message: "Invalid email or password",
			})
			return
		}
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *Handler) HandleSignup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(&middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: err.Error(),
		})
		return
	}

	resp, err := h.service.Signup(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			c.Error(&middleware.APIError{
				Status:  http.StatusConflict,
				Code:    "USER_EXISTS",
				Message: "An account with this email already exists",
			})
			return
		}
		c.Error(err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": resp})
}

func HandleVerify(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "test"})
}

func HandleGoogle(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleMicrosoft(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleApple(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleRefresh(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
