package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ledongthuc/pdf"
	"github.com/shopspring/decimal"
	"github.com/tealeg/xlsx/v3"
)

// Enums matching database schema
type ItemCategory string

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

type ItemCondition string

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

// InventoryItem represents a single inventory item
type InventoryItem struct {
	LotID           uuid.UUID
	InvoiceID       string
	AuctionID       int
	ItemName        string
	Description     string
	Category        ItemCategory
	Condition       ItemCondition
	Quantity        int
	BidAmount       decimal.Decimal
	BuyersPremium   decimal.Decimal
	SalesTax        decimal.Decimal
	ShippingCost    decimal.Decimal
	TotalCost       decimal.Decimal
	CostPerItem     decimal.Decimal
	AcquisitionDate time.Time
	Keywords        []string
}

// AuctionInfo holds auction metadata
type AuctionInfo struct {
	AuctionID            int
	InvoiceID            string
	Date                 time.Time
	BuyersPremiumPercent float64
	SalesTaxPercent      float64
}

// SeederState tracks processing state
type SeederState struct {
	ProcessedInvoices []string  `json:"processed_invoices"`
	ProcessedCount    int       `json:"processed_count"`
	LastUpdate        time.Time `json:"last_update"`
}

// CategoryClassifier handles intelligent categorization
type CategoryClassifier struct {
	categoryKeywords  map[ItemCategory][]string
	conditionKeywords map[ItemCondition][]string
}

func NewCategoryClassifier() *CategoryClassifier {
	return &CategoryClassifier{
		categoryKeywords: map[ItemCategory][]string{
			CategoryAntiques: {"antique", "victorian", "edwardian", "georgian", "art deco",
				"art nouveau", "mid century", "mcm", "vintage"},
			CategoryArt: {"painting", "print", "lithograph", "etching", "drawing",
				"sculpture", "statue", "canvas", "watercolor", "oil painting"},
			CategoryBooks: {"book", "volume", "edition", "manuscript", "atlas",
				"encyclopedia", "novel", "hardcover", "paperback"},
			CategoryCeramics: {"ceramic", "porcelain", "pottery", "stoneware", "earthenware",
				"terracotta", "faience", "majolica"},
			CategoryChina: {"china", "dinnerware", "plate", "bowl", "teacup", "saucer",
				"serving", "platter", "tureen", "gravy boat"},
			CategoryClothing: {"dress", "shirt", "pants", "jacket", "coat", "shoes",
				"hat", "scarf", "vintage clothing", "designer"},
			CategoryCoins: {"coin", "numismatic", "currency", "mint", "proof",
				"commemorative", "gold coin", "silver coin"},
			CategoryCollectibles: {"collectible", "limited edition", "memorabilia", "trading card",
				"figurine", "model", "diecast"},
			CategoryElectronics: {"electronic", "computer", "phone", "camera", "stereo",
				"radio", "television", "console", "gadget"},
			CategoryFurniture: {"table", "chair", "desk", "cabinet", "dresser", "sofa",
				"bench", "ottoman", "bookcase", "sideboard", "chest"},
			CategoryGlass: {"glass", "crystal", "cut glass", "pressed glass", "blown glass",
				"stained glass", "depression glass", "carnival glass"},
			CategoryJewelry: {"jewelry", "ring", "necklace", "bracelet", "earring",
				"brooch", "pendant", "gold", "silver", "diamond", "gemstone"},
			CategoryLinens: {"linen", "tablecloth", "napkin", "doily", "runner",
				"bedding", "quilt", "blanket", "textile", "fabric"},
			CategoryMusical: {"musical", "instrument", "piano", "guitar", "violin",
				"trumpet", "saxophone", "drum", "sheet music"},
			CategorySilver: {"sterling", "silver", "silverplate", "flatware", "hollowware",
				"tea set", "candelabra", "serving piece"},
			CategoryStamps: {"stamp", "philatelic", "postage", "first day cover",
				"postmark", "album"},
			CategoryTools: {"tool", "drill", "saw", "hammer", "wrench", "pliers",
				"vintage tool", "woodworking", "machinist"},
			CategoryToys: {"toy", "doll", "action figure", "game", "puzzle",
				"teddy bear", "train set", "lego", "vintage toy"},
		},
		conditionKeywords: map[ItemCondition][]string{
			ConditionMint:        {"mint", "pristine", "perfect", "new"},
			ConditionExcellent:   {"excellent", "near mint", "superb"},
			ConditionVeryGood:    {"very good", "vg", "great"},
			ConditionGood:        {"good", "nice", "decent"},
			ConditionFair:        {"fair", "acceptable", "wear"},
			ConditionPoor:        {"poor", "damaged", "broken", "torn"},
			ConditionRestoration: {"restored", "repaired", "refinished"},
			ConditionParts:       {"parts", "repair", "incomplete", "as-is"},
		},
	}
}

func (c *CategoryClassifier) Classify(text string) (ItemCategory, ItemCondition) {
	textLower := strings.ToLower(text)

	// Find category with highest keyword match score
	categoryScores := make(map[ItemCategory]int)
	for category, keywords := range c.categoryKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(textLower, kw) {
				score++
			}
		}
		if score > 0 {
			categoryScores[category] = score
		}
	}

	category := CategoryOther
	maxScore := 0
	for cat, score := range categoryScores {
		if score > maxScore {
			category = cat
			maxScore = score
		}
	}

	// Find condition
	condition := ConditionUnknown
	for cond, keywords := range c.conditionKeywords {
		for _, kw := range keywords {
			if strings.Contains(textLower, kw) {
				condition = cond
				break
			}
		}
		if condition != ConditionUnknown {
			break
		}
	}

	return category, condition
}

// PDFExtractor handles PDF parsing with proven logic from Python
type PDFExtractor struct {
	classifier *CategoryClassifier
	logger     *slog.Logger
	auctions   map[string]AuctionInfo
	db         *pgxpool.Pool
}

func NewPDFExtractor(db *pgxpool.Pool, logger *slog.Logger) *PDFExtractor {
	return &PDFExtractor{
		classifier: NewCategoryClassifier(),
		logger:     logger,
		auctions:   make(map[string]AuctionInfo),
		db:         db,
	}
}

// LoadAuctions loads auction metadata from Excel file
func (e *PDFExtractor) LoadAuctions(filepath string) error {
	file, err := xlsx.OpenFile(filepath)
	if err != nil {
		return fmt.Errorf("failed to open auctions file: %w", err)
	}
	if len(file.Sheets) == 0 {
		return fmt.Errorf("no sheets found in auctions file")
	}
	sheet := file.Sheets[0]

	rowIdx := 0
	err = sheet.ForEachRow(func(r *xlsx.Row) error {
		// skip header
		if rowIdx == 0 {
			rowIdx++
			return nil
		}
		rowIdx++

		// defensively get cells by index
		get := func(i int) string {
			c := r.GetCell(i)
			if c == nil {
				return ""
			}
			// FormattedValue respects number/date formats
			if s, err := c.FormattedValue(); err == nil {
				return strings.TrimSpace(s)
			}
			return strings.TrimSpace(c.String())
		}

		invoiceID := get(0)
		if invoiceID == "" {
			return nil
		}

		auctionID, _ := strconv.Atoi(get(1))

		dateStr := get(2)
		// adjust layout if your sheet uses a different format
		date, _ := time.Parse("2006-01-02", dateStr)

		bpPercent, _ := strconv.ParseFloat(get(3), 64)
		taxPercent, _ := strconv.ParseFloat(get(4), 64)

		e.auctions[invoiceID] = AuctionInfo{
			AuctionID:            auctionID,
			InvoiceID:            invoiceID,
			Date:                 date,
			BuyersPremiumPercent: bpPercent,
			SalesTaxPercent:      taxPercent,
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to iterate rows: %w", err)
	}

	e.logger.Info("Loaded auction metadata", slog.Int("count", len(e.auctions)))
	return nil
}

// ExtractItemsFromPDF extracts items using the proven Python logic
func (e *PDFExtractor) ExtractItemsFromPDF(filepath string, invoiceID string) ([]InventoryItem, error) {
	e.logger.Info("Processing PDF",
		slog.String("invoice_id", invoiceID),
		slog.String("file", filepath))

	// Get auction info
	auctionInfo := e.getAuctionInfo(invoiceID)

	// Extract text lines from PDF
	textLines, err := e.extractTextLines(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract text: %w", err)
	}

	// Parse items using fallback method (proven logic from Python)
	rawItems := e.extractFallbackItems(textLines)

	// Create inventory items
	items := make([]InventoryItem, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item := e.createInventoryItem(rawItem.description, rawItem.bid, invoiceID, auctionInfo)
		items = append(items, item)
	}

	e.logger.Info("Extracted items from PDF",
		slog.String("invoice_id", invoiceID),
		slog.Int("count", len(items)))

	return items, nil
}

func (e *PDFExtractor) extractTextLines(filepath string) ([]string, error) {
	f, r, err := pdf.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var textLines []string
	totalPages := r.NumPage()

	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			e.logger.Warn("Failed to extract text from page",
				slog.Int("page", pageNum),
				slog.LevelError)
			continue
		}

		lines := strings.Split(text, "\n")
		textLines = append(textLines, lines...)
	}

	return textLines, nil
}

type rawItem struct {
	description string
	bid         float64
}

// extractFallbackItems implements the proven extraction logic from Python
func (e *PDFExtractor) extractFallbackItems(textLines []string) []rawItem {
	// Find start of items table
	start := 0
	headerRe := regexp.MustCompile(`(?i)LOT.*PRICE`)
	for idx, line := range textLines {
		if headerRe.MatchString(line) {
			start = idx + 1
			break
		}
	}

	dashRe := regexp.MustCompile(`-{7,}`)
	footerRe := regexp.MustCompile(`(?i)A payment of`)
	priceRe := regexp.MustCompile(`\d{1,3}(?:,\d{3})*\.\d{2}$`)

	var items []rawItem
	var current *struct {
		descLines []string
		price     float64
	}

	for _, line := range textLines[start:] {
		txt := strings.TrimSpace(line)

		// Skip empty lines or stop at subtotal
		if txt == "" || strings.Contains(strings.ToUpper(txt), "SUBTOTAL") {
			continue
		}

		// Stop at footer
		if footerRe.MatchString(txt) {
			break
		}

		// Strip dash-filler
		if dashRe.MatchString(txt) {
			parts := dashRe.Split(txt, 2)
			txt = strings.TrimSpace(parts[0])
		}

		if txt == "" {
			continue
		}

		// Check if line has a price at the end
		if priceRe.MatchString(txt) {
			parts := strings.Fields(txt)
			if len(parts) == 0 {
				continue
			}

			price := parseCurrency(parts[len(parts)-1])
			if price == 0 {
				continue
			}

			// Strip lot/qty tokens if present
			var descTokens []string
			if len(parts) >= 4 && isDigit(parts[len(parts)-2]) && isDigit(parts[len(parts)-3]) {
				descTokens = parts[:len(parts)-3]
			} else {
				descTokens = parts[:len(parts)-1]
			}

			firstDesc := strings.Join(descTokens, " ")

			// Save previous item if exists
			if current != nil {
				items = append(items, rawItem{
					description: strings.Join(current.descLines, " "),
					bid:         current.price,
				})
			}

			// Start new item
			current = &struct {
				descLines []string
				price     float64
			}{
				descLines: []string{},
				price:     price,
			}

			if firstDesc != "" {
				current.descLines = append(current.descLines, firstDesc)
			}
		} else {
			// This is a continuation line
			if current != nil {
				current.descLines = append(current.descLines, txt)
			} else {
				e.logger.Warn("Orphan overflow line", slog.String("text", txt))
			}
		}
	}

	// Don't forget the last item
	if current != nil {
		items = append(items, rawItem{
			description: strings.Join(current.descLines, " "),
			bid:         current.price,
		})
	}

	return items
}

func (e *PDFExtractor) getAuctionInfo(invoiceID string) AuctionInfo {
	if info, ok := e.auctions[invoiceID]; ok {
		return info
	}

	// Return defaults
	return AuctionInfo{
		AuctionID:            0,
		InvoiceID:            invoiceID,
		Date:                 time.Now(),
		BuyersPremiumPercent: 20.0,
		SalesTaxPercent:      8.0,
	}
}

func (e *PDFExtractor) createInventoryItem(description string, bid float64, invoiceID string, auctionInfo AuctionInfo) InventoryItem {
	// Convert to decimal for precision
	bidDecimal := decimal.NewFromFloat(bid)

	// Calculate costs
	bpRate := decimal.NewFromFloat(auctionInfo.BuyersPremiumPercent / 100)
	taxRate := decimal.NewFromFloat(auctionInfo.SalesTaxPercent / 100)

	buyersPremium := bidDecimal.Mul(bpRate).Round(2)
	subtotal := bidDecimal.Add(buyersPremium)
	salesTax := subtotal.Mul(taxRate).Round(2)
	totalCost := subtotal.Add(salesTax)

	// Classify item
	category, condition := e.classifier.Classify(description)

	// Extract keywords
	keywords := extractKeywords(description)

	// Generate item name
	itemName := generateItemName(description)

	return InventoryItem{
		LotID:           uuid.New(),
		InvoiceID:       invoiceID,
		AuctionID:       auctionInfo.AuctionID,
		ItemName:        itemName,
		Description:     description,
		Category:        category,
		Condition:       condition,
		Quantity:        1,
		BidAmount:       bidDecimal,
		BuyersPremium:   buyersPremium,
		SalesTax:        salesTax,
		TotalCost:       totalCost,
		CostPerItem:     totalCost,
		AcquisitionDate: auctionInfo.Date,
		Keywords:        keywords,
	}
}

// Database operations
func (e *PDFExtractor) SaveItems(ctx context.Context, items []InventoryItem) error {
	if len(items) == 0 {
		return nil
	}

	// Begin transaction
	tx, err := e.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Prepare batch insert
	batch := &pgx.Batch{}

	for _, item := range items {
		keywordsStr := strings.Join(item.Keywords, ",")

		batch.Queue(`
			INSERT INTO inventory (
				lot_id, invoice_id, auction_id, item_name, description,
				category, condition, quantity, bid_amount, buyers_premium,
				sales_tax, shipping_cost, acquisition_date, keywords
			) VALUES (
				$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
			) ON CONFLICT (lot_id) DO NOTHING`,
			item.LotID, item.InvoiceID, item.AuctionID, item.ItemName, item.Description,
			item.Category, item.Condition, item.Quantity, item.BidAmount, item.BuyersPremium,
			item.SalesTax, item.ShippingCost, item.AcquisitionDate, keywordsStr,
		)
	}

	// Execute batch
	br := tx.SendBatch(ctx, batch)
	defer br.Close()

	// Check results
	for range items {
		_, err := br.Exec()
		if err != nil {
			return fmt.Errorf("failed to insert item: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	e.logger.Info("Saved items to database", slog.Int("count", len(items)))
	return nil
}

// Helper functions
func parseCurrency(val string) float64 {
	// Remove commas and dollar signs
	cleaned := strings.ReplaceAll(val, ",", "")
	cleaned = strings.ReplaceAll(cleaned, "$", "")
	cleaned = strings.TrimSpace(cleaned)

	result, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0.0
	}
	return result
}

func isDigit(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func generateItemName(description string) string {
	// Take first 50 characters or first sentence
	name := description
	if len(name) > 50 {
		name = name[:50]
		if idx := strings.Index(description[:50], "."); idx > 0 {
			name = description[:idx]
		}
	}

	// Clean up
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	// Remove lot number if at start
	name = regexp.MustCompile(`^\d+\s+`).ReplaceAllString(name, "")

	if name == "" {
		return "Unknown Item"
	}

	// Title case
	return strings.Title(strings.ToLower(name))
}

func extractKeywords(description string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
	}

	// Extract words
	wordRe := regexp.MustCompile(`\b[a-zA-Z]+\b`)
	words := wordRe.FindAllString(strings.ToLower(description), -1)

	// Filter and deduplicate
	seen := make(map[string]bool)
	var keywords []string

	for _, word := range words {
		if !stopWords[word] && len(word) > 2 && !seen[word] {
			keywords = append(keywords, word)
			seen[word] = true
			if len(keywords) >= 20 {
				break
			}
		}
	}

	return keywords
}

func main() {
	// Parse flags
	var (
		invoicesDir  = flag.String("invoices", "./invoices", "Directory containing PDF invoices")
		auctionsFile = flag.String("auctions", "./auctions.xlsx", "Excel file with auction metadata")
		stateFile    = flag.String("state", "./.seed_state.json", "State file for tracking progress")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
		dryRun       = flag.Bool("dry-run", false, "Preview changes without modifying database")
		force        = flag.Bool("force", false, "Reprocess all invoices")
	)
	flag.Parse()

	// Setup logging
	var slogLevel slog.Level
	switch *logLevel {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: slogLevel,
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, opts))
	slog.SetDefault(logger)

	// Database connection
	dbURL := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSL_MODE"),
	)

	ctx := context.Background()

	var db *pgxpool.Pool
	var err error

	if !*dryRun {
		db, err = pgxpool.New(ctx, dbURL)
		if err != nil {
			logger.Error("Failed to connect to database", slog.LevelError)
			os.Exit(1)
		}
		defer db.Close()
	}

	// Create extractor
	extractor := NewPDFExtractor(db, logger)

	// Load auctions if file exists
	if _, err := os.Stat(*auctionsFile); err == nil {
		if err := extractor.LoadAuctions(*auctionsFile); err != nil {
			logger.Error("Failed to load auctions", slog.LevelError)
			// Continue without auction metadata
		}
	}

	// Load state
	var state SeederState
	if !*force {
		if stateData, err := os.ReadFile(*stateFile); err == nil {
			json.Unmarshal(stateData, &state)
		}
	}

	// Process PDFs
	pdfFiles, err := filepath.Glob(filepath.Join(*invoicesDir, "*.pdf"))
	if err != nil {
		logger.Error("Failed to find PDF files", slog.LevelError)
		os.Exit(1)
	}

	totalProcessed := 0
	totalItems := 0

	for i, pdfFile := range pdfFiles {
		invoiceID := strings.TrimSuffix(filepath.Base(pdfFile), ".pdf")

		// Progress indicator
		fmt.Printf("PROGRESS: Processing %d/%d: %s\n", i+1, len(pdfFiles), invoiceID)

		// Check if already processed
		if !*force {
			processed := false
			for _, pid := range state.ProcessedInvoices {
				if pid == invoiceID {
					processed = true
					break
				}
			}
			if processed {
				logger.Info("Skipping already processed invoice", slog.String("invoice_id", invoiceID))
				continue
			}
		}

		// Extract items
		items, err := extractor.ExtractItemsFromPDF(pdfFile, invoiceID)
		if err != nil {
			logger.Error("Failed to extract items",
				slog.String("invoice_id", invoiceID),
				slog.LevelError)
			fmt.Printf("ERROR: Failed to process invoice_id:%s - %v\n", invoiceID, err)
			continue
		}

		// Save to database
		if !*dryRun && len(items) > 0 {
			if err := extractor.SaveItems(ctx, items); err != nil {
				logger.Error("Failed to save items",
					slog.String("invoice_id", invoiceID),
					slog.LevelError)
				fmt.Printf("ERROR: Failed to save invoice_id:%s - %v\n", invoiceID, err)
				continue
			}
		}

		fmt.Printf("SUCCESS: Processed invoice_id:%s - %d items\n", invoiceID, len(items))

		totalProcessed++
		totalItems += len(items)

		// Update state
		state.ProcessedInvoices = append(state.ProcessedInvoices, invoiceID)
		state.ProcessedCount = len(state.ProcessedInvoices)
		state.LastUpdate = time.Now()
	}

	// Save state
	if !*dryRun {
		stateData, _ := json.MarshalIndent(state, "", "  ")
		os.WriteFile(*stateFile, stateData, 0644)
	}

	// Summary
	logger.Info("Seed operation completed",
		slog.Int("invoices_processed", totalProcessed),
		slog.Int("items_created", totalItems))

	if *dryRun {
		fmt.Println("\n[DRY RUN] No changes were made to the database")
	}
}
