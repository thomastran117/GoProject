package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"backend/internal/application/middleware"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type turnstileResponse struct {
	Success bool `json:"success"`
}

// VerifyTurnstile validates a Cloudflare Turnstile captcha token against the
// siteverify API. Returns an APIError if the token is missing, invalid, or the
// upstream call fails.
func VerifyTurnstile(ctx context.Context, client *http.Client, secretKey, token string) error {
	if secretKey == "" {
		return fmt.Errorf("turnstile: secretKey must not be empty")
	}
	if token == "" {
		return &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "CAPTCHA_MISSING",
			Message: "Captcha token is required",
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileVerifyURL,
		strings.NewReader(url.Values{
			"secret":   {secretKey},
			"response": {token},
		}.Encode()),
	)
	if err != nil {
		return fmt.Errorf("turnstile: failed to build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return &middleware.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CAPTCHA_UNAVAILABLE",
			Message: "Could not verify captcha, please try again",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return &middleware.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CAPTCHA_UNAVAILABLE",
			Message: "Could not verify captcha, please try again",
		}
	}

	var result turnstileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("turnstile: unexpected response from siteverify: %w", err)
	}

	if !result.Success {
		return &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "CAPTCHA_INVALID",
			Message: "Captcha verification failed",
		}
	}

	return nil
}
