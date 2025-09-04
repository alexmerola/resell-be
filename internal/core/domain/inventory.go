// internal/core/domain/inventory.go
package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ItemCategory represents item categories
type ItemCategory string

// Category constants
const (
	CategoryAntiques     ItemCategory = "antiques"
	CategoryArt          ItemCategory = "art"
	CategoryBooks        ItemCategory = "books"
	CategoryCeramics     ItemCategory = "ceramics"
	CategoryChina        ItemCategory = "china"
	CategoryClothing     ItemCategory = "clothing"
	CategoryCoins        ItemCategory = "coins"
	CategoryCollectibles ItemCategory = "collectibles"
	CategoryElectronics  ItemCategory = "electronics"
	CategoryFurniture    ItemCategory = "furniture"
	CategoryGlass        ItemCategory = "glass"
	CategoryJewelry      ItemCategory = "jewelry"
	CategoryLinens       ItemCategory = "linens"
	CategoryMemorabilia  ItemCategory = "memorabilia"
	CategoryMusical      ItemCategory = "musical"
	CategoryPottery      ItemCategory = "pottery"
	CategorySilver       ItemCategory = "silver"
	CategoryStamps       ItemCategory = "stamps"
	CategoryTools        ItemCategory = "tools"
	CategoryToys         ItemCategory = "toys"
	CategoryVintage      ItemCategory = "vintage"
	CategoryOther        ItemCategory = "other"
)

// ItemCondition represents item conditions
type ItemCondition string

// Condition constants
const (
	ConditionMint        ItemCondition = "mint"
	ConditionExcellent   ItemCondition = "excellent"
	ConditionVeryGood    ItemCondition = "very_good"
	ConditionGood        ItemCondition = "good"
	ConditionFair        ItemCondition = "fair"
	ConditionPoor        ItemCondition = "poor"
	ConditionRestoration ItemCondition = "restoration"
	ConditionParts       ItemCondition = "parts"
	ConditionUnknown     ItemCondition = "unknown"
)

// MarketDemandLevel represents market demand levels
type MarketDemandLevel string

// Market demand constants
const (
	DemandVeryHigh MarketDemandLevel = "very_high"
	DemandHigh     MarketDemandLevel = "high"
	DemandMedium   MarketDemandLevel = "medium"
	DemandLow      MarketDemandLevel = "low"
	DemandVeryLow  MarketDemandLevel = "very_low"
)

// InventoryItem represents a single inventory item
type InventoryItem struct {
	LotID            uuid.UUID         `json:"lot_id"`
	InvoiceID        string            `json:"invoice_id"`
	AuctionID        int               `json:"auction_id"`
	ItemName         string            `json:"item_name"`
	Description      string            `json:"description"`
	Category         ItemCategory      `json:"category"`
	Subcategory      string            `json:"subcategory,omitempty"`
	Condition        ItemCondition     `json:"condition"`
	Quantity         int               `json:"quantity"`
	BidAmount        decimal.Decimal   `json:"bid_amount"`
	BuyersPremium    decimal.Decimal   `json:"buyers_premium"`
	SalesTax         decimal.Decimal   `json:"sales_tax"`
	ShippingCost     decimal.Decimal   `json:"shipping_cost"`
	TotalCost        decimal.Decimal   `json:"total_cost"`
	CostPerItem      decimal.Decimal   `json:"cost_per_item"`
	AcquisitionDate  time.Time         `json:"acquisition_date"`
	StorageLocation  string            `json:"storage_location,omitempty"`
	StorageBin       string            `json:"storage_bin,omitempty"`
	QRCode           string            `json:"qr_code,omitempty"`
	EstimatedValue   *decimal.Decimal  `json:"estimated_value,omitempty"`
	MarketDemand     MarketDemandLevel `json:"market_demand"`
	SeasonalityNotes string            `json:"seasonality_notes,omitempty"`
	NeedsRepair      bool              `json:"needs_repair"`
	IsConsignment    bool              `json:"is_consignment"`
	IsReturned       bool              `json:"is_returned"`
	Keywords         []string          `json:"keywords,omitempty"`
	Notes            string            `json:"notes,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	DeletedAt        *time.Time        `json:"deleted_at,omitempty"`
}

// ListingStatus represents the status of an item listing
type ListingStatus string

const (
	StatusDraft     ListingStatus = "draft"
	StatusActive    ListingStatus = "active"
	StatusPending   ListingStatus = "pending"
	StatusSold      ListingStatus = "sold"
	StatusReturned  ListingStatus = "returned"
	StatusCancelled ListingStatus = "cancelled"
)

// Validate performs domain validation on the inventory item
func (i *InventoryItem) Validate() error {
	if i.InvoiceID == "" {
		return fmt.Errorf("invoice_id is required")
	}
	if i.ItemName == "" {
		return fmt.Errorf("item_name is required")
	}
	if i.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	if i.BidAmount.IsNegative() {
		return fmt.Errorf("bid_amount cannot be negative")
	}
	if i.Category == "" {
		i.Category = CategoryOther
	}
	if i.Condition == "" {
		i.Condition = ConditionUnknown
	}
	if i.MarketDemand == "" {
		i.MarketDemand = DemandMedium
	}
	return nil
}

// CalculateTotalCost calculates and sets the total cost fields
func (i *InventoryItem) CalculateTotalCost() {
	i.TotalCost = i.BidAmount.
		Add(i.BuyersPremium).
		Add(i.SalesTax).
		Add(i.ShippingCost)

	if i.Quantity > 0 {
		i.CostPerItem = i.TotalCost.Div(decimal.NewFromInt(int64(i.Quantity)))
	}
}

// PrepareForStorage prepares the item for database storage
func (i *InventoryItem) PrepareForStorage() {
	// Ensure UUID is set
	if i.LotID == uuid.Nil {
		i.LotID = uuid.New()
	}

	// Calculate costs
	i.CalculateTotalCost()

	// Set timestamps if not set
	now := time.Now()
	if i.CreatedAt.IsZero() {
		i.CreatedAt = now
	}
	i.UpdatedAt = now

	// Set defaults
	if i.AcquisitionDate.IsZero() {
		i.AcquisitionDate = now
	}
}
