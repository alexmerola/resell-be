// internal/core/services/types.go
package services

import "github.com/ammerola/resell-be/internal/core/domain"

// ListParams contains parameters for listing inventory items
type ListParams struct {
	// Pagination
	Page      int    `json:"page"`
	PageSize  int    `json:"page_size"`
	SortBy    string `json:"sort_by"`
	SortOrder string `json:"sort_order"`

	// Filters
	Search          string  `json:"search,omitempty"`
	Category        string  `json:"category,omitempty"`
	Subcategory     string  `json:"subcategory,omitempty"`
	Condition       string  `json:"condition,omitempty"`
	StorageLocation string  `json:"storage_location,omitempty"`
	StorageBin      string  `json:"storage_bin,omitempty"`
	InvoiceID       string  `json:"invoice_id,omitempty"`
	NeedsRepair     *bool   `json:"needs_repair,omitempty"`
	IsConsignment   *bool   `json:"is_consignment,omitempty"`
	IsReturned      *bool   `json:"is_returned,omitempty"`
	MinValue        float64 `json:"min_value,omitempty"`
	MaxValue        float64 `json:"max_value,omitempty"`
}

// ListResult represents paginated list results
type ListResult struct {
	Items      []*domain.InventoryItem `json:"items"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalCount int64                   `json:"total_count"`
	TotalPages int                     `json:"total_pages"`
}
