// internal/adapters/redis/cache.go
package redis_a

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/redis/go-redis/v9"
)

// CacheKeyPrefix defines prefixes for different cache types
type CacheKeyPrefix string

const (
	PrefixInventory CacheKeyPrefix = "inv"
	PrefixDashboard CacheKeyPrefix = "dash"
	PrefixAnalytics CacheKeyPrefix = "analytics"
	PrefixSearch    CacheKeyPrefix = "search"
	PrefixExport    CacheKeyPrefix = "export"
	PrefixSession   CacheKeyPrefix = "session"
)

// Cache provides caching functionality with Redis
type Cache struct {
	client *redis.Client
	ttl    time.Duration
	logger *slog.Logger
}

// Statically assert that *Cache implements the CacheRepository interface.
var _ ports.CacheRepository = (*Cache)(nil)

// NewCache creates a new cache instance
func NewCache(client *redis.Client, ttl time.Duration, logger *slog.Logger) ports.CacheRepository {
	return &Cache{
		client: client,
		ttl:    ttl,
		logger: logger.With(slog.String("component", "cache")),
	}
}

// Set stores a value in cache with default TTL
func (c *Cache) Set(ctx context.Context, key string, value interface{}) error {
	return c.SetWithTTL(ctx, key, value, c.ttl)
}

// SetWithTTL stores a value in cache with custom TTL
func (c *Cache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to marshal cache value",
			slog.String("key", key),
			err)
		return fmt.Errorf("marshal error: %w", err)
	}

	if err := c.client.Set(ctx, key, data, ttl).Err(); err != nil {
		c.logger.ErrorContext(ctx, "failed to set cache",
			slog.String("key", key),
			err)
		return fmt.Errorf("redis set error: %w", err)
	}

	c.logger.DebugContext(ctx, "cache set",
		slog.String("key", key),
		slog.Duration("ttl", ttl))

	return nil
}

// Get retrieves a value from cache
func (c *Cache) Get(ctx context.Context, key string, dest interface{}) error {
	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			// Cache miss is not an error
			c.logger.DebugContext(ctx, "cache miss", slog.String("key", key))
			return ErrCacheMiss
		}
		c.logger.ErrorContext(ctx, "failed to get cache",
			slog.String("key", key),
			err)
		return fmt.Errorf("redis get error: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		c.logger.ErrorContext(ctx, "failed to unmarshal cache value",
			slog.String("key", key),
			err)
		return fmt.Errorf("unmarshal error: %w", err)
	}

	c.logger.DebugContext(ctx, "cache hit", slog.String("key", key))
	return nil
}

// Delete removes a key from cache
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		c.logger.ErrorContext(ctx, "failed to delete cache",
			slog.Any("keys", keys),
			err)
		return fmt.Errorf("redis del error: %w", err)
	}

	c.logger.DebugContext(ctx, "cache deleted", slog.Any("keys", keys))
	return nil
}

// DeletePattern removes all keys matching a pattern
func (c *Cache) DeletePattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		c.logger.ErrorContext(ctx, "failed to scan keys",
			slog.String("pattern", pattern),
			err)
		return fmt.Errorf("redis scan error: %w", err)
	}

	if len(keys) > 0 {
		return c.Delete(ctx, keys...)
	}

	return nil
}

// Exists checks if keys exist
func (c *Cache) Exists(ctx context.Context, keys ...string) (bool, error) {
	n, err := c.client.Exists(ctx, keys...).Result()
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to check cache existence",
			slog.Any("keys", keys),
			err)
		return false, fmt.Errorf("redis exists error: %w", err)
	}

	return n == int64(len(keys)), nil
}

// Expire sets a new expiration for a key
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	if err := c.client.Expire(ctx, key, ttl).Err(); err != nil {
		c.logger.ErrorContext(ctx, "failed to set expiration",
			slog.String("key", key),
			slog.Duration("ttl", ttl),
			err)
		return fmt.Errorf("redis expire error: %w", err)
	}

	return nil
}

// GetOrSet retrieves from cache or sets if not found
func (c *Cache) GetOrSet(ctx context.Context, key string, dest interface{},
	fetch func() (interface{}, error), ttl time.Duration) error {

	// Try to get from cache first
	err := c.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != ErrCacheMiss {
		return err // Actual error
	}

	// Cache miss - fetch and store
	value, err := fetch()
	if err != nil {
		return fmt.Errorf("fetch error: %w", err)
	}

	// Store in cache
	if err := c.SetWithTTL(ctx, key, value, ttl); err != nil {
		// Log but don't fail if cache write fails
		c.logger.WarnContext(ctx, "failed to cache value after fetch",
			slog.String("key", key),
			err)
	}

	// Copy value to destination
	data, _ := json.Marshal(value)
	json.Unmarshal(data, dest)

	return nil
}

// Increment increments a counter
func (c *Cache) Increment(ctx context.Context, key string) (int64, error) {
	val, err := c.client.Incr(ctx, key).Result()
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to increment counter",
			slog.String("key", key),
			err)
		return 0, fmt.Errorf("redis incr error: %w", err)
	}

	return val, nil
}

// IncrementBy increments a counter by a specific amount
func (c *Cache) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	val, err := c.client.IncrBy(ctx, key, value).Result()
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to increment counter by value",
			slog.String("key", key),
			slog.Int64("value", value),
			err)
		return 0, fmt.Errorf("redis incrby error: %w", err)
	}

	return val, nil
}

// SetNX sets a key only if it doesn't exist (useful for locks)
func (c *Cache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, fmt.Errorf("marshal error: %w", err)
	}

	ok, err := c.client.SetNX(ctx, key, data, ttl).Result()
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to setnx",
			slog.String("key", key),
			err)
		return false, fmt.Errorf("redis setnx error: %w", err)
	}

	return ok, nil
}

// TTL returns the time to live for a key
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to get TTL",
			slog.String("key", key),
			err)
		return 0, fmt.Errorf("redis ttl error: %w", err)
	}

	return ttl, nil
}

// Flush removes all keys from the current database
func (c *Cache) Flush(ctx context.Context) error {
	if err := c.client.FlushDB(ctx).Err(); err != nil {
		c.logger.ErrorContext(ctx, "failed to flush database", err)
		return fmt.Errorf("redis flushdb error: %w", err)
	}

	c.logger.WarnContext(ctx, "cache flushed")
	return nil
}

// Ping checks if Redis is accessible
func (c *Cache) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		c.logger.ErrorContext(ctx, "redis ping failed", err)
		return fmt.Errorf("redis ping error: %w", err)
	}

	return nil
}

// BuildKey creates a cache key with prefix
func BuildKey(prefix CacheKeyPrefix, parts ...string) string {
	key := string(prefix)
	for _, part := range parts {
		key += ":" + part
	}
	return key
}

// CacheError represents cache-specific errors
type CacheError struct {
	Op  string
	Key string
	Err error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("cache %s operation failed for key %s: %v", e.Op, e.Key, e.Err)
}

// ErrCacheMiss is returned when a key is not found in cache
var ErrCacheMiss = fmt.Errorf("cache miss")

// CacheStats holds cache statistics
type CacheStats struct {
	Hits      int64     `json:"hits"`
	Misses    int64     `json:"misses"`
	Sets      int64     `json:"sets"`
	Deletes   int64     `json:"deletes"`
	HitRate   float64   `json:"hit_rate"`
	LastReset time.Time `json:"last_reset"`
}

// CacheManager provides advanced cache management
type CacheManager struct {
	cache  ports.CacheRepository
	stats  *CacheStats
	logger *slog.Logger
}

// NewCacheManager creates a new cache manager
func NewCacheManager(cache ports.CacheRepository, logger *slog.Logger) *CacheManager {
	return &CacheManager{
		cache:  cache,
		stats:  &CacheStats{LastReset: time.Now()},
		logger: logger,
	}
}

// InvalidateInventoryCache invalidates all inventory-related cache entries
func (m *CacheManager) InvalidateInventoryCache(ctx context.Context, lotID string) error {
	patterns := []string{
		fmt.Sprintf("%s:*%s*", PrefixInventory, lotID),
		fmt.Sprintf("%s:*", PrefixDashboard),
		fmt.Sprintf("%s:*", PrefixAnalytics),
	}

	for _, pattern := range patterns {
		if err := m.cache.DeletePattern(ctx, pattern); err != nil {
			m.logger.WarnContext(ctx, "failed to invalidate cache pattern",
				slog.String("pattern", pattern),
				err)
		}
	}

	return nil
}

// WarmupCache pre-loads frequently accessed data
func (m *CacheManager) WarmupCache(ctx context.Context) error {
	m.logger.InfoContext(ctx, "warming up cache")

	// This would be implemented based on your specific warmup needs
	// For example, loading dashboard data, popular searches, etc.

	return nil
}

// GetStats returns cache statistics
func (m *CacheManager) GetStats() *CacheStats {
	if m.stats.Hits+m.stats.Misses > 0 {
		m.stats.HitRate = float64(m.stats.Hits) / float64(m.stats.Hits+m.stats.Misses)
	}
	return m.stats
}

// ResetStats resets cache statistics
func (m *CacheManager) ResetStats() {
	m.stats = &CacheStats{LastReset: time.Now()}
}
