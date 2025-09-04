package db_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/test/helpers"
)

func TestInventoryRepository_Save_Unit(t *testing.T) {
	// Setup mock database
	mockDB, _ := helpers.SetupMockDB(t)
	defer mockDB.ExpectClose()

	// Create a mock pgxpool.Pool wrapper
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	item := helpers.CreateTestInventoryItem()

	// For unit tests using actual test database
	err := repo.Save(ctx, item)
	require.NoError(t, err)
	assert.NotZero(t, item.TotalCost)
	assert.NotZero(t, item.CostPerItem)
}

func TestInventoryRepository_FindByID_Unit(t *testing.T) {
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	// Create test item
	item := helpers.CreateTestInventoryItem()
	err := repo.Save(ctx, item)
	require.NoError(t, err)

	tests := []struct {
		name        string
		lotID       uuid.UUID
		expectedNil bool
		wantError   bool
	}{
		{
			name:        "finds_existing_item",
			lotID:       item.LotID,
			expectedNil: false,
			wantError:   false,
		},
		{
			name:        "returns_nil_for_nonexistent_item",
			lotID:       uuid.New(),
			expectedNil: true,
			wantError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := repo.FindByID(ctx, tt.lotID)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectedNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				if result != nil {
					assert.Equal(t, tt.lotID, result.LotID)
				}
			}
		})
	}
}

func TestInventoryRepository_Update_Unit(t *testing.T) {
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	// Create initial item
	item := helpers.CreateTestInventoryItem()
	err := repo.Save(ctx, item)
	require.NoError(t, err)

	// Update item
	item.ItemName = "Updated Name"
	item.BidAmount = decimal.NewFromFloat(200)
	item.Quantity = 2

	err = repo.Update(ctx, item)
	require.NoError(t, err)

	// Verify update
	updated, err := repo.FindByID(ctx, item.LotID)
	require.NoError(t, err)
	assert.Equal(t, "Updated Name", updated.ItemName)
	assert.True(t, decimal.NewFromFloat(200).Equal(updated.BidAmount))
	assert.Equal(t, 2, updated.Quantity)
}

func TestInventoryRepository_Delete_Unit(t *testing.T) {
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	// Create test item
	item := helpers.CreateTestInventoryItem()
	err := repo.Save(ctx, item)
	require.NoError(t, err)

	// Verify item exists
	exists, err := repo.Exists(ctx, item.LotID)
	require.NoError(t, err)
	assert.True(t, exists)

	// Delete item
	err = repo.Delete(ctx, item.LotID)
	require.NoError(t, err)

	// Verify item no longer exists
	exists, err = repo.Exists(ctx, item.LotID)
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestInventoryRepository_FindByInvoiceID_Unit(t *testing.T) {
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	// Create items with same invoice ID
	invoiceID := "INV-GROUP-001"
	for i := 0; i < 3; i++ {
		item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
			item.InvoiceID = invoiceID
			item.ItemName = fmt.Sprintf("Item %d", i+1)
		})
		err := repo.Save(ctx, item)
		require.NoError(t, err)
	}

	// Find by invoice ID
	items, err := repo.FindByInvoiceID(ctx, invoiceID)
	require.NoError(t, err)
	assert.Len(t, items, 3)

	// Verify all items have correct invoice ID
	for _, item := range items {
		assert.Equal(t, invoiceID, item.InvoiceID)
	}
}

func TestInventoryRepository_SaveBatch_Unit(t *testing.T) {
	testDB := helpers.SetupTestDB(t)
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	ctx := context.Background()

	items := helpers.CreateTestInventoryItems(5)

	err := repo.SaveBatch(ctx, items)
	require.NoError(t, err)

	// Verify all items were saved
	for _, item := range items {
		saved, err := repo.FindByID(ctx, item.LotID)
		require.NoError(t, err)
		assert.NotNil(t, saved)
		assert.Equal(t, item.ItemName, saved.ItemName)
	}

	// Verify count
	count, err := repo.Count(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, count, int64(5))
}
