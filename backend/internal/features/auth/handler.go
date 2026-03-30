package auth

import (
	"context"
	"net/http"
	"time"

	"backend/internal/application/request"
	"backend/internal/application/middleware"

	"github.com/gin-gonic/gin"
)

const refreshCookieName = "refresh_token"

type authService interface {
	Login(ctx context.Context, email, password, captcha string, rememberMe bool) (*AuthResponse, error)
	Signup(ctx context.Context, email, password, captcha, role string, rememberMe bool) (*SignupPendingResponse, error)
	VerifyEmail(ctx context.Context, token string) (*AuthResponse, error)
	SetRole(ctx context.Context, userID uint64, role string) (*AuthResponse, error)
	Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	GoogleAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error)
	MicrosoftAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error)
	AppleAuthenticate(ctx context.Context, t string) (*AuthResponse, error)
}

type loginRequest struct {
	Email      string `json:"email"       binding:"required,email"`
	Password   string `json:"password"    binding:"required"`
	Captcha    string `json:"captcha"     binding:"required"`
	RememberMe bool   `json:"remember_me"`
}

type signupRequest struct {
	Email      string `json:"email"       binding:"required,email"`
	Password   string `json:"password"    binding:"required,min=8,strong_password"`
	Role       string `json:"role"        binding:"required,valid_signup_role"`
	Captcha    string `json:"captcha"     binding:"required"`
	RememberMe bool   `json:"remember_me"`
}

type setRoleRequest struct {
	Role string `json:"role" binding:"required,valid_signup_role"`
}

type oauthRequest struct {
	Token string `json:"token" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type verifyRequest struct {
	Token string `json:"token" binding:"required"`
}

type Handler struct {
	service authService
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

	resp, err := h.service.Login(c.Request.Context(), req.Email, req.Password, req.Captcha, req.RememberMe)
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

	resp, err := h.service.Signup(c.Request.Context(), req.Email, req.Password, req.Captcha, req.Role, req.RememberMe)
	if err != nil {
		c.Error(err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": resp})
}

func (h *Handler) HandleVerify(c *gin.Context) {
	var req verifyRequest
	if !request.BindJSON(c, &req) {
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.VerifyEmail(c.Request.Context(), req.Token)
	if err != nil {
		c.Error(err)
		return
	}

	h.formatAuthResponse(c, info, resp)
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

func (h *Handler) HandleSetRole(c *gin.Context) {
	var req setRoleRequest
	if !request.BindJSON(c, &req) {
		return
	}

	claims, ok := middleware.GetClaims(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	info, ok := middleware.GetClientInfo(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client info missing"})
		return
	}

	resp, err := h.service.SetRole(c.Request.Context(), claims.UserID, req.Role)
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
// ttl must be the same duration used when storing the token in Redis so that
// the cookie and the backing store expire together; a mismatch would leave
// clients holding a cookie that references an already-expired Redis key.
func setRefreshCookie(c *gin.Context, token string, ttl time.Duration) {
	if ttl <= 0 {
		panic("auth: setRefreshCookie called with non-positive TTL")
	}
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, token, int(ttl.Seconds()), "/", "", true, true)
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
		// resp.RefreshTTL is the same duration used to store the token in Redis,
		// keeping the cookie max-age and the Redis expiry in sync.
		setRefreshCookie(c, resp.RefreshToken, resp.RefreshTTL)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    data,
	})
}
