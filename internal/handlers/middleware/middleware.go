// internal/handlers/middleware/middleware.go
package middleware

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/ammerola/resell-be/internal/pkg/logger"
	"github.com/google/uuid"
	"golang.org/x/time/rate"
)

// RequestID middleware adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has an ID (from proxy/LB)
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to context
		ctx := context.WithValue(r.Context(), logger.ContextKeyRequestID, requestID)

		// Add to response header
		w.Header().Set("X-Request-ID", requestID)

		// Continue with request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func Logger(l *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Generate or extract request ID
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Generate trace ID for distributed tracing
			traceID := r.Header.Get("X-Trace-ID")
			if traceID == "" {
				traceID = uuid.New().String()
			}

			// Extract client IP
			clientIP := getClientIP(r)

			// Enrich context with logging fields
			ctx := r.Context()
			ctx = context.WithValue(ctx, logger.ContextKeyRequestID, requestID)
			ctx = context.WithValue(ctx, logger.ContextKeyTraceID, traceID)
			ctx = context.WithValue(ctx, logger.ContextKeyClientIP, clientIP)
			ctx = context.WithValue(ctx, logger.ContextKeyUserAgent, r.UserAgent())
			ctx = context.WithValue(ctx, logger.ContextKeyMethod, r.Method)
			ctx = context.WithValue(ctx, logger.ContextKeyPath, r.URL.Path)

			// Extract user ID from auth header or session (if available)
			if userID := extractUserID(r); userID != "" {
				ctx = context.WithValue(ctx, logger.ContextKeyUserID, userID)
			}

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Set request ID in response header
			w.Header().Set("X-Request-ID", requestID)
			w.Header().Set("X-Trace-ID", traceID)

			// Create logger with context
			contextLogger := l.WithContext(ctx)

			// Log request start
			contextLogger.Log(ctx, slog.LevelInfo, "request_started",
				slog.Group("request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("query", r.URL.RawQuery),
					slog.String("remote_addr", r.RemoteAddr),
					slog.String("client_ip", clientIP),
					slog.String("user_agent", r.UserAgent()),
					slog.String("referer", r.Referer()),
					slog.Int64("content_length", r.ContentLength),
				),
				slog.Group("ids",
					slog.String("request_id", requestID),
					slog.String("trace_id", traceID),
				),
			)

			// Process request
			next.ServeHTTP(wrapped, r.WithContext(ctx))

			// Calculate duration
			duration := time.Since(start)

			// Add response context
			ctx = context.WithValue(ctx, logger.ContextKeyStatusCode, wrapped.statusCode)
			ctx = context.WithValue(ctx, logger.ContextKeyDuration, duration)

			// Determine log level based on status code
			logLevel := slog.LevelInfo
			if wrapped.statusCode >= 500 {
				logLevel = slog.LevelError
			} else if wrapped.statusCode >= 400 {
				logLevel = slog.LevelWarn
			} else if duration > 5*time.Second {
				logLevel = slog.LevelWarn
			}

			// Log request completion
			contextLogger.Log(ctx, logLevel, "request_completed",
				slog.Group("request",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("query", r.URL.RawQuery),
				),
				slog.Group("response",
					slog.Int("status", wrapped.statusCode),
					slog.String("status_text", http.StatusText(wrapped.statusCode)),
					slog.Int("bytes", wrapped.bytesWritten),
					slog.Duration("duration", duration),
					slog.Float64("duration_ms", float64(duration.Milliseconds())),
				),
				slog.Group("performance",
					slog.Bool("slow_request", duration > 5*time.Second),
					slog.String("latency_human", duration.String()),
				),
			)

			// Log slow queries separately for monitoring
			if duration > 5*time.Second {
				l.WarnContext(ctx, "slow_request_detected",
					slog.String("path", r.URL.Path),
					slog.Duration("duration", duration),
					slog.String("threshold", "5s"),
				)
			}
		})
	}
}

// Helper function to extract user ID from request
func extractUserID(r *http.Request) string {
	// Try JWT token first
	if auth := r.Header.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			// Parse JWT and extract user ID
			// This is simplified - you'd actually validate and parse the JWT
			return "" // Would return actual user ID
		}
	}

	// Try session cookie
	if _, err := r.Cookie("session_id"); err == nil {
		// Look up session and get user ID
		return "" // Would return actual user ID from session
	}

	return ""
}

// Recovery middleware recovers from panics
func Recovery(slogger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID, _ := r.Context().Value(logger.ContextKeyRequestID).(string)

					// Log the panic
					slogger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("error", err),
						slog.String("request_id", requestID),
						slog.String("stack", string(debug.Stack())),
					)

					// Return error response
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error":"Internal Server Error","request_id":"` + requestID + `"}`))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// RateLimit middleware implements rate limiting per IP
func RateLimit(requestsPerMinute int, duration time.Duration) func(http.Handler) http.Handler {
	// Store rate limiters per IP
	limiters := &sync.Map{}

	// Cleanup old limiters periodically
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			now := time.Now()
			limiters.Range(func(key, value interface{}) bool {
				limiter := value.(*rateLimiter)
				if now.Sub(limiter.lastSeen) > 10*time.Minute {
					limiters.Delete(key)
				}
				return true
			})
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client IP
			ip := getClientIP(r)

			// Get or create rate limiter for this IP
			val, _ := limiters.LoadOrStore(ip, &rateLimiter{
				limiter:  rate.NewLimiter(rate.Every(duration/time.Duration(requestsPerMinute)), requestsPerMinute),
				lastSeen: time.Now(),
			})

			rl := val.(*rateLimiter)
			rl.lastSeen = time.Now()

			// Check rate limit
			if !rl.limiter.Allow() {
				http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CORS middleware handles Cross-Origin Resource Sharing
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" || allowedOrigin == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight requests
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// SecureHeaders middleware adds security headers
func SecureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

		// HSTS for HTTPS connections
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}

		next.ServeHTTP(w, r)
	})
}

// Timeout middleware adds request timeout
func Timeout(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})
			go func() {
				next.ServeHTTP(w, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				w.WriteHeader(http.StatusGatewayTimeout)
				w.Write([]byte(`{"error":"Request timeout"}`))
			}
		})
	}
}

// Compression middleware adds gzip compression
func Compression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Wrap response writer with gzip writer
		gz := &gzipResponseWriter{
			ResponseWriter: w,
		}
		defer gz.Close()

		gz.Header().Set("Content-Encoding", "gzip")
		next.ServeHTTP(gz, r)
	})
}

// Helper types and functions

type responseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
	written      bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.statusCode = code
		rw.ResponseWriter.WriteHeader(code)
		rw.written = true
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += n
	return n, err
}

type rateLimiter struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	xri := r.Header.Get("X-Real-IP")
	if xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}

	return r.RemoteAddr
}

// gzipResponseWriter implements gzip compression
type gzipResponseWriter struct {
	http.ResponseWriter
	writer *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	if w.writer == nil {
		w.writer = gzip.NewWriter(w.ResponseWriter)
	}
	return w.writer.Write(b)
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Close() {
	if w.writer != nil {
		w.writer.Close()
	}
}

// Flush implements http.Flusher
func (w *gzipResponseWriter) Flush() {
	if w.writer != nil {
		w.writer.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker
func (w *gzipResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("ResponseWriter does not implement Hijacker")
}

// Push implements http.Pusher
func (w *gzipResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return fmt.Errorf("ResponseWriter does not implement Pusher")
}

// ContentTypeJSON middleware ensures JSON content type
func ContentTypeJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// MetricsMiddleware records metrics for monitoring
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK,
		}

		next.ServeHTTP(wrapped, r)

		// Record metrics (would integrate with Prometheus)
		duration := time.Since(start)
		recordHTTPMetric(r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

func recordHTTPMetric(method, path string, status int, duration time.Duration) {
	// This would integrate with Prometheus or other metrics system
	// For now, just a placeholder
}
