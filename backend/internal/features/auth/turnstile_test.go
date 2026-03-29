package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"backend/internal/application/middleware"
)

// urlOverrideTransport redirects all HTTP requests to a fixed base URL,
// preserving the original path and query string.
type urlOverrideTransport struct {
	serverURL string
}

func (t *urlOverrideTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, _ := url.Parse(t.serverURL)
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = target.Scheme
	req2.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req2)
}

func turnstileServer(t *testing.T, success bool, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(turnstileResponse{Success: success})
	}))
}

func TestVerifyTurnstile_EmptySecret(t *testing.T) {
	err := VerifyTurnstile(context.Background(), http.DefaultClient, "", "some-token")
	if err == nil {
		t.Fatal("expected error for empty secret key")
	}
}

func TestVerifyTurnstile_EmptyToken(t *testing.T) {
	err := VerifyTurnstile(context.Background(), http.DefaultClient, "secret", "")
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "CAPTCHA_MISSING" {
		t.Errorf("expected CAPTCHA_MISSING, got %s", apiErr.Code)
	}
}

func TestVerifyTurnstile_Success(t *testing.T) {
	srv := turnstileServer(t, true, http.StatusOK)
	defer srv.Close()

	client := &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}
	if err := VerifyTurnstile(context.Background(), client, "secret", "valid-token"); err != nil {
		t.Errorf("expected success, got: %v", err)
	}
}

func TestVerifyTurnstile_InvalidToken(t *testing.T) {
	srv := turnstileServer(t, false, http.StatusOK)
	defer srv.Close()

	client := &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}
	err := VerifyTurnstile(context.Background(), client, "secret", "bad-token")
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "CAPTCHA_INVALID" {
		t.Errorf("expected CAPTCHA_INVALID, got %s", apiErr.Code)
	}
}

func TestVerifyTurnstile_ServerError(t *testing.T) {
	srv := turnstileServer(t, false, http.StatusInternalServerError)
	defer srv.Close()

	client := &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}
	err := VerifyTurnstile(context.Background(), client, "secret", "token")
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "CAPTCHA_UNAVAILABLE" {
		t.Errorf("expected CAPTCHA_UNAVAILABLE, got %s", apiErr.Code)
	}
}

func TestVerifyTurnstile_NetworkError(t *testing.T) {
	// Point at a server that immediately closes connections.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	client := &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}
	err := VerifyTurnstile(context.Background(), client, "secret", "token")
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "CAPTCHA_UNAVAILABLE" {
		t.Errorf("expected CAPTCHA_UNAVAILABLE, got %s", apiErr.Code)
	}
}
