// internal/adapters/db/postgres.go
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
)

// Config holds database configuration
type Config struct {
	Host               string
	Port               string
	User               string
	Password           string
	Database           string
	SSLMode            string
	MaxConnections     int32
	MinConnections     int32
	MaxConnLifetime    time.Duration
	MaxConnIdleTime    time.Duration
	HealthCheckPeriod  time.Duration
	ConnectTimeout     time.Duration
	StatementCacheMode string
	EnableQueryLogging bool
}

// DefaultConfig returns default database configuration
func DefaultConfig() *Config {
	return &Config{
		Host:               "localhost",
		Port:               "5432",
		User:               "resell",
		Password:           "resell_dev_2025",
		Database:           "resell_inventory",
		SSLMode:            "disable",
		MaxConnections:     25,
		MinConnections:     5,
		MaxConnLifetime:    time.Hour,
		MaxConnIdleTime:    time.Minute * 30,
		HealthCheckPeriod:  time.Minute,
		ConnectTimeout:     time.Second * 10,
		StatementCacheMode: "describe",
		EnableQueryLogging: false,
	}
}

// Database wraps pgxpool with additional functionality
type Database struct {
	pool   *pgxpool.Pool
	config *Config
	logger *slog.Logger
}

// NewDatabase creates a new database connection pool
func NewDatabase(ctx context.Context, config *Config, logger *slog.Logger) (*Database, error) {
	if config == nil {
		config = DefaultConfig()
	}

	poolConfig, err := buildPoolConfig(config, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build pool config: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test the connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &Database{
		pool:   pool,
		config: config,
		logger: logger,
	}

	logger.Info("database connection established",
		slog.String("host", config.Host),
		slog.String("database", config.Database),
		slog.Int("max_connections", int(config.MaxConnections)),
	)

	return db, nil
}

// buildPoolConfig creates pgxpool configuration
func buildPoolConfig(config *Config, logger *slog.Logger) (*pgxpool.Config, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s connect_timeout=%d",
		config.Host, config.Port, config.User, config.Password,
		config.Database, config.SSLMode, int(config.ConnectTimeout.Seconds()),
	)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Connection pool settings
	poolConfig.MaxConns = config.MaxConnections
	poolConfig.MinConns = config.MinConnections
	poolConfig.MaxConnLifetime = config.MaxConnLifetime
	poolConfig.MaxConnIdleTime = config.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = config.HealthCheckPeriod

	// Connection settings
	poolConfig.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
	poolConfig.ConnConfig.StatementCacheCapacity = 512

	// Setup logging if enabled
	if config.EnableQueryLogging {
		poolConfig.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   newPgxLogger(logger),
			LogLevel: tracelog.LogLevelDebug,
		}
	}

	// Before connect callback for setting up prepared statements
	poolConfig.BeforeConnect = func(ctx context.Context, cfg *pgx.ConnConfig) error {
		cfg.DefaultQueryExecMode = pgx.QueryExecModeCacheDescribe
		return nil
	}

	// After connect callback for connection setup
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		// Register any custom types here if needed
		return nil
	}

	return poolConfig, nil
}

// Pool returns the underlying pgxpool.Pool
func (db *Database) Pool() *pgxpool.Pool {
	return db.pool
}

// Close closes all database connections
func (db *Database) Close() {
	db.pool.Close()
	db.logger.Info("database connections closed")
}

// Ping verifies database connectivity
func (db *Database) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// Health returns database health information
func (db *Database) Health(ctx context.Context) map[string]interface{} {
	stats := db.pool.Stat()
	health := map[string]interface{}{
		"status":               "healthy",
		"total_connections":    stats.TotalConns(),
		"idle_connections":     stats.IdleConns(),
		"acquired_connections": stats.AcquiredConns(),
		"max_connections":      stats.MaxConns(),
		"new_connections":      stats.NewConnsCount(),
		"max_lifetime_closed":  stats.MaxLifetimeDestroyCount(),
		"idle_closed":          stats.EmptyAcquireCount(),
	}

	// Try a simple query
	ctx, cancel := context.WithTimeout(ctx, time.Second*2)
	defer cancel()

	var result int
	err := db.pool.QueryRow(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		health["status"] = "unhealthy"
		health["error"] = err.Error()
	}

	return health
}

// Transaction executes a function within a database transaction
func (db *Database) Transaction(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx failed: %v, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// TransactionWithOptions executes a function within a transaction with custom options
func (db *Database) TransactionWithOptions(ctx context.Context, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	tx, err := db.pool.BeginTx(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return fmt.Errorf("tx failed: %v, rollback failed: %w", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Batch creates a new batch for efficient bulk operations
func (db *Database) Batch() *pgx.Batch {
	return &pgx.Batch{}
}

// SendBatch sends a batch of queries to the database
func (db *Database) SendBatch(ctx context.Context, batch *pgx.Batch) pgx.BatchResults {
	return db.pool.SendBatch(ctx, batch)
}

// CopyFrom performs a bulk copy operation
func (db *Database) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rows pgx.CopyFromSource) (int64, error) {
	return db.pool.CopyFrom(ctx, tableName, columnNames, rows)
}

// Listen starts listening for PostgreSQL notifications
func (db *Database) Listen(ctx context.Context, channel string) (*pgxpool.Conn, error) {
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection for LISTEN: %w", err)
	}

	_, err = conn.Exec(ctx, "LISTEN "+channel)
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("failed to LISTEN on channel %s: %w", channel, err)
	}

	return conn, nil
}

// WaitForNotification waits for a PostgreSQL notification
func (db *Database) WaitForNotification(ctx context.Context, conn *pgxpool.Conn) (*pgconn.Notification, error) {
	return conn.Conn().WaitForNotification(ctx)
}

// Query executes a query that returns rows
func (db *Database) Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error) {
	return db.pool.Query(ctx, sql, args...)
}

// QueryRow executes a query that returns at most one row
func (db *Database) QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row {
	return db.pool.QueryRow(ctx, sql, args...)
}

// Exec executes a query that doesn't return rows
func (db *Database) Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error) {
	return db.pool.Exec(ctx, sql, args...)
}

// pgxLogger adapts slog for pgx logging
type pgxLogger struct {
	logger *slog.Logger
}

func newPgxLogger(logger *slog.Logger) *pgxLogger {
	return &pgxLogger{
		logger: logger.With(slog.String("component", "pgx")),
	}
}

func (l *pgxLogger) Log(ctx context.Context, level tracelog.LogLevel, msg string, data map[string]interface{}) {
	attrs := make([]slog.Attr, 0, len(data))
	for k, v := range data {
		attrs = append(attrs, slog.Any(k, v))
	}

	switch level {
	case tracelog.LogLevelError:
		l.logger.LogAttrs(ctx, slog.LevelError, msg, attrs...)
	case tracelog.LogLevelWarn:
		l.logger.LogAttrs(ctx, slog.LevelWarn, msg, attrs...)
	case tracelog.LogLevelInfo:
		l.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
	default:
		l.logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
	}
}

// Helper functions for scanning

// ScanOne is a helper to scan a single row into a struct
func ScanOne[T any](row pgx.Row, scanner func(pgx.Row) (*T, error)) (*T, error) {
	entity, err := scanner(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return entity, nil
}

// ScanMany is a helper to scan multiple rows into a slice of structs
func ScanMany[T any](rows pgx.Rows, scanner func(pgx.Rows) (*T, error)) ([]*T, error) {
	defer rows.Close()

	var results []*T
	for rows.Next() {
		entity, err := scanner(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Exists checks if a record exists
func (db *Database) Exists(ctx context.Context, query string, args ...interface{}) (bool, error) {
	var exists bool
	err := db.QueryRow(ctx, "SELECT EXISTS("+query+")", args...).Scan(&exists)
	return exists, err
}

// Count returns the count of records
func (db *Database) Count(ctx context.Context, query string, args ...interface{}) (int64, error) {
	var count int64
	err := db.QueryRow(ctx, "SELECT COUNT(*) FROM ("+query+") AS c", args...).Scan(&count)
	return count, err
}
