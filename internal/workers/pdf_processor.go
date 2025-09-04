// internal/workers/pdf_processor.go
package workers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/ledongthuc/pdf"
	"github.com/shopspring/decimal"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/ammerola/resell-be/internal/core/ports"
)

const (
	TypePDFProcess       = "pdf:process"
	TypeExcelImport      = "excel:import"
	TypeRefreshAnalytics = "analytics:refresh"
	TypeGenerateReport   = "report:generate"
	TypeSendEmail        = "email:send"
	TypeCleanupOldData   = "cleanup:old_data"
	TypeCleanupTempFiles = "cleanup:temp_files"
)

// PDFJobPayload represents the payload for PDF processing jobs
type PDFJobPayload struct {
	JobID     string `json:"job_id"`
	FilePath  string `json:"file_path"`
	InvoiceID string `json:"invoice_id"`
	AuctionID int    `json:"auction_id"`
	UserID    string `json:"user_id,omitempty"`
}

// PDFJobResult represents the result of PDF processing
type PDFJobResult struct {
	ItemsProcessed int      `json:"items_processed"`
	ItemsCreated   int      `json:"items_created"`
	ItemsUpdated   int      `json:"items_updated"`
	Errors         []string `json:"errors,omitempty"`
	ProcessingTime string   `json:"processing_time"`
}

// PDFProcessor handles PDF processing tasks
type PDFProcessor struct {
	service ports.InventoryService // Use the interface
	db      ports.Database         // Use the interface
	logger  *slog.Logger
}

// NewPDFProcessor creates a new PDF processor
func NewPDFProcessor(service ports.InventoryService, db ports.Database, logger *slog.Logger) *PDFProcessor {
	return &PDFProcessor{
		service: service,
		db:      db,
		logger:  logger.With(slog.String("processor", "pdf")),
	}
}

// ProcessPDF processes a PDF file and extracts inventory items
func (p *PDFProcessor) ProcessPDF(ctx context.Context, t *asynq.Task) error {
	start := time.Now()

	var payload PDFJobPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	p.logger.InfoContext(ctx, "processing PDF",
		slog.String("job_id", payload.JobID),
		slog.String("invoice_id", payload.InvoiceID))

	// Update job status to processing
	_ = p.updateJobStatus(ctx, payload.JobID, "processing", nil)

	// Extract items from PDF
	items, err := p.extractItemsFromPDF(ctx, payload.FilePath, payload.InvoiceID, payload.AuctionID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to extract items: %v", err)
		_ = p.updateJobStatus(ctx, payload.JobID, "failed", &errMsg)
		return fmt.Errorf(errMsg)
	}

	err = p.service.SaveItems(ctx, items)

	// Prepare result and update job status
	var errors []string
	status := "completed"
	if err != nil {
		status = "completed_with_errors"
		errors = append(errors, err.Error())
	}

	result := PDFJobResult{
		ItemsProcessed: len(items),
		ItemsCreated:   len(items), // We are now only creating
		ItemsUpdated:   0,
		Errors:         errors,
		ProcessingTime: time.Since(start).String(),
	}

	resultJSON, _ := json.Marshal(result)
	_ = p.updateJobStatusWithResult(ctx, payload.JobID, status, resultJSON)

	// Clean up temporary file
	if strings.HasPrefix(payload.FilePath, os.TempDir()) {
		_ = os.Remove(payload.FilePath)
	}

	p.logger.InfoContext(ctx, "PDF processing completed",
		slog.String("job_id", payload.JobID),
		slog.Int("items_processed", result.ItemsProcessed))

	return err // Return the error from the service call, if any
}

func (p *PDFProcessor) extractItemsFromPDF(ctx context.Context, filePath string, invoiceID string, auctionID int) ([]domain.InventoryItem, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	// Extract text from all pages
	var textLines []string
	totalPages := r.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			p.logger.WarnContext(ctx, "failed to extract text from page",
				slog.Int("page", pageNum),
				err)
			continue
		}

		lines := strings.Split(text, "\n")
		textLines = append(textLines, lines...)
	}

	// Parse the extracted text to find items
	rawItems := p.parseInvoiceItems(textLines)

	// Convert raw items to domain items
	items := make([]domain.InventoryItem, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item := p.createInventoryItem(rawItem, invoiceID, auctionID)
		items = append(items, item)
	}

	p.logger.InfoContext(ctx, "extracted items from PDF",
		slog.String("invoice_id", invoiceID),
		slog.Int("count", len(items)))

	return items, nil
}

type rawInvoiceItem struct {
	description string
	bidAmount   decimal.Decimal
	quantity    int
}

func (p *PDFProcessor) parseInvoiceItems(lines []string) []rawInvoiceItem {
	var items []rawInvoiceItem

	// Patterns for parsing invoice lines
	headerRe := regexp.MustCompile(`(?i)(LOT.*PRICE|LEAD.*ITEM.*PRICE)`)
	footerRe := regexp.MustCompile(`(?i)(A payment of|SUBTOTAL|TOTAL)`)
	priceRe := regexp.MustCompile(`\$?\s*\d{1,3}(?:,\d{3})*\.\d{2}\s*$`)

	// Find start of items section
	startIdx := 0
	for i, line := range lines {
		if headerRe.MatchString(line) {
			startIdx = i + 1
			break
		}
	}

	// Buffer for multi-line descriptions
	var descBuffer []string

	for i := startIdx; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Check if we've reached the footer
		if footerRe.MatchString(line) {
			break
		}

		// Check if line ends with a price
		if priceRe.MatchString(line) {
			// Extract price
			priceStr := priceRe.FindString(line)
			bidAmount := p.parseCurrency(priceStr)

			// Extract description (everything before the price)
			description := strings.TrimSpace(priceRe.ReplaceAllString(line, ""))

			// Add buffered descriptions if any
			if len(descBuffer) > 0 {
				fullDesc := strings.Join(append(descBuffer, description), " ")
				fullDesc = p.cleanDescription(fullDesc)

				if fullDesc != "" {
					items = append(items, rawInvoiceItem{
						description: fullDesc,
						bidAmount:   bidAmount,
						quantity:    1,
					})
				}

				// Clear buffer
				descBuffer = descBuffer[:0]
			} else if description != "" {
				// Single-line item
				description = p.cleanDescription(description)
				if description != "" {
					items = append(items, rawInvoiceItem{
						description: description,
						bidAmount:   bidAmount,
						quantity:    1,
					})
				}
			}
		} else {
			// This is part of a multi-line description
			descBuffer = append(descBuffer, line)
		}
	}

	return items
}

func (p *PDFProcessor) cleanDescription(desc string) string {
	// Remove item numbers and lot numbers
	desc = regexp.MustCompile(`^\d+\s+`).ReplaceAllString(desc, "")
	desc = regexp.MustCompile(`\b\d{5,6}\s+\d{1,3}\s+[A-Z0-9]+\b`).ReplaceAllString(desc, "")

	// Remove multiple spaces
	desc = regexp.MustCompile(`\s+`).ReplaceAllString(desc, " ")

	// Remove dashes used as fillers
	desc = regexp.MustCompile(`-{3,}`).ReplaceAllString(desc, "")

	return strings.TrimSpace(desc)
}

func (p *PDFProcessor) parseCurrency(val string) decimal.Decimal {
	// Remove dollar sign, commas, and spaces
	cleaned := strings.ReplaceAll(val, "$", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = strings.TrimSpace(cleaned)

	d, err := decimal.NewFromString(cleaned)
	if err != nil {
		return decimal.Zero
	}
	return d
}

func (p *PDFProcessor) createInventoryItem(raw rawInvoiceItem, invoiceID string, auctionID int) domain.InventoryItem {
	// Calculate buyer's premium and sales tax (using typical auction percentages)
	bpRate := decimal.NewFromFloat(0.18)     // 18% buyer's premium
	taxRate := decimal.NewFromFloat(0.08625) // 8.625% NY sales tax

	buyersPremium := raw.bidAmount.Mul(bpRate).Round(2)
	subtotal := raw.bidAmount.Add(buyersPremium)
	salesTax := subtotal.Mul(taxRate).Round(2)

	// Categorize item based on description
	category, condition := p.categorizeItem(raw.description)

	// Generate item name from description
	itemName := p.generateItemName(raw.description)

	return domain.InventoryItem{
		LotID:           uuid.New(),
		InvoiceID:       invoiceID,
		AuctionID:       auctionID,
		ItemName:        itemName,
		Description:     raw.description,
		Category:        category,
		Condition:       condition,
		Quantity:        raw.quantity,
		BidAmount:       raw.bidAmount,
		BuyersPremium:   buyersPremium,
		SalesTax:        salesTax,
		AcquisitionDate: time.Now(),
		Keywords:        p.extractKeywords(raw.description),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func (p *PDFProcessor) categorizeItem(description string) (domain.ItemCategory, domain.ItemCondition) {
	descLower := strings.ToLower(description)

	// Simple categorization based on keywords
	if strings.Contains(descLower, "painting") || strings.Contains(descLower, "print") {
		return domain.CategoryArt, domain.ConditionGood
	}
	if strings.Contains(descLower, "furniture") || strings.Contains(descLower, "table") || strings.Contains(descLower, "chair") {
		return domain.CategoryFurniture, domain.ConditionGood
	}
	if strings.Contains(descLower, "jewelry") || strings.Contains(descLower, "ring") || strings.Contains(descLower, "necklace") {
		return domain.CategoryJewelry, domain.ConditionGood
	}
	if strings.Contains(descLower, "glass") || strings.Contains(descLower, "crystal") {
		return domain.CategoryGlass, domain.ConditionGood
	}
	if strings.Contains(descLower, "china") || strings.Contains(descLower, "porcelain") {
		return domain.CategoryChina, domain.ConditionGood
	}
	if strings.Contains(descLower, "silver") || strings.Contains(descLower, "sterling") {
		return domain.CategorySilver, domain.ConditionGood
	}

	// Condition assessment
	condition := domain.ConditionGood
	if strings.Contains(descLower, "mint") {
		condition = domain.ConditionMint
	} else if strings.Contains(descLower, "excellent") {
		condition = domain.ConditionExcellent
	} else if strings.Contains(descLower, "damage") || strings.Contains(descLower, "repair") {
		condition = domain.ConditionFair
	}

	return domain.CategoryOther, condition
}

func (p *PDFProcessor) generateItemName(description string) string {
	// Take first 60 characters or first sentence
	name := description
	if len(name) > 60 {
		name = name[:60]
		if idx := strings.Index(description[:60], "."); idx > 0 {
			name = description[:idx]
		}
	}

	// Clean up and title case
	name = strings.TrimSpace(name)
	if name == "" {
		return "Unknown Item"
	}

	return name
}

func (p *PDFProcessor) extractKeywords(description string) []string {
	// Simple keyword extraction
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
	}

	words := strings.Fields(strings.ToLower(description))
	var keywords []string
	seen := make(map[string]bool)

	for _, word := range words {
		word = strings.Trim(word, ".,!?;:")
		if !stopWords[word] && len(word) > 2 && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
			if len(keywords) >= 10 {
				break
			}
		}
	}

	return keywords
}

func (p *PDFProcessor) checkItemExists(ctx context.Context, lotID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM inventory WHERE lot_id = $1 AND deleted_at IS NULL)`
	var exists bool
	err := p.db.QueryRow(ctx, query, lotID).Scan(&exists)
	return exists, err
}

func (p *PDFProcessor) updateJobStatus(ctx context.Context, jobID string, status string, errorMsg *string) error {
	query := `
		UPDATE async_jobs 
		SET status = $2, error = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	_, err := p.db.Exec(ctx, query, jobID, status, errorMsg)
	return err
}

func (p *PDFProcessor) updateJobStatusWithResult(ctx context.Context, jobID string, status string, result json.RawMessage) error {
	query := `
		UPDATE async_jobs 
		SET status = $2, result = $3, completed_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	_, err := p.db.Exec(ctx, query, jobID, status, result)
	return err
}
