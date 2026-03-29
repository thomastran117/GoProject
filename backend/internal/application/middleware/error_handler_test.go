package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newErrorRouter(handler gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	r.Use(ErrorHandler())
	r.GET("/", handler)
	return r
}

func TestErrorHandler_NoErrors_PassesThrough(t *testing.T) {
	r := newErrorRouter(func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestErrorHandler_APIError_ReturnsCorrectStatusAndCode(t *testing.T) {
	r := newErrorRouter(func(c *gin.Context) {
		c.Error(&APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "bad creds",
		})
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	body := responseBody(t, w)
	if body["success"] != false {
		t.Errorf("expected success=false, got %v", body["success"])
	}
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "INVALID_CREDENTIALS" {
		t.Errorf("expected INVALID_CREDENTIALS, got %v", errBlock["code"])
	}
	if errBlock["message"] != "bad creds" {
		t.Errorf("expected 'bad creds', got %v", errBlock["message"])
	}
}

func TestErrorHandler_UnknownError_Returns500(t *testing.T) {
	r := newErrorRouter(func(c *gin.Context) {
		c.Error(errors.New("something exploded"))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	body := responseBody(t, w)
	errBlock := body["error"].(map[string]any)
	if errBlock["code"] != "INTERNAL_SERVER_ERROR" {
		t.Errorf("expected INTERNAL_SERVER_ERROR, got %v", errBlock["code"])
	}
}

func TestErrorHandler_ResponseAlreadyWritten_IsNotOverwritten(t *testing.T) {
	r := newErrorRouter(func(c *gin.Context) {
		// Write a response AND add an error — the middleware should not
		// overwrite the already-written response.
		c.JSON(http.StatusOK, gin.H{"success": true})
		c.Error(errors.New("should be ignored"))
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (original response preserved), got %d", w.Code)
	}
}

func TestAPIError_ErrorMethod_ReturnsMessage(t *testing.T) {
	e := &APIError{Status: 400, Code: "BAD", Message: "bad input"}
	if e.Error() != "bad input" {
		t.Errorf("expected 'bad input', got %q", e.Error())
	}
}
