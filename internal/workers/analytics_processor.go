// internal/workers/analytics_processor.go
package workers

import (
	"log/slog"

	"github.com/ammerola/resell-be/internal/adapters/db"
)

// AnalyticsProcessor handles analytics refresh tasks
type AnalyticsProcessor struct {
	db     *db.Database
	logger *slog.Logger
}

// NewAnalyticsProcessor creates a new analytics processor
func NewAnalyticsProcessor(db *db.Database, logger *slog.Logger) *AnalyticsProcessor {
	return &AnalyticsProcessor{
		db:     db,
		logger: logger.With(slog.String("processor", "analytics")),
	}
}
