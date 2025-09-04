// internal/handlers/inventory.go
package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
)

// InventoryHandler handles inventory-related HTTP requests
type InventoryHandler struct {
	service ports.InventoryService
	logger  *slog.Logger
}

// NewInventoryHandler creates a new inventory handler
func NewInventoryHandler(service ports.InventoryService, logger *slog.Logger) *InventoryHandler {
	return &InventoryHandler{
		service: service,
		logger:  logger.With(slog.String("handler", "inventory")),
	}
}

// GetInventory handles GET /api/v1/inventory/{id}
func (h *InventoryHandler) GetInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	// Parse UUID
	lotID, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid inventory ID format")
		return
	}

	// Get inventory item
	item, err := h.service.GetByID(ctx, lotID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to get inventory item",
			slog.String("lot_id", idStr),
			slog.String("error", err.Error()))

		if err.Error() == "inventory item not found: "+idStr {
			h.respondError(w, http.StatusNotFound, "Inventory item not found")
			return
		}

		h.respondError(w, http.StatusInternalServerError, "Failed to retrieve inventory item")
		return
	}

	h.respondJSON(w, http.StatusOK, item)
}

// ListInventory handles GET /api/v1/inventory
func (h *InventoryHandler) ListInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	params := h.parseListParams(r)

	// List inventory items
	result, err := h.service.List(ctx, params)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to list inventory items",
			slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to list inventory items")
		return
	}

	h.respondJSON(w, http.StatusOK, result)
}

// CreateInventory handles POST /api/v1/inventory
func (h *InventoryHandler) CreateInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req CreateInventoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if err := req.Validate(); err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert to domain model
	item := req.ToDomain()

	// Save inventory item
	if err := h.service.SaveItem(ctx, item); err != nil {
		h.logger.ErrorContext(ctx, "failed to create inventory item",
			slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to create inventory item")
		return
	}

	h.logger.InfoContext(ctx, "inventory item created",
		slog.String("lot_id", item.LotID.String()),
		slog.String("item_name", item.ItemName))

	h.respondJSON(w, http.StatusCreated, item)
}

// UpdateInventory handles PUT /api/v1/inventory/{id}
func (h *InventoryHandler) UpdateInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	// Parse UUID
	lotID, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid inventory ID format")
		return
	}

	// Parse request body
	var req UpdateInventoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if err := req.Validate(); err != nil {
		h.respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Convert to domain model
	item := req.ToDomain()

	// Update inventory item
	if err := h.service.UpdateItem(ctx, lotID, item); err != nil {
		h.logger.ErrorContext(ctx, "failed to update inventory item",
			slog.String("lot_id", idStr),
			slog.String("error", err.Error()))

		if err.Error() == "inventory item not found: "+idStr {
			h.respondError(w, http.StatusNotFound, "Inventory item not found")
			return
		}

		h.respondError(w, http.StatusInternalServerError, "Failed to update inventory item")
		return
	}

	// Retrieve updated item
	updatedItem, err := h.service.GetByID(ctx, lotID)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to retrieve updated item",
			slog.String("lot_id", idStr),
			slog.String("error", err.Error()))
		// Still return success even if we can't retrieve the updated item
		h.respondJSON(w, http.StatusOK, map[string]string{"message": "Inventory item updated successfully"})
		return
	}

	h.logger.InfoContext(ctx, "inventory item updated",
		slog.String("lot_id", idStr))

	h.respondJSON(w, http.StatusOK, updatedItem)
}

// DeleteInventory handles DELETE /api/v1/inventory/{id}
func (h *InventoryHandler) DeleteInventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")

	// Parse UUID
	lotID, err := uuid.Parse(idStr)
	if err != nil {
		h.respondError(w, http.StatusBadRequest, "Invalid inventory ID format")
		return
	}

	// Check for permanent delete flag
	permanent := r.URL.Query().Get("permanent") == "true"

	// Delete inventory item
	if err := h.service.DeleteItem(ctx, lotID, permanent); err != nil {
		h.logger.ErrorContext(ctx, "failed to delete inventory item",
			slog.String("lot_id", idStr),
			slog.Bool("permanent", permanent),
			slog.String("error", err.Error()))

		if err.Error() == "inventory item not found: "+idStr {
			h.respondError(w, http.StatusNotFound, "Inventory item not found")
			return
		}

		h.respondError(w, http.StatusInternalServerError, "Failed to delete inventory item")
		return
	}

	h.logger.InfoContext(ctx, "inventory item deleted",
		slog.String("lot_id", idStr),
		slog.Bool("permanent", permanent))

	h.respondJSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Inventory item deleted successfully",
		"lot_id":    idStr,
		"permanent": permanent,
	})
}

// parseListParams parses query parameters for listing inventory
func (h *InventoryHandler) parseListParams(r *http.Request) ports.ListParams {
	params := ports.ListParams{
		Page:      1,
		PageSize:  50,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	// Parse pagination
	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			params.Page = p
		}
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		if l, err := strconv.Atoi(limit); err == nil && l > 0 {
			if l > 100 {
				params.PageSize = 100
			} else {
				params.PageSize = l
			}
		}
	}

	// Parse filters
	params.Search = r.URL.Query().Get("search")
	params.Category = r.URL.Query().Get("category")
	params.Condition = r.URL.Query().Get("condition")
	params.StorageLocation = r.URL.Query().Get("storage_location")
	params.StorageBin = r.URL.Query().Get("storage_bin")
	params.InvoiceID = r.URL.Query().Get("invoice_id")

	if needsRepair := r.URL.Query().Get("needs_repair"); needsRepair != "" {
		if val, err := strconv.ParseBool(needsRepair); err == nil {
			params.NeedsRepair = &val
		}
	}

	// Parse sorting
	if sortBy := r.URL.Query().Get("sort"); sortBy != "" {
		params.SortBy = sortBy
	}

	if order := r.URL.Query().Get("order"); order == "asc" || order == "desc" {
		params.SortOrder = order
	}

	return params
}

// Helper methods

func (h *InventoryHandler) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode JSON response",
			slog.String("error", err.Error()))
	}
}

func (h *InventoryHandler) respondError(w http.ResponseWriter, status int, message string) {
	h.respondJSON(w, status, map[string]string{"error": message})
}

// Request/Response DTOs

// CreateInventoryRequest represents the request body for creating inventory
type CreateInventoryRequest struct {
	InvoiceID        string           `json:"invoice_id"`
	AuctionID        int              `json:"auction_id,omitempty"`
	ItemName         string           `json:"item_name"`
	Description      string           `json:"description,omitempty"`
	Category         string           `json:"category,omitempty"`
	Subcategory      string           `json:"subcategory,omitempty"`
	Condition        string           `json:"condition,omitempty"`
	Quantity         int              `json:"quantity"`
	BidAmount        decimal.Decimal  `json:"bid_amount"`
	BuyersPremium    decimal.Decimal  `json:"buyers_premium,omitempty"`
	SalesTax         decimal.Decimal  `json:"sales_tax,omitempty"`
	ShippingCost     decimal.Decimal  `json:"shipping_cost,omitempty"`
	AcquisitionDate  *time.Time       `json:"acquisition_date,omitempty"`
	StorageLocation  string           `json:"storage_location,omitempty"`
	StorageBin       string           `json:"storage_bin,omitempty"`
	EstimatedValue   *decimal.Decimal `json:"estimated_value,omitempty"`
	MarketDemand     string           `json:"market_demand,omitempty"`
	SeasonalityNotes string           `json:"seasonality_notes,omitempty"`
	NeedsRepair      bool             `json:"needs_repair,omitempty"`
	IsConsignment    bool             `json:"is_consignment,omitempty"`
	IsReturned       bool             `json:"is_returned,omitempty"`
	Keywords         []string         `json:"keywords,omitempty"`
	Notes            string           `json:"notes,omitempty"`
	AutoCategorize   bool             `json:"auto_categorize,omitempty"`
}

// Validate validates the create inventory request
func (r *CreateInventoryRequest) Validate() error {
	if r.InvoiceID == "" {
		return fmt.Errorf("invoice_id is required")
	}
	if r.ItemName == "" {
		return fmt.Errorf("item_name is required")
	}
	if r.Quantity <= 0 {
		r.Quantity = 1
	}
	if r.BidAmount.IsNegative() {
		return fmt.Errorf("bid_amount cannot be negative")
	}
	return nil
}

// ToDomain converts the request to a domain model
func (r *CreateInventoryRequest) ToDomain() *domain.InventoryItem {
	item := &domain.InventoryItem{
		LotID:            uuid.New(),
		InvoiceID:        r.InvoiceID,
		AuctionID:        r.AuctionID,
		ItemName:         r.ItemName,
		Description:      r.Description,
		Category:         domain.ItemCategory(r.Category),
		Subcategory:      r.Subcategory,
		Condition:        domain.ItemCondition(r.Condition),
		Quantity:         r.Quantity,
		BidAmount:        r.BidAmount,
		BuyersPremium:    r.BuyersPremium,
		SalesTax:         r.SalesTax,
		ShippingCost:     r.ShippingCost,
		StorageLocation:  r.StorageLocation,
		StorageBin:       r.StorageBin,
		EstimatedValue:   r.EstimatedValue,
		MarketDemand:     domain.MarketDemandLevel(r.MarketDemand),
		SeasonalityNotes: r.SeasonalityNotes,
		NeedsRepair:      r.NeedsRepair,
		IsConsignment:    r.IsConsignment,
		IsReturned:       r.IsReturned,
		Keywords:         r.Keywords,
		Notes:            r.Notes,
	}

	if r.AcquisitionDate != nil {
		item.AcquisitionDate = *r.AcquisitionDate
	} else {
		item.AcquisitionDate = time.Now()
	}

	// Set defaults
	if item.Category == "" {
		item.Category = domain.CategoryOther
	}
	if item.Condition == "" {
		item.Condition = domain.ConditionUnknown
	}
	if item.MarketDemand == "" {
		item.MarketDemand = domain.DemandMedium
	}

	return item
}

// UpdateInventoryRequest represents the request body for updating inventory
type UpdateInventoryRequest struct {
	InvoiceID        string           `json:"invoice_id"`
	AuctionID        int              `json:"auction_id,omitempty"`
	ItemName         string           `json:"item_name"`
	Description      string           `json:"description,omitempty"`
	Category         string           `json:"category,omitempty"`
	Subcategory      string           `json:"subcategory,omitempty"`
	Condition        string           `json:"condition,omitempty"`
	Quantity         int              `json:"quantity"`
	BidAmount        decimal.Decimal  `json:"bid_amount"`
	BuyersPremium    decimal.Decimal  `json:"buyers_premium,omitempty"`
	SalesTax         decimal.Decimal  `json:"sales_tax,omitempty"`
	ShippingCost     decimal.Decimal  `json:"shipping_cost,omitempty"`
	AcquisitionDate  time.Time        `json:"acquisition_date"`
	StorageLocation  string           `json:"storage_location,omitempty"`
	StorageBin       string           `json:"storage_bin,omitempty"`
	EstimatedValue   *decimal.Decimal `json:"estimated_value,omitempty"`
	MarketDemand     string           `json:"market_demand,omitempty"`
	SeasonalityNotes string           `json:"seasonality_notes,omitempty"`
	NeedsRepair      bool             `json:"needs_repair,omitempty"`
	IsConsignment    bool             `json:"is_consignment,omitempty"`
	IsReturned       bool             `json:"is_returned,omitempty"`
	Keywords         []string         `json:"keywords,omitempty"`
	Notes            string           `json:"notes,omitempty"`
}

// Validate validates the update inventory request
func (r *UpdateInventoryRequest) Validate() error {
	if r.InvoiceID == "" {
		return fmt.Errorf("invoice_id is required")
	}
	if r.ItemName == "" {
		return fmt.Errorf("item_name is required")
	}
	if r.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if r.BidAmount.IsNegative() {
		return fmt.Errorf("bid_amount cannot be negative")
	}
	return nil
}

// ToDomain converts the request to a domain model
func (r *UpdateInventoryRequest) ToDomain() *domain.InventoryItem {
	item := &domain.InventoryItem{
		InvoiceID:        r.InvoiceID,
		AuctionID:        r.AuctionID,
		ItemName:         r.ItemName,
		Description:      r.Description,
		Category:         domain.ItemCategory(r.Category),
		Subcategory:      r.Subcategory,
		Condition:        domain.ItemCondition(r.Condition),
		Quantity:         r.Quantity,
		BidAmount:        r.BidAmount,
		BuyersPremium:    r.BuyersPremium,
		SalesTax:         r.SalesTax,
		ShippingCost:     r.ShippingCost,
		AcquisitionDate:  r.AcquisitionDate,
		StorageLocation:  r.StorageLocation,
		StorageBin:       r.StorageBin,
		EstimatedValue:   r.EstimatedValue,
		MarketDemand:     domain.MarketDemandLevel(r.MarketDemand),
		SeasonalityNotes: r.SeasonalityNotes,
		NeedsRepair:      r.NeedsRepair,
		IsConsignment:    r.IsConsignment,
		IsReturned:       r.IsReturned,
		Keywords:         r.Keywords,
		Notes:            r.Notes,
	}

	// Set defaults
	if item.Category == "" {
		item.Category = domain.CategoryOther
	}
	if item.Condition == "" {
		item.Condition = domain.ConditionUnknown
	}
	if item.MarketDemand == "" {
		item.MarketDemand = domain.DemandMedium
	}

	return item
}
