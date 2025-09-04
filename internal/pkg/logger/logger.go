// internal/pkg/logger/logger.go
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ContextKey represents keys for context values
type ContextKey string

const (
	// Context keys for logging
	ContextKeyRequestID   ContextKey = "request_id"
	ContextKeyUserID      ContextKey = "user_id"
	ContextKeySessionID   ContextKey = "session_id"
	ContextKeyTraceID     ContextKey = "trace_id"
	ContextKeySpanID      ContextKey = "span_id"
	ContextKeyClientIP    ContextKey = "client_ip"
	ContextKeyUserAgent   ContextKey = "user_agent"
	ContextKeyMethod      ContextKey = "method"
	ContextKeyPath        ContextKey = "path"
	ContextKeyStatusCode  ContextKey = "status_code"
	ContextKeyDuration    ContextKey = "duration_ms"
	ContextKeyEnvironment ContextKey = "environment"
	ContextKeyService     ContextKey = "service"
	ContextKeyVersion     ContextKey = "version"
)

// OutputConfig defines logging output destinations
type OutputConfig struct {
	Type    string         `json:"type"` // console, file, elasticsearch, datadog, etc.
	Level   string         `json:"level"`
	Format  string         `json:"format"`
	Options map[string]any `json:"options"`
}

// LogConfig holds logger configuration
type LogConfig struct {
	Level            string         `json:"level"`
	Format           string         `json:"format"`
	Output           string         `json:"output"`
	AddSource        bool           `json:"add_source"`
	SampleRate       float64        `json:"sample_rate"`
	Environment      string         `json:"environment"`
	ServiceName      string         `json:"service_name"`
	ServiceVersion   string         `json:"service_version"`
	EnableSampling   bool           `json:"enable_sampling"`
	EnableStackTrace bool           `json:"enable_stack_trace"`
	Fields           map[string]any `json:"fields"`
	Outputs          []OutputConfig `json:"outputs"`
}

// Logger wraps slog.Logger with additional functionality
type Logger struct {
	*slog.Logger
	config      *LogConfig
	handlers    []slog.Handler
	contextKeys []ContextKey
}

// Global logger instance
var (
	defaultLogger *Logger
)

// SetupLogger initializes the enhanced logger with production features
func SetupLogger(level string, format string) *Logger {
	config := &LogConfig{
		Level:            level,
		Format:           format,
		Output:           "stdout",
		AddSource:        true,
		EnableSampling:   false,
		EnableStackTrace: level == "debug",
		ServiceName:      os.Getenv("SERVICE_NAME"),
		ServiceVersion:   os.Getenv("SERVICE_VERSION"),
		Environment:      os.Getenv("APP_ENV"),
	}

	logger := NewLogger(config)
	defaultLogger = logger
	slog.SetDefault(logger.Logger)

	return logger
}

// NewLogger creates a new enhanced logger
func NewLogger(config *LogConfig) *Logger {
	if config == nil {
		config = &LogConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		}
	}

	// Create base handler options
	opts := &slog.HandlerOptions{
		Level:     parseLevel(config.Level),
		AddSource: config.AddSource,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Customize attribute formatting
			return replaceAttr(config, groups, a)
		},
	}

	// Create primary handler based on format
	var primaryHandler slog.Handler
	writer := getWriter(config.Output)

	switch config.Format {
	case "json":
		primaryHandler = slog.NewJSONHandler(writer, opts)
	case "text":
		primaryHandler = NewPrettyTextHandler(writer, opts)
	default:
		primaryHandler = slog.NewJSONHandler(writer, opts)
	}

	// Wrap with context handler for automatic context extraction
	primaryHandler = NewContextHandler(primaryHandler, config)

	// Add sampling if enabled
	if config.EnableSampling && config.SampleRate > 0 && config.SampleRate < 1.0 {
		primaryHandler = NewSamplingHandler(primaryHandler, config.SampleRate)
	}

	// Add sanitization handler
	primaryHandler = NewSanitizationHandler(primaryHandler)

	// Create multi-handler if multiple outputs configured
	handlers := []slog.Handler{primaryHandler}

	for _, output := range config.Outputs {
		handler := createOutputHandler(output, parseLevel(output.Level))
		if handler != nil {
			handlers = append(handlers, handler)
		}
	}

	// Use multi-handler if multiple handlers
	var finalHandler slog.Handler
	if len(handlers) > 1 {
		finalHandler = NewMultiHandler(handlers...)
	} else {
		finalHandler = primaryHandler
	}

	// Add global fields
	if config.ServiceName != "" || config.Environment != "" {
		attrs := []slog.Attr{}
		if config.ServiceName != "" {
			attrs = append(attrs, slog.String("service", config.ServiceName))
		}
		if config.ServiceVersion != "" {
			attrs = append(attrs, slog.String("version", config.ServiceVersion))
		}
		if config.Environment != "" {
			attrs = append(attrs, slog.String("env", config.Environment))
		}
		finalHandler = finalHandler.WithAttrs(attrs)
	}

	logger := &Logger{
		Logger:      slog.New(finalHandler),
		config:      config,
		handlers:    handlers,
		contextKeys: defaultContextKeys(),
	}

	return logger
}

// WithContext creates a logger with context values automatically extracted
func (l *Logger) WithContext(ctx context.Context) *slog.Logger {
	attrs := extractContextAttrs(ctx, l.contextKeys)
	if len(attrs) > 0 {
		return l.Logger.With(attrs...)
	}
	return l.Logger
}

// LogWithContext logs with automatic context extraction
func (l *Logger) LogWithContext(ctx context.Context, level slog.Level, msg string, args ...any) {
	logger := l.WithContext(ctx)

	// Add caller information for error/debug levels
	if level >= slog.LevelError || l.config.EnableStackTrace {
		pc, file, line, ok := runtime.Caller(1)
		if ok {
			fn := runtime.FuncForPC(pc)
			args = append(args,
				slog.String("caller", fmt.Sprintf("%s:%d", file, line)),
				slog.String("function", fn.Name()),
			)
		}
	}

	// Add stack trace for errors if enabled
	if level >= slog.LevelError && l.config.EnableStackTrace {
		args = append(args, slog.String("stack", string(getStackTrace())))
	}

	logger.Log(ctx, level, msg, args...)
}

// InfoContext logs at info level with context
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.LogWithContext(ctx, slog.LevelInfo, msg, args...)
}

// WarnContext logs at warn level with context
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.LogWithContext(ctx, slog.LevelWarn, msg, args...)
}

// ErrorContext logs at error level with context
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.LogWithContext(ctx, slog.LevelError, msg, args...)
}

// DebugContext logs at debug level with context
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.LogWithContext(ctx, slog.LevelDebug, msg, args...)
}

// Helper functions

func parseLevel(level string) slog.Leveler {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func getWriter(output string) io.Writer {
	switch output {
	case "stdout":
		return os.Stdout
	case "stderr":
		return os.Stderr
	default:
		if strings.HasPrefix(output, "file:") {
			filename := strings.TrimPrefix(output, "file:")
			file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return os.Stdout
			}
			return file
		}
		return os.Stdout
	}
}

func defaultContextKeys() []ContextKey {
	return []ContextKey{
		ContextKeyRequestID,
		ContextKeyUserID,
		ContextKeySessionID,
		ContextKeyTraceID,
		ContextKeySpanID,
		ContextKeyClientIP,
		ContextKeyUserAgent,
		ContextKeyMethod,
		ContextKeyPath,
		ContextKeyStatusCode,
		ContextKeyDuration,
	}
}

func extractContextAttrs(ctx context.Context, keys []ContextKey) []any {
	attrs := []any{}

	for _, key := range keys {
		if val := ctx.Value(key); val != nil {
			keyStr := string(key)
			switch v := val.(type) {
			case string:
				if v != "" {
					attrs = append(attrs, slog.String(keyStr, v))
				}
			case int:
				attrs = append(attrs, slog.Int(keyStr, v))
			case int64:
				attrs = append(attrs, slog.Int64(keyStr, v))
			case float64:
				attrs = append(attrs, slog.Float64(keyStr, v))
			case bool:
				attrs = append(attrs, slog.Bool(keyStr, v))
			case time.Duration:
				attrs = append(attrs, slog.Duration(keyStr, v))
			case time.Time:
				attrs = append(attrs, slog.Time(keyStr, v))
			case uuid.UUID:
				attrs = append(attrs, slog.String(keyStr, v.String()))
			default:
				attrs = append(attrs, slog.Any(keyStr, v))
			}
		}
	}

	return attrs
}

func getStackTrace() []byte {
	buf := make([]byte, 1024*8)
	n := runtime.Stack(buf, false)
	return buf[:n]
}

func replaceAttr(config *LogConfig, _ []string, a slog.Attr) slog.Attr {
	// Customize time format
	if a.Key == slog.TimeKey {
		if t, ok := a.Value.Any().(time.Time); ok {
			a.Value = slog.StringValue(t.Format(time.RFC3339Nano))
		}
	}

	// Rename level key for some log aggregators
	if a.Key == slog.LevelKey && config.Format == "json" {
		a.Key = "severity"
	}

	// Add milliseconds to duration
	if strings.HasSuffix(a.Key, "_ms") {
		if d, ok := a.Value.Any().(time.Duration); ok {
			a.Value = slog.Float64Value(float64(d.Milliseconds()))
		}
	}

	return a
}

func createOutputHandler(output OutputConfig, level slog.Leveler) slog.Handler {
	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: true,
	}
	baseHandler := slog.NewJSONHandler(io.Discard, opts)

	switch output.Type {
	case "elasticsearch":
		var elkCfg ELKConfig
		if cfgBytes, err := json.Marshal(output.Options); err == nil {
			_ = json.Unmarshal(cfgBytes, &elkCfg)
		}
		return NewELKHandler(elkCfg, baseHandler)

	case "file":
		if filename, ok := output.Options["filename"].(string); ok {
			if file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
				return slog.NewJSONHandler(file, opts)
			}
		}
	}

	return nil
}

// GetDefault returns the default logger instance
func GetDefault() *Logger {
	if defaultLogger == nil {
		defaultLogger = NewLogger(nil)
	}
	return defaultLogger
}

// WithFields adds fields to the logger
func WithFields(fields map[string]any) *slog.Logger {
	logger := GetDefault()
	attrs := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	return logger.With(attrs...)
}

// FromContext extracts logger from context or returns default
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value("logger").(*Logger); ok {
		return l.WithContext(ctx)
	}
	return GetDefault().WithContext(ctx)
}

// WithLogger adds logger to context
func WithLogger(ctx context.Context, logger *Logger) context.Context {
	return context.WithValue(ctx, "logger", logger)
}
