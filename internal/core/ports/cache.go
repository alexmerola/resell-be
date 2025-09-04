// internal/core/ports/cache.go
package ports

import (
	"context"
	"time"
)

// CacheRepository defines the interface for cache operations
type CacheRepository interface {
	// Basic operations
	Set(ctx context.Context, key string, value interface{}) error
	SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string, dest interface{}) error
	Delete(ctx context.Context, keys ...string) error
	DeletePattern(ctx context.Context, pattern string) error
	Exists(ctx context.Context, keys ...string) (bool, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// Advanced operations
	GetOrSet(ctx context.Context, key string, dest interface{},
		fetch func() (interface{}, error), ttl time.Duration) error

	// Counter operations
	Increment(ctx context.Context, key string) (int64, error)
	IncrementBy(ctx context.Context, key string, value int64) (int64, error)

	// Conditional operations
	SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error)

	// Utility operations
	TTL(ctx context.Context, key string) (time.Duration, error)
	Flush(ctx context.Context) error
	Ping(ctx context.Context) error
}
