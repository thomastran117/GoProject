package google

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"backend/internal/application/middleware"
)

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"
const googleIssuer1 = "accounts.google.com"
const googleIssuer2 = "https://accounts.google.com"

const googleRetryMax = 3
const googleRetryBase = 100 * time.Millisecond
const googleRetryMaxDelay = 1 * time.Second

// verifyTimeout caps the total time spent verifying a single token,
// including all retry attempts.
const verifyTimeout = 15 * time.Second

// TokenInfo holds the validated claims from a Google ID token.
type TokenInfo struct {
	Sub           string `json:"sub"`
	Aud           string `json:"aud"`
	Email         string `json:"email"`
	EmailVerified string `json:"email_verified"`
	Iss           string `json:"iss"`
	Exp           string `json:"exp"`
	ErrorDesc     string `json:"error_description"`
}

var errInvalidToken = &middleware.APIError{
	Status:  http.StatusUnauthorized,
	Code:    "INVALID_GOOGLE_TOKEN",
	Message: "Google token is invalid or expired",
}

// Client verifies Google ID tokens against Google's tokeninfo endpoint.
type Client struct {
	httpClient *http.Client
	clientID   string
}

// NewClient creates a Client. If clientID is non-empty, the aud claim of every
// verified token must match it.
func NewClient(httpClient *http.Client, clientID string) *Client {
	return &Client{httpClient: httpClient, clientID: clientID}
}

// VerifyIDToken calls Google's tokeninfo endpoint to validate the ID token
// and returns the parsed claims on success. Transient errors (network failures,
// 429, 5xx) are retried up to googleRetryMax times with exponential backoff and
// jitter. Permanent errors (4xx) are returned immediately.
// A hard verifyTimeout is applied over the caller's context so the total
// verification time (including all retries) is always bounded.
func (c *Client) VerifyIDToken(ctx context.Context, idToken string) (*TokenInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	var (
		body    TokenInfo
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

		resp, err := c.httpClient.Do(req)
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
			return nil, errInvalidToken // 4xx — permanent, don't retry
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
		return nil, errInvalidToken
	}

	// Validate issuer explicitly rather than relying solely on Google's check.
	if info.Iss != googleIssuer1 && info.Iss != googleIssuer2 {
		return nil, errInvalidToken
	}

	// Validate expiry explicitly. Exp is a Unix timestamp string.
	var exp int64
	if _, err := fmt.Sscanf(info.Exp, "%d", &exp); err != nil || exp == 0 {
		return nil, errInvalidToken
	}
	if time.Now().Unix() >= exp {
		return nil, errInvalidToken
	}

	if c.clientID != "" && info.Aud != c.clientID {
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
