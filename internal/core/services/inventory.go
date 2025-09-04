// internal/core/services/inventory.go
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// PgxPool interface defines the contract for database operations
type PgxPool interface {
	Begin(ctx context.Context) (pgx.Tx, error)
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// InventoryService handles inventory business logic
type InventoryService struct {
	repo   ports.InventoryRepository
	db     PgxPool
	logger *slog.Logger
}

// Statically assert that *InventoryService implements the InventoryService interface.
var _ ports.InventoryService = (*InventoryService)(nil)

// NewInventoryService creates a new inventory service
func NewInventoryService(repo ports.InventoryRepository, db PgxPool, logger *slog.Logger) *InventoryService {
	return &InventoryService{
		repo:   repo,
		db:     db,
		logger: logger.With(slog.String("service", "inventory")),
	}
}

// SaveItems saves multiple inventory items with transaction support
func (s *InventoryService) SaveItems(ctx context.Context, items []domain.InventoryItem) error {
	if len(items) == 0 {
		s.logger.InfoContext(ctx, "no items to save")
		return nil
	}

	// Validate all items first
	for i := range items {
		if err := items[i].Validate(); err != nil {
			return fmt.Errorf("validation failed for item %s: %w", items[i].ItemName, err)
		}
		items[i].PrepareForStorage()
	}

	// Use repository's batch save
	if err := s.repo.SaveBatch(ctx, items); err != nil {
		return fmt.Errorf("failed to save items batch: %w", err)
	}

	s.logger.InfoContext(ctx, "saved inventory items",
		slog.Int("count", len(items)))

	return nil
}

// SaveItem saves a single inventory item
func (s *InventoryService) SaveItem(ctx context.Context, item *domain.InventoryItem) error {
	if err := item.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	item.PrepareForStorage()

	if err := s.repo.Save(ctx, item); err != nil {
		return fmt.Errorf("failed to save item: %w", err)
	}

	s.logger.InfoContext(ctx, "saved inventory item",
		slog.String("lot_id", item.LotID.String()),
		slog.String("invoice_id", item.InvoiceID),
		slog.String("item_name", item.ItemName))

	return nil
}

// BulkUpsert performs a bulk upsert operation with optimizations
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
	}

	return nil
}

// GetByID retrieves an inventory item by ID
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

	// Recalculate costs
	item.CalculateTotalCost()

	if err := s.repo.Update(ctx, item); err != nil {
		return fmt.Errorf("failed to update item: %w", err)
	}

	s.logger.InfoContext(ctx, "updated inventory item",
		slog.String("lot_id", lotID.String()))

	return nil
}

// DeleteItem deletes an inventory item (soft delete by default)
func (s *InventoryService) DeleteItem(ctx context.Context, lotID uuid.UUID, permanent bool) error {
	// Check if item exists
	exists, err := s.repo.Exists(ctx, lotID)
	if err != nil {
		return fmt.Errorf("failed to check item existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("inventory item not found: %s", lotID)
	}

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
func (s *InventoryService) List(ctx context.Context, params ports.ListParams) (*ports.ListResult, error) {
	items, totalCount, err := s.getFilteredItems(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list inventory items: %w", err)
	}

	// Calculate total pages
	var totalPages int
	if params.PageSize > 0 {
		totalPages = int(totalCount) / params.PageSize
		if int(totalCount)%params.PageSize > 0 {
			totalPages++
		}
	}

	result := &ports.ListResult{
		Items:      items,
		Page:       params.Page,
		PageSize:   params.PageSize,
		TotalCount: totalCount,
		TotalPages: totalPages,
	}

	return result, nil
}

// getFilteredItems is a helper method that queries the database directly
func (s *InventoryService) getFilteredItems(ctx context.Context, params ports.ListParams) ([]*domain.InventoryItem, int64, error) {
	// Build query with filters
	baseQuery := `
		SELECT 
			lot_id, invoice_id, auction_id, item_name, description,
			category, subcategory, condition, quantity,
			bid_amount, buyers_premium, sales_tax, shipping_cost,
			total_cost, cost_per_item, acquisition_date,
			storage_location, storage_bin, qr_code,
			estimated_value, market_demand, seasonality_notes,
			needs_repair, is_consignment, is_returned,
			keywords, notes, created_at, updated_at
		FROM inventory
		WHERE deleted_at IS NULL
	`

	// Add filters dynamically
	var conditions []string
	var args []interface{}
	argCount := 1

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf("search_vector @@ plainto_tsquery('english', $%d)", argCount))
		args = append(args, params.Search)
		argCount++
	}

	if params.Category != "" {
		conditions = append(conditions, fmt.Sprintf("category = $%d", argCount))
		args = append(args, params.Category)
		argCount++
	}

	if params.Condition != "" {
		conditions = append(conditions, fmt.Sprintf("condition = $%d", argCount))
		args = append(args, params.Condition)
		argCount++
	}

	if params.InvoiceID != "" {
		conditions = append(conditions, fmt.Sprintf("invoice_id = $%d", argCount))
		args = append(args, params.InvoiceID)
		argCount++
	}

	if params.NeedsRepair != nil {
		conditions = append(conditions, fmt.Sprintf("needs_repair = $%d", argCount))
		args = append(args, *params.NeedsRepair)
		argCount++
	}

	// Build final query
	if len(conditions) > 0 {
		baseQuery += " AND " + strings.Join(conditions, " AND ")
	}

	// Get count
	countQuery := "SELECT COUNT(*) FROM (" + baseQuery + ") as t"
	var totalCount int64
	err := s.db.QueryRow(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	// Add ordering and pagination
	orderBy := "created_at DESC"
	if params.SortBy != "" {
		direction := "ASC"
		if params.SortOrder == "desc" {
			direction = "DESC"
		}
		orderBy = fmt.Sprintf("%s %s", params.SortBy, direction)
	}

	baseQuery += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, argCount, argCount+1)
	args = append(args, params.PageSize, (params.Page-1)*params.PageSize)

	// Execute query
	rows, err := s.db.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []*domain.InventoryItem
	for rows.Next() {
		item := &domain.InventoryItem{}
		var keywordsStr, subcategory, storageLocation, storageBin, qrCode, seasonalityNotes, notes *string

		err := rows.Scan(
			&item.LotID, &item.InvoiceID, &item.AuctionID, &item.ItemName, &item.Description,
			&item.Category, &subcategory, &item.Condition, &item.Quantity,
			&item.BidAmount, &item.BuyersPremium, &item.SalesTax, &item.ShippingCost,
			&item.TotalCost, &item.CostPerItem, &item.AcquisitionDate,
			&storageLocation, &storageBin, &qrCode,
			&item.EstimatedValue, &item.MarketDemand, &seasonalityNotes,
			&item.NeedsRepair, &item.IsConsignment, &item.IsReturned,
			&keywordsStr, &notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, 0, err
		}

		// Handle nullable fields
		if subcategory != nil {
			item.Subcategory = *subcategory
		}
		if storageLocation != nil {
			item.StorageLocation = *storageLocation
		}
		if storageBin != nil {
			item.StorageBin = *storageBin
		}
		if qrCode != nil {
			item.QRCode = *qrCode
		}
		if seasonalityNotes != nil {
			item.SeasonalityNotes = *seasonalityNotes
		}
		if notes != nil {
			item.Notes = *notes
		}
		if keywordsStr != nil && *keywordsStr != "" {
			item.Keywords = strings.Split(*keywordsStr, ",")
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return items, totalCount, nil
}
