package domain_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ammerola/resell-be/internal/core/domain"
)

func TestInventoryItem_Validate(t *testing.T) {
	tests := []struct {
		name      string
		item      *domain.InventoryItem
		wantError bool
		errorMsg  string
	}{
		{
			name: "valid_item_with_all_fields",
			item: &domain.InventoryItem{
				InvoiceID:       "INV-001",
				ItemName:        "Victorian Tea Set",
				Quantity:        1,
				BidAmount:       decimal.NewFromFloat(100),
				Category:        domain.CategoryAntiques,
				Condition:       domain.ConditionExcellent,
				MarketDemand:    domain.DemandHigh,
				AcquisitionDate: time.Now(),
			},
			wantError: false,
		},
		{
			name: "missing_invoice_id",
			item: &domain.InventoryItem{
				ItemName:  "Test Item",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
			},
			wantError: true,
			errorMsg:  "invoice_id is required",
		},
		{
			name: "missing_item_name",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
			},
			wantError: true,
			errorMsg:  "item_name is required",
		},
		{
			name: "zero_quantity",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				Quantity:  0,
				BidAmount: decimal.NewFromFloat(100),
			},
			wantError: true,
			errorMsg:  "quantity must be positive",
		},
		{
			name: "negative_quantity",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				Quantity:  -5,
				BidAmount: decimal.NewFromFloat(100),
			},
			wantError: true,
			errorMsg:  "quantity must be positive",
		},
		{
			name: "negative_bid_amount",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(-50),
			},
			wantError: true,
			errorMsg:  "bid_amount cannot be negative",
		},
		{
			name: "sets_default_category_when_empty",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
				Category:  "", // Empty category
			},
			wantError: false,
		},
		{
			name: "sets_default_condition_when_empty",
			item: &domain.InventoryItem{
				InvoiceID: "INV-001",
				ItemName:  "Test Item",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
				Condition: "", // Empty condition
			},
			wantError: false,
		},
		{
			name: "sets_default_market_demand_when_empty",
			item: &domain.InventoryItem{
				InvoiceID:    "INV-001",
				ItemName:     "Test Item",
				Quantity:     1,
				BidAmount:    decimal.NewFromFloat(100),
				MarketDemand: "", // Empty market demand
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.item.Validate()

			if tt.wantError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)

				// Check defaults were set
				if tt.name == "sets_default_category_when_empty" {
					assert.Equal(t, domain.CategoryOther, tt.item.Category)
				}
				if tt.name == "sets_default_condition_when_empty" {
					assert.Equal(t, domain.ConditionUnknown, tt.item.Condition)
				}
				if tt.name == "sets_default_market_demand_when_empty" {
					assert.Equal(t, domain.DemandMedium, tt.item.MarketDemand)
				}
			}
		})
	}
}

func TestInventoryItem_CalculateTotalCost(t *testing.T) {
	tests := []struct {
		name            string
		bidAmount       decimal.Decimal
		buyersPremium   decimal.Decimal
		salesTax        decimal.Decimal
		shippingCost    decimal.Decimal
		quantity        int
		expectedTotal   decimal.Decimal
		expectedPerItem decimal.Decimal
	}{
		{
			name:            "standard_calculation",
			bidAmount:       decimal.NewFromFloat(100),
			buyersPremium:   decimal.NewFromFloat(18),
			salesTax:        decimal.NewFromFloat(10.18),
			shippingCost:    decimal.NewFromFloat(15),
			quantity:        1,
			expectedTotal:   decimal.NewFromFloat(143.18),
			expectedPerItem: decimal.NewFromFloat(143.18),
		},
		{
			name:            "multiple_quantity",
			bidAmount:       decimal.NewFromFloat(200),
			buyersPremium:   decimal.NewFromFloat(36),
			salesTax:        decimal.NewFromFloat(20.36),
			shippingCost:    decimal.NewFromFloat(20),
			quantity:        2,
			expectedTotal:   decimal.NewFromFloat(276.36),
			expectedPerItem: decimal.NewFromFloat(138.18),
		},
		{
			name:            "zero_values",
			bidAmount:       decimal.NewFromFloat(50),
			buyersPremium:   decimal.Zero,
			salesTax:        decimal.Zero,
			shippingCost:    decimal.Zero,
			quantity:        1,
			expectedTotal:   decimal.NewFromFloat(50),
			expectedPerItem: decimal.NewFromFloat(50),
		},
		{
			name:            "all_fees_no_bid",
			bidAmount:       decimal.Zero,
			buyersPremium:   decimal.NewFromFloat(10),
			salesTax:        decimal.NewFromFloat(5),
			shippingCost:    decimal.NewFromFloat(15),
			quantity:        1,
			expectedTotal:   decimal.NewFromFloat(30),
			expectedPerItem: decimal.NewFromFloat(30),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := &domain.InventoryItem{
				BidAmount:     tt.bidAmount,
				BuyersPremium: tt.buyersPremium,
				SalesTax:      tt.salesTax,
				ShippingCost:  tt.shippingCost,
				Quantity:      tt.quantity,
			}

			item.CalculateTotalCost()

			assert.True(t, item.TotalCost.Equal(tt.expectedTotal),
				"Expected total: %s, Got: %s", tt.expectedTotal, item.TotalCost)
			assert.True(t, item.CostPerItem.Equal(tt.expectedPerItem),
				"Expected per item: %s, Got: %s", tt.expectedPerItem, item.CostPerItem)
		})
	}
}

func TestInventoryItem_PrepareForStorage(t *testing.T) {
	t.Run("generates_uuid_when_nil", func(t *testing.T) {
		item := &domain.InventoryItem{
			LotID: uuid.Nil,
		}

		item.PrepareForStorage()

		assert.NotEqual(t, uuid.Nil, item.LotID)
		assert.NotZero(t, item.CreatedAt)
		assert.NotZero(t, item.UpdatedAt)
	})

	t.Run("preserves_existing_uuid", func(t *testing.T) {
		existingID := uuid.New()
		item := &domain.InventoryItem{
			LotID: existingID,
		}

		item.PrepareForStorage()

		assert.Equal(t, existingID, item.LotID)
	})

	t.Run("sets_timestamps", func(t *testing.T) {
		item := &domain.InventoryItem{}
		now := time.Now()

		item.PrepareForStorage()

		assert.WithinDuration(t, now, item.CreatedAt, time.Second)
		assert.WithinDuration(t, now, item.UpdatedAt, time.Second)
		assert.WithinDuration(t, now, item.AcquisitionDate, time.Second)
	})

	t.Run("calculates_costs", func(t *testing.T) {
		item := &domain.InventoryItem{
			BidAmount:     decimal.NewFromFloat(100),
			BuyersPremium: decimal.NewFromFloat(18),
			SalesTax:      decimal.NewFromFloat(10.18),
			Quantity:      1,
		}

		item.PrepareForStorage()

		expectedTotal := decimal.NewFromFloat(128.18)
		assert.True(t, item.TotalCost.Equal(expectedTotal))
		assert.True(t, item.CostPerItem.Equal(expectedTotal))
	})
}

// Benchmarks
func BenchmarkInventoryItem_Validate(b *testing.B) {
	item := &domain.InventoryItem{
		InvoiceID: "INV-001",
		ItemName:  "Test Item",
		Quantity:  1,
		BidAmount: decimal.NewFromFloat(100),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = item.Validate()
	}
}

func BenchmarkInventoryItem_CalculateTotalCost(b *testing.B) {
	item := &domain.InventoryItem{
		BidAmount:     decimal.NewFromFloat(100),
		BuyersPremium: decimal.NewFromFloat(18),
		SalesTax:      decimal.NewFromFloat(10.18),
		ShippingCost:  decimal.NewFromFloat(15),
		Quantity:      1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		item.CalculateTotalCost()
	}
}
