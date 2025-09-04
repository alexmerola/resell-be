//go:build integration
// +build integration

package db_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/suite"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/test/helpers"
)

type InventoryRepositorySuite struct {
	suite.Suite
	testDB *helpers.TestDB
	repo   *db.InventoryRepository
	ctx    context.Context
}

func (s *InventoryRepositorySuite) SetupSuite() {
	s.testDB = helpers.SetupTestDB(s.T())
	s.repo = db.NewInventoryRepository(s.testDB.Database, helpers.TestLogger())
	s.ctx = context.Background()
}

func (s *InventoryRepositorySuite) TearDownSuite() {
	// Cleanup handled by helpers.SetupTestDB
}

func (s *InventoryRepositorySuite) SetupTest() {
	// Clear data before each test
	helpers.TruncateAllTables(s.T(), s.testDB.PgxPool)
}

func (s *InventoryRepositorySuite) TestSave() {
	item := helpers.CreateTestInventoryItem()

	err := s.repo.Save(s.ctx, item)
	s.NoError(err)
	s.NotEqual(uuid.Nil, item.LotID)
	s.NotZero(item.TotalCost)
	s.NotZero(item.CostPerItem)

	// Verify item was saved
	saved, err := s.repo.FindByID(s.ctx, item.LotID)
	s.NoError(err)
	s.NotNil(saved)
	s.Equal(item.ItemName, saved.ItemName)
	s.Equal(item.InvoiceID, saved.InvoiceID)
	s.True(item.BidAmount.Equal(saved.BidAmount))
}

func (s *InventoryRepositorySuite) TestSaveBatch() {
	items := []domain.InventoryItem{
		*helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
			i.InvoiceID = "BATCH-001"
			i.ItemName = "Batch Item 1"
		}),
		*helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
			i.InvoiceID = "BATCH-001"
			i.ItemName = "Batch Item 2"
		}),
		*helpers.CreateTestInventoryItem(func(i *domain.InventoryItem) {
			i.InvoiceID = "BATCH-001"
			i.ItemName = "Batch Item 3"
		}),
	}

	err := s.repo.SaveBatch(s.ctx, items)
	s.NoError(err)

	// Verify all items were saved
	for _, item := range items {
		saved, err := s.repo.FindByID(s.ctx, item.LotID)
		s.NoError(err)
		s.NotNil(saved)
		s.Equal(item.ItemName, saved.ItemName)
	}

	// Verify count
	count, err := s.repo.Count(s.ctx)
	s.NoError(err)
	s.Equal(int64(3), count)
}

func (s *InventoryRepositorySuite) TestUpdate() {
	// Create initial item
	item := helpers.CreateTestInventoryItem()
	err := s.repo.Save(s.ctx, item)
	s.NoError(err)

	// Update item
	item.ItemName = "Updated Name"
	item.BidAmount = decimal.NewFromFloat(200)
	item.Quantity = 2
	item.Notes = "Updated notes"

	err = s.repo.Update(s.ctx, item)
	s.NoError(err)

	// Verify update
	updated, err := s.repo.FindByID(s.ctx, item.LotID)
	s.NoError(err)
	s.Equal("Updated Name", updated.ItemName)
	s.True(decimal.NewFromFloat(200).Equal(updated.BidAmount))
	s.Equal(2, updated.Quantity)
	s.Equal("Updated notes", updated.Notes)
	s.True(updated.UpdatedAt.After(updated.CreatedAt))
}

func (s *InventoryRepositorySuite) TestFindByID() {
	s.Run("existing_item", func() {
		item := helpers.CreateTestInventoryItem()
		err := s.repo.Save(s.ctx, item)
		s.NoError(err)

		found, err := s.repo.FindByID(s.ctx, item.LotID)
		s.NoError(err)
		s.NotNil(found)
		s.Equal(item.LotID, found.LotID)
		s.Equal(item.ItemName, found.ItemName)
	})

	s.Run("non_existent_item", func() {
		found, err := s.repo.FindByID(s.ctx, uuid.New())
		s.NoError(err)
		s.Nil(found)
	})

	s.Run("soft_deleted_item", func() {
		item := helpers.CreateTestInventoryItem()
		err := s.repo.Save(s.ctx, item)
		s.NoError(err)

		err = s.repo.SoftDelete(s.ctx, item.LotID)
		s.NoError(err)

		found, err := s.repo.FindByID(s.ctx, item.LotID)
		s.NoError(err)
		s.Nil(found)
	})
}

func (s *InventoryRepositorySuite) TestFindByInvoiceID() {
	// Create items with same invoice ID
	invoiceID := "INV-GROUP-001"
	for i := 0; i < 3; i++ {
		item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
			item.InvoiceID = invoiceID
			item.ItemName = fmt.Sprintf("Item %d", i+1)
		})
		err := s.repo.Save(s.ctx, item)
		s.NoError(err)
	}

	// Create item with different invoice ID
	otherItem := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
		item.InvoiceID = "INV-OTHER-001"
	})
	err := s.repo.Save(s.ctx, otherItem)
	s.NoError(err)

	// Find by invoice ID
	items, err := s.repo.FindByInvoiceID(s.ctx, invoiceID)
	s.NoError(err)
	s.Len(items, 3)

	// Verify all items have correct invoice ID
	for _, item := range items {
		s.Equal(invoiceID, item.InvoiceID)
	}
}

func (s *InventoryRepositorySuite) TestDelete() {
	item := helpers.CreateTestInventoryItem()
	err := s.repo.Save(s.ctx, item)
	s.NoError(err)

	// Verify item exists
	exists, err := s.repo.Exists(s.ctx, item.LotID)
	s.NoError(err)
	s.True(exists)

	// Delete item
	err = s.repo.Delete(s.ctx, item.LotID)
	s.NoError(err)

	// Verify item no longer exists
	exists, err = s.repo.Exists(s.ctx, item.LotID)
	s.NoError(err)
	s.False(exists)

	found, err := s.repo.FindByID(s.ctx, item.LotID)
	s.NoError(err)
	s.Nil(found)
}

func (s *InventoryRepositorySuite) TestSoftDelete() {
	item := helpers.CreateTestInventoryItem()
	err := s.repo.Save(s.ctx, item)
	s.NoError(err)

	// Soft delete
	err = s.repo.SoftDelete(s.ctx, item.LotID)
	s.NoError(err)

	// Item should not be found by normal queries
	found, err := s.repo.FindByID(s.ctx, item.LotID)
	s.NoError(err)
	s.Nil(found)

	// But should still exist in database (verify with direct query)
	var deletedAt *time.Time
	query := `SELECT deleted_at FROM inventory WHERE lot_id = $1`
	err = s.testDB.PgxPool.QueryRow(s.ctx, query, item.LotID).Scan(&deletedAt)
	s.NoError(err)
	s.NotNil(deletedAt)
}

func (s *InventoryRepositorySuite) TestFindAll_Pagination() {
	// Create 25 items
	for i := 0; i < 25; i++ {
		item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
			item.ItemName = fmt.Sprintf("Item %02d", i)
			item.CreatedAt = time.Now().Add(time.Duration(i) * time.Minute)
		})
		err := s.repo.Save(s.ctx, item)
		s.NoError(err)
	}

	// Test first page
	params := db.InventoryQueryParams{
		Limit:     10,
		Offset:    0,
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	items, totalCount, err := s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(items, 10)
	s.Equal(int64(25), totalCount)

	// Verify ordering (newest first)
	s.Equal("Item 24", items[0].ItemName)
	s.Equal("Item 15", items[9].ItemName)

	// Test second page
	params.Offset = 10
	items, totalCount, err = s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(items, 10)
	s.Equal(int64(25), totalCount)
	s.Equal("Item 14", items[0].ItemName)
}

func (s *InventoryRepositorySuite) TestFindAll_Filtering() {
	// Create items with different categories and conditions
	categories := []domain.ItemCategory{
		domain.CategoryAntiques,
		domain.CategoryArt,
		domain.CategoryFurniture,
	}

	for i, category := range categories {
		for j := 0; j < 3; j++ {
			item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
				item.Category = category
				item.ItemName = fmt.Sprintf("%s Item %d", category, j)
				item.StorageLocation = fmt.Sprintf("Location %d", i)
			})
			err := s.repo.Save(s.ctx, item)
			s.NoError(err)
		}
	}

	// Filter by category
	params := db.InventoryQueryParams{
		Category: string(domain.CategoryAntiques),
		Limit:    10,
	}

	items, totalCount, err := s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(items, 3)
	s.Equal(int64(3), totalCount)

	for _, item := range items {
		s.Equal(domain.CategoryAntiques, item.Category)
	}

	// Filter by storage location
	params = db.InventoryQueryParams{
		StorageLocation: "Location 1",
		Limit:           10,
	}

	items, totalCount, err = s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(items, 3)
	s.Equal(int64(3), totalCount)
}

func (s *InventoryRepositorySuite) TestFindAll_Search() {
	// Create items with searchable content
	items := []struct {
		name        string
		description string
		keywords    string
	}{
		{"Victorian Tea Set", "Antique porcelain tea service", "victorian,porcelain,antique"},
		{"Modern Coffee Table", "Contemporary glass and steel design", "modern,glass,steel"},
		{"Vintage Radio", "1950s Zenith tube radio", "vintage,zenith,radio,tube"},
	}

	for _, testItem := range items {
		item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
			item.ItemName = testItem.name
			item.Description = testItem.description
			item.Keywords = strings.Split(testItem.keywords, ",")
		})
		err := s.repo.Save(s.ctx, item)
		s.NoError(err)
	}

	// Search for "victorian"
	params := db.InventoryQueryParams{
		Search: "victorian",
		Limit:  10,
	}

	results, totalCount, err := s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(results, 1)
	s.Equal(int64(1), totalCount)
	s.Contains(results[0].ItemName, "Victorian")

	// Search for "glass"
	params.Search = "glass"
	results, totalCount, err = s.repo.FindAll(s.ctx, params)
	s.NoError(err)
	s.Len(results, 1)
	s.Equal(int64(1), totalCount)
	s.Contains(results[0].ItemName, "Coffee Table")
}

func (s *InventoryRepositorySuite) TestConcurrentOperations() {
	// Test concurrent writes don't cause issues
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			item := helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
				item.ItemName = fmt.Sprintf("Concurrent Item %d", idx)
			})
			err := s.repo.Save(context.Background(), item)
			s.NoError(err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all items were created
	count, err := s.repo.Count(s.ctx)
	s.NoError(err)
	s.Equal(int64(10), count)
}

func TestInventoryRepositorySuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}
	suite.Run(t, new(InventoryRepositorySuite))
}
