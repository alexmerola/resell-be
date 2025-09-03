## 📥 Enhanced PDF Seeder Documentation

### Overview
The seeder (`cmd/seeder/main.go`) now implements proven extraction logic from the Python prototype, providing robust PDF parsing with multi-line item support and intelligent price detection.

### Key Improvements

#### 1. Enhanced Text Extraction Algorithm
```go
// Multi-pattern header detection
headerRe := regexp.MustCompile(`(?i)LOT.*PRICE`)
// Fallback: detect items starting with numbers
fallbackRe := regexp.MustCompile(`^\s*\d+\s+\S`)

// Flexible footer detection
footerRe := regexp.MustCompile(`(?i)(A payment of|SUBTOTAL|SUB TOTAL|TOTAL|TAX|SHIPPING|BUYER'S PREMIUM)`)
```

#### 2. Robust Price Detection
```go
// Primary pattern: matches $1,234.56 or 1234.56
priceRe := regexp.MustCompile(`\d{1,3}(?:,\d{3})*\.\d{2}$`)

// Currency parsing handles:
// - Dollar signs
// - Commas
// - Whitespace
// - Various formats
```

#### 3. Multi-Line Item Support
The seeder now correctly handles items that span multiple lines:
- Initial item description with price
- Continuation lines without prices
- Proper buffering until next item detected
- Orphan line handling with logging

### Usage Guide

#### Basic Usage
```bash
# Standard processing
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -log-level=info

# Dry run to test extraction
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -dry-run=true \
  -log-level=debug

# Force reprocessing
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -force=true
```

#### Auction Metadata File Format
Create `auctions.xlsx` with these columns:
| invoice_id | auction_id | date | buyers_premium_percent | sales_tax_percent |
|------------|------------|------|------------------------|-------------------|
| INV001 | 12345 | 2025-01-15 | 20.0 | 8.0 |
| INV002 | 12346 | 2025-01-22 | 18.0 | 8.5 |

#### PDF Structure Requirements
The seeder expects PDFs with this general structure:
```
[Header content]
LOT   DESCRIPTION                           PRICE
------------------------------------------------
1     Victorian Tea Set                     150.00
      Sterling Silver, circa 1890
      Excellent condition
2     Antique Mahogany Desk               1,250.00
      With brass fittings
------------------------------------------------
SUBTOTAL                                 1,400.00
```

### Troubleshooting

#### No Items Extracted
1. **Check PDF text extraction**:
   ```bash
   # Enable debug logging
   -log-level=debug
   ```
   This will show where headers are detected and what text is being processed.

2. **Verify PDF format**:
   - Ensure PDFs are text-based (not scanned images)
   - Check that price format matches: `XXX.XX`
   - Verify header contains "LOT" and "PRICE"

3. **Try alternative patterns**:
   If your PDFs have different headers, modify the regex patterns in `extractFallbackItemsEnhanced()`

#### Incorrect Item Parsing
1. **Multi-line items not combining**:
   - Check that continuation lines don't contain prices
   - Ensure proper indentation/formatting in PDF

2. **Prices not detected**:
   - Verify price format (must end with `.XX`)
   - Check for special characters or spacing issues

3. **Wrong lot/quantity removal**:
   - Adjust the logic that strips trailing numbers if your format differs

#### Database Issues
1. **Duplicate key errors**:
   - The seeder uses `ON CONFLICT (lot_id) DO NOTHING`
   - UUIDs should be unique, but check for logic issues

2. **Transaction failures**:
   - Check database connection
   - Verify all enum values match database schema
   - Ensure proper permissions

### Performance Optimization

#### Batch Processing
The seeder processes items in batches:
```go
batch := &pgx.Batch{}
for _, item := range items {
    batch.Queue(insertQuery, args...)
}
br := tx.SendBatch(ctx, batch)
```

#### State Management
Progress is saved every 10 invoices to prevent re-processing:
```json
{
  "processed_invoices": ["INV001", "INV002"],
  "processed_count": 2,
  "last_update": "2025-01-15T10:30:00Z"
}
```

### Extending the Seeder

#### Adding New Categories
Update the `CategoryClassifier`:
```go
categoryKeywords: map[ItemCategory][]string{
    CategoryNewType: {"keyword1", "keyword2", "keyword3"},
}
```

#### Custom Extraction Logic
For different PDF formats, create alternative extractors:
```go
func (e *PDFExtractor) extractCustomFormat(textLines []string) []rawItem {
    // Implement custom parsing logic
}
```

#### Additional Metadata
Extend the `InventoryItem` struct and database schema:
```go
type InventoryItem struct {
    // ... existing fields
    CustomField1 string
    CustomField2 decimal.Decimal
}
```

### Monitoring & Logging

#### Log Levels
- **DEBUG**: Shows line-by-line processing, regex matches
- **INFO**: Summary of items extracted per invoice
- **WARN**: Skipped items, parsing issues
- **ERROR**: Fatal errors, database failures

#### Progress Tracking
The seeder outputs real-time progress:
```
PROGRESS: Processing 5/20: INV005
SUCCESS: Processed invoice_id:INV005 - 15 items
```

#### Final Summary
```
📊 SEEDING OPERATION SUMMARY
============================
Total PDFs Processed: 20
Total Items Extracted: 342
Average Items per Invoice: 17.1

⚠️ Failed Invoices (2):
  - INV008
  - INV015
```

### Best Practices

1. **Always run dry-run first** to verify extraction before database changes
2. **Keep auction metadata updated** for accurate cost calculations
3. **Use debug logging** when adding new invoice formats
4. **Save state files** to allow resuming after interruptions
5. **Monitor failed invoices** and investigate patterns
6. **Test with sample PDFs** before bulk processing
7. **Backup database** before large seeding operations

### Common PDF Patterns

#### Pattern 1: Simple List
```
1  Item Description              100.00
2  Another Item                   50.00
```

#### Pattern 2: Multi-line with Details
```
LOT 1
Victorian Tea Set
Sterling silver, complete service for 8
Estimated value: $500-700           350.00
```

#### Pattern 3: Tabular with Quantity
```
Lot  Qty  Description           Bid    Total
1    2    Antique Chairs       100    200.00
```

The enhanced seeder handles all these patterns through intelligent buffering and continuation line detection.