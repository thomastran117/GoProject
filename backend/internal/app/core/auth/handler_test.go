package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"backend/internal/app/core/token"
	"backend/internal/app/utilities/validators"
	"backend/internal/config/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	govalidator "github.com/go-playground/validator/v10"
)

func init() {
	gin.SetMode(gin.TestMode)
	if v, ok := binding.Validator.Engine().(*govalidator.Validate); ok {
		validators.Register(v)
	}
}

// --- mock service ---

type mockService struct {
	loginFn     func(ctx context.Context, email, password, captcha string, rememberMe bool) (*AuthResponse, error)
	signupFn    func(ctx context.Context, email, password, captcha, role string, rememberMe bool) (*AuthResponse, error)
	setRoleFn   func(ctx context.Context, userID uint64, role string) (*AuthResponse, error)
	refreshFn   func(ctx context.Context, refreshToken string) (*AuthResponse, error)
	logoutFn    func(ctx context.Context, refreshToken string) error
	googleFn    func(ctx context.Context, idToken string) (*AuthResponse, error)
	microsoftFn func(ctx context.Context, idToken string) (*AuthResponse, error)
	appleFn     func(ctx context.Context, t string) (*AuthResponse, error)
}

func (m *mockService) Login(ctx context.Context, email, password, captcha string, rememberMe bool) (*AuthResponse, error) {
	return m.loginFn(ctx, email, password, captcha, rememberMe)
}
func (m *mockService) Signup(ctx context.Context, email, password, captcha, role string, rememberMe bool) (*AuthResponse, error) {
	return m.signupFn(ctx, email, password, captcha, role, rememberMe)
}
func (m *mockService) SetRole(ctx context.Context, userID uint64, role string) (*AuthResponse, error) {
	return m.setRoleFn(ctx, userID, role)
}
func (m *mockService) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	return m.refreshFn(ctx, refreshToken)
}
func (m *mockService) Logout(ctx context.Context, refreshToken string) error {
	return m.logoutFn(ctx, refreshToken)
}
func (m *mockService) GoogleAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	return m.googleFn(ctx, idToken)
}
func (m *mockService) MicrosoftAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	return m.microsoftFn(ctx, idToken)
}
func (m *mockService) AppleAuthenticate(ctx context.Context, t string) (*AuthResponse, error) {
	return m.appleFn(ctx, t)
}

// --- test helpers ---

func okAuthResponse() *AuthResponse {
	return &AuthResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		RefreshTTL:   24 * time.Hour,
		User:         UserData{ID: 1, Email: "user@example.com", Role: "student"},
	}
}

// newRouter builds a gin engine with ErrorHandler + ClientInfoMiddleware already applied.
func newRouter(svc authService) *gin.Engine {
	h := &Handler{service: svc}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.ClientInfoMiddleware())
	r.POST("/login", h.HandleLogin)
	r.POST("/signup", h.HandleSignup)
	r.GET("/verify", h.HandleVerify)
	r.POST("/google", h.HandleGoogle)
	r.POST("/microsoft", h.HandleMicrosoft)
	r.POST("/apple", h.HandleApple)
	r.POST("/refresh", h.HandleRefresh)
	r.POST("/logout", h.Logout)
	return r
}

// newRouterNoClientInfo builds a router WITHOUT ClientInfoMiddleware so handlers
// hit the "client info missing" early-exit path.
func newRouterNoClientInfo(svc authService) *gin.Engine {
	h := &Handler{service: svc}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.POST("/login", h.HandleLogin)
	r.POST("/signup", h.HandleSignup)
	r.POST("/refresh", h.HandleRefresh)
	r.POST("/logout", h.Logout)
	return r
}

// claimsMiddleware injects the given claims into the gin context, simulating
// an authenticated request without needing a real JWT.
func claimsMiddleware(claims *token.AccessClaims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("auth_claims", claims)
		c.Next()
	}
}

// newSetRoleRouter builds a router for HandleSetRole tests.
// If claims is non-nil they are injected; otherwise the handler receives no claims (→ 401).
func newSetRoleRouter(svc authService, claims *token.AccessClaims) *gin.Engine {
	h := &Handler{service: svc}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.Use(middleware.ClientInfoMiddleware())
	if claims != nil {
		r.POST("/role", claimsMiddleware(claims), h.HandleSetRole)
	} else {
		r.POST("/role", h.HandleSetRole)
	}
	return r
}

// newSetRoleRouterNoClientInfo builds a router for HandleSetRole with claims but
// without ClientInfoMiddleware, hitting the "client info missing" path.
func newSetRoleRouterNoClientInfo(svc authService, claims *token.AccessClaims) *gin.Engine {
	h := &Handler{service: svc}
	r := gin.New()
	r.Use(middleware.ErrorHandler())
	r.POST("/role", claimsMiddleware(claims), h.HandleSetRole)
	return r
}

func testClaims() *token.AccessClaims {
	return &token.AccessClaims{UserID: 1, Email: "user@example.com", Role: ""}
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return bytes.NewBuffer(b)
}

func getResponseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %s", err, w.Body.String())
	}
	return body
}

// desktopUA is a User-Agent that resolves to a non-mobile (web) client.
const desktopUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"

// mobileUA is a User-Agent that resolves to a mobile client.
const mobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) Mobile/15E148"

// --- cookie helpers ---

func TestSetRefreshCookie_SetsCorrectAttributes(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	setRefreshCookie(c, "my-refresh-token", time.Hour)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a cookie to be set")
	}
	cookie := cookies[0]
	if cookie.Name != refreshCookieName {
		t.Errorf("expected cookie name %q, got %q", refreshCookieName, cookie.Name)
	}
	if cookie.Value != "my-refresh-token" {
		t.Errorf("expected cookie value 'my-refresh-token', got %q", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}
	if cookie.MaxAge <= 0 {
		t.Errorf("expected positive MaxAge, got %d", cookie.MaxAge)
	}
}

func TestClearRefreshCookie_SetsNegativeMaxAge(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	clearRefreshCookie(c)

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected a cookie to be set")
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("expected negative MaxAge to clear cookie, got %d", cookies[0].MaxAge)
	}
}

// --- extractRefreshToken ---

func TestExtractRefreshToken_Web_FromCookie(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-token"})
	c.Request = req

	info := middleware.ClientInfo{IsMobile: false}
	token, ok := extractRefreshToken(c, info)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if token != "cookie-token" {
		t.Errorf("expected 'cookie-token', got %q", token)
	}
}

func TestExtractRefreshToken_Web_MissingCookie_Returns401(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := middleware.ClientInfo{IsMobile: false}
	_, ok := extractRefreshToken(c, info)
	if ok {
		t.Fatal("expected ok=false when cookie is missing")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestExtractRefreshToken_Mobile_FromBody(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := jsonBody(t, map[string]string{"refresh_token": "body-token"})
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	info := middleware.ClientInfo{IsMobile: true}
	token, ok := extractRefreshToken(c, info)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if token != "body-token" {
		t.Errorf("expected 'body-token', got %q", token)
	}
}

// --- formatAuthResponse ---

func TestFormatAuthResponse_Web_SetsRefreshCookieAndReturnsAccessToken(t *testing.T) {
	r := newRouter(&mockService{})
	w := httptest.NewRecorder()

	// Reach formatAuthResponse by making a successful login.
	r2 := newRouter(&mockService{
		loginFn: func(_ context.Context, _, _, _ string, _ bool) (*AuthResponse, error) {
			return okAuthResponse(), nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/login", jsonBody(t, map[string]any{
		"email": "user@example.com", "password": "password123", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r2.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Refresh token must be in a cookie, not the body.
	body := getResponseBody(t, w)
	data := body["data"].(map[string]any)
	if _, ok := data["refresh_token"]; ok {
		t.Error("refresh_token should not appear in body for web clients")
	}
	if data["access_token"] != "access-token" {
		t.Errorf("expected access_token in body, got %v", data["access_token"])
	}

	var found bool
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected refresh_token cookie to be set")
	}
	_ = r
}

func TestFormatAuthResponse_Mobile_ReturnsBothTokensInBody(t *testing.T) {
	r := newRouter(&mockService{
		loginFn: func(_ context.Context, _, _, _ string, _ bool) (*AuthResponse, error) {
			return okAuthResponse(), nil
		},
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", jsonBody(t, map[string]any{
		"email": "user@example.com", "password": "password123", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", mobileUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := getResponseBody(t, w)
	data := body["data"].(map[string]any)
	if data["refresh_token"] != "refresh-token" {
		t.Errorf("expected refresh_token in body for mobile, got %v", data["refresh_token"])
	}
}

// --- HandleVerify ---

func TestHandleVerify_Returns501(t *testing.T) {
	r := newRouter(&mockService{})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/verify", nil))
	if w.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", w.Code)
	}
}

// --- HandleLogin ---

func TestHandleLogin_MissingClientInfo_Returns400(t *testing.T) {
	r := newRouterNoClientInfo(&mockService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", jsonBody(t, map[string]any{
		"email": "user@example.com", "password": "password123", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleLogin_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(&mockService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewBufferString(`{bad json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleLogin_ServiceError_ReturnsErrorStatus(t *testing.T) {
	svc := &mockService{
		loginFn: func(_ context.Context, _, _, _ string, _ bool) (*AuthResponse, error) {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: "INVALID_CREDENTIALS", Message: "bad creds"}
		},
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", jsonBody(t, map[string]any{
		"email": "user@example.com", "password": "wrong", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	body := getResponseBody(t, w)
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("unexpected error code: %v", errBlock["code"])
	}
}

func TestHandleLogin_Success_Web(t *testing.T) {
	svc := &mockService{
		loginFn: func(_ context.Context, _, _, _ string, _ bool) (*AuthResponse, error) {
			return okAuthResponse(), nil
		},
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/login", jsonBody(t, map[string]any{
		"email": "user@example.com", "password": "password123", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := getResponseBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
}

// --- HandleSignup ---

func TestHandleSignup_InvalidJSON_Returns400(t *testing.T) {
	r := newRouter(&mockService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signup", bytes.NewBufferString(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSignup_InvalidRole_Returns400(t *testing.T) {
	for _, role := range []string{"admin", "superuser", ""} {
		r := newRouter(&mockService{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/signup", jsonBody(t, map[string]any{
			"email": "new@example.com", "password": "Str0ng!Pass", "role": role, "captcha": "token",
		}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", desktopUA)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("role %q: expected 400, got %d: %s", role, w.Code, w.Body.String())
		}
	}
}

func TestHandleSignup_Success(t *testing.T) {
	svc := &mockService{
		signupFn: func(_ context.Context, _, _, _, _ string, _ bool) (*AuthResponse, error) {
			return okAuthResponse(), nil
		},
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/signup", jsonBody(t, map[string]any{
		"email": "new@example.com", "password": "Str0ng!Pass", "role": "student", "captcha": "token",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// --- HandleRefresh ---

func TestHandleRefresh_MissingClientInfo_Returns400(t *testing.T) {
	r := newRouterNoClientInfo(&mockService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "tok"})
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleRefresh_MissingCookie_Returns401(t *testing.T) {
	r := newRouter(&mockService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("User-Agent", desktopUA)
	// No cookie attached.
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRefresh_ServiceError_ReturnsErrorStatus(t *testing.T) {
	svc := &mockService{
		refreshFn: func(_ context.Context, _ string) (*AuthResponse, error) {
			return nil, &middleware.APIError{Status: http.StatusUnauthorized, Code: "INVALID_REFRESH_TOKEN", Message: "expired"}
		},
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("User-Agent", desktopUA)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "expired-token"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleRefresh_Success(t *testing.T) {
	svc := &mockService{
		refreshFn: func(_ context.Context, _ string) (*AuthResponse, error) {
			return okAuthResponse(), nil
		},
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	req.Header.Set("User-Agent", desktopUA)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "valid-token"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Logout ---

func TestLogout_Web_ClearsRefreshCookie(t *testing.T) {
	svc := &mockService{
		logoutFn: func(_ context.Context, _ string) error { return nil },
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("User-Agent", desktopUA)
	req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "tok"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// The cookie should be cleared (MaxAge < 0).
	var cleared bool
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Error("expected refresh_token cookie to be cleared after web logout")
	}
}

func TestLogout_Mobile_Returns200WithoutClearingCookie(t *testing.T) {
	svc := &mockService{
		logoutFn: func(_ context.Context, _ string) error { return nil },
	}
	r := newRouter(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", jsonBody(t, map[string]string{
		"refresh_token": "tok",
	}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", mobileUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			t.Error("mobile logout should not set or clear refresh_token cookie")
		}
	}
}

// --- HandleSetRole ---

func TestHandleSetRole_InvalidJSON_Returns400(t *testing.T) {
	r := newSetRoleRouter(&mockService{}, testClaims())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/role", bytes.NewBufferString(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSetRole_InvalidRole_Returns400(t *testing.T) {
	for _, role := range []string{"admin", "superuser", ""} {
		r := newSetRoleRouter(&mockService{}, testClaims())
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/role", jsonBody(t, map[string]any{"role": role}))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", desktopUA)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("role %q: expected 400, got %d: %s", role, w.Code, w.Body.String())
		}
	}
}

func TestHandleSetRole_NoClaims_Returns401(t *testing.T) {
	r := newSetRoleRouter(&mockService{}, nil)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/role", jsonBody(t, map[string]any{"role": "student"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSetRole_MissingClientInfo_Returns400(t *testing.T) {
	r := newSetRoleRouterNoClientInfo(&mockService{}, testClaims())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/role", jsonBody(t, map[string]any{"role": "student"}))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSetRole_ServiceError_ReturnsErrorStatus(t *testing.T) {
	svc := &mockService{
		setRoleFn: func(_ context.Context, _ uint64, _ string) (*AuthResponse, error) {
			return nil, &middleware.APIError{Status: http.StatusConflict, Code: "ROLE_ALREADY_SET", Message: "role already set"}
		},
	}
	r := newSetRoleRouter(svc, testClaims())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/role", jsonBody(t, map[string]any{"role": "teacher"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
	body := getResponseBody(t, w)
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "ROLE_ALREADY_SET" {
		t.Errorf("unexpected error code: %v", errBlock["code"])
	}
}

func TestHandleSetRole_Success(t *testing.T) {
	svc := &mockService{
		setRoleFn: func(_ context.Context, _ uint64, _ string) (*AuthResponse, error) {
			return &AuthResponse{
				AccessToken:  "new-access-token",
				RefreshToken: "new-refresh-token",
				RefreshTTL:   24 * time.Hour,
				User:         UserData{ID: 1, Email: "user@example.com", Role: "student"},
			}, nil
		},
	}
	r := newSetRoleRouter(svc, testClaims())
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/role", jsonBody(t, map[string]any{"role": "student"}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", desktopUA)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := getResponseBody(t, w)
	if body["success"] != true {
		t.Errorf("expected success=true, got %v", body["success"])
	}
	data := body["data"].(map[string]any)
	if data["access_token"] != "new-access-token" {
		t.Errorf("expected new access token, got %v", data["access_token"])
	}
}
