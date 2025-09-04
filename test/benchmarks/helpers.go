// test/benchmarks/helpers.go
package benchmarks

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// MockPDFProcessor provides PDF processing capabilities for benchmarks
type MockPDFProcessor struct {
	logger *slog.Logger
}

// createBenchmarkProcessor creates a processor for benchmark tests
func createBenchmarkProcessor() *MockPDFProcessor {
	return &MockPDFProcessor{
		logger: slog.Default(),
	}
}

// ExtractItemsFromPDF simulates PDF extraction
func (p *MockPDFProcessor) ExtractItemsFromPDF(ctx context.Context, content []byte, invoiceID string, auctionID int) ([]domain.InventoryItem, error) {
	// Simulate parsing PDF content
	items := make([]domain.InventoryItem, 0, 100)

	// Mock extraction logic - in production this would use actual PDF library
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		item := domain.InventoryItem{
			LotID:     uuid.New(),
			InvoiceID: invoiceID,
			AuctionID: auctionID,
			ItemName:  fmt.Sprintf("Item %d: %s", i, line),
			Quantity:  1,
			BidAmount: decimal.NewFromFloat(100),
			Category:  p.CategorizeItem(line),
		}
		items = append(items, item)
	}

	return items, nil
}

// CategorizeItem determines the category based on description
func (p *MockPDFProcessor) CategorizeItem(description string) domain.ItemCategory {
	descLower := strings.ToLower(description)

	// Simple categorization logic for benchmarks
	switch {
	case strings.Contains(descLower, "antique") || strings.Contains(descLower, "victorian"):
		return domain.CategoryAntiques
	case strings.Contains(descLower, "painting") || strings.Contains(descLower, "art"):
		return domain.CategoryArt
	case strings.Contains(descLower, "jewelry") || strings.Contains(descLower, "ring"):
		return domain.CategoryJewelry
	case strings.Contains(descLower, "furniture") || strings.Contains(descLower, "chair") || strings.Contains(descLower, "table"):
		return domain.CategoryFurniture
	case strings.Contains(descLower, "crystal") || strings.Contains(descLower, "glass"):
		return domain.CategoryGlass
	case strings.Contains(descLower, "book"):
		return domain.CategoryBooks
	case strings.Contains(descLower, "toy") || strings.Contains(descLower, "game"):
		return domain.CategoryToys
	case strings.Contains(descLower, "tool"):
		return domain.CategoryTools
	default:
		return domain.CategoryOther
	}
}

// createLargePDFContent creates simulated PDF content for benchmarks
func createLargePDFContent(numItems int) []byte {
	var content strings.Builder

	// Simulate PDF structure with invoice header
	content.WriteString("INVOICE #12345\n")
	content.WriteString("AUCTION DATE: 2024-01-15\n")
	content.WriteString("=====================================\n\n")

	// Generate item lines
	itemDescriptions := []string{
		"Antique Victorian silver tea set with ornate engravings",
		"Modern abstract painting on canvas by local artist",
		"Vintage Lionel train set in original box with tracks",
		"Crystal wine glasses set of 12 Waterford pattern",
		"Mahogany dining table with six matching chairs",
		"Gold pocket watch with chain, circa 1890",
		"Collection of first edition books, various authors",
		"Persian rug 8x10 hand-woven wool traditional pattern",
		"Brass telescope on wooden tripod, nautical style",
		"China cabinet with glass doors, oak construction",
	}

	for i := 0; i < numItems; i++ {
		desc := itemDescriptions[i%len(itemDescriptions)]
		content.WriteString(fmt.Sprintf("LOT %d: %s - $%.2f\n", i+1, desc, float64(100+i*10)))
	}

	return []byte(content.String())
}
