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

	"backend/internal/app/core/token"
	"backend/internal/config/middleware"

	jwt "github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type AuthResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	User         UserData `json:"user"`
}

type UserData struct {
	ID    uint64 `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

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

type Service struct {
	repo              *Repository
	googleClientID    string
	microsoftClientID string
	httpClient        *http.Client
	jwksCache         msJWKSCache
}

func NewService(repo *Repository, googleClientID, microsoftClientID string) *Service {
	return &Service{
		repo:              repo,
		googleClientID:    googleClientID,
		microsoftClientID: microsoftClientID,
		// Shared across all OAuth requests. The 10-second timeout is a hard
		// cap; per-request context deadlines still take precedence.
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// --- public interface ---

func (s *Service) Login(ctx context.Context, email, password string) (*AuthResponse, error) {
	user, err := s.repo.FindByEmail(email)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid email or password",
		}
	}

	if err := s.ComparePassword(user.PasswordHash, password); err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_CREDENTIALS",
			Message: "Invalid email or password",
		}
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

func (s *Service) Signup(ctx context.Context, email, password, role string) (*AuthResponse, error) {
	existing, err := s.repo.FindByEmail(email)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusConflict,
			Code:    "USER_EXISTS",
			Message: "An account with this email already exists",
		}
	}

	hash, err := s.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.Create(email, string(hash), role)
	if err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Refresh validates the given refresh token, rotates it (revoke + issue new),
// and returns a fresh token pair.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, error) {
	userID, err := token.ValidateRefresh(ctx, refreshToken)
	if err != nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "INVALID_REFRESH_TOKEN",
			Message: "Refresh token is invalid or has expired",
		}
	}

	user, err := s.repo.FindByID(userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, &middleware.APIError{
			Status:  http.StatusUnauthorized,
			Code:    "USER_NOT_FOUND",
			Message: "User no longer exists",
		}
	}

	if err := token.RevokeRefresh(ctx, refreshToken); err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// Logout revokes the refresh token, invalidating the session.
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	return token.RevokeRefresh(ctx, refreshToken)
}

func (s *Service) AppleAuthenticate(ctx context.Context, t string) (*AuthResponse, error) {
	return nil, nil
}

func (s *Service) MicrosoftAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	claims, err := s.verifyMicrosoftIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}

	user, err := s.repo.FindOrCreateByMicrosoftID(claims.OID, email)
	if err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

func (s *Service) GoogleAuthenticate(ctx context.Context, idToken string) (*AuthResponse, error) {
	claims, err := s.verifyGoogleIDToken(ctx, idToken)
	if err != nil {
		return nil, err
	}

	user, err := s.repo.FindOrCreateByGoogleID(claims.Sub, claims.Email)
	if err != nil {
		return nil, err
	}

	pair, err := token.GeneratePair(ctx, user.ID, user.Email, user.Role)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		User:         UserData{ID: user.ID, Email: user.Email, Role: user.Role},
	}, nil
}

// --- private helpers ---

// HashPassword generates a bcrypt hash from the given plaintext password.
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword compares a plaintext password against a stored hash.
func (s *Service) ComparePassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"
const googleIssuer1 = "accounts.google.com"
const googleIssuer2 = "https://accounts.google.com"

const googleRetryMax = 3
const googleRetryBase = 100 * time.Millisecond
const googleRetryMaxDelay = 1 * time.Second

// oauthVerifyTimeout caps the total time spent verifying a single OAuth token,
// including all retry attempts. Applied on top of any caller-supplied deadline.
const oauthVerifyTimeout = 15 * time.Second

// jwksCacheTTL controls how long Microsoft's public keys are cached before
// a background refresh is triggered. Key rotation forces an earlier refresh.
const jwksCacheTTL = 1 * time.Hour

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

// --- Microsoft OAuth ---

const msJWKSURL = "https://login.microsoftonline.com/common/discovery/v2.0/keys"
const msIssuerPrefix = "https://login.microsoftonline.com/"
const msIssuerSuffix = "/v2.0"
const msRetryMax = 3
const msRetryBase = 100 * time.Millisecond
const msRetryMaxDelay = 1 * time.Second

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
