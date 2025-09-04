// internal/workers/excel_processor.go
package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/shopspring/decimal"
	"github.com/tealeg/xlsx/v3"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/services"
)

// ExcelProcessor handles Excel import tasks
type ExcelProcessor struct {
	service *services.InventoryService
	db      *db.Database
	logger  *slog.Logger
}

// NewExcelProcessor creates a new Excel processor
func NewExcelProcessor(service *services.InventoryService, db *db.Database, logger *slog.Logger) *ExcelProcessor {
	return &ExcelProcessor{
		service: service,
		db:      db,
		logger:  logger.With(slog.String("processor", "excel")),
	}
}

// ProcessExcel processes an Excel file and imports inventory items
func (p *ExcelProcessor) ProcessExcel(ctx context.Context, t *asynq.Task) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	jobID := payload["job_id"].(string)
	filePath := payload["file_path"].(string)

	p.logger.InfoContext(ctx, "processing Excel file",
		slog.String("job_id", jobID),
		slog.String("file_path", filePath))

	// Open Excel file
	file, err := xlsx.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to open Excel file: %w", err)
	}

	var items []domain.InventoryItem

	// Process first sheet
	if len(file.Sheets) > 0 {
		sheet := file.Sheets[0]
		rowIdx := 0

		err = sheet.ForEachRow(func(r *xlsx.Row) error {
			// Skip header row
			if rowIdx == 0 {
				rowIdx++
				return nil
			}
			rowIdx++

			item := p.parseRow(r)
			if item != nil {
				items = append(items, *item)
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to process Excel rows: %w", err)
		}
	}

	// Save items
	if len(items) > 0 {
		if err := p.service.SaveItems(ctx, items); err != nil {
			return fmt.Errorf("failed to save items: %w", err)
		}
	}

	// Clean up temp file
	if strings.HasPrefix(filePath, "/tmp/") {
		os.Remove(filePath)
	}

	p.logger.InfoContext(ctx, "Excel processing completed",
		slog.String("job_id", jobID),
		slog.Int("items_processed", len(items)))

	return nil
}

func (p *ExcelProcessor) parseRow(r *xlsx.Row) *domain.InventoryItem {
	get := func(i int) string {
		c := r.GetCell(i)
		if c == nil {
			return ""
		}
		return strings.TrimSpace(c.String())
	}

	getDecimal := func(i int) decimal.Decimal {
		s := get(i)
		if s == "" {
			return decimal.Zero
		}
		d, _ := decimal.NewFromString(strings.TrimPrefix(s, "$"))
		return d
	}

	// Parse required fields
	itemName := get(3) // Assuming column D is item name
	if itemName == "" {
		return nil
	}

	return &domain.InventoryItem{
		LotID:           uuid.New(),
		InvoiceID:       get(0),
		ItemName:        itemName,
		Description:     get(4),
		Category:        domain.ItemCategory(strings.ToLower(get(5))),
		Condition:       domain.ItemCondition(strings.ToLower(strings.ReplaceAll(get(6), " ", "_"))),
		Quantity:        1,
		BidAmount:       getDecimal(7),
		BuyersPremium:   getDecimal(8),
		SalesTax:        getDecimal(9),
		ShippingCost:    getDecimal(10),
		AcquisitionDate: time.Now(),
	}
}

// RefreshAnalytics refreshes analytics materialized views
func (p *AnalyticsProcessor) RefreshAnalytics(ctx context.Context, t *asynq.Task) error {
	p.logger.InfoContext(ctx, "refreshing analytics")

	// Refresh materialized view
	query := `REFRESH MATERIALIZED VIEW CONCURRENTLY inventory_excel_export_mat`

	if _, err := p.db.Exec(ctx, query); err != nil {
		return fmt.Errorf("failed to refresh materialized view: %w", err)
	}

	p.logger.InfoContext(ctx, "analytics refreshed successfully")
	return nil
}

// GenerateReport generates analytics reports
func (p *AnalyticsProcessor) GenerateReport(ctx context.Context, t *asynq.Task) error {
	var payload map[string]interface{}
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	reportType := payload["type"].(string)

	p.logger.InfoContext(ctx, "generating report",
		slog.String("type", reportType))

	// Report generation logic would go here
	switch reportType {
	case "monthly":
		// Generate monthly report
	case "quarterly":
		// Generate quarterly report
	case "annual":
		// Generate annual report
	}

	p.logger.InfoContext(ctx, "report generated successfully")
	return nil
}
