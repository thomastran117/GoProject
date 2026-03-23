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

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
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

	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) HandleSignup(c *gin.Context) {
	var req signupRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.Signup(c.Request.Context(), req.Email, req.Password, req.Role)
	if err != nil {
		c.Error(err)
		return
	}

	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) HandleVerify(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "Not ready"})
}

func (h *Handler) HandleGoogle(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.GoogleAuthenticate(c.Request.Context(), req.Token)
	if err != nil {
		c.Error(err)
		return
	}


	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) HandleMicrosoft(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.MicrosoftAuthenticate(c.Request.Context(), req.Token)
	if err != nil {
		c.Error(err)
		return
	}

	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) HandleApple(c *gin.Context) {
	var req oauthRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.AppleAuthenticate(c.Request.Context(), req.Token)
	if err != nil {
		c.Error(err)
		return
	}

	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) HandleRefresh(c *gin.Context) {
	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	refreshToken, ok := extractRefreshToken(c, info)
	if !ok {
		return
	}

	resp, err := h.service.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		c.Error(err)
		return
	}

	h.formatAuthResponse(c, info, resp)
}

func (h *Handler) Logout(c *gin.Context) {
	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	refreshToken, ok := extractRefreshToken(c, info)
	if !ok {
		return
	}

	if err := h.service.Logout(c.Request.Context(), refreshToken); err != nil {
		c.Error(err)
		return
	}

	if !info.IsMobile {
		clearRefreshCookie(c)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- private helpers ---

// setRefreshCookie writes the refresh token as an HttpOnly cookie.
// HttpOnly prevents JS access; Secure enforces HTTPS-only transmission.
func setRefreshCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, token, refreshCookieTTL, "/", "", true, true)
}

// clearRefreshCookie expires the refresh cookie immediately.
func clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, "/", "", true, true)
}

// extractRefreshToken reads the refresh token from the cookie (web) or request body (mobile).
func extractRefreshToken(c *gin.Context, info middleware.ClientInfo) (string, bool) {
	if info.IsMobile {
		var req refreshRequest
		if !request.BindJSON(c, &req) {
			return "", false
		}
		return req.RefreshToken, true
	}

	token, err := c.Cookie(refreshCookieName)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token missing"})
		return "", false
	}
	return token, true
}

func (h *Handler) formatAuthResponse(c *gin.Context, info middleware.ClientInfo, resp *AuthResponse) {
	data := gin.H{
		"message":      "Authentication successful",
		"access_token": resp.AccessToken,
		"user":         resp.User,
	}

	if info.IsMobile {
		data["refresh_token"] = resp.RefreshToken
	} else {
		setRefreshCookie(c, resp.RefreshToken)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}
