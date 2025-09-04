// internal/pkg/logger/handlers.go
package logger

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ContextHandler extracts values from context and adds them to log records
type ContextHandler struct {
	handler slog.Handler
	config  *LogConfig
}

// NewContextHandler creates a handler that enriches logs with context values
func NewContextHandler(handler slog.Handler, config *LogConfig) *ContextHandler {
	return &ContextHandler{
		handler: handler,
		config:  config,
	}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	// Extract context values and add as attributes
	contextAttrs := extractContextAttrs(ctx, defaultContextKeys())

	// Create new record with context attributes
	if len(contextAttrs) > 0 {
		newRecord := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)

		// Copy existing attributes
		record.Attrs(func(a slog.Attr) bool {
			newRecord.AddAttrs(a)
			return true
		})

		// Add context attributes
		for i := 0; i < len(contextAttrs); i += 2 {
			if attr, ok := contextAttrs[i].(slog.Attr); ok {
				newRecord.AddAttrs(attr)
			}
		}

		return h.handler.Handle(ctx, newRecord)
	}

	return h.handler.Handle(ctx, record)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{
		handler: h.handler.WithAttrs(attrs),
		config:  h.config,
	}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{
		handler: h.handler.WithGroup(name),
		config:  h.config,
	}
}

// SamplingHandler implements log sampling for high-volume production environments
type SamplingHandler struct {
	handler    slog.Handler
	sampleRate float64
	mu         sync.Mutex
	rng        *rand.Rand
}

// NewSamplingHandler creates a handler that samples logs
func NewSamplingHandler(handler slog.Handler, sampleRate float64) *SamplingHandler {
	return &SamplingHandler{
		handler:    handler,
		sampleRate: sampleRate,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (h *SamplingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	// Always log warnings and errors
	if level >= slog.LevelWarn {
		return h.handler.Enabled(ctx, level)
	}

	// Sample info and debug logs
	h.mu.Lock()
	sample := h.rng.Float64() < h.sampleRate
	h.mu.Unlock()

	return sample && h.handler.Enabled(ctx, level)
}

func (h *SamplingHandler) Handle(ctx context.Context, record slog.Record) error {
	// Add sampling metadata
	record.AddAttrs(slog.Float64("sample_rate", h.sampleRate))
	return h.handler.Handle(ctx, record)
}

func (h *SamplingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SamplingHandler{
		handler:    h.handler.WithAttrs(attrs),
		sampleRate: h.sampleRate,
		rng:        h.rng,
	}
}

func (h *SamplingHandler) WithGroup(name string) slog.Handler {
	return &SamplingHandler{
		handler:    h.handler.WithGroup(name),
		sampleRate: h.sampleRate,
		rng:        h.rng,
	}
}

// SanitizationHandler removes or masks sensitive data
type SanitizationHandler struct {
	handler   slog.Handler
	patterns  []*regexp.Regexp
	blacklist []string
}

// NewSanitizationHandler creates a handler that sanitizes sensitive data
func NewSanitizationHandler(handler slog.Handler) *SanitizationHandler {
	return &SanitizationHandler{
		handler: handler,
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(password|pwd|pass|secret|token|key|auth|jwt|bearer|api[-_]?key)\s*[:=]\s*["']?([^"'\s]+)`),
			regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`), // Email
			regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),                               // SSN
			regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),                         // Credit card
		},
		blacklist: []string{
			"password", "pwd", "secret", "token", "auth", "jwt",
			"credit_card", "ssn", "social_security", "api_key",
		},
	}
}

func (h *SanitizationHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *SanitizationHandler) Handle(ctx context.Context, record slog.Record) error {
	// Sanitize message
	sanitizedMsg := h.sanitizeString(record.Message)
	newRecord := slog.NewRecord(record.Time, record.Level, sanitizedMsg, record.PC)

	// Sanitize attributes
	record.Attrs(func(a slog.Attr) bool {
		sanitized := h.sanitizeAttr(a)
		newRecord.AddAttrs(sanitized)
		return true
	})

	return h.handler.Handle(ctx, newRecord)
}

func (h *SanitizationHandler) sanitizeAttr(attr slog.Attr) slog.Attr {
	// Check if attribute key is sensitive
	lowerKey := strings.ToLower(attr.Key)
	for _, blacklisted := range h.blacklist {
		if strings.Contains(lowerKey, blacklisted) {
			attr.Value = slog.StringValue("***REDACTED***")
			return attr
		}
	}

	// Sanitize string values
	if s, ok := attr.Value.Any().(string); ok {
		attr.Value = slog.StringValue(h.sanitizeString(s))
	}

	return attr
}

func (h *SanitizationHandler) sanitizeString(s string) string {
	for _, pattern := range h.patterns {
		s = pattern.ReplaceAllString(s, "$1=***REDACTED***")
	}
	return s
}

func (h *SanitizationHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SanitizationHandler{
		handler:   h.handler.WithAttrs(attrs),
		patterns:  h.patterns,
		blacklist: h.blacklist,
	}
}

func (h *SanitizationHandler) WithGroup(name string) slog.Handler {
	return &SanitizationHandler{
		handler:   h.handler.WithGroup(name),
		patterns:  h.patterns,
		blacklist: h.blacklist,
	}
}

// MultiHandler sends logs to multiple handlers
type MultiHandler struct {
	handlers []slog.Handler
}

// NewMultiHandler creates a handler that sends to multiple destinations
func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, record); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("multi-handler errors: %v", errs)
	}
	return nil
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: newHandlers}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		newHandlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: newHandlers}
}

// PrettyTextHandler provides human-readable colored output for development
type PrettyTextHandler struct {
	*slog.TextHandler
	opts *slog.HandlerOptions
	mu   sync.Mutex
	w    io.Writer
}

// NewPrettyTextHandler creates a pretty text handler
func NewPrettyTextHandler(w io.Writer, opts *slog.HandlerOptions) *PrettyTextHandler {
	return &PrettyTextHandler{
		TextHandler: slog.NewTextHandler(w, opts),
		opts:        opts,
		w:           w,
	}
}

func (h *PrettyTextHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Format timestamp
	timestamp := r.Time.Format("2006-01-02 15:04:05.000")

	// Color codes for levels
	levelColor := h.getLevelColor(r.Level)
	resetColor := "\033[0m"

	// Format level
	level := r.Level.String()

	// Build output
	fmt.Fprintf(h.w, "%s%s %s%s%s %s",
		levelColor,
		timestamp,
		strings.ToUpper(level),
		resetColor,
		strings.Repeat(" ", 7-len(level)),
		r.Message,
	)

	// Add attributes
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(h.w, " %s%s=%v%s", "\033[36m", a.Key, a.Value, resetColor)
		return true
	})

	fmt.Fprintln(h.w)

	return nil
}

func (h *PrettyTextHandler) getLevelColor(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "\033[37m" // White
	case slog.LevelInfo:
		return "\033[34m" // Blue
	case slog.LevelWarn:
		return "\033[33m" // Yellow
	case slog.LevelError:
		return "\033[31m" // Red
	default:
		return "\033[0m" // Reset
	}
}

// ElasticsearchHandler sends logs to Elasticsearch
type ElasticsearchHandler struct {
	handler slog.Handler
	client  *ElasticsearchClient
	index   string
	buffer  []map[string]any
	mu      sync.Mutex
}

// NewElasticsearchHandler creates handler for Elasticsearch
func NewElasticsearchHandler(options map[string]any, opts *slog.HandlerOptions) slog.Handler {
	// This would integrate with actual Elasticsearch client
	// For now, returning a JSON handler as placeholder
	return slog.NewJSONHandler(io.Discard, opts)
}

// ElasticsearchClient would be the actual ES client
type ElasticsearchClient struct {
	url   string
	index string
}

func (c *ElasticsearchClient) BulkIndex(docs []map[string]any) error {
	// Implementation would send to Elasticsearch
	data, _ := json.Marshal(docs)
	fmt.Printf("Would send to ES: %s\n", data)
	return nil
}
