package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
	"github.com/ammerola/resell-be/internal/core/services"
	"github.com/ammerola/resell-be/test/helpers"
)

func BenchmarkInventoryOperations(b *testing.B) {
	// Setup
	testDB := helpers.SetupTestDB(&testing.T{})
	defer testDB.Database.Close()

	repo := db.NewInventoryRepository(testDB.Database, helpers.TestLogger())
	service := services.NewInventoryService(repo, testDB.PgxPool, helpers.TestLogger())
	ctx := context.Background()

	b.Run("Create", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			item := &domain.InventoryItem{
				LotID:     uuid.New(),
				InvoiceID: fmt.Sprintf("BENCH-%d", i),
				ItemName:  fmt.Sprintf("Benchmark Item %d", i),
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
			}
			_ = service.SaveItem(ctx, item)
		}
	})

	// Pre-create items for read benchmarks
	var itemIDs []uuid.UUID
	for i := 0; i < 100; i++ {
		item := helpers.CreateTestInventoryItem()
		_ = service.SaveItem(ctx, item)
		itemIDs = append(itemIDs, item.LotID)
	}

	b.Run("Read", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			id := itemIDs[i%len(itemIDs)]
			_, _ = service.GetByID(ctx, id)
		}
	})

	b.Run("List", func(b *testing.B) {
		params := ports.ListParams{
			Page:     1,
			PageSize: 50,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.List(ctx, params)
		}
	})

	b.Run("Search", func(b *testing.B) {
		params := ports.ListParams{
			Search:   "test",
			Page:     1,
			PageSize: 50,
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = service.List(ctx, params)
		}
	})

	b.Run("BatchCreate", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			items := make([]domain.InventoryItem, 100)
			for j := range items {
				items[j] = *helpers.CreateTestInventoryItem(func(item *domain.InventoryItem) {
					item.InvoiceID = fmt.Sprintf("BATCH-%d-%d", i, j)
				})
			}
			_ = service.SaveItems(ctx, items)
		}
	})
}

func BenchmarkPDFProcessing(b *testing.B) {
	// Benchmark PDF extraction performance
	processor := createBenchmarkProcessor()
	pdfContent := createLargePDFContent(100) // 100 items

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = processor.ExtractItemsFromPDF(
			context.Background(),
			pdfContent,
			fmt.Sprintf("BENCH-%d", i),
			12345,
		)
	}
}

func BenchmarkCategorization(b *testing.B) {
	processor := createBenchmarkProcessor()
	descriptions := []string{
		"Antique Victorian silver tea set with ornate engravings",
		"Modern abstract painting on canvas by local artist",
		"Vintage Lionel train set in original box with tracks",
		"Crystal wine glasses set of 12 Waterford pattern",
		"Mahogany dining table with six matching chairs",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		desc := descriptions[i%len(descriptions)]
		processor.CategorizeItem(desc)
	}
}

// Memory allocation benchmarks
func BenchmarkMemoryAllocation(b *testing.B) {
	b.Run("InventoryItem", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = &domain.InventoryItem{
				LotID:     uuid.New(),
				InvoiceID: "TEST-001",
				ItemName:  "Test Item",
				Quantity:  1,
				BidAmount: decimal.NewFromFloat(100),
			}
		}
	})

	b.Run("ListResult", func(b *testing.B) {
		items := make([]*domain.InventoryItem, 100)
		for i := range items {
			items[i] = helpers.CreateTestInventoryItem()
		}

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = &ports.ListResult{
				Items:      items,
				Page:       1,
				PageSize:   50,
				TotalCount: 100,
				TotalPages: 2,
			}
		}
	})
}
