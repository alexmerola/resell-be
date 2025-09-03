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
			CategoryArt: {"painting", "print", "lithograph", "etching", "drawing", "framed",
				"sculpture", "statue", "canvas", "watercolor", "oil painting", "serigraph"},
			CategoryBooks: {"book", "volume", "edition", "manuscript", "atlas",
				"encyclopedia", "novel", "hardcover", "paperback"},
			CategoryCeramics: {"ceramic", "porcelain", "pottery", "stoneware", "earthenware",
				"terracotta", "faience", "majolica", "capodimonte", "capidimonte"},
			CategoryChina: {"china", "dinnerware", "plate", "bowl", "teacup", "saucer",
				"serving", "platter", "tureen", "gravy boat", "ming"},
			CategoryClothing: {"dress", "shirt", "pants", "jacket", "coat", "shoes",
				"hat", "scarf", "vintage clothing", "designer"},
			CategoryCoins: {"coin", "numismatic", "currency", "mint", "proof",
				"commemorative", "gold coin", "silver coin"},
			CategoryCollectibles: {"collectible", "limited edition", "memorabilia", "trading card",
				"figurine", "model", "diecast", "precious moments", "danbury mint", "enesco", "lladro"},
			CategoryElectronics: {"electronic", "computer", "phone", "camera", "stereo",
				"radio", "television", "console", "gadget", "sewing machine", "grinder"},
			CategoryFurniture: {"table", "chair", "desk", "cabinet", "dresser", "sofa", "lamp",
				"bench", "ottoman", "bookcase", "sideboard", "chest", "console", "barstool", "shelves"},
			CategoryGlass: {"glass", "crystal", "cut glass", "pressed glass", "blown glass",
				"stained glass", "depression glass", "carnival glass", "art glass", "vase", "bowl"},
			CategoryJewelry: {"jewelry", "ring", "necklace", "bracelet", "earring",
				"brooch", "pendant", "gold", "silver", "diamond", "gemstone", "sterling"},
			CategoryLinens: {"linen", "tablecloth", "napkin", "doily", "runner",
				"bedding", "quilt", "blanket", "textile", "fabric"},
			CategoryMusical: {"musical", "instrument", "piano", "guitar", "violin",
				"trumpet", "saxophone", "drum", "sheet music", "music box"},
			CategorySilver: {"sterling", "silver", "silverplate", "flatware", "hollowware",
				"tea set", "candelabra", "serving piece"},
			CategoryStamps: {"stamp", "philatelic", "postage", "first day cover",
				"postmark", "album"},
			CategoryTools: {"tool", "drill", "saw", "hammer", "wrench", "pliers",
				"vintage tool", "woodworking", "machinist", "grinder"},
			CategoryToys: {"toy", "doll", "action figure", "game", "puzzle",
				"teddy bear", "train set", "lego", "vintage toy", "lionel"},
			CategoryVintage: {"brass", "cherub", "andirons", "bookend", "dolphin", "copper", "bronze"},
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

// PDFExtractor handles PDF parsing with enhanced logic
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
		// Skip header
		if rowIdx == 0 {
			rowIdx++
			return nil
		}
		rowIdx++

		// Get cells by index
		get := func(i int) string {
			c := r.GetCell(i)
			if c == nil {
				return ""
			}
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

// ExtractItemsFromPDF extracts items from your specific PDF format
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

	// Parse items using the corrected logic
	rawItems := e.extractItemsFromInvoice(textLines)

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
				slog.String("error", err.Error()))
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

// extractItemsFromInvoice - Fixed to match the working Python logic
// extractItemsFromInvoice - robust line-buffering and zero-price support
func (e *PDFExtractor) extractItemsFromInvoice(textLines []string) []rawItem {
	var items []rawItem

	// Header/footer and helpers
	headerRe := regexp.MustCompile(`(?i)(LOT.*PRICE|LEAD.*ITEM.*PRICE)`)
	dashRe := regexp.MustCompile(`-{7,}`)
	footerRe := regexp.MustCompile(`(?i)(A payment of|SUBTOTAL)`)
	// allow optional $ and thousands separators, anchored to end of line
	priceRe := regexp.MustCompile(`\$?\s*\d{1,3}(?:,\d{3})*\.\d{2}\s*$`)

	// Find start (line after header)
	start := 0
	for idx, line := range textLines {
		if headerRe.MatchString(line) {
			start = idx + 1
			e.logger.Debug("Found header", slog.Int("line", idx))
			break
		}
	}
	if start == 0 {
		e.logger.Warn("No header found, starting from beginning")
	}

	// Buffer description lines until we see a price
	var pendingDesc []string

	// helper to finalize one item
	addItem := func(desc string, price float64) {
		desc = cleanDescription(desc)
		if strings.TrimSpace(desc) == "" {
			return
		}
		items = append(items, rawItem{
			description: desc,
			bid:         price, // may be 0.00
		})
	}

	for i := start; i < len(textLines); i++ {
		line := strings.TrimSpace(textLines[i])
		if line == "" {
			continue
		}

		// Hit footer: stop parsing items
		if footerRe.MatchString(line) {
			e.logger.Debug("Found footer, stopping", slog.String("line", line))
			break
		}

		// Strip long filler dashes if present (keep left part as content)
		if dashRe.MatchString(line) {
			parts := dashRe.Split(line, 2)
			line = strings.TrimSpace(parts[0])
			if line == "" {
				continue
			}
		}

		// If the line ends with a price, finalize the buffered description + inline desc fragment
		if priceRe.MatchString(line) {
			// Extract numeric price string
			priceStr := strings.TrimSpace(priceRe.FindString(line))
			price := parseCurrency(priceStr)

			// Description fragment on same line (before the price)
			descPart := strings.TrimSpace(priceRe.ReplaceAllString(line, ""))

			// Some PDFs place lot/metadata between desc and price on the same line
			// Example patterns like "18488 17" or "6607 28" or "131811 65 G2CG2C"
			metaRe := regexp.MustCompile(`\b[0-9A-Z]{2,}(?:\s+[0-9A-Z]{1,}){0,3}$`)
			descPart = strings.TrimSpace(metaRe.ReplaceAllString(descPart, ""))

			// Merge: buffered + inline fragment
			fullDesc := strings.Join(append(pendingDesc, descPart), " ")
			fullDesc = strings.TrimSpace(fullDesc)

			addItem(fullDesc, price)

			// Reset buffer for next item
			pendingDesc = pendingDesc[:0]
			continue
		}

		// Otherwise, this is part of the description—buffer it
		pendingDesc = append(pendingDesc, line)
	}

	// Note: do NOT emit a trailing buffered item without a detected price.
	// These invoices always have a SUBTOTAL after items; if we never saw a price,
	// we likely buffered non-item text (headers/notes).
	e.logger.Info("Extracted raw items", slog.Int("count", len(items)))
	return items
}

func cleanDescription(desc string) string {
	// Remove item IDs and lot numbers that might be embedded
	desc = regexp.MustCompile(`\b\d{5,6}\s+\d{1,3}\s+[A-Z0-9]+\b`).ReplaceAllString(desc, "")

	// Remove standalone numbers that are likely IDs
	desc = regexp.MustCompile(`^\d+\s+`).ReplaceAllString(desc, "")
	desc = regexp.MustCompile(`\s+\d{4,}$`).ReplaceAllString(desc, "")

	// Remove multiple spaces
	desc = regexp.MustCompile(`\s+`).ReplaceAllString(desc, " ")

	// Remove dashes used as fillers
	desc = regexp.MustCompile(`-{3,}`).ReplaceAllString(desc, " ")

	// Clean up
	desc = strings.TrimSpace(desc)

	return desc
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
		BuyersPremiumPercent: 18.0,  // Common default
		SalesTaxPercent:      8.625, // NY sales tax
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

// SaveItems persists inventory items to the database
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

	// Process all batch results
	for range items {
		_, err := br.Exec()
		if err != nil {
			br.Close()
			return fmt.Errorf("failed to insert item: %w", err)
		}
	}

	// Close batch results
	if err := br.Close(); err != nil {
		return fmt.Errorf("failed to close batch results: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	e.logger.Info("Saved items to database", slog.Int("count", len(items)))
	return nil
}

// Helper functions
func parseCurrency(val string) float64 {
	// Remove dollar sign, commas, and spaces
	cleaned := strings.ReplaceAll(val, "$", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	cleaned = strings.TrimSpace(cleaned)

	result, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0.0
	}
	return result
}

func generateItemName(description string) string {
	// Take first 60 characters or first sentence
	name := description
	if len(name) > 60 {
		name = name[:60]
		if idx := strings.Index(description[:60], "."); idx > 0 {
			name = description[:idx]
		}
	}

	// Clean up
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)

	// Remove any leading numbers
	name = regexp.MustCompile(`^\d+\s+`).ReplaceAllString(name, "")

	if name == "" {
		return "Unknown Item"
	}

	// Title case
	words := strings.Fields(strings.ToLower(name))
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(string(word[0])) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

func extractKeywords(description string) []string {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "was": true, "are": true, "were": true, "total": true,
		"set": true, "lot": true, "pair": true,
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
		getEnv("DB_USER", "resell"),
		getEnv("DB_PASSWORD", "resell_dev_2025"),
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_NAME", "resell_inventory"),
		getEnv("DB_SSL_MODE", "disable"),
	)

	ctx := context.Background()

	var db *pgxpool.Pool
	var err error

	if !*dryRun {
		db, err = pgxpool.New(ctx, dbURL)
		if err != nil {
			logger.Error("Failed to connect to database", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer db.Close()
	}

	// Create extractor
	extractor := NewPDFExtractor(db, logger)

	// Load auctions if file exists
	if _, err := os.Stat(*auctionsFile); err == nil {
		if err := extractor.LoadAuctions(*auctionsFile); err != nil {
			logger.Error("Failed to load auctions", slog.String("error", err.Error()))
			// Continue without auction metadata
		}
	}

	// Load state
	type SeederState struct {
		ProcessedInvoices []string  `json:"processed_invoices"`
		ProcessedCount    int       `json:"processed_count"`
		LastUpdate        time.Time `json:"last_update"`
	}

	var state SeederState
	if !*force {
		if stateData, err := os.ReadFile(*stateFile); err == nil {
			json.Unmarshal(stateData, &state)
		}
	}

	// Process PDFs
	pdfFiles, err := filepath.Glob(filepath.Join(*invoicesDir, "*.pdf"))
	if err != nil {
		logger.Error("Failed to find PDF files", slog.String("error", err.Error()))
		os.Exit(1)
	}

	totalProcessed := 0
	totalItems := 0
	failedInvoices := []string{}
	successDetails := map[string]int{}

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
				slog.String("error", err.Error()))
			failedInvoices = append(failedInvoices, invoiceID)
			fmt.Printf("ERROR: Failed to process invoice_id:%s - %v\n", invoiceID, err)
			continue
		}

		if len(items) == 0 {
			logger.Warn("No items extracted",
				slog.String("invoice_id", invoiceID))
			fmt.Printf("WARNING: No items found in invoice_id:%s\n", invoiceID)
			failedInvoices = append(failedInvoices, fmt.Sprintf("%s (0 items)", invoiceID))
			continue
		}

		// Save to database
		if !*dryRun && len(items) > 0 {
			if err := extractor.SaveItems(ctx, items); err != nil {
				logger.Error("Failed to save items",
					slog.String("invoice_id", invoiceID),
					slog.String("error", err.Error()))
				failedInvoices = append(failedInvoices, invoiceID)
				fmt.Printf("ERROR: Failed to save invoice_id:%s - %v\n", invoiceID, err)
				continue
			}
		}

		fmt.Printf("SUCCESS: Processed invoice_id:%s - %d items\n", invoiceID, len(items))
		successDetails[invoiceID] = len(items)

		totalProcessed++
		totalItems += len(items)

		// Update state
		state.ProcessedInvoices = append(state.ProcessedInvoices, invoiceID)
		state.ProcessedCount = len(state.ProcessedInvoices)
		state.LastUpdate = time.Now()

		// Save state periodically
		if !*dryRun && i%10 == 0 {
			stateData, _ := json.MarshalIndent(state, "", "  ")
			os.WriteFile(*stateFile, stateData, 0644)
		}
	}

	// Save final state
	if !*dryRun {
		stateData, _ := json.MarshalIndent(state, "", "  ")
		os.WriteFile(*stateFile, stateData, 0644)
	}

	// Summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("📊 SEEDING OPERATION SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total PDFs Processed: %d\n", totalProcessed)
	fmt.Printf("Total Items Extracted: %d\n", totalItems)
	if totalProcessed > 0 {
		fmt.Printf("Average Items per Invoice: %.1f\n", float64(totalItems)/float64(totalProcessed))
	}

	// Show successful extractions
	if len(successDetails) > 0 {
		fmt.Printf("\n✅ Successfully Processed (%d invoices):\n", len(successDetails))
		for inv, count := range successDetails {
			fmt.Printf("  - %s: %d items\n", inv, count)
		}
	}

	if len(failedInvoices) > 0 {
		fmt.Printf("\n⚠️  Failed/Empty Invoices (%d):\n", len(failedInvoices))
		for _, inv := range failedInvoices {
			fmt.Printf("  - %s\n", inv)
		}
	}

	logger.Info("Seed operation completed",
		slog.Int("invoices_processed", totalProcessed),
		slog.Int("items_created", totalItems),
		slog.Int("failed_invoices", len(failedInvoices)))

	if *dryRun {
		fmt.Println("\n[DRY RUN] No changes were made to the database")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
