// internal/handlers/export_test.go
package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redis_a "github.com/ammerola/resell-be/internal/adapters/redis_adapter"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/ammerola/resell-be/internal/handlers"
	"github.com/ammerola/resell-be/test/helpers"
	"github.com/ammerola/resell-be/test/mocks"
)

// MockRows implements pgx.Rows interface for testing
type mockRows struct {
	data   []handlers.ExcelExportRow
	index  int
	closed bool
}

func (m *mockRows) Close() {
	m.closed = true
}

func (m *mockRows) Err() error {
	return nil
}

func (m *mockRows) Next() bool {
	if m.index < len(m.data) {
		m.index++
		return true
	}
	return false
}

func (m *mockRows) Scan(dest ...interface{}) error {
	if m.index == 0 || m.index > len(m.data) {
		return pgx.ErrNoRows
	}
	return nil
}

func (m *mockRows) Values() ([]interface{}, error) {
	return nil, nil
}

func (m *mockRows) RawValues() [][]byte {
	return nil
}

func (m *mockRows) Conn() *pgx.Conn {
	return nil
}

func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription {
	return []pgconn.FieldDescription{}
}

func (m *mockRows) CommandTag() pgconn.CommandTag {
	return pgconn.CommandTag{}
}

func createMockRows() pgx.Rows {
	return &mockRows{
		data: []handlers.ExcelExportRow{
			{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
			},
		},
	}
}

func TestExportHandler_ExportJSON(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		setupMocks     func(*mocks.MockDatabase, *mocks.MockCacheRepository)
		expectedStatus int
		validateBody   func(*testing.T, []byte)
	}{
		{
			name:        "exports_json_with_default_params",
			queryParams: map[string]string{},
			setupMocks: func(db *mocks.MockDatabase, cache *mocks.MockCacheRepository) {
				// Cache miss
				cache.EXPECT().
					Get(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(redis_a.ErrCacheMiss)

				// Query database
				db.EXPECT().
					Query(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(createMockRows(), nil)

				// Cache result
				cache.EXPECT().
					Set(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectedStatus: http.StatusOK,
			validateBody: func(t *testing.T, body []byte) {
				var response handlers.JSONExportResponse
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.Inventory)
				assert.Contains(t, response.Metadata.Columns, "all")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDB := mocks.NewMockDatabase(ctrl)
			mockCache := mocks.NewMockCacheRepository(ctrl)
			mockService := mocks.NewMockInventoryService(ctrl)
			logger := helpers.TestLogger()

			handler := handlers.NewExportHandler(mockService, mockDB, mockCache, logger)

			tt.setupMocks(mockDB, mockCache)

			// Create request
			req := httptest.NewRequest("GET", "/api/v1/export/json", nil)
			if len(tt.queryParams) > 0 {
				q := req.URL.Query()
				for k, v := range tt.queryParams {
					q.Add(k, v)
				}
				req.URL.RawQuery = q.Encode()
			}
			w := httptest.NewRecorder()

			// Execute
			handler.ExportJSON(w, req)

			// Assert
			resp := w.Result()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

			if tt.validateBody != nil {
				tt.validateBody(t, w.Body.Bytes())
			}
		})
	}
}

func TestExportHandler_ExportExcel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := mocks.NewMockDatabase(ctrl)
	mockCache := newTestCacheMock()
	mockService := mocks.NewMockInventoryService(ctrl)
	logger := helpers.TestLogger()

	handler := handlers.NewExportHandler(mockService, mockDB, mockCache, logger)

	// Setup mock expectations
	mockDB.EXPECT().
		Query(gomock.Any(), gomock.Any()).
		Return(createMockRows(), nil)

	// Create request
	req := httptest.NewRequest("GET", "/api/v1/export/excel", nil)
	w := httptest.NewRecorder()

	// Execute
	handler.ExportExcel(w, req)

	// Assert
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		resp.Header.Get("Content-Type"))
	assert.Contains(t, resp.Header.Get("Content-Disposition"), "inventory_export_")
	assert.NotEmpty(t, w.Body.Bytes())
}

// testCacheMock implements ports.CacheRepository for testing
type testCacheMock struct {
	mu       sync.RWMutex
	data     map[string][]byte
	ttls     map[string]time.Time
	counters map[string]int64
}

// Ensure testCacheMock implements ports.CacheRepository
var _ ports.CacheRepository = (*testCacheMock)(nil)

// newTestCacheMock creates a new test cache mock
func newTestCacheMock() *testCacheMock {
	return &testCacheMock{
		data:     make(map[string][]byte),
		ttls:     make(map[string]time.Time),
		counters: make(map[string]int64),
	}
}

// Set stores a value with default TTL
func (m *testCacheMock) Set(ctx context.Context, key string, value interface{}) error {
	return m.SetWithTTL(ctx, key, value, time.Hour) // Default 1 hour TTL
}

// SetWithTTL stores a value with custom TTL
func (m *testCacheMock) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	m.data[key] = data
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}

	return nil
}

// Get retrieves a value from cache
func (m *testCacheMock) Get(ctx context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, exists := m.data[key]
	if !exists {
		return redis_a.ErrCacheMiss
	}

	// Check TTL
	if expiry, hasTTL := m.ttls[key]; hasTTL && time.Now().After(expiry) {
		delete(m.data, key)
		delete(m.ttls, key)
		return redis_a.ErrCacheMiss
	}

	return json.Unmarshal(data, dest)
}

// Delete removes keys from cache
func (m *testCacheMock) Delete(ctx context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, key := range keys {
		delete(m.data, key)
		delete(m.ttls, key)
		delete(m.counters, key)
	}

	return nil
}

// DeletePattern removes all keys matching a pattern (simple implementation)
func (m *testCacheMock) DeletePattern(ctx context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Simple pattern matching - in real implementation you'd use regex
	var keysToDelete []string
	for key := range m.data {
		// For testing purposes, simple contains check
		if pattern == "*" || key == pattern {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(m.data, key)
		delete(m.ttls, key)
		delete(m.counters, key)
	}

	return nil
}

// Exists checks if all keys exist
func (m *testCacheMock) Exists(ctx context.Context, keys ...string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, key := range keys {
		if _, exists := m.data[key]; !exists {
			return false, nil
		}

		// Check TTL
		if expiry, hasTTL := m.ttls[key]; hasTTL && time.Now().After(expiry) {
			return false, nil
		}
	}

	return true, nil
}

// Expire sets TTL for a key
func (m *testCacheMock) Expire(ctx context.Context, key string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.data[key]; !exists {
		return nil // Redis behavior - doesn't error if key doesn't exist
	}

	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	} else {
		delete(m.ttls, key)
	}

	return nil
}

// GetOrSet retrieves from cache or sets if not found
func (m *testCacheMock) GetOrSet(ctx context.Context, key string, dest interface{},
	fetch func() (interface{}, error), ttl time.Duration) error {

	err := m.Get(ctx, key, dest)
	if err == nil {
		return nil // Cache hit
	}

	if err != redis_a.ErrCacheMiss {
		return err
	}

	// Cache miss - fetch and store
	value, err := fetch()
	if err != nil {
		return err
	}

	// Store in cache
	if err := m.SetWithTTL(ctx, key, value, ttl); err != nil {
		return err
	}

	// Copy value to destination
	data, _ := json.Marshal(value)
	return json.Unmarshal(data, dest)
}

// Increment increments a counter
func (m *testCacheMock) Increment(ctx context.Context, key string) (int64, error) {
	return m.IncrementBy(ctx, key, 1)
}

// IncrementBy increments a counter by a specific amount
func (m *testCacheMock) IncrementBy(ctx context.Context, key string, value int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.counters[key] += value
	return m.counters[key], nil
}

// SetNX sets a key only if it doesn't exist
func (m *testCacheMock) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if key exists and is not expired
	if _, exists := m.data[key]; exists {
		if expiry, hasTTL := m.ttls[key]; !hasTTL || time.Now().Before(expiry) {
			return false, nil // Key exists and is not expired
		}
	}

	// Key doesn't exist or is expired, set it
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	m.data[key] = data
	if ttl > 0 {
		m.ttls[key] = time.Now().Add(ttl)
	}

	return true, nil
}

// TTL returns the time to live for a key
func (m *testCacheMock) TTL(ctx context.Context, key string) (time.Duration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, exists := m.data[key]; !exists {
		return -2 * time.Second, nil // Redis convention: -2 for non-existent key
	}

	expiry, hasTTL := m.ttls[key]
	if !hasTTL {
		return -1 * time.Second, nil // Redis convention: -1 for key with no TTL
	}

	remaining := time.Until(expiry)
	if remaining <= 0 {
		// Key has expired
		delete(m.data, key)
		delete(m.ttls, key)
		return -2 * time.Second, nil
	}

	return remaining, nil
}

// Flush removes all keys
func (m *testCacheMock) Flush(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string][]byte)
	m.ttls = make(map[string]time.Time)
	m.counters = make(map[string]int64)

	return nil
}

// Ping checks connectivity (always succeeds in mock)
func (m *testCacheMock) Ping(ctx context.Context) error {
	return nil
}
