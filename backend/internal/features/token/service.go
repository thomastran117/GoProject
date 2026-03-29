package token

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"backend/internal/features/cache"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	accessTTL            = 15 * time.Minute
	RefreshTTLRememberMe = 7 * 24 * time.Hour
	RefreshTTLDefault    = 24 * time.Hour
)

type AccessClaims struct {
	UserID uint64 `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

var (
	secret       string
	cacheService *cache.Service
)

func Init(s string, c *cache.Service) {
	secret = s
	cacheService = c
}

// GeneratePair issues a signed JWT access token and an opaque UUID refresh token.
// The refresh token is stored in Redis; it is the sole source of truth for its validity.
// refreshTTL controls how long the refresh token is valid (use RefreshTTLRememberMe or RefreshTTLDefault).
func GeneratePair(ctx context.Context, userID uint64, email, role string, refreshTTL time.Duration) (*TokenPair, error) {
	type accessResult struct {
		token string
		err   error
	}
	type refreshResult struct {
		token string
		err   error
	}

	accessCh := make(chan accessResult, 1)
	refreshCh := make(chan refreshResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				accessCh <- accessResult{"", fmt.Errorf("token: panic in generateAccess: %v", r)}
			}
		}()
		t, err := generateAccess(ctx, userID, email, role)
		accessCh <- accessResult{t, err}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				refreshCh <- refreshResult{"", fmt.Errorf("token: panic in generateRefresh: %v", r)}
			}
		}()
		t, err := generateRefresh(ctx, userID, refreshTTL)
		refreshCh <- refreshResult{t, err}
	}()

	aRes := <-accessCh
	rRes := <-refreshCh

	if aRes.err != nil {
		return nil, aRes.err
	}
	if rRes.err != nil {
		return nil, rRes.err
	}

	return &TokenPair{
		AccessToken:  aRes.token,
		RefreshToken: rRes.token,
	}, nil
}

// ValidateAccess parses and validates an access JWT, returning its claims.
func ValidateAccess(tokenStr string) (*AccessClaims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &AccessClaims{}, keyFunc)
	if err != nil {
		return nil, err
	}

	claims, ok := t.Claims.(*AccessClaims)
	if !ok || !t.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// ValidateRefresh looks up the opaque refresh token in Redis and returns its owner's userID.
func ValidateRefresh(ctx context.Context, token string) (uint64, error) {
	val, err := cacheService.Get(ctx, refreshKey(token))
	if err == redis.Nil {
		return 0, fmt.Errorf("token: refresh token invalid or expired")
	}
	if err != nil {
		return 0, err
	}

	userID, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("token: malformed refresh token payload")
	}
	return userID, nil
}

// RevokeRefresh deletes a refresh token from Redis (single-device logout).
func RevokeRefresh(ctx context.Context, token string) error {
	return cacheService.Delete(ctx, refreshKey(token))
}

// --- private helpers ---

func generateAccess(ctx context.Context, userID uint64, email, role string) (string, error) {
	claims := AccessClaims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

func generateRefresh(ctx context.Context, userID uint64, ttl time.Duration) (string, error) {
	token := uuid.NewString()
	val := strconv.FormatUint(userID, 10)
	if err := cacheService.Set(ctx, refreshKey(token), val, ttl); err != nil {
		return "", err
	}
	return token, nil
}

func keyFunc(t *jwt.Token) (any, error) {
	if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("token: unexpected signing method %v", t.Header["alg"])
	}
	return []byte(secret), nil
}

func refreshKey(token string) string {
	return "refresh:" + token
}
