// internal/pkg/logger/elk.go
package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ELKConfig holds configuration for ELK stack integration
type ELKConfig struct {
	ElasticsearchURL string        `json:"elasticsearch_url"`
	IndexPattern     string        `json:"index_pattern"`
	BatchSize        int           `json:"batch_size"`
	FlushInterval    time.Duration `json:"flush_interval"`
	Username         string        `json:"username"`
	Password         string        `json:"password"`
	EnableBatching   bool          `json:"enable_batching"`
}

// ELKHandler sends logs to Elasticsearch
type ELKHandler struct {
	client      *http.Client
	config      ELKConfig
	buffer      []LogEntry
	mu          sync.Mutex
	flushTimer  *time.Timer
	baseHandler slog.Handler
}

// LogEntry represents a log entry for Elasticsearch
type LogEntry struct {
	Timestamp   time.Time              `json:"@timestamp"`
	Level       string                 `json:"level"`
	Message     string                 `json:"message"`
	Service     string                 `json:"service"`
	Environment string                 `json:"environment"`
	Version     string                 `json:"version"`
	RequestID   string                 `json:"request_id,omitempty"`
	TraceID     string                 `json:"trace_id,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	ClientIP    string                 `json:"client_ip,omitempty"`
	Method      string                 `json:"method,omitempty"`
	Path        string                 `json:"path,omitempty"`
	StatusCode  int                    `json:"status_code,omitempty"`
	Duration    float64                `json:"duration_ms,omitempty"`
	Fields      map[string]interface{} `json:"fields,omitempty"`
	Error       *ErrorInfo             `json:"error,omitempty"`
}

// ErrorInfo contains error details
type ErrorInfo struct {
	Type       string `json:"type"`
	Message    string `json:"message"`
	StackTrace string `json:"stack_trace,omitempty"`
	Code       string `json:"code,omitempty"`
}

// NewELKHandler creates a new ELK handler
func NewELKHandler(cfg ELKConfig, baseHandler slog.Handler) *ELKHandler {
	handler := &ELKHandler{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config:      cfg,
		buffer:      make([]LogEntry, 0, cfg.BatchSize),
		baseHandler: baseHandler,
	}

	if cfg.EnableBatching {
		handler.startFlusher()
	}

	return handler
}

func (h *ELKHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.baseHandler.Enabled(ctx, level)
}

func (h *ELKHandler) Handle(ctx context.Context, record slog.Record) error {
	// Also handle with base handler
	if err := h.baseHandler.Handle(ctx, record); err != nil {
		return err
	}

	// Create log entry
	entry := h.createLogEntry(ctx, record)

	if h.config.EnableBatching {
		h.mu.Lock()
		h.buffer = append(h.buffer, entry)
		shouldFlush := len(h.buffer) >= h.config.BatchSize
		h.mu.Unlock()

		if shouldFlush {
			go h.flush()
		}
	} else {
		// Send immediately
		go h.sendToElasticsearch([]LogEntry{entry})
	}

	return nil
}

func (h *ELKHandler) createLogEntry(ctx context.Context, record slog.Record) LogEntry {
	entry := LogEntry{
		Timestamp:   record.Time,
		Level:       record.Level.String(),
		Message:     record.Message,
		Service:     getContextString(ctx, ContextKeyService),
		Environment: getContextString(ctx, ContextKeyEnvironment),
		Version:     getContextString(ctx, ContextKeyVersion),
		RequestID:   getContextString(ctx, ContextKeyRequestID),
		TraceID:     getContextString(ctx, ContextKeyTraceID),
		UserID:      getContextString(ctx, ContextKeyUserID),
		ClientIP:    getContextString(ctx, ContextKeyClientIP),
		Method:      getContextString(ctx, ContextKeyMethod),
		Path:        getContextString(ctx, ContextKeyPath),
		Fields:      make(map[string]interface{}),
	}

	// Extract status code
	if statusCode, ok := ctx.Value(ContextKeyStatusCode).(int); ok {
		entry.StatusCode = statusCode
	}

	// Extract duration
	if duration, ok := ctx.Value(ContextKeyDuration).(time.Duration); ok {
		entry.Duration = float64(duration.Milliseconds())
	}

	// Extract attributes
	record.Attrs(func(a slog.Attr) bool {
		entry.Fields[a.Key] = a.Value.Any()

		// Check for error details
		if a.Key == "error" || a.Key == "err" {
			if err, ok := a.Value.Any().(error); ok {
				entry.Error = &ErrorInfo{
					Type:    fmt.Sprintf("%T", err),
					Message: err.Error(),
				}
			}
		}

		if a.Key == "stack" || a.Key == "stacktrace" {
			if stack, ok := a.Value.Any().(string); ok && entry.Error != nil {
				entry.Error.StackTrace = stack
			}
		}

		return true
	})

	return entry
}

func (h *ELKHandler) sendToElasticsearch(entries []LogEntry) {
	if len(entries) == 0 {
		return
	}

	// Create bulk request
	var buf bytes.Buffer
	for _, entry := range entries {
		// Bulk API metadata
		indexName := fmt.Sprintf("%s-%s", h.config.IndexPattern, time.Now().Format("2006.01.02"))
		meta := map[string]interface{}{
			"index": map[string]string{
				"_index": indexName,
			},
		}

		metaJSON, _ := json.Marshal(meta)
		buf.Write(metaJSON)
		buf.WriteByte('\n')

		// Document
		docJSON, _ := json.Marshal(entry)
		buf.Write(docJSON)
		buf.WriteByte('\n')
	}

	// Send bulk request
	url := fmt.Sprintf("%s/_bulk", h.config.ElasticsearchURL)
	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return
	}

	req.Header.Set("Content-Type", "application/x-ndjson")

	if h.config.Username != "" && h.config.Password != "" {
		req.SetBasicAuth(h.config.Username, h.config.Password)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		// Log to stderr if we can't send to Elasticsearch
		fmt.Printf("Failed to send logs to Elasticsearch: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("Elasticsearch returned error status: %d\n", resp.StatusCode)
	}
}

func (h *ELKHandler) flush() {
	h.mu.Lock()
	if len(h.buffer) == 0 {
		h.mu.Unlock()
		return
	}

	entries := make([]LogEntry, len(h.buffer))
	copy(entries, h.buffer)
	h.buffer = h.buffer[:0]
	h.mu.Unlock()

	h.sendToElasticsearch(entries)
}

func (h *ELKHandler) startFlusher() {
	go func() {
		ticker := time.NewTicker(h.config.FlushInterval)
		defer ticker.Stop()

		for range ticker.C {
			h.flush()
		}
	}()
}

func (h *ELKHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ELKHandler{
		client:      h.client,
		config:      h.config,
		buffer:      h.buffer,
		baseHandler: h.baseHandler.WithAttrs(attrs),
	}
}

func (h *ELKHandler) WithGroup(name string) slog.Handler {
	return &ELKHandler{
		client:      h.client,
		config:      h.config,
		buffer:      h.buffer,
		baseHandler: h.baseHandler.WithGroup(name),
	}
}

// Helper function to get string from context
func getContextString(ctx context.Context, key ContextKey) string {
	if val := ctx.Value(key); val != nil {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

// SetupELKLogging configures ELK logging for production
func SetupELKLogging(cfg ELKConfig) *Logger {
	// Create base logger config
	logConfig := &LogConfig{
		Level:          "info",
		Format:         "json",
		Output:         "stdout",
		AddSource:      true,
		EnableSampling: true,
		SampleRate:     0.1, // Sample 10% of debug/info logs
		ServiceName:    cfg.IndexPattern,
		Environment:    "production",
	}

	// Create base logger
	baseLogger := NewLogger(logConfig)

	// Create ELK handler
	elkHandler := NewELKHandler(cfg, baseLogger.handlers[0])

	// Replace handler with ELK handler
	baseLogger.Logger = slog.New(elkHandler)
	baseLogger.handlers = []slog.Handler{elkHandler}

	return baseLogger
}
