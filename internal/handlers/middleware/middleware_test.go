package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/ammerola/resell-be/internal/handlers/middleware"
	"github.com/ammerola/resell-be/test/helpers"
)

func TestRequestID(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request ID is in context
		requestID := r.Context().Value("request_id")
		assert.NotNil(t, requestID)
		assert.NotEmpty(t, requestID.(string))

		w.WriteHeader(http.StatusOK)
	})

	// Wrap with RequestID middleware
	wrapped := middleware.RequestID(handler)

	tests := []struct {
		name              string
		existingRequestID string
		validateResponse  func(*testing.T, *http.Response)
	}{
		{
			name:              "generates_new_request_id",
			existingRequestID: "",
			validateResponse: func(t *testing.T, resp *http.Response) {
				requestID := resp.Header.Get("X-Request-ID")
				assert.NotEmpty(t, requestID)
				assert.Len(t, requestID, 36) // UUID length
			},
		},
		{
			name:              "uses_existing_request_id",
			existingRequestID: "existing-id-123",
			validateResponse: func(t *testing.T, resp *http.Response) {
				requestID := resp.Header.Get("X-Request-ID")
				assert.Equal(t, "existing-id-123", requestID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.existingRequestID != "" {
				req.Header.Set("X-Request-ID", tt.existingRequestID)
			}
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			resp := w.Result()
			tt.validateResponse(t, resp)
		})
	}
}

func TestLogger(t *testing.T) {
	logger := helpers.TestLogger()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	wrapped := middleware.Logger(logger)(handler)

	req := httptest.NewRequest("GET", "/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), "request_id", "test-123"))
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())
}

func TestRecovery(t *testing.T) {
	logger := helpers.TestLogger()

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "recovers_from_panic",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("test panic")
			}),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Internal Server Error",
		},
		{
			name: "passes_through_normal_response",
			handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("normal response"))
			}),
			expectedStatus: http.StatusOK,
			expectedBody:   "normal response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := middleware.Recovery(logger)(tt.handler)

			req := httptest.NewRequest("GET", "/test", nil)
			req = req.WithContext(context.WithValue(req.Context(), "request_id", "test-123"))
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}
}

func TestRateLimit(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Allow 2 requests per second
	wrapped := middleware.RateLimit(2, time.Second)(handler)

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		w := httptest.NewRecorder()

		wrapped.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	// Third request should be rate limited
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)

	// Different IP should work
	req.RemoteAddr = "192.168.1.1:5678"
	w = httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCORS(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	tests := []struct {
		name           string
		allowedOrigins []string
		requestOrigin  string
		requestMethod  string
		expectedStatus int
		checkHeaders   func(*testing.T, http.Header)
	}{
		{
			name:           "allows_wildcard_origin",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://example.com",
			requestMethod:  "GET",
			expectedStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, headers http.Header) {
				assert.Equal(t, "https://example.com", headers.Get("Access-Control-Allow-Origin"))
			},
		},
		{
			name:           "allows_specific_origin",
			allowedOrigins: []string{"https://app.example.com", "https://admin.example.com"},
			requestOrigin:  "https://app.example.com",
			requestMethod:  "GET",
			expectedStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, headers http.Header) {
				assert.Equal(t, "https://app.example.com", headers.Get("Access-Control-Allow-Origin"))
			},
		},
		{
			name:           "handles_preflight_request",
			allowedOrigins: []string{"*"},
			requestOrigin:  "https://example.com",
			requestMethod:  "OPTIONS",
			expectedStatus: http.StatusNoContent,
			checkHeaders: func(t *testing.T, headers http.Header) {
				assert.Equal(t, "https://example.com", headers.Get("Access-Control-Allow-Origin"))
				assert.NotEmpty(t, headers.Get("Access-Control-Allow-Methods"))
				assert.NotEmpty(t, headers.Get("Access-Control-Allow-Headers"))
			},
		},
		{
			name:           "blocks_unallowed_origin",
			allowedOrigins: []string{"https://allowed.com"},
			requestOrigin:  "https://notallowed.com",
			requestMethod:  "GET",
			expectedStatus: http.StatusOK,
			checkHeaders: func(t *testing.T, headers http.Header) {
				assert.Empty(t, headers.Get("Access-Control-Allow-Origin"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := middleware.CORS(tt.allowedOrigins)(handler)

			req := httptest.NewRequest(tt.requestMethod, "/test", nil)
			req.Header.Set("Origin", tt.requestOrigin)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			tt.checkHeaders(t, w.Header())
		})
	}
}

func TestTimeout(t *testing.T) {
	tests := []struct {
		name           string
		timeout        time.Duration
		handlerDelay   time.Duration
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "completes_within_timeout",
			timeout:        100 * time.Millisecond,
			handlerDelay:   10 * time.Millisecond,
			expectedStatus: http.StatusOK,
			expectedBody:   "success",
		},
		{
			name:           "times_out",
			timeout:        50 * time.Millisecond,
			handlerDelay:   200 * time.Millisecond,
			expectedStatus: http.StatusGatewayTimeout,
			expectedBody:   "Request timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-time.After(tt.handlerDelay):
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("success"))
				case <-r.Context().Done():
					return
				}
			})

			wrapped := middleware.Timeout(tt.timeout)(handler)

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			wrapped.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}
}
