package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"backend/internal/config/middleware"
)

const turnstileVerifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

type turnstileResponse struct {
	Success bool `json:"success"`
}

// VerifyTurnstile validates a Cloudflare Turnstile captcha token against the
// siteverify API. Returns an APIError if the token is missing, invalid, or the
// upstream call fails.
func VerifyTurnstile(ctx context.Context, client *http.Client, secretKey, token string) error {
	resp, err := client.PostForm(turnstileVerifyURL, url.Values{
		"secret":   {secretKey},
		"response": {token},
	})
	if err != nil {
		return &middleware.APIError{
			Status:  http.StatusServiceUnavailable,
			Code:    "CAPTCHA_UNAVAILABLE",
			Message: "Could not verify captcha, please try again",
		}
	}
	defer resp.Body.Close()

	var result turnstileResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || !result.Success {
		return &middleware.APIError{
			Status:  http.StatusBadRequest,
			Code:    "CAPTCHA_INVALID",
			Message: "Captcha verification failed",
		}
	}

	return nil
}
