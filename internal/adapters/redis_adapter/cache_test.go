package redis_a_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	redis_a "github.com/ammerola/resell-be/internal/adapters/redis_adapter"
	"github.com/ammerola/resell-be/test/helpers"
)

func TestCache_SetAndGet(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	tests := []struct {
		name      string
		key       string
		value     interface{}
		wantError bool
	}{
		{
			name:  "stores_and_retrieves_string",
			key:   "test:string",
			value: "test value",
		},
		{
			name: "stores_and_retrieves_struct",
			key:  "test:struct",
			value: struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}{ID: "123", Name: "Test"},
		},
		{
			name:  "stores_and_retrieves_slice",
			key:   "test:slice",
			value: []string{"item1", "item2", "item3"},
		},
		{
			name: "stores_and_retrieves_map",
			key:  "test:map",
			value: map[string]interface{}{
				"field1": "value1",
				"field2": 123,
				"field3": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set value
			err := cache.Set(ctx, tt.key, tt.value)
			require.NoError(t, err)

			// Get value
			var result interface{}
			if _, ok := tt.value.(string); ok {
				var strResult string
				err = cache.Get(ctx, tt.key, &strResult)
				result = strResult
			} else if _, ok := tt.value.([]string); ok {
				var sliceResult []string
				err = cache.Get(ctx, tt.key, &sliceResult)
				result = sliceResult
			} else {
				// For complex types, unmarshal to json.RawMessage first
				var jsonResult json.RawMessage
				err = cache.Get(ctx, tt.key, &jsonResult)
				require.NoError(t, err)

				expectedJSON, _ := json.Marshal(tt.value)
				assert.JSONEq(t, string(expectedJSON), string(jsonResult))
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.value, result)
		})
	}
}

func TestCache_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	// Set with short TTL
	err := cache.SetWithTTL(ctx, "ttl:test", "value", 100*time.Millisecond)
	require.NoError(t, err)

	// Verify it exists
	var result string
	err = cache.Get(ctx, "ttl:test", &result)
	require.NoError(t, err)
	assert.Equal(t, "value", result)

	// Fast forward time in miniredis
	mr.FastForward(200 * time.Millisecond)

	// Should be expired
	err = cache.Get(ctx, "ttl:test", &result)
	assert.Equal(t, redis_a.ErrCacheMiss, err)
}

func TestCache_Delete(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	// Set multiple keys
	keys := []string{"del:1", "del:2", "del:3"}
	for _, key := range keys {
		err := cache.Set(ctx, key, "value")
		require.NoError(t, err)
	}

	// Delete keys
	err := cache.Delete(ctx, keys...)
	require.NoError(t, err)

	// Verify all deleted
	for _, key := range keys {
		var result string
		err := cache.Get(ctx, key, &result)
		assert.Equal(t, redis_a.ErrCacheMiss, err)
	}
}

func TestCache_DeletePattern(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	// Set keys with pattern
	keysToDelete := []string{"pattern:1", "pattern:2", "pattern:3"}
	keysToKeep := []string{"other:1", "different:2"}

	for _, key := range append(keysToDelete, keysToKeep...) {
		err := cache.Set(ctx, key, "value")
		require.NoError(t, err)
	}

	// Delete by pattern
	err := cache.DeletePattern(ctx, "pattern:*")
	require.NoError(t, err)

	// Verify pattern keys deleted
	for _, key := range keysToDelete {
		var result string
		err := cache.Get(ctx, key, &result)
		assert.Equal(t, redis_a.ErrCacheMiss, err)
	}

	// Verify other keys still exist
	for _, key := range keysToKeep {
		var result string
		err := cache.Get(ctx, key, &result)
		require.NoError(t, err)
		assert.Equal(t, "value", result)
	}
}

func TestCache_GetOrSet(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	fetchCount := 0
	fetchFunc := func() (interface{}, error) {
		fetchCount++
		return "fetched value", nil
	}

	// First call should fetch
	var result1 string
	err := cache.GetOrSet(ctx, "getorset:test", &result1, fetchFunc, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "fetched value", result1)
	assert.Equal(t, 1, fetchCount)

	// Second call should get from cache
	var result2 string
	err = cache.GetOrSet(ctx, "getorset:test", &result2, fetchFunc, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, "fetched value", result2)
	assert.Equal(t, 1, fetchCount) // Should not increment
}

func TestCache_IncrementOperations(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	// Test Increment
	val, err := cache.Increment(ctx, "counter:test")
	require.NoError(t, err)
	assert.Equal(t, int64(1), val)

	val, err = cache.Increment(ctx, "counter:test")
	require.NoError(t, err)
	assert.Equal(t, int64(2), val)

	// Test IncrementBy
	val, err = cache.IncrementBy(ctx, "counter:test", 5)
	require.NoError(t, err)
	assert.Equal(t, int64(7), val)

	val, err = cache.IncrementBy(ctx, "counter:test", -2)
	require.NoError(t, err)
	assert.Equal(t, int64(5), val)
}

func TestCache_SetNX(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())

	// First SetNX should succeed
	ok, err := cache.SetNX(ctx, "setnx:test", "first", time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)

	// Second SetNX should fail
	ok, err = cache.SetNX(ctx, "setnx:test", "second", time.Minute)
	require.NoError(t, err)
	assert.False(t, ok)

	// Verify value is still "first"
	var result string
	err = cache.Get(ctx, "setnx:test", &result)
	require.NoError(t, err)
	assert.Equal(t, "first", result)
}

func TestCacheManager_InvalidateInventoryCache(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache := redis_a.NewCache(client, 5*time.Minute, helpers.TestLogger())
	manager := redis_a.NewCacheManager(cache, helpers.TestLogger())

	// Set various cache keys
	lotID := "test-lot-123"
	keys := map[string]string{
		"inv:test-lot-123:details": "inventory details",
		"inv:list:page1":           "inventory list",
		"dash:summary":             "dashboard data",
		"analytics:monthly":        "analytics data",
		"other:data":               "should not be deleted",
	}

	for key, value := range keys {
		err := cache.Set(ctx, key, value)
		require.NoError(t, err)
	}

	// Invalidate inventory cache
	err := manager.InvalidateInventoryCache(ctx, lotID)
	require.NoError(t, err)

	// Verify related keys are invalidated
	invalidated := []string{"inv:test-lot-123:details", "inv:list:page1", "dash:summary", "analytics:monthly"}
	for _, key := range invalidated {
		var result string
		err := cache.Get(ctx, key, &result)
		assert.Equal(t, redis_a.ErrCacheMiss, err, "Key should be invalidated: %s", key)
	}

	// Verify unrelated keys still exist
	var otherResult string
	err = cache.Get(ctx, "other:data", &otherResult)
	require.NoError(t, err)
	assert.Equal(t, "should not be deleted", otherResult)
}

func TestCache_BuildKey(t *testing.T) {
	tests := []struct {
		name     string
		prefix   redis_a.CacheKeyPrefix
		parts    []string
		expected string
	}{
		{
			name:     "inventory_key",
			prefix:   redis_a.PrefixInventory,
			parts:    []string{"123", "details"},
			expected: "inv:123:details",
		},
		{
			name:     "dashboard_key",
			prefix:   redis_a.PrefixDashboard,
			parts:    []string{"summary", "2024"},
			expected: "dash:summary:2024",
		},
		{
			name:     "single_part",
			prefix:   redis_a.PrefixSearch,
			parts:    []string{"query"},
			expected: "search:query",
		},
		{
			name:     "no_parts",
			prefix:   redis_a.PrefixSession,
			parts:    []string{},
			expected: "session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redis_a.BuildKey(tt.prefix, tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
