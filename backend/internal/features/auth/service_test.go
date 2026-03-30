package auth

import (
	"context"
	"testing"

	"backend/internal/application/middleware"
	"backend/internal/features/cache"
	"backend/internal/features/token"

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
	token.Init("test-secret", cache.NewService(client))
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
}

func newTestService() *Service {
	return &Service{} // repo is nil — only used for methods that don't touch the DB
}

func newTestServiceWithSchoolFn(fn func(ctx context.Context, id uint64) (bool, error)) *Service {
	return &Service{skipTurnstile: true, schoolExists: fn}
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
	if ttl != token.RefreshTTLRememberMe {
		t.Errorf("expected %v, got %v", token.RefreshTTLRememberMe, ttl)
	}
}

func TestRefreshTTLFor_Default(t *testing.T) {
	ttl := refreshTTLFor(false)
	if ttl != token.RefreshTTLDefault {
		t.Errorf("expected %v, got %v", token.RefreshTTLDefault, ttl)
	}
}

// --- Logout ---

func TestLogout_RevokesRefreshToken(t *testing.T) {
	initTokenService(t)
	ctx := context.Background()
	svc := newTestService()

	pair, err := token.GeneratePair(ctx, 1, "user@example.com", "user", token.RefreshTTLDefault)
	if err != nil {
		t.Fatalf("GeneratePair: %v", err)
	}

	if err := svc.Logout(ctx, pair.RefreshToken); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Token should be invalid after logout.
	_, err = token.ValidateRefresh(ctx, pair.RefreshToken)
	if err == nil {
		t.Error("expected refresh token to be invalid after logout")
	}
}

// --- Signup school validation ---

func TestSignup_TeacherMissingSchoolID(t *testing.T) {
	svc := newTestServiceWithSchoolFn(nil)
	_, err := svc.Signup(context.Background(), "teacher@example.com", "Str0ng!Pass", "", RoleTeacher, nil, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *middleware.APIError, got %T", err)
	}
	if apiErr.Code != "SCHOOL_REQUIRED" {
		t.Errorf("expected code SCHOOL_REQUIRED, got %s", apiErr.Code)
	}
}

func TestSignup_TeacherSchoolNotFound(t *testing.T) {
	svc := newTestServiceWithSchoolFn(func(_ context.Context, _ uint64) (bool, error) {
		return false, nil
	})
	id := uint64(99)
	_, err := svc.Signup(context.Background(), "teacher@example.com", "Str0ng!Pass", "", RoleTeacher, &id, false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	apiErr, ok := err.(*middleware.APIError)
	if !ok {
		t.Fatalf("expected *middleware.APIError, got %T", err)
	}
	if apiErr.Code != "SCHOOL_NOT_FOUND" {
		t.Errorf("expected code SCHOOL_NOT_FOUND, got %s", apiErr.Code)
	}
}
