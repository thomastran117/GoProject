package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

func googleServer(t *testing.T, info TokenInfo, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(info)
	}))
}

func newClient(srv *httptest.Server) *Client {
	return NewClient(&http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}, "")
}

func validTokenInfo() TokenInfo {
	return TokenInfo{
		Sub:           "google-sub-123",
		Aud:           "",
		Email:         "user@example.com",
		EmailVerified: "true",
		Iss:           googleIssuer1,
		Exp:           fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()),
	}
}

// assertAPIError fails the test unless err is a *middleware.APIError with the expected code.
func assertAPIError(t *testing.T, err error, code string) {
	t.Helper()
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *middleware.APIError, got %T: %v", err, err)
	}
	if apiErr.Code != code {
		t.Errorf("expected code %q, got %q", code, apiErr.Code)
	}
}

func TestVerifyIDToken_Valid(t *testing.T) {
	srv := googleServer(t, validTokenInfo(), http.StatusOK)
	defer srv.Close()

	c := newClient(srv)
	info, err := c.VerifyIDToken(context.Background(), "fake-id-token")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if info.Sub != "google-sub-123" {
		t.Errorf("expected Sub 'google-sub-123', got %q", info.Sub)
	}
}

func TestVerifyIDToken_BadIssuer(t *testing.T) {
	info := validTokenInfo()
	info.Iss = "https://evil.com"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	c := newClient(srv)
	_, err := c.VerifyIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyIDToken_Expired(t *testing.T) {
	info := validTokenInfo()
	info.Exp = fmt.Sprintf("%d", time.Now().Add(-time.Hour).Unix())
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	c := newClient(srv)
	_, err := c.VerifyIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyIDToken_UnverifiedEmail(t *testing.T) {
	info := validTokenInfo()
	info.EmailVerified = "false"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	c := newClient(srv)
	_, err := c.VerifyIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "UNVERIFIED_EMAIL")
}

func TestVerifyIDToken_AudienceMismatch(t *testing.T) {
	info := validTokenInfo()
	info.Aud = "other-client"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	c := NewClient(&http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}}, "my-client-id")
	_, err := c.VerifyIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyIDToken_4xxResponseIsPermanentError(t *testing.T) {
	srv := googleServer(t, TokenInfo{}, http.StatusBadRequest)
	defer srv.Close()

	c := newClient(srv)
	_, err := c.VerifyIDToken(context.Background(), "bad-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyIDToken_5xxRetriesAndFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := newClient(srv)
	_, err := c.VerifyIDToken(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if calls != googleRetryMax {
		t.Errorf("expected %d retry attempts, got %d", googleRetryMax, calls)
	}
}
