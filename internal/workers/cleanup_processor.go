// internal/workers/cleanup_processor.go
package workers

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/pkg/config"
	"github.com/hibiken/asynq"
)

// CleanupProcessor handles cleanup tasks
type CleanupProcessor struct {
	db     *db.Database
	config *config.Config
	logger *slog.Logger
}

// NewCleanupProcessor creates a new cleanup processor
func NewCleanupProcessor(db *db.Database, config *config.Config, logger *slog.Logger) *CleanupProcessor {
	return &CleanupProcessor{
		db:     db,
		config: config,
		logger: logger.With(slog.String("processor", "cleanup")),
	}
}

// CleanupOldData removes old data from the database
func (p *CleanupProcessor) CleanupOldData(ctx context.Context, t *asynq.Task) error {
	p.logger.InfoContext(ctx, "cleaning up old data")

	// Clean up old activity logs (older than 90 days)
	query := `DELETE FROM activity_logs WHERE created_at < NOW() - INTERVAL '90 days'`

	result, err := p.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to cleanup activity logs: %w", err)
	}

	p.logger.InfoContext(ctx, "old data cleaned up",
		slog.Int64("rows_deleted", result.RowsAffected()))

	return nil
}

// CleanupTempFiles removes old temporary files
func (p *CleanupProcessor) CleanupTempFiles(ctx context.Context, t *asynq.Task) error {
	p.logger.InfoContext(ctx, "cleaning up temp files")

	tempDir := p.config.FileProcessing.TempDir
	maxAge := 24 * time.Hour

	var deletedCount int
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && time.Since(info.ModTime()) > maxAge {
			if err := os.Remove(path); err != nil {
				p.logger.WarnContext(ctx, "failed to delete temp file",
					slog.String("file", path),
					err)
			} else {
				deletedCount++
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk temp directory: %w", err)
	}

	p.logger.InfoContext(ctx, "temp files cleaned up",
		slog.Int("files_deleted", deletedCount))

	return nil
}
