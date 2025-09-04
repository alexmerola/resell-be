// internal/adapters/db/inventory_repository.go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
)

// inventoryRepository implements ports.InventoryRepository
type inventoryRepository struct {
	db     *Database
	logger *slog.Logger
	qb     squirrel.StatementBuilderType // Query builder with PostgreSQL placeholders
}

// NewInventoryRepository creates a new inventory repository with optimized query builder
func NewInventoryRepository(db *Database, logger *slog.Logger) ports.InventoryRepository {
	return &inventoryRepository{
		db:     db,
		logger: logger.With(slog.String("repository", "inventory")),
		qb:     squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar),
	}
}

// Save creates a new inventory item with all fields properly handled
func (r *inventoryRepository) Save(ctx context.Context, item *domain.InventoryItem) error {
	query := r.qb.Insert("inventory").
		Columns(
			"lot_id", "invoice_id", "auction_id", "item_name", "description",
			"category", "subcategory", "condition", "quantity",
			"bid_amount", "buyers_premium", "sales_tax", "shipping_cost",
			"acquisition_date", "storage_location", "storage_bin", "qr_code",
			"estimated_value", "market_demand", "seasonality_notes",
			"needs_repair", "is_consignment", "is_returned",
			"keywords", "notes", "created_at", "updated_at",
		).
		Values(
			item.LotID, item.InvoiceID, item.AuctionID, item.ItemName, item.Description,
			item.Category, item.Subcategory, item.Condition, item.Quantity,
			item.BidAmount, item.BuyersPremium, item.SalesTax, item.ShippingCost,
			item.AcquisitionDate, item.StorageLocation, item.StorageBin, item.QRCode,
			item.EstimatedValue, item.MarketDemand, item.SeasonalityNotes,
			item.NeedsRepair, item.IsConsignment, item.IsReturned,
			strings.Join(item.Keywords, ","), item.Notes, item.CreatedAt, item.UpdatedAt,
		).
		Suffix("RETURNING lot_id, total_cost, cost_per_item, created_at, updated_at")

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert query: %w", err)
	}

	err = r.db.QueryRow(ctx, sql, args...).Scan(
		&item.LotID,
		&item.TotalCost,
		&item.CostPerItem,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save inventory item: %w", err)
	}

	r.logger.DebugContext(ctx, "inventory item saved",
		slog.String("lot_id", item.LotID.String()),
		slog.String("invoice_id", item.InvoiceID))

	return nil
}

// SaveBatch efficiently saves multiple inventory items in a single transaction
func (r *inventoryRepository) SaveBatch(ctx context.Context, items []domain.InventoryItem) error {
	if len(items) == 0 {
		return nil
	}

	return r.db.Transaction(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}

		insertQuery := r.qb.Insert("inventory").
			Columns(
				"lot_id", "invoice_id", "auction_id", "item_name", "description",
				"category", "subcategory", "condition", "quantity",
				"bid_amount", "buyers_premium", "sales_tax", "shipping_cost",
				"acquisition_date", "storage_location", "storage_bin", "qr_code",
				"estimated_value", "market_demand", "seasonality_notes",
				"needs_repair", "is_consignment", "is_returned",
				"keywords", "notes", "created_at", "updated_at",
			).
			Suffix("RETURNING lot_id, total_cost, cost_per_item")

		for i := range items {
			keywordsStr := strings.Join(items[i].Keywords, ",")

			sql, args, err := insertQuery.Values(
				items[i].LotID, items[i].InvoiceID, items[i].AuctionID, items[i].ItemName, items[i].Description,
				items[i].Category, items[i].Subcategory, items[i].Condition, items[i].Quantity,
				items[i].BidAmount, items[i].BuyersPremium, items[i].SalesTax, items[i].ShippingCost,
				items[i].AcquisitionDate, items[i].StorageLocation, items[i].StorageBin, items[i].QRCode,
				items[i].EstimatedValue, items[i].MarketDemand, items[i].SeasonalityNotes,
				items[i].NeedsRepair, items[i].IsConsignment, items[i].IsReturned,
				keywordsStr, items[i].Notes, items[i].CreatedAt, items[i].UpdatedAt,
			).ToSql()

			if err != nil {
				return fmt.Errorf("failed to build batch insert query for item %d: %w", i, err)
			}

			batch.Queue(sql, args...)
		}

		br := tx.SendBatch(ctx, batch)
		defer br.Close()

		for i := range items {
			err := br.QueryRow().Scan(
				&items[i].LotID,
				&items[i].TotalCost,
				&items[i].CostPerItem,
			)
			if err != nil {
				return fmt.Errorf("failed to save item %d: %w", i, err)
			}
		}

		return nil
	})
}

// Update updates an existing inventory item
func (r *inventoryRepository) Update(ctx context.Context, item *domain.InventoryItem) error {
	item.UpdatedAt = time.Now()
	keywordsStr := strings.Join(item.Keywords, ",")

	query := r.qb.Update("inventory").
		Set("invoice_id", item.InvoiceID).
		Set("auction_id", item.AuctionID).
		Set("item_name", item.ItemName).
		Set("description", item.Description).
		Set("category", item.Category).
		Set("subcategory", item.Subcategory).
		Set("condition", item.Condition).
		Set("quantity", item.Quantity).
		Set("bid_amount", item.BidAmount).
		Set("buyers_premium", item.BuyersPremium).
		Set("sales_tax", item.SalesTax).
		Set("shipping_cost", item.ShippingCost).
		Set("acquisition_date", item.AcquisitionDate).
		Set("storage_location", item.StorageLocation).
		Set("storage_bin", item.StorageBin).
		Set("qr_code", item.QRCode).
		Set("estimated_value", item.EstimatedValue).
		Set("market_demand", item.MarketDemand).
		Set("seasonality_notes", item.SeasonalityNotes).
		Set("needs_repair", item.NeedsRepair).
		Set("is_consignment", item.IsConsignment).
		Set("is_returned", item.IsReturned).
		Set("keywords", keywordsStr).
		Set("notes", item.Notes).
		Set("updated_at", item.UpdatedAt).
		Where(squirrel.Eq{"lot_id": item.LotID}).
		Where("deleted_at IS NULL").
		Suffix("RETURNING total_cost, cost_per_item")

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	err = r.db.QueryRow(ctx, sql, args...).Scan(
		&item.TotalCost,
		&item.CostPerItem,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("inventory item not found: %s", item.LotID)
		}
		return fmt.Errorf("failed to update inventory item: %w", err)
	}

	r.logger.DebugContext(ctx, "inventory item updated",
		slog.String("lot_id", item.LotID.String()))

	return nil
}

// FindByID retrieves a single inventory item by ID
func (r *inventoryRepository) FindByID(ctx context.Context, lotID uuid.UUID) (*domain.InventoryItem, error) {
	query := r.qb.Select(r.inventoryColumns()...).
		From("inventory").
		Where(squirrel.Eq{"lot_id": lotID}).
		Where("deleted_at IS NULL")

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	row := r.db.QueryRow(ctx, sql, args...)
	return r.scanInventoryItem(row)
}

// FindByInvoiceID retrieves all items for a specific invoice
func (r *inventoryRepository) FindByInvoiceID(ctx context.Context, invoiceID string) ([]domain.InventoryItem, error) {
	query := r.qb.Select(r.inventoryColumns()...).
		From("inventory").
		Where(squirrel.Eq{"invoice_id": invoiceID}).
		Where("deleted_at IS NULL").
		OrderBy("created_at DESC")

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query inventory items: %w", err)
	}
	defer rows.Close()

	return r.scanInventoryItems(rows)
}

// FindAll retrieves inventory items with comprehensive filtering, sorting, and pagination
// This is the SINGLE source of truth for inventory queries - all filtering logic lives here
func (r *inventoryRepository) FindAll(ctx context.Context, params ports.ListParams) ([]*domain.InventoryItem, int64, error) {
	// Build the base query with all columns
	baseQuery := r.qb.Select(r.inventoryColumns()...).
		From("inventory").
		Where("deleted_at IS NULL")

	// Apply search filter using PostgreSQL's full-text search
	if params.Search != "" {
		baseQuery = baseQuery.Where(
			"search_vector @@ plainto_tsquery('english', ?)",
			params.Search,
		)
	}

	// Apply category filter
	if params.Category != "" {
		baseQuery = baseQuery.Where(squirrel.Eq{"category": params.Category})
	}

	// Apply condition filter
	if params.Condition != "" {
		baseQuery = baseQuery.Where(squirrel.Eq{"condition": params.Condition})
	}

	// Apply storage location filter
	if params.StorageLocation != "" {
		baseQuery = baseQuery.Where(squirrel.Eq{"storage_location": params.StorageLocation})
	}

	// Apply storage bin filter
	if params.StorageBin != "" {
		baseQuery = baseQuery.Where(squirrel.Eq{"storage_bin": params.StorageBin})
	}

	// Apply invoice ID filter
	if params.InvoiceID != "" {
		baseQuery = baseQuery.Where(squirrel.Eq{"invoice_id": params.InvoiceID})
	}

	// Apply needs repair filter
	if params.NeedsRepair != nil {
		baseQuery = baseQuery.Where(squirrel.Eq{"needs_repair": *params.NeedsRepair})
	}

	// First, get the total count before pagination
	countQuery := r.qb.Select("COUNT(*)").
		From("inventory").
		Where("deleted_at IS NULL")

	// Apply the same filters to the count query
	if params.Search != "" {
		countQuery = countQuery.Where(
			"search_vector @@ plainto_tsquery('english', ?)",
			params.Search,
		)
	}
	if params.Category != "" {
		countQuery = countQuery.Where(squirrel.Eq{"category": params.Category})
	}
	if params.Condition != "" {
		countQuery = countQuery.Where(squirrel.Eq{"condition": params.Condition})
	}
	if params.StorageLocation != "" {
		countQuery = countQuery.Where(squirrel.Eq{"storage_location": params.StorageLocation})
	}
	if params.StorageBin != "" {
		countQuery = countQuery.Where(squirrel.Eq{"storage_bin": params.StorageBin})
	}
	if params.InvoiceID != "" {
		countQuery = countQuery.Where(squirrel.Eq{"invoice_id": params.InvoiceID})
	}
	if params.NeedsRepair != nil {
		countQuery = countQuery.Where(squirrel.Eq{"needs_repair": *params.NeedsRepair})
	}

	// Execute count query
	countSQL, countArgs, err := countQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var totalCount int64
	err = r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&totalCount)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count inventory items: %w", err)
	}

	// Apply sorting
	orderBy := r.buildOrderBy(params.SortBy, params.SortOrder)
	baseQuery = baseQuery.OrderBy(orderBy)

	// Apply pagination
	if params.PageSize > 0 {
		offset := (params.Page - 1) * params.PageSize
		baseQuery = baseQuery.Limit(uint64(params.PageSize)).Offset(uint64(offset))
	}

	// Execute main query
	sql, args, err := baseQuery.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing inventory query",
		slog.String("sql", sql),
		slog.Any("args", args))

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query inventory items: %w", err)
	}
	defer rows.Close()

	items, err := r.scanInventoryItemPointers(rows)
	if err != nil {
		return nil, 0, err
	}

	return items, totalCount, nil
}

// Delete performs a hard delete of an inventory item
func (r *inventoryRepository) Delete(ctx context.Context, lotID uuid.UUID) error {
	query := r.qb.Delete("inventory").
		Where(squirrel.Eq{"lot_id": lotID})

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to delete inventory item: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inventory item not found: %s", lotID)
	}

	r.logger.InfoContext(ctx, "inventory item deleted",
		slog.String("lot_id", lotID.String()))

	return nil
}

// SoftDelete marks an item as deleted without removing it from the database
func (r *inventoryRepository) SoftDelete(ctx context.Context, lotID uuid.UUID) error {
	now := time.Now()

	query := r.qb.Update("inventory").
		Set("deleted_at", now).
		Set("updated_at", now).
		Where(squirrel.Eq{"lot_id": lotID}).
		Where("deleted_at IS NULL")

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build soft delete query: %w", err)
	}

	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to soft delete inventory item: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inventory item not found: %s", lotID)
	}

	r.logger.InfoContext(ctx, "inventory item soft deleted",
		slog.String("lot_id", lotID.String()))

	return nil
}

// Count returns the total number of non-deleted inventory items
func (r *inventoryRepository) Count(ctx context.Context) (int64, error) {
	query := r.qb.Select("COUNT(*)").
		From("inventory").
		Where("deleted_at IS NULL")

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var count int64
	err = r.db.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count inventory items: %w", err)
	}

	return count, nil
}

// Exists checks if an inventory item exists
func (r *inventoryRepository) Exists(ctx context.Context, lotID uuid.UUID) (bool, error) {
	query := r.qb.Select("1").
		From("inventory").
		Where(squirrel.Eq{"lot_id": lotID}).
		Where("deleted_at IS NULL").
		Limit(1)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build exists query: %w", err)
	}

	var exists int
	err = r.db.QueryRow(ctx, sql, args...).Scan(&exists)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return true, nil
}

// Helper methods

// inventoryColumns returns the standard set of columns to select
func (r *inventoryRepository) inventoryColumns() []string {
	return []string{
		"lot_id", "invoice_id", "auction_id", "item_name", "description",
		"category", "subcategory", "condition", "quantity",
		"bid_amount", "buyers_premium", "sales_tax", "shipping_cost",
		"total_cost", "cost_per_item", "acquisition_date",
		"storage_location", "storage_bin", "qr_code",
		"estimated_value", "market_demand", "seasonality_notes",
		"needs_repair", "is_consignment", "is_returned",
		"keywords", "notes", "created_at", "updated_at",
	}
}

// buildOrderBy constructs the ORDER BY clause based on sort parameters
func (r *inventoryRepository) buildOrderBy(sortBy, sortOrder string) string {
	// Default sorting
	if sortBy == "" {
		sortBy = "created_at"
	}
	if sortOrder == "" {
		sortOrder = "desc"
	}

	// Validate sort order
	if sortOrder != "asc" && sortOrder != "desc" {
		sortOrder = "desc"
	}

	// Map user-friendly sort fields to database columns
	var column string
	switch sortBy {
	case "name":
		column = "item_name"
	case "acquisition_date", "acquisition":
		column = "acquisition_date"
	case "value", "total_cost", "cost":
		column = "total_cost"
	case "updated", "updated_at":
		column = "updated_at"
	case "created", "created_at":
		column = "created_at"
	case "category":
		column = "category"
	case "condition":
		column = "condition"
	case "quantity":
		column = "quantity"
	default:
		column = "created_at"
	}

	return fmt.Sprintf("%s %s NULLS LAST", column, strings.ToUpper(sortOrder))
}

// scanInventoryItem scans a single row into an InventoryItem
func (r *inventoryRepository) scanInventoryItem(row pgx.Row) (*domain.InventoryItem, error) {
	item := &domain.InventoryItem{}
	var keywordsStr sql.NullString
	var subcategory sql.NullString
	var storageLocation, storageBin, qrCode sql.NullString
	var estimatedValue pgtype.Numeric
	var seasonalityNotes sql.NullString
	var notes sql.NullString

	err := row.Scan(
		&item.LotID, &item.InvoiceID, &item.AuctionID, &item.ItemName, &item.Description,
		&item.Category, &subcategory, &item.Condition, &item.Quantity,
		&item.BidAmount, &item.BuyersPremium, &item.SalesTax, &item.ShippingCost,
		&item.TotalCost, &item.CostPerItem, &item.AcquisitionDate,
		&storageLocation, &storageBin, &qrCode,
		&estimatedValue, &item.MarketDemand, &seasonalityNotes,
		&item.NeedsRepair, &item.IsConsignment, &item.IsReturned,
		&keywordsStr, &notes, &item.CreatedAt, &item.UpdatedAt,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan inventory item: %w", err)
	}

	// Handle nullable fields
	item.Subcategory = subcategory.String
	item.StorageLocation = storageLocation.String
	item.StorageBin = storageBin.String
	item.QRCode = qrCode.String
	item.SeasonalityNotes = seasonalityNotes.String
	item.Notes = notes.String

	// Handle estimated value conversion
	if estimatedValue.Valid {
		if v, err := estimatedValue.Value(); err == nil && v != nil {
			item.EstimatedValue = r.convertToDecimal(v)
		}
	}

	// Parse keywords
	if keywordsStr.Valid && keywordsStr.String != "" {
		item.Keywords = strings.Split(keywordsStr.String, ",")
	}

	return item, nil
}

// scanInventoryItems scans multiple rows into a slice of InventoryItems
func (r *inventoryRepository) scanInventoryItems(rows pgx.Rows) ([]domain.InventoryItem, error) {
	var items []domain.InventoryItem

	for rows.Next() {
		item := domain.InventoryItem{}
		var keywordsStr, subcategory sql.NullString
		var storageLocation, storageBin, qrCode sql.NullString
		var estimatedValue pgtype.Numeric
		var seasonalityNotes, notes sql.NullString

		err := rows.Scan(
			&item.LotID, &item.InvoiceID, &item.AuctionID, &item.ItemName, &item.Description,
			&item.Category, &subcategory, &item.Condition, &item.Quantity,
			&item.BidAmount, &item.BuyersPremium, &item.SalesTax, &item.ShippingCost,
			&item.TotalCost, &item.CostPerItem, &item.AcquisitionDate,
			&storageLocation, &storageBin, &qrCode,
			&estimatedValue, &item.MarketDemand, &seasonalityNotes,
			&item.NeedsRepair, &item.IsConsignment, &item.IsReturned,
			&keywordsStr, &notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inventory item: %w", err)
		}

		// Handle nullable fields
		item.Subcategory = subcategory.String
		item.StorageLocation = storageLocation.String
		item.StorageBin = storageBin.String
		item.QRCode = qrCode.String
		item.SeasonalityNotes = seasonalityNotes.String
		item.Notes = notes.String

		// Handle estimated value
		if estimatedValue.Valid {
			if v, err := estimatedValue.Value(); err == nil && v != nil {
				item.EstimatedValue = r.convertToDecimal(v)
			}
		}

		// Parse keywords
		if keywordsStr.Valid && keywordsStr.String != "" {
			item.Keywords = strings.Split(keywordsStr.String, ",")
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return items, nil
}

// scanInventoryItemPointers scans multiple rows into a slice of InventoryItem pointers
func (r *inventoryRepository) scanInventoryItemPointers(rows pgx.Rows) ([]*domain.InventoryItem, error) {
	var items []*domain.InventoryItem

	for rows.Next() {
		item := &domain.InventoryItem{}
		var keywordsStr, subcategory sql.NullString
		var storageLocation, storageBin, qrCode sql.NullString
		var estimatedValue pgtype.Numeric
		var seasonalityNotes, notes sql.NullString

		err := rows.Scan(
			&item.LotID, &item.InvoiceID, &item.AuctionID, &item.ItemName, &item.Description,
			&item.Category, &subcategory, &item.Condition, &item.Quantity,
			&item.BidAmount, &item.BuyersPremium, &item.SalesTax, &item.ShippingCost,
			&item.TotalCost, &item.CostPerItem, &item.AcquisitionDate,
			&storageLocation, &storageBin, &qrCode,
			&estimatedValue, &item.MarketDemand, &seasonalityNotes,
			&item.NeedsRepair, &item.IsConsignment, &item.IsReturned,
			&keywordsStr, &notes, &item.CreatedAt, &item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan inventory item: %w", err)
		}

		// Handle nullable fields
		item.Subcategory = subcategory.String
		item.StorageLocation = storageLocation.String
		item.StorageBin = storageBin.String
		item.QRCode = qrCode.String
		item.SeasonalityNotes = seasonalityNotes.String
		item.Notes = notes.String

		// Handle estimated value
		if estimatedValue.Valid {
			if v, err := estimatedValue.Value(); err == nil && v != nil {
				item.EstimatedValue = r.convertToDecimal(v)
			}
		}

		// Parse keywords
		if keywordsStr.Valid && keywordsStr.String != "" {
			item.Keywords = strings.Split(keywordsStr.String, ",")
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return items, nil
}

// convertToDecimal converts various types to decimal.Decimal
func (r *inventoryRepository) convertToDecimal(v interface{}) *decimal.Decimal {
	var d decimal.Decimal
	var err error

	switch t := v.(type) {
	case string:
		d, err = decimal.NewFromString(t)
	case []byte:
		d, err = decimal.NewFromString(string(t))
	case float64:
		d = decimal.NewFromFloat(t)
	case int64:
		d = decimal.NewFromInt(t)
	default:
		// Try to convert via string representation as last resort
		d, err = decimal.NewFromString(fmt.Sprint(v))
	}

	if err != nil {
		return nil
	}

	return &d
}
