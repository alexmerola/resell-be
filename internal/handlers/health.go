// internal/handlers/health.go
package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/pkg/config"
)

// HealthHandler handles health check endpoints
type HealthHandler struct {
	db        *db.Database
	redis     *redis.Client
	asynq     *asynq.Inspector
	config    *config.Config
	logger    *slog.Logger
	startTime time.Time
}

// NewHealthHandler creates a new health handler
func NewHealthHandler(
	database *db.Database,
	redisClient *redis.Client,
	asynqInspector *asynq.Inspector,
	cfg *config.Config,
	logger *slog.Logger,
) *HealthHandler {
	return &HealthHandler{
		db:        database,
		redis:     redisClient,
		asynq:     asynqInspector,
		config:    cfg,
		logger:    logger.With(slog.String("handler", "health")),
		startTime: time.Now(),
	}
}

// HealthStatus represents the health status of the application
type HealthStatus struct {
	Status      string                 `json:"status"`
	Version     string                 `json:"version"`
	Environment string                 `json:"environment"`
	Uptime      string                 `json:"uptime"`
	Timestamp   time.Time              `json:"timestamp"`
	Services    map[string]ServiceInfo `json:"services"`
	System      SystemInfo             `json:"system"`
}

// ServiceInfo represents the status of a service dependency
type ServiceInfo struct {
	Status       string                 `json:"status"`
	Message      string                 `json:"message,omitempty"`
	ResponseTime string                 `json:"response_time,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
}

// SystemInfo represents system-level information
type SystemInfo struct {
	GoVersion      string `json:"go_version"`
	NumGoroutines  int    `json:"num_goroutines"`
	NumCPU         int    `json:"num_cpu"`
	MemoryAllocMB  uint64 `json:"memory_alloc_mb"`
	MemorySysMB    uint64 `json:"memory_sys_mb"`
	GCPauseTotalMs uint64 `json:"gc_pause_total_ms"`
	NumGC          uint32 `json:"num_gc"`
}

// Health handles the /health endpoint
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	health := HealthStatus{
		Status:      "healthy",
		Version:     h.config.App.Version,
		Environment: h.config.App.Environment,
		Uptime:      time.Since(h.startTime).Round(time.Second).String(),
		Timestamp:   time.Now(),
		Services:    make(map[string]ServiceInfo),
		System:      h.getSystemInfo(),
	}

	// Check database
	dbStatus := h.checkDatabase(ctx)
	health.Services["database"] = dbStatus
	if dbStatus.Status != "healthy" {
		health.Status = "degraded"
	}

	// Check Redis
	redisStatus := h.checkRedis(ctx)
	health.Services["redis"] = redisStatus
	if redisStatus.Status != "healthy" {
		health.Status = "degraded"
	}

	// Check Asynq if inspector is available
	if h.asynq != nil {
		asynqStatus := h.checkAsynq(ctx)
		health.Services["asynq"] = asynqStatus
		if asynqStatus.Status != "healthy" {
			health.Status = "degraded"
		}
	}

	// Set response status code
	statusCode := http.StatusOK
	if health.Status == "degraded" {
		statusCode = http.StatusServiceUnavailable
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(health); err != nil {
		h.logger.ErrorContext(ctx, "failed to encode health response",
			slog.String("error", err.Error()))
	}
}

// Readiness handles the /ready endpoint
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	ready := true
	details := make(map[string]string)

	// Check database readiness
	if err := h.db.Ping(ctx); err != nil {
		ready = false
		details["database"] = "not ready"
	} else {
		details["database"] = "ready"
	}

	// Check Redis readiness
	if err := h.redis.Ping(ctx).Err(); err != nil {
		ready = false
		details["redis"] = "not ready"
	} else {
		details["redis"] = "ready"
	}

	// Prepare response
	response := map[string]interface{}{
		"ready":   ready,
		"details": details,
	}

	// Set response status
	statusCode := http.StatusOK
	if !ready {
		statusCode = http.StatusServiceUnavailable
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.ErrorContext(ctx, "failed to encode readiness response",
			slog.String("error", err.Error()))
	}
}

// checkDatabase checks the health of the database connection
func (h *HealthHandler) checkDatabase(ctx context.Context) ServiceInfo {
	start := time.Now()
	info := ServiceInfo{
		Status:  "healthy",
		Details: make(map[string]interface{}),
	}

	// Ping database
	if err := h.db.Ping(ctx); err != nil {
		info.Status = "unhealthy"
		info.Message = err.Error()
		h.logger.ErrorContext(ctx, "database health check failed",
			slog.String("error", err.Error()))
		return info
	}

	// Get pool statistics
	poolStats := h.db.Health(ctx)
	for k, v := range poolStats {
		info.Details[k] = v
	}

	info.ResponseTime = time.Since(start).String()
	return info
}

// checkRedis checks the health of the Redis connection
func (h *HealthHandler) checkRedis(ctx context.Context) ServiceInfo {
	start := time.Now()
	info := ServiceInfo{
		Status:  "healthy",
		Details: make(map[string]interface{}),
	}

	// Ping Redis
	pong, err := h.redis.Ping(ctx).Result()
	if err != nil {
		info.Status = "unhealthy"
		info.Message = err.Error()
		h.logger.ErrorContext(ctx, "redis health check failed",
			slog.String("error", err.Error()))
		return info
	}

	info.Details["ping"] = pong

	// Get Redis info
	if _, err := h.redis.Info(ctx, "server", "clients", "memory").Result(); err == nil {
		// Parse some basic info (simplified)
		info.Details["info"] = "available"
	}

	// Get pool stats
	poolStats := h.redis.PoolStats()
	info.Details["total_conns"] = poolStats.TotalConns
	info.Details["idle_conns"] = poolStats.IdleConns
	info.Details["stale_conns"] = poolStats.StaleConns

	info.ResponseTime = time.Since(start).String()
	return info
}

// checkAsynq checks the health of the Asynq queue system
func (h *HealthHandler) checkAsynq(ctx context.Context) ServiceInfo {
	start := time.Now()
	info := ServiceInfo{
		Status:  "healthy",
		Details: make(map[string]interface{}),
	}

	// Get queue statistics
	queues, err := h.asynq.Queues()
	if err != nil {
		info.Status = "unhealthy"
		info.Message = err.Error()
		h.logger.ErrorContext(ctx, "asynq health check failed",
			slog.String("error", err.Error()))
		return info
	}

	queueStats := make(map[string]interface{})
	for _, queue := range queues {
		qInfo, err := h.asynq.GetQueueInfo(queue)
		if err == nil {
			queueStats[queue] = map[string]interface{}{
				"size":      qInfo.Size,
				"active":    qInfo.Active,
				"pending":   qInfo.Pending,
				"scheduled": qInfo.Scheduled,
				"retry":     qInfo.Retry,
				"archived":  qInfo.Archived,
				"completed": qInfo.Completed,
			}
		}
	}

	info.Details["queues"] = queueStats

	// Get server info
	servers, err := h.asynq.Servers()
	if err == nil && len(servers) > 0 {
		info.Details["servers"] = len(servers)
		info.Details["workers"] = servers[0].ActiveWorkers
	}

	info.ResponseTime = time.Since(start).String()
	return info
}

// getSystemInfo returns system-level information
func (h *HealthHandler) getSystemInfo() SystemInfo {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return SystemInfo{
		GoVersion:      runtime.Version(),
		NumGoroutines:  runtime.NumGoroutine(),
		NumCPU:         runtime.NumCPU(),
		MemoryAllocMB:  memStats.Alloc / 1024 / 1024,
		MemorySysMB:    memStats.Sys / 1024 / 1024,
		GCPauseTotalMs: memStats.PauseTotalNs / 1000 / 1000,
		NumGC:          memStats.NumGC,
	}
}
