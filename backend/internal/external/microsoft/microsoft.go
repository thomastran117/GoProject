package microsoft

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

	"backend/internal/application/middleware"

	jwt "github.com/golang-jwt/jwt/v5"
)

const msJWKSURL = "https://login.microsoftonline.com/common/discovery/v2.0/keys"
const msIssuerPrefix = "https://login.microsoftonline.com/"
const msIssuerSuffix = "/v2.0"
const msRetryMax = 3
const msRetryBase = 100 * time.Millisecond
const msRetryMaxDelay = 1 * time.Second

// verifyTimeout caps the total time spent verifying a single token,
// including JWKS fetch and all retry attempts.
const verifyTimeout = 15 * time.Second

// jwksCacheTTL controls how long Microsoft's public keys are cached before
// a background refresh is triggered. Key rotation forces an earlier refresh.
const jwksCacheTTL = 1 * time.Hour

// jwksCache holds Microsoft's public keys with a TTL so we don't refetch
// on every request. Guarded by a RWMutex for safe concurrent access.
type jwksCache struct {
	mu        sync.RWMutex
	keys      *jwkSet
	expiresAt time.Time
}

func (c *jwksCache) get() *jwkSet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.keys != nil && time.Now().Before(c.expiresAt) {
		return c.keys
	}
	return nil
}

func (c *jwksCache) set(keys *jwkSet) {
	c.mu.Lock()
	c.keys = keys
	c.expiresAt = time.Now().Add(jwksCacheTTL)
	c.mu.Unlock()
}

func (c *jwksCache) invalidate() {
	c.mu.Lock()
	c.expiresAt = time.Time{}
	c.mu.Unlock()
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string   `json:"kid"`
	X5C []string `json:"x5c"`
}

// publicKeyForKid finds the JWK matching kid and extracts its RSA public key
// from the first X.509 certificate in the x5c chain.
func (ks *jwkSet) publicKeyForKid(kid string) (*rsa.PublicKey, error) {
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

// Claims holds the validated claims from a Microsoft ID token.
type Claims struct {
	jwt.RegisteredClaims
	OID               string `json:"oid"`
	Email             string `json:"email"`
	PreferredUsername string `json:"preferred_username"`
	TenantID          string `json:"tid"`
}

var errInvalidToken = &middleware.APIError{
	Status:  http.StatusUnauthorized,
	Code:    "INVALID_MICROSOFT_TOKEN",
	Message: "Microsoft token is invalid or expired",
}

// Client verifies Microsoft ID tokens using JWKS from Microsoft's discovery endpoint.
type Client struct {
	httpClient *http.Client
	clientID   string
	cache      jwksCache
}

// NewClient creates a Client. If clientID is non-empty, the aud claim of every
// verified token must contain it.
func NewClient(httpClient *http.Client, clientID string) *Client {
	return &Client{httpClient: httpClient, clientID: clientID}
}

// VerifyIDToken validates a Microsoft ID token by fetching the JWKS,
// verifying the JWT signature, and checking all required claims.
// Transient errors (network failures, 429, 5xx) are retried with exponential
// backoff. Permanent errors are returned immediately.
// A hard verifyTimeout is applied over the caller's context so the total
// verification time (including JWKS fetch and all retries) is always bounded.
func (c *Client) VerifyIDToken(ctx context.Context, idToken string) (*Claims, error) {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	// Parse unverified first to extract the key ID from the JWT header.
	unverified, _, err := jwt.NewParser().ParseUnverified(idToken, &Claims{})
	if err != nil {
		return nil, errInvalidToken
	}
	kid, ok := unverified.Header["kid"].(string)
	if !ok || kid == "" {
		return nil, errInvalidToken
	}

	keys, err := c.fetchJWKS(ctx)
	if err != nil {
		return nil, err
	}

	pubKey, err := keys.publicKeyForKid(kid)
	if err != nil {
		// kid missing — likely a key rotation; invalidate the cache and retry once.
		c.cache.invalidate()
		keys, err = c.fetchJWKS(ctx)
		if err != nil {
			return nil, err
		}
		pubKey, err = keys.publicKeyForKid(kid)
		if err != nil {
			return nil, errInvalidToken
		}
	}

	var claims Claims
	_, err = jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithExpirationRequired(),
	).ParseWithClaims(idToken, &claims, func(_ *jwt.Token) (any, error) {
		return pubKey, nil
	})
	if err != nil {
		return nil, errInvalidToken
	}

	// Validate issuer format: https://login.microsoftonline.com/{tenantId}/v2.0
	if !strings.HasPrefix(claims.Issuer, msIssuerPrefix) || !strings.HasSuffix(claims.Issuer, msIssuerSuffix) {
		return nil, errInvalidToken
	}

	// Microsoft tokens can carry multiple audiences (e.g. resource URIs alongside
	// the client ID). Always require a non-empty audience, and when a client ID is
	// configured verify it is present in the list.
	if len(claims.Audience) == 0 {
		return nil, errInvalidToken
	}
	if c.clientID != "" && !slices.Contains([]string(claims.Audience), c.clientID) {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_MICROSOFT_TOKEN",
			Message: "Microsoft token audience mismatch",
		}
	}

	if claims.OID == "" {
		return nil, errInvalidToken
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

// fetchJWKS returns Microsoft's public key set, serving from the in-memory
// cache when valid and fetching fresh keys (with retry/backoff) otherwise.
func (c *Client) fetchJWKS(ctx context.Context) (*jwkSet, error) {
	if cached := c.cache.get(); cached != nil {
		return cached, nil
	}

	var (
		keys    jwkSet
		lastErr error
	)

	for attempt := 0; attempt < msRetryMax; attempt++ {
		if attempt > 0 {
			delay := msRetryBase << uint(attempt-1)
			if delay > msRetryMaxDelay {
				delay = msRetryMaxDelay
			}
			// Non-cryptographic jitter; spreads retries to avoid thundering herd.
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

		resp, err := c.httpClient.Do(req)
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

		err = json.NewDecoder(resp.Body).Decode(&keys)
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

	c.cache.set(&keys)
	return &keys, nil
}
