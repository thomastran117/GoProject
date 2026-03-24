package token

import (
	"context"
	"testing"
	"time"

	"backend/internal/app/core/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

func setupTest(t *testing.T) func() {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	Init("test-secret", cache.NewService(client))

	return func() {
		client.Close()
		mr.Close()
	}
}

func TestGeneratePair_ReturnsNonEmptyTokens(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	pair, err := GeneratePair(context.Background(), 1, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pair.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if pair.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
}

func TestGeneratePair_DifferentRefreshTokensEachCall(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	p1, err := GeneratePair(ctx, 1, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p2, err := GeneratePair(ctx, 1, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p1.RefreshToken == p2.RefreshToken {
		t.Error("expected unique refresh tokens on each call")
	}
}

func TestValidateAccess_ValidToken(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	pair, err := GeneratePair(context.Background(), 42, "alice@example.com", "admin", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := ValidateAccess(pair.AccessToken)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("expected UserID 42, got %d", claims.UserID)
	}
	if claims.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", claims.Email)
	}
	if claims.Role != "admin" {
		t.Errorf("expected role admin, got %s", claims.Role)
	}
}

func TestValidateAccess_InvalidToken(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	_, err := ValidateAccess("not.a.valid.token")
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

func TestValidateAccess_TamperedToken(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	pair, err := GeneratePair(context.Background(), 1, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tampered := pair.AccessToken + "x"
	_, err = ValidateAccess(tampered)
	if err == nil {
		t.Error("expected error for tampered token, got nil")
	}
}

func TestValidateAccess_WrongSigningKey(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Sign a token with a different secret.
	claims := AccessClaims{
		UserID: 1,
		Email:  "user@example.com",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(accessTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("wrong-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	_, err = ValidateAccess(tokenStr)
	if err == nil {
		t.Error("expected error for token signed with wrong key, got nil")
	}
}

func TestValidateRefresh_Valid(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	pair, err := GeneratePair(ctx, 99, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	userID, err := ValidateRefresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if userID != 99 {
		t.Errorf("expected userID 99, got %d", userID)
	}
}

func TestValidateRefresh_UnknownToken(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	_, err := ValidateRefresh(context.Background(), "non-existent-token")
	if err == nil {
		t.Error("expected error for unknown refresh token, got nil")
	}
}

func TestRevokeRefresh_PreventsSubsequentValidation(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	ctx := context.Background()
	pair, err := GeneratePair(ctx, 7, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := RevokeRefresh(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("unexpected error revoking token: %v", err)
	}

	_, err = ValidateRefresh(ctx, pair.RefreshToken)
	if err == nil {
		t.Error("expected error after revocation, got nil")
	}
}

func TestRevokeRefresh_NonExistentTokenIsNoOp(t *testing.T) {
	cleanup := setupTest(t)
	defer cleanup()

	// Revoking a non-existent token should not return an error.
	if err := RevokeRefresh(context.Background(), "ghost-token"); err != nil {
		t.Errorf("expected no error revoking non-existent token, got: %v", err)
	}
}

func TestGeneratePair_RememberMeTTLLongerThanDefault(t *testing.T) {
	if RefreshTTLRememberMe <= RefreshTTLDefault {
		t.Error("expected RefreshTTLRememberMe to be greater than RefreshTTLDefault")
	}
}
