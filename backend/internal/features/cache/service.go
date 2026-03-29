package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	client *redis.Client
}

func NewService(client *redis.Client) *Service {
	return &Service{client: client}
}

// Set stores a string value under key with the given TTL.
// Pass 0 for ttl to persist without expiry.
func (s *Service) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

// Get retrieves the string value for key.
// Returns redis.Nil if the key does not exist.
func (s *Service) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

// Delete removes one or more keys. Missing keys are ignored.
func (s *Service) Delete(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

// Exists returns true if key is present in the cache.
func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, key).Result()
	return n > 0, err
}

// Expire updates the TTL on an existing key.
func (s *Service) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

// TTL returns the remaining time-to-live for key.
// Returns -1 if the key has no expiry, -2 if the key does not exist.
func (s *Service) TTL(ctx context.Context, key string) (time.Duration, error) {
	return s.client.TTL(ctx, key).Result()
}

// Increment atomically increments the integer stored at key by 1.
// The key is created with value 1 if it does not exist.
func (s *Service) Increment(ctx context.Context, key string) (int64, error) {
	return s.client.Incr(ctx, key).Result()
}

// IncrementBy atomically increments the integer stored at key by n.
func (s *Service) IncrementBy(ctx context.Context, key string, n int64) (int64, error) {
	return s.client.IncrBy(ctx, key, n).Result()
}

// SetJSON marshals value to JSON and stores it under key with the given TTL.
func (s *Service) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, b, ttl).Err()
}

// GetJSON retrieves the value stored at key and unmarshals it into dest.
// Returns redis.Nil if the key does not exist.
func (s *Service) GetJSON(ctx context.Context, key string, dest any) error {
	b, err := s.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

// GetOrSet returns the cached string for key. If the key is missing, fn is
// called to produce the value, which is then stored with ttl before returning.
func (s *Service) GetOrSet(ctx context.Context, key string, ttl time.Duration, fn func() (string, error)) (string, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == nil {
		return val, nil
	}
	if err != redis.Nil {
		return "", err
	}

	val, err = fn()
	if err != nil {
		return "", err
	}

	if setErr := s.client.Set(ctx, key, val, ttl).Err(); setErr != nil {
		return "", setErr
	}
	return val, nil
}

// GetOrSetJSON is like GetOrSet but marshals/unmarshals the value as JSON.
// dest must be a pointer. fn should return a value that can be marshalled.
func (s *Service) GetOrSetJSON(ctx context.Context, key string, ttl time.Duration, dest any, fn func() (any, error)) error {
	err := s.GetJSON(ctx, key, dest)
	if err == nil {
		return nil
	}
	if err != redis.Nil {
		return err
	}

	val, err := fn()
	if err != nil {
		return err
	}

	if setErr := s.SetJSON(ctx, key, val, ttl); setErr != nil {
		return setErr
	}

	b, err := json.Marshal(val)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

// FlushByPattern deletes all keys matching the given glob-style pattern.
// It uses SCAN to avoid blocking the server on large keyspaces.
func (s *Service) FlushByPattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := s.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}
