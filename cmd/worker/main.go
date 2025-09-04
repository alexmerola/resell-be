// cmd/worker/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/core/services"
	"github.com/ammerola/resell-be/internal/pkg/config"
	"github.com/ammerola/resell-be/internal/pkg/logger"
	"github.com/ammerola/resell-be/internal/workers"
	"github.com/hibiken/asynq"
)

func main() {
	// Setup logger
	slogger := logger.SetupLogger("info", "json")

	// Load configuration
	cfg, err := config.Load(slogger)
	if err != nil {
		slogger.Error("failed to load configuration", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Reconfigure logger with loaded settings
	slogger = logger.SetupLogger(cfg.App.LogLevel, cfg.App.LogFormat)
	slogger.Info("starting worker",
		slog.String("environment", cfg.App.Environment),
		slog.String("redis_addr", cfg.Asynq.RedisAddr))

	// Initialize database
	ctx := context.Background()
	database, err := initDatabase(ctx, cfg, slogger)
	if err != nil {
		slogger.Error("failed to initialize database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer database.Close()

	// Initialize repositories and services
	inventoryRepo := db.NewInventoryRepository(database, slogger)
	inventoryService := services.NewInventoryService(inventoryRepo, database.Pool(), slogger)

	// Create Asynq server
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.Asynq.RedisAddr,
			Password: cfg.Asynq.RedisPassword,
			DB:       cfg.Asynq.RedisDB,
		},
		asynq.Config{
			Concurrency:     cfg.Asynq.Concurrency,
			Queues:          cfg.Asynq.Queues,
			StrictPriority:  cfg.Asynq.StrictPriority,
			ErrorHandler:    asynq.ErrorHandlerFunc(handleError),
			RetryDelayFunc:  exponentialBackoff,
			ShutdownTimeout: cfg.Asynq.ShutdownTimeout,
			HealthCheckFunc: healthCheck,
			Logger:          newAsynqLogger(slogger),
		},
	)

	// Create task handlers
	mux := asynq.NewServeMux()

	// Register PDF processing handler
	pdfProcessor := workers.NewPDFProcessor(inventoryService, database, slogger)
	mux.HandleFunc(workers.TypePDFProcess, pdfProcessor.ProcessPDF)

	// Register Excel processing handler
	excelProcessor := workers.NewExcelProcessor(inventoryService, database, slogger)
	mux.HandleFunc(workers.TypeExcelImport, excelProcessor.ProcessExcel)

	// Register analytics handler
	analyticsProcessor := workers.NewAnalyticsProcessor(database, slogger)
	mux.HandleFunc(workers.TypeRefreshAnalytics, analyticsProcessor.RefreshAnalytics)
	mux.HandleFunc(workers.TypeGenerateReport, analyticsProcessor.GenerateReport)

	// Register email notification handler
	notificationProcessor := workers.NewNotificationProcessor(cfg, slogger)
	mux.HandleFunc(workers.TypeSendEmail, notificationProcessor.SendEmail)

	// Register cleanup handler
	cleanupProcessor := workers.NewCleanupProcessor(database, cfg, slogger)
	mux.HandleFunc(workers.TypeCleanupOldData, cleanupProcessor.CleanupOldData)
	mux.HandleFunc(workers.TypeCleanupTempFiles, cleanupProcessor.CleanupTempFiles)

	// Handle shutdown gracefully
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Run(mux); err != nil {
			slogger.Error("failed to run worker server", slog.String("error", err.Error()))
			shutdown <- syscall.SIGTERM
		}
	}()

	slogger.Info("worker started successfully",
		slog.Int("concurrency", cfg.Asynq.Concurrency),
		slog.Any("queues", cfg.Asynq.Queues))

	// Wait for shutdown signal
	sig := <-shutdown
	slogger.Info("shutdown signal received", slog.String("signal", sig.String()))

	// Gracefully shutdown
	srv.Shutdown()
	slogger.Info("worker shutdown complete")
}

func initDatabase(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*db.Database, error) {
	dbConfig := &db.Config{
		Host:               cfg.Database.Host,
		Port:               cfg.Database.Port,
		User:               cfg.Database.User,
		Password:           cfg.Database.Password,
		Database:           cfg.Database.Name,
		SSLMode:            cfg.Database.SSLMode,
		MaxConnections:     10, // Fewer connections for worker
		MinConnections:     2,
		MaxConnLifetime:    cfg.Database.MaxConnLifetime,
		MaxConnIdleTime:    cfg.Database.MaxConnIdleTime,
		HealthCheckPeriod:  cfg.Database.HealthCheckPeriod,
		ConnectTimeout:     cfg.Database.ConnectTimeout,
		StatementCacheMode: cfg.Database.StatementCacheMode,
		EnableQueryLogging: cfg.Database.EnableQueryLogging,
	}

	return db.NewDatabase(ctx, dbConfig, logger)
}

func handleError(ctx context.Context, task *asynq.Task, err error) {
	slog.ErrorContext(ctx, "task processing failed",
		slog.String("type", task.Type()),
		slog.String("payload", string(task.Payload())),
		slog.String("error", err.Error()))
}

func exponentialBackoff(n int, e error, t *asynq.Task) time.Duration {
	baseDelay := time.Second
	maxDelay := 10 * time.Minute
	delay := baseDelay * time.Duration(1<<uint(n))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func healthCheck(err error) {
	if err != nil {
		slog.Error("worker health check failed", slog.String("error", err.Error()))
	}
}

// asynqLogger adapts slog for Asynq
type asynqLogger struct {
	logger *slog.Logger
}

func newAsynqLogger(logger *slog.Logger) *asynqLogger {
	return &asynqLogger{
		logger: logger.With(slog.String("component", "asynq")),
	}
}

func (l *asynqLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
	os.Exit(1)
}
