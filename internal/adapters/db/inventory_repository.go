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
}

// NewInventoryRepository creates a new inventory repository
func NewInventoryRepository(db *Database, logger *slog.Logger) ports.InventoryRepository {
	return &inventoryRepository{
		db:     db,
		logger: logger.With(slog.String("repository", "inventory")),
	}
}

// Save creates a new inventory item
func (r *inventoryRepository) Save(ctx context.Context, item *domain.InventoryItem) error {
	query := `
		INSERT INTO inventory (
			lot_id, invoice_id, auction_id, item_name, description,
			category, subcategory, condition, quantity, 
			bid_amount, buyers_premium, sales_tax, shipping_cost,
			acquisition_date, storage_location, storage_bin, qr_code,
			estimated_value, market_demand, seasonality_notes,
			needs_repair, is_consignment, is_returned,
			keywords, notes, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			$21, $22, $23, $24, $25, $26, $27
		) RETURNING lot_id, total_cost, cost_per_item, created_at, updated_at`

	// Prepare keywords as comma-separated string
	keywordsStr := strings.Join(item.Keywords, ",")

	// Prepare values
	args := []interface{}{
		item.LotID, item.InvoiceID, item.AuctionID, item.ItemName, item.Description,
		item.Category, item.Subcategory, item.Condition, item.Quantity,
		item.BidAmount, item.BuyersPremium, item.SalesTax, item.ShippingCost,
		item.AcquisitionDate, item.StorageLocation, item.StorageBin, item.QRCode,
		item.EstimatedValue, item.MarketDemand, item.SeasonalityNotes,
		item.NeedsRepair, item.IsConsignment, item.IsReturned,
		keywordsStr, item.Notes, item.CreatedAt, item.UpdatedAt,
	}

	err := r.db.QueryRow(ctx, query, args...).Scan(
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

// SaveBatch saves multiple inventory items in a transaction
func (r *inventoryRepository) SaveBatch(ctx context.Context, items []domain.InventoryItem) error {
	if len(items) == 0 {
		return nil
	}

	return r.db.Transaction(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}

		query := `
			INSERT INTO inventory (
				lot_id, invoice_id, auction_id, item_name, description,
				category, subcategory, condition, quantity, 
				bid_amount, buyers_premium, sales_tax, shipping_cost,
				acquisition_date, storage_location, storage_bin, qr_code,
				estimated_value, market_demand, seasonality_notes,
				needs_repair, is_consignment, is_returned,
				keywords, notes, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 
				$11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
				$21, $22, $23, $24, $25, $26, $27
			) RETURNING lot_id, total_cost, cost_per_item`

		for i := range items {
			keywordsStr := strings.Join(items[i].Keywords, ",")

			batch.Queue(query,
				items[i].LotID, items[i].InvoiceID, items[i].AuctionID, items[i].ItemName, items[i].Description,
				items[i].Category, items[i].Subcategory, items[i].Condition, items[i].Quantity,
				items[i].BidAmount, items[i].BuyersPremium, items[i].SalesTax, items[i].ShippingCost,
				items[i].AcquisitionDate, items[i].StorageLocation, items[i].StorageBin, items[i].QRCode,
				items[i].EstimatedValue, items[i].MarketDemand, items[i].SeasonalityNotes,
				items[i].NeedsRepair, items[i].IsConsignment, items[i].IsReturned,
				keywordsStr, items[i].Notes, items[i].CreatedAt, items[i].UpdatedAt,
			)
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
	query := `
		UPDATE inventory SET
			invoice_id = $2, auction_id = $3, item_name = $4, description = $5,
			category = $6, subcategory = $7, condition = $8, quantity = $9,
			bid_amount = $10, buyers_premium = $11, sales_tax = $12, shipping_cost = $13,
			acquisition_date = $14, storage_location = $15, storage_bin = $16, qr_code = $17,
			estimated_value = $18, market_demand = $19, seasonality_notes = $20,
			needs_repair = $21, is_consignment = $22, is_returned = $23,
			keywords = $24, notes = $25, updated_at = $26
		WHERE lot_id = $1 AND deleted_at IS NULL
		RETURNING total_cost, cost_per_item`

	keywordsStr := strings.Join(item.Keywords, ",")
	item.UpdatedAt = time.Now()

	args := []interface{}{
		item.LotID, item.InvoiceID, item.AuctionID, item.ItemName, item.Description,
		item.Category, item.Subcategory, item.Condition, item.Quantity,
		item.BidAmount, item.BuyersPremium, item.SalesTax, item.ShippingCost,
		item.AcquisitionDate, item.StorageLocation, item.StorageBin, item.QRCode,
		item.EstimatedValue, item.MarketDemand, item.SeasonalityNotes,
		item.NeedsRepair, item.IsConsignment, item.IsReturned,
		keywordsStr, item.Notes, item.UpdatedAt,
	}

	err := r.db.QueryRow(ctx, query, args...).Scan(
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

// FindByID retrieves an inventory item by ID
func (r *inventoryRepository) FindByID(ctx context.Context, lotID uuid.UUID) (*domain.InventoryItem, error) {
	query := `
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
		WHERE lot_id = $1 AND deleted_at IS NULL`

	item := &domain.InventoryItem{}
	var keywordsStr sql.NullString
	var subcategory sql.NullString
	var storageLocation, storageBin, qrCode sql.NullString
	var estimatedValue pgtype.Numeric
	var seasonalityNotes sql.NullString
	var notes sql.NullString

	err := r.db.QueryRow(ctx, query, lotID).Scan(
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
		return nil, fmt.Errorf("failed to find inventory item: %w", err)
	}

	// Handle nullable fields
	item.Subcategory = subcategory.String
	item.StorageLocation = storageLocation.String
	item.StorageBin = storageBin.String
	item.QRCode = qrCode.String
	item.SeasonalityNotes = seasonalityNotes.String
	item.Notes = notes.String

	if estimatedValue.Valid {
		if v, err := estimatedValue.Value(); err == nil && v != nil {
			switch t := v.(type) {
			case string:
				if d, err := decimal.NewFromString(t); err == nil {
					item.EstimatedValue = &d
				}
			case []byte:
				if d, err := decimal.NewFromString(string(t)); err == nil {
					item.EstimatedValue = &d
				}
			case float64:
				d := decimal.NewFromFloat(t)
				item.EstimatedValue = &d
			case int64:
				d := decimal.NewFromInt(t)
				item.EstimatedValue = &d
			default:
				if d, err := decimal.NewFromString(fmt.Sprint(v)); err == nil {
					item.EstimatedValue = &d
				}
			}
		}
	}

	if keywordsStr.Valid && keywordsStr.String != "" {
		item.Keywords = strings.Split(keywordsStr.String, ",")
	}

	return item, nil
}

// FindByInvoiceID retrieves all items for a specific invoice
func (r *inventoryRepository) FindByInvoiceID(ctx context.Context, invoiceID string) ([]domain.InventoryItem, error) {
	query := `
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
		WHERE invoice_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := r.db.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query inventory items: %w", err)
	}
	defer rows.Close()

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

		if estimatedValue.Valid {
			if v, err := estimatedValue.Value(); err == nil && v != nil {
				switch t := v.(type) {
				case string:
					if d, err := decimal.NewFromString(t); err == nil {
						item.EstimatedValue = &d
					}
				case []byte:
					if d, err := decimal.NewFromString(string(t)); err == nil {
						item.EstimatedValue = &d
					}
				case float64:
					d := decimal.NewFromFloat(t)
					item.EstimatedValue = &d
				case int64:
					d := decimal.NewFromInt(t)
					item.EstimatedValue = &d
				default:
					if d, err := decimal.NewFromString(fmt.Sprint(v)); err == nil {
						item.EstimatedValue = &d
					}
				}
			}
		}

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

// Delete performs a hard delete
func (r *inventoryRepository) Delete(ctx context.Context, lotID uuid.UUID) error {
	query := `DELETE FROM inventory WHERE lot_id = $1`

	tag, err := r.db.Exec(ctx, query, lotID)
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

// SoftDelete marks an item as deleted
func (r *inventoryRepository) SoftDelete(ctx context.Context, lotID uuid.UUID) error {
	query := `UPDATE inventory SET deleted_at = $2, updated_at = $2 WHERE lot_id = $1 AND deleted_at IS NULL`

	now := time.Now()
	tag, err := r.db.Exec(ctx, query, lotID, now)
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

// Count returns the total number of inventory items
func (r *inventoryRepository) Count(ctx context.Context) (int64, error) {
	query := `SELECT COUNT(*) FROM inventory WHERE deleted_at IS NULL`

	var count int64
	err := r.db.QueryRow(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count inventory items: %w", err)
	}

	return count, nil
}

// Exists checks if an inventory item exists
func (r *inventoryRepository) Exists(ctx context.Context, lotID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM inventory WHERE lot_id = $1 AND deleted_at IS NULL)`

	var exists bool
	err := r.db.QueryRow(ctx, query, lotID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return exists, nil
}

// FindAll retrieves inventory items with filtering and pagination
func (r *inventoryRepository) FindAll(ctx context.Context, params InventoryQueryParams) ([]*domain.InventoryItem, int64, error) {
	// Build base query
	qb := squirrel.Select(
		"lot_id", "invoice_id", "auction_id", "item_name", "description",
		"category", "subcategory", "condition", "quantity",
		"bid_amount", "buyers_premium", "sales_tax", "shipping_cost",
		"total_cost", "cost_per_item", "acquisition_date",
		"storage_location", "storage_bin", "qr_code",
		"estimated_value", "market_demand", "seasonality_notes",
		"needs_repair", "is_consignment", "is_returned",
		"keywords", "notes", "created_at", "updated_at",
	).From("inventory").
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar)

	// Apply filters
	if params.Search != "" {
		qb = qb.Where("search_vector @@ plainto_tsquery('english', ?)", params.Search)
	}
	if params.Category != "" {
		qb = qb.Where(squirrel.Eq{"category": params.Category})
	}
	if params.Condition != "" {
		qb = qb.Where(squirrel.Eq{"condition": params.Condition})
	}
	if params.StorageLocation != "" {
		qb = qb.Where(squirrel.Eq{"storage_location": params.StorageLocation})
	}
	if params.StorageBin != "" {
		qb = qb.Where(squirrel.Eq{"storage_bin": params.StorageBin})
	}
	if params.InvoiceID != "" {
		qb = qb.Where(squirrel.Eq{"invoice_id": params.InvoiceID})
	}
	if params.NeedsRepair != nil {
		qb = qb.Where(squirrel.Eq{"needs_repair": *params.NeedsRepair})
	}

	// Count total items (before pagination)
	countQb := qb.Column("COUNT(*) OVER()").Limit(1)
	countSQL, countArgs, err := countQb.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build count query: %w", err)
	}

	var totalCount int64
	err = r.db.QueryRow(ctx, countSQL, countArgs...).Scan(&totalCount)
	if err != nil && err != pgx.ErrNoRows {
		return nil, 0, fmt.Errorf("failed to count inventory items: %w", err)
	}

	// Apply sorting
	orderBy := "created_at DESC"
	if params.SortBy != "" {
		direction := "ASC"
		if params.SortOrder == "desc" {
			direction = "DESC"
		}

		switch params.SortBy {
		case "name":
			orderBy = fmt.Sprintf("item_name %s", direction)
		case "acquisition_date":
			orderBy = fmt.Sprintf("acquisition_date %s", direction)
		case "value":
			orderBy = fmt.Sprintf("total_cost %s", direction)
		case "updated":
			orderBy = fmt.Sprintf("updated_at %s", direction)
		default:
			orderBy = fmt.Sprintf("created_at %s", direction)
		}
	}
	qb = qb.OrderBy(orderBy)

	// Apply pagination
	if params.Limit > 0 {
		qb = qb.Limit(uint64(params.Limit))
	}
	if params.Offset > 0 {
		qb = qb.Offset(uint64(params.Offset))
	}

	// Execute query
	sql_query, args, err := qb.ToSql()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to build query: %w", err)
	}

	rows, err := r.db.Query(ctx, sql_query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query inventory items: %w", err)
	}
	defer rows.Close()

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
			return nil, 0, fmt.Errorf("failed to scan inventory item: %w", err)
		}

		// Handle nullable fields
		item.Subcategory = subcategory.String
		item.StorageLocation = storageLocation.String
		item.StorageBin = storageBin.String
		item.QRCode = qrCode.String
		item.SeasonalityNotes = seasonalityNotes.String
		item.Notes = notes.String

		if estimatedValue.Valid {
			if v, err := estimatedValue.Value(); err == nil && v != nil {
				switch t := v.(type) {
				case string:
					if d, err := decimal.NewFromString(t); err == nil {
						item.EstimatedValue = &d
					}
				case []byte:
					if d, err := decimal.NewFromString(string(t)); err == nil {
						item.EstimatedValue = &d
					}
				case float64:
					d := decimal.NewFromFloat(t)
					item.EstimatedValue = &d
				case int64:
					d := decimal.NewFromInt(t)
					item.EstimatedValue = &d
				default:
					if d, err := decimal.NewFromString(fmt.Sprint(v)); err == nil {
						item.EstimatedValue = &d
					}
				}
			}
		}

		if keywordsStr.Valid && keywordsStr.String != "" {
			item.Keywords = strings.Split(keywordsStr.String, ",")
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating rows: %w", err)
	}

	return items, totalCount, nil
}

// InventoryQueryParams holds query parameters for listing inventory
type InventoryQueryParams struct {
	Search          string
	Category        string
	Condition       string
	StorageLocation string
	StorageBin      string
	InvoiceID       string
	NeedsRepair     *bool
	SortBy          string
	SortOrder       string
	Limit           int
	Offset          int
}
