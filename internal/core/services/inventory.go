// internal/core/services/inventory.go
package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PgxPool interface defines the contract for database operations needed by the service
type PgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
}

// InventoryService orchestrates business logic for inventory management
// It delegates ALL data access operations to the repository
type InventoryService struct {
	repo   ports.InventoryRepository
	db     PgxPool // Only used for transaction management, not queries
	logger *slog.Logger
}

// Statically assert that *InventoryService implements the InventoryService interface
var _ ports.InventoryService = (*InventoryService)(nil)

// NewInventoryService creates a new inventory service instance
func NewInventoryService(repo ports.InventoryRepository, db PgxPool, logger *slog.Logger) *InventoryService {
	return &InventoryService{
		repo:   repo,
		db:     db,
		logger: logger.With(slog.String("service", "inventory")),
	}
}

// SaveItem validates and saves a single inventory item
func (s *InventoryService) SaveItem(ctx context.Context, item *domain.InventoryItem) error {
	// Business validation
	if err := item.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Prepare item for storage (sets UUID, timestamps, calculates totals)
	item.PrepareForStorage()

	// Delegate to repository for actual persistence
	if err := s.repo.Save(ctx, item); err != nil {
		return fmt.Errorf("failed to save item: %w", err)
	}

	s.logger.InfoContext(ctx, "saved inventory item",
		slog.String("lot_id", item.LotID.String()),
		slog.String("invoice_id", item.InvoiceID),
		slog.String("item_name", item.ItemName))

	return nil
}

// SaveItems saves multiple inventory items in batch
func (s *InventoryService) SaveItems(ctx context.Context, items []domain.InventoryItem) error {
	if len(items) == 0 {
		s.logger.InfoContext(ctx, "no items to save")
		return nil
	}

	// Validate and prepare all items
	for i := range items {
		if err := items[i].Validate(); err != nil {
			return fmt.Errorf("validation failed for item %s: %w", items[i].ItemName, err)
		}
		items[i].PrepareForStorage()
	}

	// Delegate to repository for batch save
	if err := s.repo.SaveBatch(ctx, items); err != nil {
		return fmt.Errorf("failed to save items batch: %w", err)
	}

	s.logger.InfoContext(ctx, "saved inventory items",
		slog.Int("count", len(items)))

	return nil
}

// BulkUpsert performs a bulk upsert operation in batches for efficiency
func (s *InventoryService) BulkUpsert(ctx context.Context, items []domain.InventoryItem) error {
	const batchSize = 100

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		if err := s.SaveItems(ctx, batch); err != nil {
			return fmt.Errorf("failed to save batch %d-%d: %w", i, end, err)
		}

		s.logger.DebugContext(ctx, "bulk upsert batch completed",
			slog.Int("batch_start", i),
			slog.Int("batch_end", end))
	}

	return nil
}

// GetByID retrieves an inventory item by its ID
func (s *InventoryService) GetByID(ctx context.Context, lotID uuid.UUID) (*domain.InventoryItem, error) {
	item, err := s.repo.FindByID(ctx, lotID)
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory item: %w", err)
	}

	if item == nil {
		return nil, fmt.Errorf("inventory item not found: %s", lotID)
	}

	return item, nil
}

// GetByInvoiceID retrieves all items for a specific invoice
func (s *InventoryService) GetByInvoiceID(ctx context.Context, invoiceID string) ([]domain.InventoryItem, error) {
	items, err := s.repo.FindByInvoiceID(ctx, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get items by invoice ID: %w", err)
	}
	return items, nil
}

// UpdateItem updates an existing inventory item
func (s *InventoryService) UpdateItem(ctx context.Context, lotID uuid.UUID, item *domain.InventoryItem) error {
	// Ensure the ID matches
	item.LotID = lotID

	// Validate the item
	if err := item.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Recalculate financial fields
	item.CalculateTotalCost()

	// Delegate to repository
	if err := s.repo.Update(ctx, item); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	s.logger.InfoContext(ctx, "updated inventory item",
		slog.String("lot_id", lotID.String()))

	return nil
}

// DeleteItem deletes an inventory item (soft or permanent)
func (s *InventoryService) DeleteItem(ctx context.Context, lotID uuid.UUID, permanent bool) error {
	// Check if item exists
	exists, err := s.repo.Exists(ctx, lotID)
	if err != nil {
		return fmt.Errorf("failed to check item existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("inventory item not found: %s", lotID)
	}

	// Perform deletion
	if permanent {
		err = s.repo.Delete(ctx, lotID)
	} else {
		err = s.repo.SoftDelete(ctx, lotID)
	}

	if err != nil {
		return fmt.Errorf("failed to delete item: %w", err)
	}

	s.logger.InfoContext(ctx, "deleted inventory item",
		slog.String("lot_id", lotID.String()),
		slog.Bool("permanent", permanent))

	return nil
}

// List retrieves inventory items with filtering and pagination
// This method now simply delegates to the repository, which handles ALL query logic
func (s *InventoryService) List(ctx context.Context, params ports.ListParams) (*ports.ListResult, error) {
	// Validate and normalize parameters
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 50
	}
	if params.PageSize > 1000 {
		params.PageSize = 1000 // Reasonable max to prevent abuse
	}

	// Delegate ALL query logic to the repository
	items, totalCount, err := s.repo.FindAll(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory items: %w", err)
	}

	// Calculate pagination metadata
	totalPages := 0
	if params.PageSize > 0 && totalCount > 0 {
		totalPages = int(totalCount) / params.PageSize
		if int(totalCount)%params.PageSize > 0 {
			totalPages++
		}
	}

	// Build result
	result := &ports.ListResult{
		Items:      items,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	s.logger.DebugContext(ctx, "listed inventory items",
		slog.Int("count", len(items)),
		slog.Int64("total", totalCount),
		slog.Int("page", params.Page))

	return result, nil
}

// GetStatistics returns aggregate statistics about the inventory
// This is a business logic method that could use specialized repository methods
func (s *InventoryService) GetStatistics(ctx context.Context) (*InventoryStatistics, error) {
	totalCount, err := s.repo.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory count: %w", err)
	}

	// In a full implementation, you might have specialized repository methods
	// for getting category counts, total value, etc.
	stats := &InventoryStatistics{
		TotalItems: totalCount,
		// Additional statistics would be populated here
	}

	return stats, nil
}

// InventoryStatistics holds aggregate statistics about the inventory
type InventoryStatistics struct {
	TotalItems       int64          `json:"total_items"`
	TotalValue       string         `json:"total_value"`
	CategoryCounts   map[string]int `json:"category_counts,omitempty"`
	ConditionCounts  map[string]int `json:"condition_counts,omitempty"`
	AverageItemValue string         `json:"average_item_value,omitempty"`
}
