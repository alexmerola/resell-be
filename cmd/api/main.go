// cmd/api/main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"github.com/ammerola/resell-be/internal/adapters/db"
	redis_a "github.com/ammerola/resell-be/internal/adapters/redis_adapter"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/ammerola/resell-be/internal/core/services"
	"github.com/ammerola/resell-be/internal/handlers"
	"github.com/ammerola/resell-be/internal/handlers/middleware"
	"github.com/ammerola/resell-be/internal/pkg/config"
	"github.com/ammerola/resell-be/internal/pkg/logger"
)

// Build information injected at compile time
var (
	Version   = "dev"
	BuildTime = "unknown"
	GoVersion = "unknown"
)

func main() {
	// Initialize structured logger
	cfg := &config.Config{} // Temporary for initial logging
	slogger := logger.SetupLogger("debug", "json")

	slogger.Info("starting resell inventory management system",
		slog.String("version", Version),
		slog.String("build_time", BuildTime),
		slog.String("go_version", GoVersion),
	)

	// Load configuration
	slogger.Info("loading configuration")
	cfg, err := config.Load(slogger)
	if err != nil {
		slogger.Error("failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Reconfigure logger with loaded settings
	slogger = logger.SetupLogger(cfg.App.LogLevel, cfg.App.LogFormat)
	slogger.Info("configuration loaded",
		slog.String("environment", cfg.App.Environment),
		slog.String("log_level", cfg.App.LogLevel),
	)

	// Create application context
	ctx := context.Background()

	// Initialize dependencies
	deps, err := initializeDependencies(ctx, cfg, slogger)
	if err != nil {
		slogger.Error("failed to initialize dependencies", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer deps.cleanup()

	// Run database migrations if enabled
	if cfg.App.Environment != "production" {
		if err := runMigrations(ctx, cfg, slogger); err != nil {
			slogger.Error("failed to run migrations", slog.String("error", err.Error()))
			// Don't exit in development, just warn
		}
	}

	// Setup HTTP server
	server := setupHTTPServer(cfg, deps, slogger)

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		slogger.Info("starting HTTP server",
			slog.String("address", cfg.GetServerAddress()),
			slog.Bool("tls", cfg.Server.TLSEnabled),
		)

		if cfg.Server.TLSEnabled {
			serverErrors <- server.ListenAndServeTLS(cfg.Server.TLSCertFile, cfg.Server.TLSKeyFile)
		} else {
			serverErrors <- server.ListenAndServe()
		}
	}()

	// Setup signal handling for graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	// Wait for shutdown signal or server error
	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slogger.Error("server error", slog.String("error", err.Error()))
		}
	case sig := <-shutdown:
		slogger.Info("shutdown signal received",
			slog.String("signal", sig.String()),
		)

		// Create shutdown context with timeout
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.GracefulTimeout)
		defer shutdownCancel()

		// Gracefully shutdown HTTP server
		if err := server.Shutdown(shutdownCtx); err != nil {
			slogger.Error("failed to gracefully shutdown server", slog.String("error", err.Error()))
			server.Close()
		}

		// Stop Asynq client
		if deps.asynqClient != nil {
			if err := deps.asynqClient.Close(); err != nil {
				slogger.Error("failed to close Asynq client", slog.String("error", err.Error()))
			}
		}

		slogger.Info("server shutdown complete")
	}
}

// dependencies holds all application dependencies
type dependencies struct {
	database         ports.Database
	redisClient      *redis.Client
	redisCache       ports.CacheRepository
	asynqClient      *asynq.Client
	asynqInspector   *asynq.Inspector
	inventoryService *services.InventoryService
	inventoryHandler *handlers.InventoryHandler
	healthHandler    *handlers.HealthHandler
	dashboardHandler *handlers.DashboardHandler
	exportHandler    *handlers.ExportHandler
	importHandler    *handlers.ImportHandler
}

func (d *dependencies) cleanup() {
	if d.database != nil {
		d.database.Close()
	}
	if d.redisClient != nil {
		d.redisClient.Close()
	}
	if d.asynqClient != nil {
		d.asynqClient.Close()
	}
}

func initializeDependencies(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*dependencies, error) {
	deps := &dependencies{}

	// Initialize database connection
	logger.Info("connecting to database",
		slog.String("host", cfg.Database.Host),
		slog.String("database", cfg.Database.Name),
	)

	database, err := db.NewDatabase(ctx, &db.Config{
		Host:               cfg.Database.Host,
		Port:               cfg.Database.Port,
		User:               cfg.Database.User,
		Password:           cfg.Database.Password,
		Database:           cfg.Database.Name,
		SSLMode:            cfg.Database.SSLMode,
		MaxConnections:     cfg.Database.MaxConnections,
		MinConnections:     cfg.Database.MinConnections,
		MaxConnLifetime:    cfg.Database.MaxConnLifetime,
		MaxConnIdleTime:    cfg.Database.MaxConnIdleTime,
		HealthCheckPeriod:  cfg.Database.HealthCheckPeriod,
		ConnectTimeout:     cfg.Database.ConnectTimeout,
		StatementCacheMode: cfg.Database.StatementCacheMode,
		EnableQueryLogging: cfg.Database.EnableQueryLogging,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}
	deps.database = database

	// Initialize Redis client
	logger.Info("connecting to Redis",
		slog.String("host", cfg.Redis.Host),
		slog.String("port", cfg.Redis.Port),
	)

	redisOpts := &redis.Options{
		Addr:            fmt.Sprintf("%s:%s", cfg.Redis.Host, cfg.Redis.Port),
		Password:        cfg.Redis.Password,
		DB:              cfg.Redis.DB,
		MaxRetries:      cfg.Redis.MaxRetries,
		MinRetryBackoff: cfg.Redis.MinRetryBackoff,
		MaxRetryBackoff: cfg.Redis.MaxRetryBackoff,
		DialTimeout:     cfg.Redis.DialTimeout,
		ReadTimeout:     cfg.Redis.ReadTimeout,
		WriteTimeout:    cfg.Redis.WriteTimeout,
		PoolSize:        cfg.Redis.PoolSize,
		MinIdleConns:    cfg.Redis.MinIdleConns,
		ConnMaxLifetime: cfg.Redis.MaxConnAge,
		PoolTimeout:     cfg.Redis.PoolTimeout,
		ConnMaxIdleTime: cfg.Redis.IdleTimeout,
	}

	redisClient := redis.NewClient(redisOpts)

	// Test Redis connection
	if err := redisClient.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}
	deps.redisClient = redisClient

	// Create Redis cache wrapper
	deps.redisCache = redis_a.NewCache(redisClient, cfg.Redis.TTL, logger)

	// Initialize Asynq client
	logger.Info("initializing Asynq client")

	asynqRedisOpt := asynq.RedisClientOpt{
		Addr:     cfg.Asynq.RedisAddr,
		Password: cfg.Asynq.RedisPassword,
		DB:       cfg.Asynq.RedisDB,
	}

	asynqClient := asynq.NewClient(asynqRedisOpt)
	deps.asynqClient = asynqClient

	asynqInspector := asynq.NewInspector(asynqRedisOpt)
	deps.asynqInspector = asynqInspector

	// Initialize repositories
	inventoryRepo := db.NewInventoryRepository(database, logger)

	// Initialize services
	deps.inventoryService = services.NewInventoryService(inventoryRepo, database.Pool(), logger)

	// Initialize handlers
	deps.inventoryHandler = handlers.NewInventoryHandler(deps.inventoryService, logger)
	deps.healthHandler = handlers.NewHealthHandler(
		database,
		redisClient,
		asynqInspector,
		cfg,
		logger,
	)
	deps.dashboardHandler = handlers.NewDashboardHandler(database, deps.redisCache, logger)
	deps.exportHandler = handlers.NewExportHandler(deps.inventoryService, database, deps.redisCache, logger)

	// Calculate max file size in bytes
	maxFileSize := int64(cfg.FileProcessing.PDFMaxSizeMB * 1024 * 1024)
	deps.importHandler = handlers.NewImportHandler(asynqClient, logger, maxFileSize, cfg.FileProcessing.TempDir)

	logger.Info("all dependencies initialized successfully")
	return deps, nil
}

func setupHTTPServer(cfg *config.Config, deps *dependencies, logger *slog.Logger) *http.Server {
	// Create new ServeMux using Go 1.22+ features
	mux := http.NewServeMux()

	// Setup middleware chain
	var handler http.Handler = mux

	// Apply middleware in reverse order (innermost first)
	if cfg.App.Environment != "test" {
		handler = middleware.RequestID(handler)
		handler = middleware.Logger(logger)(handler)
		handler = middleware.Recovery(logger)(handler)
	}

	if cfg.Security.RateLimitRequests > 0 {
		handler = middleware.RateLimit(cfg.Security.RateLimitRequests, cfg.Security.RateLimitDuration)(handler)
	}

	if len(cfg.Security.AllowedOrigins) > 0 {
		handler = middleware.CORS(cfg.Security.AllowedOrigins)(handler)
	}

	if cfg.Security.SecureHeaders {
		handler = middleware.SecureHeaders(handler)
	}

	// Register routes using Go 1.22 method-specific routing
	registerRoutes(mux, deps, logger, cfg)

	// Create HTTP server
	server := &http.Server{
		Addr:           cfg.GetServerAddress(),
		Handler:        handler,
		ReadTimeout:    cfg.Server.ReadTimeout,
		WriteTimeout:   cfg.Server.WriteTimeout,
		IdleTimeout:    cfg.Server.IdleTimeout,
		MaxHeaderBytes: cfg.Server.MaxHeaderBytes,
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	return server
}

func registerRoutes(mux *http.ServeMux, deps *dependencies, logger *slog.Logger, cfg *config.Config) {
	apiV1 := "/api/v1"

	// Health and readiness endpoints
	if cfg.Server.EnableHealthCheck {
		mux.HandleFunc("GET /health", deps.healthHandler.Health)
		mux.HandleFunc("GET /ready", deps.healthHandler.Readiness)
		mux.HandleFunc("GET "+apiV1+"/health", deps.healthHandler.Health)
	}

	// Inventory endpoints - using the real handlers
	mux.HandleFunc("GET "+apiV1+"/inventory/{id}", deps.inventoryHandler.GetInventory)
	mux.HandleFunc("GET "+apiV1+"/inventory", deps.inventoryHandler.ListInventory)
	mux.HandleFunc("POST "+apiV1+"/inventory", deps.inventoryHandler.CreateInventory)
	mux.HandleFunc("PUT "+apiV1+"/inventory/{id}", deps.inventoryHandler.UpdateInventory)
	mux.HandleFunc("DELETE "+apiV1+"/inventory/{id}", deps.inventoryHandler.DeleteInventory)

	// Import endpoints
	mux.HandleFunc("POST "+apiV1+"/import/pdf", deps.importHandler.ImportPDF)
	mux.HandleFunc("POST "+apiV1+"/import/excel", deps.importHandler.ImportExcel)
	mux.HandleFunc("POST "+apiV1+"/import/batch", deps.importHandler.ImportBatch)
	mux.HandleFunc("GET "+apiV1+"/import/status/{jobId}", deps.importHandler.ImportStatus)

	// Export endpoints
	mux.HandleFunc("GET "+apiV1+"/export/excel", deps.exportHandler.ExportExcel)
	mux.HandleFunc("GET "+apiV1+"/export/json", deps.exportHandler.ExportJSON)
	mux.HandleFunc("GET "+apiV1+"/export/pdf", deps.exportHandler.ExportPDF)

	// Dashboard endpoints
	mux.HandleFunc("GET "+apiV1+"/dashboard", deps.dashboardHandler.GetDashboard)
	mux.HandleFunc("GET "+apiV1+"/dashboard/analytics", deps.dashboardHandler.GetAnalytics)

	// Platform listing endpoints (placeholder handlers for now)
	mux.HandleFunc("GET "+apiV1+"/platforms/{platform}/listings", handlePlatformListings)
	mux.HandleFunc("POST "+apiV1+"/platforms/{platform}/list", handleCreateListing)
	mux.HandleFunc("PUT "+apiV1+"/platforms/{platform}/listings/{id}", handleUpdateListing)

	// Search endpoint
	mux.HandleFunc("GET "+apiV1+"/search", handleSearch)

	// File serving with wildcard
	mux.HandleFunc("GET "+apiV1+"/files/{path...}", handleFiles)

	// Metrics endpoint
	if cfg.Server.EnableMetrics {
		// mux.Handle("GET /metrics", promhttp.Handler())
	}

	// pprof endpoints (development only)
	if cfg.Server.EnablePprof && cfg.IsDevelopment() {
		mux.HandleFunc("GET /debug/pprof/", http.HandlerFunc(http.DefaultServeMux.ServeHTTP))
	}
}

// Placeholder handlers for unimplemented endpoints
func handlePlatformListings(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Listings for platform %s"}`, platform)
}

func handleCreateListing(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Create listing on %s"}`, platform)
}

func handleUpdateListing(w http.ResponseWriter, r *http.Request) {
	platform := r.PathValue("platform")
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Update listing %s on %s"}`, id, platform)
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Search results for: %s"}`, query)
}

func handleFiles(w http.ResponseWriter, r *http.Request) {
	path := r.PathValue("path")
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"message": "Serving file: %s"}`, path)
}

func runMigrations(ctx context.Context, cfg *config.Config, logger *slog.Logger) error {
	logger.Info("running database migrations")

	migrationConfig := &db.MigrationConfig{
		DatabaseURL: cfg.GetDatabaseURL(),
		SourcePath:  cfg.Database.MigrationPath,
		TableName:   "schema_migrations",
		SchemaName:  "public",
	}

	return db.RunMigrationsWithRetry(ctx, migrationConfig, logger, 3)
}
