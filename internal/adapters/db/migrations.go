// internal/adapters/db/migrations.go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// MigrationConfig holds migration configuration
type MigrationConfig struct {
	DatabaseURL      string
	SourcePath       string
	EmbeddedSource   embed.FS
	UseEmbedded      bool
	TableName        string
	SchemaName       string
	ForceDirty       bool
	LockTimeout      time.Duration
	StatementTimeout time.Duration
}

// Migrator handles database migrations
type Migrator struct {
	migrate *migrate.Migrate
	config  *MigrationConfig
	logger  *slog.Logger
	db      *sql.DB
}

// NewMigrator creates a new migrator instance
func NewMigrator(config *MigrationConfig, logger *slog.Logger) (*Migrator, error) {
	if config == nil {
		return nil, fmt.Errorf("migration config is required")
	}

	// Set defaults
	if config.TableName == "" {
		config.TableName = "schema_migrations"
	}
	if config.SchemaName == "" {
		config.SchemaName = "public"
	}
	if config.LockTimeout == 0 {
		config.LockTimeout = time.Minute * 5
	}
	if config.StatementTimeout == 0 {
		config.StatementTimeout = time.Minute * 10
	}

	// Open database connection using pgx stdlib
	db, err := sql.Open("pgx", config.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Create postgres driver instance with configuration
	pgConfig := &postgres.Config{
		MigrationsTable:  config.TableName,
		SchemaName:       config.SchemaName,
		StatementTimeout: config.StatementTimeout,
	}

	driver, err := postgres.WithInstance(db, pgConfig)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create postgres driver: %w", err)
	}

	// Create source driver
	var sourceDriver source.Driver
	if config.UseEmbedded {
		d, err := iofs.New(config.EmbeddedSource, "migrations")
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create embedded source driver: %w", err)
		}
		sourceDriver = d
	} else {
		// Use file source
		m, err := migrate.New(
			"file://"+config.SourcePath,
			config.DatabaseURL,
		)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create file source migration: %w", err)
		}

		return &Migrator{
			migrate: m,
			config:  config,
			logger:  logger,
			db:      db,
		}, nil
	}

	// Create migration instance with embedded source
	if config.UseEmbedded {
		m, err := migrate.NewWithInstance(
			"iofs", sourceDriver,
			"postgres", driver,
		)
		if err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to create migration instance: %w", err)
		}

		return &Migrator{
			migrate: m,
			config:  config,
			logger:  logger,
			db:      db,
		}, nil
	}

	return nil, fmt.Errorf("unreachable code")
}

// Up runs all available migrations
func (m *Migrator) Up(ctx context.Context) error {
	m.logger.InfoContext(ctx, "running migrations up")

	// Check if migrations are needed
	version, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if dirty && m.config.ForceDirty {
		m.logger.WarnContext(ctx, "forcing dirty migration",
			slog.Uint64("version", uint64(version)))
		if err := m.migrate.Force(int(version)); err != nil {
			return fmt.Errorf("failed to force version: %w", err)
		}
	}

	// Run migrations
	if err := m.migrate.Up(); err != nil {
		if err == migrate.ErrNoChange {
			m.logger.InfoContext(ctx, "no migrations to run")
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Get new version
	newVersion, _, err := m.migrate.Version()
	if err != nil {
		m.logger.WarnContext(ctx, "failed to get new version", "err", err)
	} else {
		m.logger.InfoContext(ctx, "migrations completed",
			slog.Uint64("version", uint64(newVersion)))
	}

	return nil
}

// Down rolls back last migration
func (m *Migrator) Down(ctx context.Context) error {
	m.logger.InfoContext(ctx, "rolling back last migration")

	version, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state at version %d", version)
	}

	if err := m.migrate.Steps(-1); err != nil {
		if err == migrate.ErrNoChange {
			m.logger.InfoContext(ctx, "no migrations to rollback")
			return nil
		}
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	newVersion, _, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		m.logger.WarnContext(ctx, "failed to get new version", "err", err)
	} else {
		m.logger.InfoContext(ctx, "migration rolled back",
			slog.Uint64("from_version", uint64(version)),
			slog.Uint64("to_version", uint64(newVersion)))
	}

	return nil
}

// DownTo rolls back to a specific version
func (m *Migrator) DownTo(ctx context.Context, targetVersion uint) error {
	m.logger.InfoContext(ctx, "rolling back to version",
		slog.Uint64("target_version", uint64(targetVersion)))

	currentVersion, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state at version %d", currentVersion)
	}

	if uint(currentVersion) <= targetVersion {
		m.logger.InfoContext(ctx, "already at or below target version")
		return nil
	}

	if err := m.migrate.Migrate(targetVersion); err != nil {
		return fmt.Errorf("failed to migrate to version %d: %w", targetVersion, err)
	}

	m.logger.InfoContext(ctx, "migrated to target version",
		slog.Uint64("from_version", uint64(currentVersion)),
		slog.Uint64("to_version", uint64(targetVersion)))

	return nil
}

// Migrate runs migrations to a specific version (up or down)
func (m *Migrator) Migrate(ctx context.Context, targetVersion uint) error {
	m.logger.InfoContext(ctx, "migrating to version",
		slog.Uint64("target_version", uint64(targetVersion)))

	if err := m.migrate.Migrate(targetVersion); err != nil {
		if err == migrate.ErrNoChange {
			m.logger.InfoContext(ctx, "already at target version")
			return nil
		}
		return fmt.Errorf("failed to migrate to version %d: %w", targetVersion, err)
	}

	return nil
}

// Force sets the version without running migrations
func (m *Migrator) Force(ctx context.Context, version int) error {
	m.logger.WarnContext(ctx, "forcing migration version",
		slog.Int("version", version))

	if err := m.migrate.Force(version); err != nil {
		return fmt.Errorf("failed to force version: %w", err)
	}

	return nil
}

// Version returns current migration version
func (m *Migrator) Version(ctx context.Context) (uint, bool, error) {
	version, dirty, err := m.migrate.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			m.logger.InfoContext(ctx, "no migrations applied yet")
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get version: %w", err)
	}

	m.logger.InfoContext(ctx, "current migration version",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty))

	return version, dirty, nil
}

// Drop drops the entire database schema
func (m *Migrator) Drop(ctx context.Context) error {
	m.logger.WarnContext(ctx, "dropping all migrations")

	if err := m.migrate.Drop(); err != nil {
		return fmt.Errorf("failed to drop migrations: %w", err)
	}

	m.logger.InfoContext(ctx, "all migrations dropped")
	return nil
}

// Status returns the status of all migrations
func (m *Migrator) Status(ctx context.Context) (*MigrationStatus, error) {
	version, dirty, err := m.migrate.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}

	status := &MigrationStatus{
		CurrentVersion: version,
		IsDirty:        dirty,
		Applied:        make([]AppliedMigration, 0),
		Pending:        make([]PendingMigration, 0),
	}

	// Query applied migrations from database
	query := fmt.Sprintf(`
		SELECT version, dirty
		FROM %s.%s
		ORDER BY version ASC
	`, m.config.SchemaName, m.config.TableName)

	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query migrations: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var applied AppliedMigration
		if err := rows.Scan(&applied.Version, &applied.Dirty); err != nil {
			return nil, fmt.Errorf("failed to scan migration: %w", err)
		}
		status.Applied = append(status.Applied, applied)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate migrations: %w", err)
	}

	// Note: Getting pending migrations would require parsing the source
	// This is complex and depends on the source driver implementation

	return status, nil
}

// Close closes the migrator and releases resources
func (m *Migrator) Close() error {
	if m.migrate != nil {
		sourceErr, dbErr := m.migrate.Close()
		if sourceErr != nil || dbErr != nil {
			return fmt.Errorf("failed to close migrator - source: %v, db: %v", sourceErr, dbErr)
		}
	}

	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	m.logger.Info("migrator closed")
	return nil
}

// MigrationStatus represents the current status of migrations
type MigrationStatus struct {
	CurrentVersion uint               `json:"current_version"`
	IsDirty        bool               `json:"is_dirty"`
	Applied        []AppliedMigration `json:"applied"`
	Pending        []PendingMigration `json:"pending"`
}

// AppliedMigration represents an applied migration
type AppliedMigration struct {
	Version uint `json:"version"`
	Dirty   bool `json:"dirty"`
}

// PendingMigration represents a pending migration
type PendingMigration struct {
	Version     uint   `json:"version"`
	Description string `json:"description"`
}

// RunMigrationsWithRetry runs migrations with retry logic
func RunMigrationsWithRetry(ctx context.Context, config *MigrationConfig, logger *slog.Logger, maxRetries int) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if i > 0 {
			waitTime := time.Duration(i) * time.Second * 2
			logger.InfoContext(ctx, "retrying migration",
				slog.Int("attempt", i+1),
				slog.Duration("wait", waitTime))
			time.Sleep(waitTime)
		}

		migrator, err := NewMigrator(config, logger)
		if err != nil {
			lastErr = fmt.Errorf("failed to create migrator: %w", err)
			logger.ErrorContext(ctx, "failed to create migrator",
				"err", err,
				slog.Int("attempt", i+1))
			continue
		}

		err = migrator.Up(ctx)
		closeErr := migrator.Close()

		if err == nil && closeErr == nil {
			return nil
		}

		if err != nil {
			lastErr = err
			logger.ErrorContext(ctx, "migration failed",
				"err", err,
				slog.Int("attempt", i+1))
		}
		if closeErr != nil {
			logger.ErrorContext(ctx, "failed to close migrator",
				"closeErr", closeErr)
		}
	}

	return fmt.Errorf("migrations failed after %d attempts: %w", maxRetries, lastErr)
}

// ValidateMigrations validates migration files
func ValidateMigrations(sourcePath string) error {
	// This would validate that:
	// 1. Migration files are properly named
	// 2. Up and down migrations exist for each version
	// 3. SQL syntax is valid (basic check)
	// 4. No version gaps exist

	// Implementation would depend on specific validation requirements
	return nil
}

// MigrationHook is a function that runs before or after a migration
type MigrationHook func(ctx context.Context, version uint, direction string) error

// MigrationHooks holds before and after hooks for migrations
type MigrationHooks struct {
	BeforeUp   MigrationHook
	AfterUp    MigrationHook
	BeforeDown MigrationHook
	AfterDown  MigrationHook
}

// MigratorWithHooks wraps a migrator with hooks
type MigratorWithHooks struct {
	*Migrator
	hooks MigrationHooks
}

// NewMigratorWithHooks creates a migrator with hooks
func NewMigratorWithHooks(config *MigrationConfig, logger *slog.Logger, hooks MigrationHooks) (*MigratorWithHooks, error) {
	migrator, err := NewMigrator(config, logger)
	if err != nil {
		return nil, err
	}

	return &MigratorWithHooks{
		Migrator: migrator,
		hooks:    hooks,
	}, nil
}

// Up runs migrations up with hooks
func (m *MigratorWithHooks) Up(ctx context.Context) error {
	version, _, _ := m.Version(ctx)

	if m.hooks.BeforeUp != nil {
		if err := m.hooks.BeforeUp(ctx, version, "up"); err != nil {
			return fmt.Errorf("before up hook failed: %w", err)
		}
	}

	if err := m.Migrator.Up(ctx); err != nil {
		return err
	}

	if m.hooks.AfterUp != nil {
		newVersion, _, _ := m.Version(ctx)
		if err := m.hooks.AfterUp(ctx, newVersion, "up"); err != nil {
			return fmt.Errorf("after up hook failed: %w", err)
		}
	}

	return nil
}

// Down runs migrations down with hooks
func (m *MigratorWithHooks) Down(ctx context.Context) error {
	version, _, _ := m.Version(ctx)

	if m.hooks.BeforeDown != nil {
		if err := m.hooks.BeforeDown(ctx, version, "down"); err != nil {
			return fmt.Errorf("before down hook failed: %w", err)
		}
	}

	if err := m.Migrator.Down(ctx); err != nil {
		return err
	}

	if m.hooks.AfterDown != nil {
		newVersion, _, _ := m.Version(ctx)
		if err := m.hooks.AfterDown(ctx, newVersion, "down"); err != nil {
			return fmt.Errorf("after down hook failed: %w", err)
		}
	}

	return nil
}
