package auth

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"backend/internal/config/middleware"

	jwt "github.com/golang-jwt/jwt/v5"
)

const msJWKSURL = "https://login.microsoftonline.com/common/discovery/v2.0/keys"
const msIssuerPrefix = "https://login.microsoftonline.com/"
const msIssuerSuffix = "/v2.0"
const msRetryMax = 3
const msRetryBase = 100 * time.Millisecond
const msRetryMaxDelay = 1 * time.Second

// jwksCacheTTL controls how long Microsoft's public keys are cached before
// a background refresh is triggered. Key rotation forces an earlier refresh.
const jwksCacheTTL = 1 * time.Hour

// msJWKSCache holds Microsoft's public keys with a TTL so we don't refetch
// on every request. Guarded by a RWMutex for safe concurrent access.
type msJWKSCache struct {
	mu        sync.RWMutex
	keys      *msJWKSet
	expiresAt time.Time
}

func (c *msJWKSCache) get() *msJWKSet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.keys != nil && time.Now().Before(c.expiresAt) {
		return c.keys
	}
	return nil
}

func (c *msJWKSCache) set(keys *msJWKSet) {
	c.mu.Lock()
	c.keys = keys
	c.expiresAt = time.Now().Add(jwksCacheTTL)
	c.mu.Unlock()
}

func (c *msJWKSCache) invalidate() {
	c.mu.Lock()
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

type msJWKSet struct {
	Keys []msJWK `json:"keys"`
}

type msJWK struct {
	Kid string   `json:"kid"`
	X5C []string `json:"x5c"`
}

// publicKeyForKid finds the JWK matching kid and extracts its RSA public key
// from the first X.509 certificate in the x5c chain.
func (ks *msJWKSet) publicKeyForKid(kid string) (*rsa.PublicKey, error) {
	for _, k := range ks.Keys {
		if k.Kid != kid || len(k.X5C) == 0 {
			continue
		}
		der, err := base64.StdEncoding.DecodeString(k.X5C[0])
		if err != nil {
			return nil, fmt.Errorf("failed to decode x5c certificate: %w", err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("failed to parse x5c certificate: %w", err)
		}
		pub, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, fmt.Errorf("unexpected key type in microsoft JWKS")
		}
		return pub, nil
	}
	return nil, fmt.Errorf("no microsoft JWKS key found for kid %q", kid)
}

type msClaims struct {
	jwt.RegisteredClaims
	OID               string `json:"oid"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	TenantID          string `json:"tid"`
}

var errInvalidMicrosoftToken = &middleware.APIError{
	Status:  http.StatusUnauthorized,
	Code:    "INVALID_MICROSOFT_TOKEN",
	Message: "Microsoft token is invalid or expired",
}

// verifyMicrosoftIDToken validates a Microsoft ID token by fetching the JWKS,
// verifying the JWT signature, and checking all required claims.
// Transient errors (network failures, 429, 5xx) are retried with exponential
// backoff. Permanent errors are returned immediately.
// A hard oauthVerifyTimeout is applied over the caller's context so the total
// verification time (including JWKS fetch and all retries) is always bounded.
func (s *Service) verifyMicrosoftIDToken(ctx context.Context, idToken string) (*msClaims, error) {
	ctx, cancel := context.WithTimeout(ctx, oauthVerifyTimeout)
	defer cancel()

	// Parse unverified first to extract the key ID from the JWT header.
	unverified, _, err := jwt.NewParser().ParseUnverified(idToken, &msClaims{})
	if err != nil {
		return nil, errInvalidMicrosoftToken
	}
	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errInvalidMicrosoftToken
	}

	jwks, err := s.fetchMicrosoftJWKS(ctx)
	if err != nil {
		return nil, err
	}

	pubKey, err := jwks.publicKeyForKid(kid)
	if err != nil {
		// kid missing — likely a key rotation; invalidate the cache and retry once.
		s.jwksCache.invalidate()
		jwks, err = s.fetchMicrosoftJWKS(ctx)
		if err != nil {
			return nil, err
		}
		pubKey, err = jwks.publicKeyForKid(kid)
		if err != nil {
			return nil, errInvalidMicrosoftToken
		}
	}

	var claims msClaims
	_, err = jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	).ParseWithClaims(idToken, &claims, func(_ *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		return nil, errInvalidMicrosoftToken
	}

	// Validate issuer format: https://login.microsoftonline.com/{tenantId}/v2.0
	if !strings.HasPrefix(claims.Issuer, msIssuerPrefix) || !strings.HasSuffix(claims.Issuer, msIssuerSuffix) {
		return nil, errInvalidMicrosoftToken
	}

	// Microsoft tokens can carry multiple audiences (e.g. resource URIs alongside
	// the client ID). Always require a non-empty audience, and when a client ID is
	// configured verify it is present in the list.
	if len(claims.Audience) == 0 {
		return nil, errInvalidMicrosoftToken
	}
	if s.microsoftClientID != "" && !slices.Contains([]string(claims.Audience), s.microsoftClientID) {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_MICROSOFT_TOKEN",
			Message: "Microsoft token audience mismatch",
		}
	}

	if claims.OID == "" {
		return nil, errInvalidMicrosoftToken
	}

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}
	if email == "" {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_MICROSOFT_TOKEN",
			Message: "Microsoft token contains no email address",
		}
	}

	return &claims, nil
}

// fetchMicrosoftJWKS returns Microsoft's public key set, serving from the
// in-memory cache when valid and fetching fresh keys (with retry/backoff)
// otherwise.
func (s *Service) fetchMicrosoftJWKS(ctx context.Context) (*msJWKSet, error) {
	if cached := s.jwksCache.get(); cached != nil {
		return cached, nil
	}

	var (
		jwks    msJWKSet
		lastErr error
	)

	for attempt := 0; attempt < msRetryMax; attempt++ {
		if attempt > 0 {
			delay := msRetryBase << uint(attempt-1)
			if delay > msRetryMaxDelay {
				delay = msRetryMaxDelay
			}
			// Non-cryptographic jitter; see oauthHTTPClient comment.
			jitter := time.Duration(rand.Int64N(int64(delay / 2)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay + jitter):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, msJWKSURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build microsoft JWKS request: %w", err)
		}

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("microsoft JWKS fetch failed: %w", err)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= http.StatusInternalServerError {
			resp.Body.Close()
			lastErr = fmt.Errorf("microsoft JWKS returned %d", resp.StatusCode)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("microsoft JWKS returned unexpected status %d", resp.StatusCode)
		}

		err = json.NewDecoder(resp.Body).Decode(&jwks)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to parse microsoft JWKS: %w", err)
		}

		lastErr = nil
		break
	}

	if lastErr != nil {
		return nil, fmt.Errorf("microsoft JWKS fetch failed after %d attempts: %w", msRetryMax, lastErr)
	}

	s.jwksCache.set(&jwks)
	return &jwks, nil
}
