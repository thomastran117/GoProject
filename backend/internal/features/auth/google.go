package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"backend/internal/config/middleware"
)

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"
const googleIssuer1 = "accounts.google.com"
const googleIssuer2 = "https://accounts.google.com"

const googleRetryMax = 3
const googleRetryBase = 100 * time.Millisecond
const googleRetryMaxDelay = 1 * time.Second

type googleTokenInfo struct {
	Sub           string `json:"sub"`
	Aud           string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Iss           string `json:"iss"`
	Exp           string `json:"exp"`
	ErrorDesc     string `json:"error_description"`
}

var errInvalidGoogleToken = &middleware.APIError{
	Status:  http.StatusUnauthorized,
	Code:    "INVALID_GOOGLE_TOKEN",
	Message: "Google token is invalid or expired",
}

// verifyGoogleIDToken calls Google's tokeninfo endpoint to validate the ID token
// and returns the parsed claims on success. Transient errors (network failures,
// 429, 5xx) are retried up to googleRetryMax times with exponential backoff and
// jitter. Permanent errors (4xx) are returned immediately.
// A hard oauthVerifyTimeout is applied over the caller's context so the total
// verification time (including all retries) is always bounded.
func (s *Service) verifyGoogleIDToken(ctx context.Context, idToken string) (*googleTokenInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, oauthVerifyTimeout)
	defer cancel()

	var (
		body    googleTokenInfo
		lastErr error
	)

	for attempt := 0; attempt < googleRetryMax; attempt++ {
		if attempt > 0 {
			delay := googleRetryBase << uint(attempt-1)
			if delay > googleRetryMaxDelay {
				delay = googleRetryMaxDelay
			}
			// Non-cryptographic jitter (math/rand/v2 is fine here) spreads
			// retries to avoid thundering herd against the tokeninfo endpoint.
			jitter := time.Duration(rand.Int64N(int64(delay / 2)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleTokenInfoURL+"?id_token="+idToken, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build google tokeninfo request: %w", err)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("google token verification failed: %w", err)
			continue // network error — transient, retry
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			lastErr = fmt.Errorf("google tokeninfo returned %d", resp.StatusCode)
			continue // transient, retry
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, errInvalidGoogleToken // 4xx — permanent, don't retry
		}

		err = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to parse google token response: %w", err)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("google token verification failed after %d attempts: %w", googleRetryMax, lastErr)
	}

	info := &body

	if info.ErrorDesc != "" || info.Sub == "" {
		return nil, errInvalidGoogleToken
	}

	// Validate issuer explicitly rather than relying solely on Google's check.
	if info.Iss != googleIssuer1 && info.Iss != googleIssuer2 {
		return nil, errInvalidGoogleToken
	}

	// Validate expiry explicitly. Exp is a Unix timestamp string.
	var exp int64
	if _, err := fmt.Sscanf(info.Exp, "%d", &exp); err != nil || exp == 0 {
		return nil, errInvalidGoogleToken
	}
	if time.Now().Unix() >= exp {
		return nil, errInvalidGoogleToken
	}

	if s.googleClientID != "" && info.Aud != s.googleClientID {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_GOOGLE_TOKEN",
			Message: "Google token audience mismatch",
		}
	}

	// Strict boolean check — reject anything that isn't exactly "true".
	if info.EmailVerified != "true" {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "UNVERIFIED_EMAIL",
			Message: "Google account email is not verified",
		}
	}

	return info, nil
}
