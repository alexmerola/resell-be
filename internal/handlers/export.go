// internal/handlers/export.go
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tealeg/xlsx/v3"

	redis_a "github.com/ammerola/resell-be/internal/adapters/redis_adapter"
	"github.com/ammerola/resell-be/internal/core/ports"
)

// ExportParams defines parameters for export operations
type ExportParams struct {
	Columns        []string   `json:"columns"`
	IncludeDeleted bool       `json:"include_deleted"`
	DateFrom       *time.Time `json:"date_from"`
	DateTo         *time.Time `json:"date_to"`
	Format         string     `json:"format"`
	Filters        []any      `json:"filters"`
}

// ExcelExportRow represents a row in the Excel export materialized view
type ExcelExportRow struct {
	LotID           *string    `db:"lot_id"`
	InvoiceID       string     `db:"invoice_id"`
	AuctionID       int        `db:"auction_id"`
	ItemName        string     `db:"item_name"`
	Description     string     `db:"description"`
	Category        string     `db:"category"`
	Condition       string     `db:"condition"`
	Quantity        int        `db:"quantity"`
	BidAmount       *float64   `db:"bid_amount"`
	BuyersPremium   *float64   `db:"buyers_premium"`
	SalesTax        *float64   `db:"sales_tax"`
	ShippingCost    *float64   `db:"shipping_cost"`
	TotalCost       *float64   `db:"total_cost"`
	CostPerItem     *float64   `db:"cost_per_item"`
	AcquisitionDate *time.Time `db:"acquisition_date"`
	StorageLocation *string    `db:"storage_location"`
	StorageBin      *string    `db:"storage_bin"`
	EbayListed      bool       `db:"ebay_listed"`
	EbayPrice       *float64   `db:"ebay_price"`
	EbayURL         *string    `db:"ebay_url"`
	EbaySold        bool       `db:"ebay_sold"`
	EtsyListed      bool       `db:"etsy_listed"`
	EtsyPrice       *float64   `db:"etsy_price"`
	EtsyURL         *string    `db:"etsy_url"`
	EtsySold        bool       `db:"etsy_sold"`
	SalePrice       *float64   `db:"sale_price"`
	NetProfit       *float64   `db:"net_profit"`
	ROIPercent      *float64   `db:"roi_percent"`
	DaysToSell      *int       `db:"days_to_sell"`
	CreatedAt       time.Time  `db:"created_at"`
	UpdatedAt       time.Time  `db:"updated_at"`
}

// JSONExportResponse represents the JSON export response structure
type JSONExportResponse struct {
	Inventory []map[string]any `json:"inventory"`
	Metadata  ExportMetadata   `json:"metadata"`
}

// ExportMetadata contains metadata about the export
type ExportMetadata struct {
	ExportDate     time.Time `json:"export_date"`
	TotalItems     int       `json:"total_items"`
	FiltersApplied []any     `json:"filters_applied"`
	IncludeDeleted bool      `json:"include_deleted"`
	Columns        []string  `json:"columns"`
}

// ExportHandler handles export operations
type ExportHandler struct {
	inventoryService ports.InventoryService
	db               ports.Database
	cache            ports.CacheRepository
	logger           *slog.Logger
}

// NewExportHandler creates a new export handler
func NewExportHandler(inventoryService ports.InventoryService, db ports.Database, cache ports.CacheRepository, logger *slog.Logger) *ExportHandler {
	return &ExportHandler{
		inventoryService: inventoryService,
		db:               db,
		cache:            cache,
		logger:           logger.With(slog.String("handler", "export")),
	}
}

// ExportExcel handles GET /api/v1/export/excel
func (h *ExportHandler) ExportExcel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters for filters
	params := h.parseExportParams(r)

	h.logger.InfoContext(ctx, "Starting Excel export",
		slog.Any("params", params))

	// Get all inventory data at once (optimal for small datasets)
	data, err := h.getInventoryData(ctx, params)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to retrieve inventory data", slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to retrieve data")
		return
	}

	// Generate Excel file in memory
	excelData, err := h.generateExcelFile(data, params)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to generate Excel file", slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to generate Excel file")
		return
	}

	// Set response headers
	filename := fmt.Sprintf("inventory_export_%s.xlsx", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(excelData)))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Write file data
	if _, err := w.Write(excelData); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write Excel response", slog.String("error", err.Error()))
		return
	}

	h.logger.InfoContext(ctx, "Excel export completed successfully",
		slog.Int("total_rows", len(data)),
		slog.String("filename", filename))
}

// ExportJSON handles GET /api/v1/export/json
func (h *ExportHandler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse parameters
	params := h.parseExportParams(r)

	h.logger.InfoContext(ctx, "Starting JSON export",
		slog.Any("params", params))

	// Check cache first
	cacheKey := redis_a.BuildKey(redis_a.PrefixExport, "json", h.getCacheKeyFromParams(params))
	var cachedData []byte
	if err := h.cache.Get(ctx, cacheKey, &cachedData); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("Content-Length", strconv.Itoa(len(cachedData)))

		if _, err := w.Write(cachedData); err != nil {
			h.logger.ErrorContext(ctx, "Failed to write cached JSON response", slog.String("error", err.Error()))
			return
		}

		h.logger.InfoContext(ctx, "JSON export served from cache")
		return
	}

	// Get inventory data
	data, err := h.getInventoryData(ctx, params)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to retrieve inventory data", slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to retrieve data")
		return
	}

	// Convert to JSON-friendly format
	jsonData := make([]map[string]any, 0, len(data))
	for _, item := range data {
		jsonData = append(jsonData, h.itemToJSONMap(&item, params.Columns))
	}

	// Create response with metadata
	response := JSONExportResponse{
		Inventory: jsonData,
		Metadata: ExportMetadata{
			ExportDate:     time.Now(),
			TotalItems:     len(jsonData),
			FiltersApplied: params.Filters,
			IncludeDeleted: params.IncludeDeleted,
			Columns:        params.Columns,
		},
	}

	// Marshal response
	responseData, err := json.Marshal(response)
	if err != nil {
		h.logger.ErrorContext(ctx, "Failed to marshal JSON response", slog.String("error", err.Error()))
		h.respondError(w, http.StatusInternalServerError, "Failed to generate JSON")
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("Content-Length", strconv.Itoa(len(responseData)))

	// Write response
	if _, err := w.Write(responseData); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write JSON response", slog.String("error", err.Error()))
		return
	}

	// Cache the result for 5 minutes (async)
	go func() {
		cacheCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := h.cache.Set(cacheCtx, cacheKey, responseData); err != nil {
			h.logger.WarnContext(cacheCtx, "Failed to cache JSON response", slog.String("error", err.Error()))
		}
	}()

	h.logger.InfoContext(ctx, "JSON export completed successfully",
		slog.Int("total_rows", len(data)))
}

// ExportPDF handles GET /api/v1/export/pdf
func (h *ExportHandler) ExportPDF(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	h.logger.InfoContext(ctx, "PDF export not yet implemented")

	// Set response headers for future PDF implementation
	filename := fmt.Sprintf("inventory_report_%s.pdf", time.Now().Format("20060102_150405"))
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	// Placeholder implementation
	placeholder := []byte("%PDF-1.4\n1 0 obj\n<<\n/Type /Catalog\n/Pages 2 0 R\n>>\nendobj\n2 0 obj\n<<\n/Type /Pages\n/Kids [3 0 R]\n/Count 1\n>>\nendobj\n3 0 obj\n<<\n/Type /Page\n/Parent 2 0 R\n/MediaBox [0 0 612 792]\n/Contents 4 0 R\n>>\nendobj\n4 0 obj\n<<\n/Length 44\n>>\nstream\nBT\n/F1 12 Tf\n72 720 Td\n(PDF export coming soon!) Tj\nET\nendstream\nendobj\nxref\n0 5\n0000000000 65535 f \n0000000009 00000 n \n0000000058 00000 n \n0000000115 00000 n \n0000000206 00000 n \ntrailer\n<<\n/Size 5\n/Root 1 0 R\n>>\nstartxref\n299\n%%EOF")

	w.Header().Set("Content-Length", strconv.Itoa(len(placeholder)))

	if _, err := w.Write(placeholder); err != nil {
		h.logger.ErrorContext(ctx, "Failed to write PDF response", slog.String("error", err.Error()))
		return
	}

	h.logger.InfoContext(ctx, "PDF placeholder response sent")
}

// Helper methods

// parseExportParams parses and validates export parameters from the request
func (h *ExportHandler) parseExportParams(r *http.Request) *ExportParams {
	params := &ExportParams{
		Columns: []string{"all"},
		Filters: make([]any, 0),
	}

	// Parse columns
	if cols := r.URL.Query().Get("columns"); cols != "" {
		params.Columns = strings.Split(strings.TrimSpace(cols), ",")
		// Clean up column names
		for i, col := range params.Columns {
			params.Columns[i] = strings.TrimSpace(col)
		}
	}

	// Parse include_deleted flag
	params.IncludeDeleted = r.URL.Query().Get("include_deleted") == "true"

	// Parse date range
	if from := r.URL.Query().Get("date_from"); from != "" {
		if t, err := time.Parse("2006-01-02", from); err == nil {
			params.DateFrom = &t
			params.Filters = append(params.Filters, t)
		}
	}

	if to := r.URL.Query().Get("date_to"); to != "" {
		if t, err := time.Parse("2006-01-02", to); err == nil {
			params.DateTo = &t
			params.Filters = append(params.Filters, t)
		}
	}

	// Parse format
	params.Format = r.URL.Query().Get("format")
	if params.Format == "" {
		params.Format = "xlsx"
	}

	return params
}

// getInventoryData retrieves all inventory data based on export parameters
func (h *ExportHandler) getInventoryData(ctx context.Context, params *ExportParams) ([]ExcelExportRow, error) {
	query := h.buildExportQuery(params)

	rows, err := h.db.Query(ctx, query, params.getQueryArgs()...)
	if err != nil {
		return nil, fmt.Errorf("failed to query inventory data: %w", err)
	}
	defer rows.Close()

	var data []ExcelExportRow
	for rows.Next() {
		var item ExcelExportRow
		if err := rows.Scan(&item); err != nil {
			h.logger.WarnContext(ctx, "Failed to scan inventory row", slog.String("error", err.Error()))
			continue
		}
		data = append(data, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating inventory rows: %w", err)
	}

	return data, nil
}

// buildExportQuery constructs the SQL query based on export parameters
func (h *ExportHandler) buildExportQuery(params *ExportParams) string {
	query := "SELECT * FROM inventory_excel_export_mat WHERE 1=1"

	if params.DateFrom != nil {
		query += " AND acquisition_date >= $1"
	}
	if params.DateTo != nil {
		if params.DateFrom != nil {
			query += " AND acquisition_date <= $2"
		} else {
			query += " AND acquisition_date <= $1"
		}
	}
	if !params.IncludeDeleted {
		query += " AND deleted_at IS NULL"
	}

	query += " ORDER BY created_at DESC"
	return query
}

// generateExcelFile creates an Excel file in memory from the data
func (h *ExportHandler) generateExcelFile(data []ExcelExportRow, params *ExportParams) ([]byte, error) {
	// Create new Excel file
	file := xlsx.NewFile()

	// Add worksheet
	sheet, err := file.AddSheet("Inventory")
	if err != nil {
		return nil, fmt.Errorf("failed to add worksheet: %w", err)
	}

	// Get headers and add header row
	headers := h.getExcelHeaders(params.Columns)
	headerRow := sheet.AddRow()
	for _, header := range headers {
		cell := headerRow.AddCell()
		cell.Value = header
		cell.GetStyle().Font.Bold = true
		cell.GetStyle().Fill.PatternType = "solid"
		cell.GetStyle().Fill.FgColor = "CCCCCC"
	}

	// Add data rows
	for _, item := range data {
		dataRow := sheet.AddRow()
		rowData := h.itemToExcelRow(&item, params.Columns)

		for _, value := range rowData {
			cell := dataRow.AddCell()
			cell.Value = value
		}
	}

	// Auto-fit column widths (approximate)
	for i := 0; i < len(headers); i++ {
		sheet.SetColWidth(i, i, 15) // Set reasonable default width
	}

	// Save to buffer
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		return nil, fmt.Errorf("failed to write Excel file to buffer: %w", err)
	}

	return buffer.Bytes(), nil
}

// getExcelHeaders returns the appropriate headers based on requested columns
func (h *ExportHandler) getExcelHeaders(columns []string) []string {
	allHeaders := []string{
		"Lot ID", "Invoice ID", "Auction ID", "Item Name", "Description",
		"Category", "Condition", "Quantity", "Bid Amount", "Buyer's Premium",
		"Sales Tax", "Shipping Cost", "Total Cost", "Cost Per Item",
		"Acquisition Date", "Storage Location", "Storage Bin",
		"eBay Listed", "eBay Price", "eBay URL", "eBay Sold",
		"Etsy Listed", "Etsy Price", "Etsy URL", "Etsy Sold",
		"Sale Price", "Net Profit", "ROI %", "Days to Sell",
		"Created At", "Updated At",
	}

	if len(columns) == 1 && columns[0] == "all" {
		return allHeaders
	}

	// Map requested columns to headers
	headerMap := map[string]string{
		"lot_id":           "Lot ID",
		"invoice_id":       "Invoice ID",
		"auction_id":       "Auction ID",
		"item_name":        "Item Name",
		"description":      "Description",
		"category":         "Category",
		"condition":        "Condition",
		"quantity":         "Quantity",
		"bid_amount":       "Bid Amount",
		"buyers_premium":   "Buyer's Premium",
		"sales_tax":        "Sales Tax",
		"shipping_cost":    "Shipping Cost",
		"total_cost":       "Total Cost",
		"cost_per_item":    "Cost Per Item",
		"acquisition_date": "Acquisition Date",
		"storage_location": "Storage Location",
		"storage_bin":      "Storage Bin",
		"ebay_listed":      "eBay Listed",
		"ebay_price":       "eBay Price",
		"ebay_url":         "eBay URL",
		"ebay_sold":        "eBay Sold",
		"etsy_listed":      "Etsy Listed",
		"etsy_price":       "Etsy Price",
		"etsy_url":         "Etsy URL",
		"etsy_sold":        "Etsy Sold",
		"sale_price":       "Sale Price",
		"net_profit":       "Net Profit",
		"roi_percent":      "ROI %",
		"days_to_sell":     "Days to Sell",
		"created_at":       "Created At",
		"updated_at":       "Updated At",
	}

	var selectedHeaders []string
	for _, col := range columns {
		if header, exists := headerMap[col]; exists {
			selectedHeaders = append(selectedHeaders, header)
		}
	}

	if len(selectedHeaders) == 0 {
		return allHeaders // Fallback to all headers if none match
	}

	return selectedHeaders
}

// itemToExcelRow converts a data item to Excel row values
func (h *ExportHandler) itemToExcelRow(item *ExcelExportRow, columns []string) []string {
	allValues := []string{
		h.safeStringValue(item.LotID),
		item.InvoiceID,
		strconv.Itoa(item.AuctionID),
		item.ItemName,
		item.Description,
		item.Category,
		item.Condition,
		strconv.Itoa(item.Quantity),
		h.safeFloatValue(item.BidAmount),
		h.safeFloatValue(item.BuyersPremium),
		h.safeFloatValue(item.SalesTax),
		h.safeFloatValue(item.ShippingCost),
		h.safeFloatValue(item.TotalCost),
		h.safeFloatValue(item.CostPerItem),
		h.safeDateValue(item.AcquisitionDate),
		h.safeStringValue(item.StorageLocation),
		h.safeStringValue(item.StorageBin),
		h.safeBoolValue(item.EbayListed),
		h.safeFloatValue(item.EbayPrice),
		h.safeStringValue(item.EbayURL),
		h.safeBoolValue(item.EbaySold),
		h.safeBoolValue(item.EtsyListed),
		h.safeFloatValue(item.EtsyPrice),
		h.safeStringValue(item.EtsyURL),
		h.safeBoolValue(item.EtsySold),
		h.safeFloatValue(item.SalePrice),
		h.safeFloatValue(item.NetProfit),
		h.safeFloatValue(item.ROIPercent),
		h.safeIntValue(item.DaysToSell),
		item.CreatedAt.Format("2006-01-02 15:04:05"),
		item.UpdatedAt.Format("2006-01-02 15:04:05"),
	}

	if len(columns) == 1 && columns[0] == "all" {
		return allValues
	}

	// Return only requested columns - would need column mapping logic here
	return allValues // For simplicity, returning all for now
}

// itemToJSONMap converts a data item to a JSON-friendly map
func (h *ExportHandler) itemToJSONMap(item *ExcelExportRow, columns []string) map[string]any {
	result := make(map[string]any)

	result["lot_id"] = item.LotID
	result["invoice_id"] = item.InvoiceID
	result["auction_id"] = item.AuctionID
	result["item_name"] = item.ItemName
	result["description"] = item.Description
	result["category"] = item.Category
	result["condition"] = item.Condition
	result["quantity"] = item.Quantity
	result["bid_amount"] = item.BidAmount
	result["buyers_premium"] = item.BuyersPremium
	result["sales_tax"] = item.SalesTax
	result["shipping_cost"] = item.ShippingCost
	result["total_cost"] = item.TotalCost
	result["cost_per_item"] = item.CostPerItem
	result["acquisition_date"] = item.AcquisitionDate
	result["storage_location"] = item.StorageLocation
	result["storage_bin"] = item.StorageBin
	result["ebay_listed"] = item.EbayListed
	result["ebay_price"] = item.EbayPrice
	result["ebay_url"] = item.EbayURL
	result["ebay_sold"] = item.EbaySold
	result["etsy_listed"] = item.EtsyListed
	result["etsy_price"] = item.EtsyPrice
	result["etsy_url"] = item.EtsyURL
	result["etsy_sold"] = item.EtsySold
	result["sale_price"] = item.SalePrice
	result["net_profit"] = item.NetProfit
	result["roi_percent"] = item.ROIPercent
	result["days_to_sell"] = item.DaysToSell
	result["created_at"] = item.CreatedAt
	result["updated_at"] = item.UpdatedAt

	// If specific columns requested, filter the result
	if len(columns) > 0 && !(len(columns) == 1 && columns[0] == "all") {
		filtered := make(map[string]any)
		for _, col := range columns {
			if value, exists := result[col]; exists {
				filtered[col] = value
			}
		}
		return filtered
	}

	return result
}

// Utility methods for safe value conversion

func (h *ExportHandler) safeStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (h *ExportHandler) safeFloatValue(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *value)
}

func (h *ExportHandler) safeDateValue(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}

func (h *ExportHandler) safeBoolValue(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func (h *ExportHandler) safeIntValue(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func (h *ExportHandler) getCacheKeyFromParams(params *ExportParams) string {
	// Create a simple cache key from params
	key := fmt.Sprintf("cols_%s_del_%t", strings.Join(params.Columns, ","), params.IncludeDeleted)
	if params.DateFrom != nil {
		key += fmt.Sprintf("_from_%s", params.DateFrom.Format("20060102"))
	}
	if params.DateTo != nil {
		key += fmt.Sprintf("_to_%s", params.DateTo.Format("20060102"))
	}
	return key
}

func (h *ExportHandler) respondError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := map[string]string{
		"error":   message,
		"status":  "error",
		"message": message,
	}

	json.NewEncoder(w).Encode(response)
}

// getQueryArgs returns the query arguments based on export parameters
func (params *ExportParams) getQueryArgs() []any {
	var args []any

	if params.DateFrom != nil {
		args = append(args, *params.DateFrom)
	}
	if params.DateTo != nil {
		args = append(args, *params.DateTo)
	}

	return args
}
