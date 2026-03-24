package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"context"

	"backend/internal/config/middleware"
)

func googleServer(t *testing.T, info googleTokenInfo, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(info)
	}))
}

func newGoogleService(srv *httptest.Server) *Service {
	return &Service{
		httpClient: &http.Client{Transport: &urlOverrideTransport{serverURL: srv.URL}},
	}
}

func validGoogleInfo() googleTokenInfo {
	return googleTokenInfo{
		Sub:           "google-sub-123",
		Aud:           "",
		Email:         "user@example.com",
		EmailVerified: "true",
		Iss:           googleIssuer1,
		Exp:           fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()),
	}
}

func TestVerifyGoogleIDToken_Valid(t *testing.T) {
	srv := googleServer(t, validGoogleInfo(), http.StatusOK)
	defer srv.Close()

	svc := newGoogleService(srv)
	info, err := svc.verifyGoogleIDToken(context.Background(), "fake-id-token")
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if info.Sub != "google-sub-123" {
		t.Errorf("expected Sub 'google-sub-123', got %q", info.Sub)
	}
}

func TestVerifyGoogleIDToken_BadIssuer(t *testing.T) {
	info := validGoogleInfo()
	info.Iss = "https://evil.com"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	svc := newGoogleService(srv)
	_, err := svc.verifyGoogleIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyGoogleIDToken_Expired(t *testing.T) {
	info := validGoogleInfo()
	info.Exp = fmt.Sprintf("%d", time.Now().Add(-time.Hour).Unix())
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	svc := newGoogleService(srv)
	_, err := svc.verifyGoogleIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyGoogleIDToken_UnverifiedEmail(t *testing.T) {
	info := validGoogleInfo()
	info.EmailVerified = "false"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	svc := newGoogleService(srv)
	_, err := svc.verifyGoogleIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "UNVERIFIED_EMAIL")
}

func TestVerifyGoogleIDToken_AudienceMismatch(t *testing.T) {
	info := validGoogleInfo()
	info.Aud = "other-client"
	srv := googleServer(t, info, http.StatusOK)
	defer srv.Close()

	svc := newGoogleService(srv)
	svc.googleClientID = "my-client-id"
	_, err := svc.verifyGoogleIDToken(context.Background(), "fake-id-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyGoogleIDToken_4xxResponseIsPermanentError(t *testing.T) {
	srv := googleServer(t, googleTokenInfo{}, http.StatusBadRequest)
	defer srv.Close()

	svc := newGoogleService(srv)
	_, err := svc.verifyGoogleIDToken(context.Background(), "bad-token")
	assertAPIError(t, err, "INVALID_GOOGLE_TOKEN")
}

func TestVerifyGoogleIDToken_5xxRetriesAndFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := newGoogleService(srv)
	_, err := svc.verifyGoogleIDToken(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error after retries")
	}
	if calls != googleRetryMax {
		t.Errorf("expected %d retry attempts, got %d", googleRetryMax, calls)
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
