package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/features/cache"
	"backend/internal/features/token"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// initTokenService spins up a miniredis instance and initialises the token
// package. Returns a cleanup function that the caller should defer.
func initTokenService(t *testing.T) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	token.Init("test-secret", cache.NewService(client))
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
}

// newAuthRouter returns a minimal router with Authenticate protecting /protected.
// The handler stores the retrieved claims in the response so tests can inspect them.
func newAuthRouter() *gin.Engine {
	r := gin.New()
	r.GET("/protected", Authenticate(), func(c *gin.Context) {
		claims, ok := GetClaims(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "claims missing"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user_id": claims.UserID,
			"email":   claims.Email,
			"role":    claims.Role,
		})
	})
	return r
}

func responseBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %s", err, w.Body.String())
	}
	return m
}

// --- extractBearerToken ---

func TestExtractBearerToken_ValidHeader(t *testing.T) {
	tok, ok := extractBearerToken("Bearer my-token")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if tok != "my-token" {
		t.Errorf("expected 'my-token', got %q", tok)
	}
}

func TestExtractBearerToken_CaseInsensitiveScheme(t *testing.T) {
	_, ok := extractBearerToken("bearer my-token")
	if !ok {
		t.Error("expected ok=true for lowercase 'bearer'")
	}
	_, ok = extractBearerToken("BEARER my-token")
	if !ok {
		t.Error("expected ok=true for uppercase 'BEARER'")
	}
}

func TestExtractBearerToken_EmptyHeader(t *testing.T) {
	_, ok := extractBearerToken("")
	if ok {
		t.Error("expected ok=false for empty header")
	}
}

func TestExtractBearerToken_WrongScheme(t *testing.T) {
	_, ok := extractBearerToken("Basic dXNlcjpwYXNz")
	if ok {
		t.Error("expected ok=false for Basic scheme")
	}
}

func TestExtractBearerToken_MissingTokenValue(t *testing.T) {
	_, ok := extractBearerToken("Bearer")
	if ok {
		t.Error("expected ok=false when token value is missing")
	}
}

func TestExtractBearerToken_ExtraFields(t *testing.T) {
	_, ok := extractBearerToken("Bearer tok extra")
	if ok {
		t.Error("expected ok=false for header with more than two fields")
	}
}

// --- Authenticate middleware ---

func TestAuthenticate_MissingHeader_Returns401(t *testing.T) {
	initTokenService(t)
	r := newAuthRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	body := responseBody(t, w)
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "MISSING_TOKEN" {
		t.Errorf("expected MISSING_TOKEN, got %v", errBlock["code"])
	}
}

func TestAuthenticate_MalformedHeader_Returns401(t *testing.T) {
	initTokenService(t)
	r := newAuthRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "NotBearer stuff")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthenticate_InvalidToken_Returns401(t *testing.T) {
	initTokenService(t)
	r := newAuthRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer this.is.not.valid")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	body := responseBody(t, w)
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "INVALID_TOKEN" {
		t.Errorf("expected INVALID_TOKEN, got %v", errBlock["code"])
	}
}

func TestAuthenticate_ValidToken_CallsNextAndStoresClaims(t *testing.T) {
	initTokenService(t)

	pair, err := token.GeneratePair(
		t.Context(), 42, "alice@example.com", "admin",
		token.RefreshTTLDefault,
	)
	if err != nil {
		t.Fatalf("GeneratePair: %v", err)
	}

	r := newAuthRouter()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := responseBody(t, w)
	if body["email"] != "alice@example.com" {
		t.Errorf("expected email 'alice@example.com', got %v", body["email"])
	}
	if body["role"] != "admin" {
		t.Errorf("expected role 'admin', got %v", body["role"])
	}
}

// --- GetClaims ---

func TestGetClaims_WithoutMiddleware_ReturnsFalse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	_, ok := GetClaims(c)
	if ok {
		t.Error("expected ok=false when middleware was not applied")
	}
}

func TestGetClaims_WrongType_ReturnsFalse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(claimsKey, "not-a-claims-struct")
	_, ok := GetClaims(c)
	if ok {
		t.Error("expected ok=false for wrong type stored under claimsKey")
	}
}
