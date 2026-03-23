package auth

import (
	"net/http"

	"backend/internal/app/utilities/request"
	"backend/internal/config/middleware"

	"github.com/gin-gonic/gin"
)

const refreshCookieName = "refresh_token"
const refreshCookieTTL = 7 * 24 * 60 * 60

type loginRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required"`
	Captcha  string `json:"captcha"  binding:"required"`
}

type signupRequest struct {
	Email    string `json:"email"    binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,strong_password"`
	Role     string `json:"role"     binding:"required"`
	Captcha  string `json:"captcha"  binding:"required"`
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

// --- public interface ---

func (h *Handler) HandleLogin(c *gin.Context) {
	var req loginRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		c.Error(err)
		return
	}

	if info.IsMobile{
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"message": "Login successful",
			"access_token": resp.AccessToken,
			"refresh_token": resp.RefreshToken,
			"user":         resp.User,
		}})
	} else {
		setRefreshCookie(c, resp.RefreshToken)
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
			"message": "Login successful",
			"access_token": resp.AccessToken,
			"user":         resp.User,
		}})
	}
}

func (h *Handler) HandleSignup(c *gin.Context) {
	var req signupRequest
	if !request.BindJSON(c, &req) {
		return
	}

	resp, err := h.service.Signup(c.Request.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		c.Error(err)
		return
	}

	setRefreshCookie(c, resp.RefreshToken)
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{
		"access_token": resp.AccessToken,
		"user":         resp.User,
	}})
}

func HandleVerify(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "test"})
}

func HandleGoogle(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleMicrosoft(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func HandleApple(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
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

// --- private helpers ---

// setRefreshCookie writes the refresh token as an HttpOnly cookie.
// HttpOnly prevents JS access; Secure enforces HTTPS-only transmission.
func setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, token, refreshCookieTTL, "/", "", true, true)
}
