package auth

import (
	"context"
	"testing"

	"backend/internal/features/cache"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

func initTokenService(t *testing.T) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	Init("test-secret", cache.NewService(client))
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
}

func newTestService() *Service {
	return &Service{} // repo is nil — only used for methods that don't touch the DB
}

// --- HashPassword / ComparePassword ---

func TestHashPassword_ProducesValidBcryptHash(t *testing.T) {
	svc := newTestService()
	hash, err := svc.HashPassword("s3cur3P@ss!")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("s3cur3P@ss!")); err != nil {
		t.Errorf("produced hash does not verify: %v", err)
	}
}

func TestHashPassword_DifferentHashEachCall(t *testing.T) {
	svc := newTestService()
	h1, _ := svc.HashPassword("password")
	h2, _ := svc.HashPassword("password")
	if h1 == h2 {
		t.Error("expected different hashes per call (bcrypt uses random salt)")
	}
}

func TestComparePassword_Match(t *testing.T) {
	svc := newTestService()
	hash, _ := svc.HashPassword("correct")
	if err := svc.ComparePassword(hash, "correct"); err != nil {
		t.Errorf("expected match, got: %v", err)
	}
}

func TestComparePassword_Mismatch(t *testing.T) {
	svc := newTestService()
	hash, _ := svc.HashPassword("correct")
	if err := svc.ComparePassword(hash, "wrong"); err == nil {
		t.Error("expected error for wrong password, got nil")
	}
}

// --- refreshTTLFor ---

func TestRefreshTTLFor_RememberMe(t *testing.T) {
	ttl := refreshTTLFor(true)
	if ttl != RefreshTTLRememberMe {
		t.Errorf("expected %v, got %v", RefreshTTLRememberMe, ttl)
	}
}

func TestRefreshTTLFor_Default(t *testing.T) {
	ttl := refreshTTLFor(false)
	if ttl != RefreshTTLDefault {
		t.Errorf("expected %v, got %v", RefreshTTLDefault, ttl)
	}
}

// --- Logout ---

func TestLogout_RevokesRefreshToken(t *testing.T) {
	initTokenService(t)
	ctx := context.Background()
	svc := newTestService()

	pair, err := GeneratePair(ctx, 1, "user@example.com", "user", RefreshTTLDefault)
	if err != nil {
		t.Fatalf("GeneratePair: %v", err)
	}

	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Token should be invalid after logout.
	_, err = ValidateRefresh(ctx, pair.RefreshToken)
	if err == nil {
		t.Error("expected refresh token to be invalid after logout")
	}
}
