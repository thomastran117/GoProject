package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTest(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})
	return NewService(client), mr
}

// --- Set / Get ---

func TestSetAndGet(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	if err := svc.Set(ctx, "key", "value", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := svc.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "value" {
		t.Errorf("expected 'value', got %q", got)
	}
}

func TestGet_MissingKeyReturnsRedisNil(t *testing.T) {
	svc, _ := setupTest(t)

	_, err := svc.Get(context.Background(), "missing")
	if err != redis.Nil {
		t.Errorf("expected redis.Nil, got %v", err)
	}
}

func TestSet_KeyExpiresAfterTTL(t *testing.T) {
	svc, mr := setupTest(t)
	ctx := context.Background()

	if err := svc.Set(ctx, "expiring", "val", 1*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	mr.FastForward(2 * time.Second)

	_, err := svc.Get(ctx, "expiring")
	if err != redis.Nil {
		t.Errorf("expected key to have expired (redis.Nil), got %v", err)
	}
}

// --- Delete ---

func TestDelete_RemovesKey(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "k", "v", 0)
	if err := svc.Delete(ctx, "k"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := svc.Get(ctx, "k")
	if err != redis.Nil {
		t.Errorf("expected key to be gone, got %v", err)
	}
}

func TestDelete_MissingKeyIsNoOp(t *testing.T) {
	svc, _ := setupTest(t)
	if err := svc.Delete(context.Background(), "ghost"); err != nil {
		t.Errorf("expected no error deleting missing key, got %v", err)
	}
}

// --- Exists ---

func TestExists_PresentKey(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "present", "yes", 0)
	ok, err := svc.Exists(ctx, "present")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !ok {
		t.Error("expected key to exist")
	}
}

func TestExists_MissingKey(t *testing.T) {
	svc, _ := setupTest(t)
	ok, err := svc.Exists(context.Background(), "absent")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if ok {
		t.Error("expected key to not exist")
	}
}

// --- Expire / TTL ---

func TestExpireAndTTL(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "k", "v", 0)
	if err := svc.Expire(ctx, "k", 10*time.Second); err != nil {
		t.Fatalf("Expire: %v", err)
	}

	ttl, err := svc.TTL(ctx, "k")
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > 10*time.Second {
		t.Errorf("expected TTL between 0 and 10s, got %v", ttl)
	}
}

func TestTTL_NoPersistentKey(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "forever", "v", 0)
	ttl, err := svc.TTL(ctx, "forever")
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl != -1 {
		t.Errorf("expected TTL -1 for persistent key, got %v", ttl)
	}
}

// --- Increment ---

func TestIncrement(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	v1, err := svc.Increment(ctx, "counter")
	if err != nil {
		t.Fatalf("Increment: %v", err)
	}
	if v1 != 1 {
		t.Errorf("expected 1, got %d", v1)
	}

	v2, _ := svc.Increment(ctx, "counter")
	if v2 != 2 {
		t.Errorf("expected 2, got %d", v2)
	}
}

func TestIncrementBy(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	v, err := svc.IncrementBy(ctx, "counter", 5)
	if err != nil {
		t.Fatalf("IncrementBy: %v", err)
	}
	if v != 5 {
		t.Errorf("expected 5, got %d", v)
	}

	v2, _ := svc.IncrementBy(ctx, "counter", 3)
	if v2 != 8 {
		t.Errorf("expected 8, got %d", v2)
	}
}

// --- SetJSON / GetJSON ---

type testPayload struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestSetJSONAndGetJSON(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	in := testPayload{Name: "Alice", Age: 30}
	if err := svc.SetJSON(ctx, "obj", in, 0); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}

	var out testPayload
	if err := svc.GetJSON(ctx, "obj", &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out != in {
		t.Errorf("expected %+v, got %+v", in, out)
	}
}

func TestGetJSON_MissingKeyReturnsRedisNil(t *testing.T) {
	svc, _ := setupTest(t)
	var out testPayload
	err := svc.GetJSON(context.Background(), "missing", &out)
	if err != redis.Nil {
		t.Errorf("expected redis.Nil, got %v", err)
	}
}

// --- GetOrSet ---

func TestGetOrSet_CacheMiss_CallsFnAndStores(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	called := 0
	val, err := svc.GetOrSet(ctx, "k", time.Minute, func() (string, error) {
		called++
		return "computed", nil
	})
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if val != "computed" {
		t.Errorf("expected 'computed', got %q", val)
	}
	if called != 1 {
		t.Errorf("expected fn called once, got %d", called)
	}
}

func TestGetOrSet_CacheHit_DoesNotCallFn(t *testing.T) {
	svc, _ := setupTest(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "k", "cached", 0)

	called := 0
	val, err := svc.GetOrSet(ctx, "k", time.Minute, func() (string, error) {
		called++
		return "computed", nil
	})
	if err != nil {
		t.Fatalf("GetOrSet: %v", err)
	}
	if val != "cached" {
		t.Errorf("expected 'cached', got %q", val)
	}
	if called != 0 {
		t.Errorf("expected fn not called, got %d calls", called)
	}
}
