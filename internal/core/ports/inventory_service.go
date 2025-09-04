// internal/core/ports/inventory_service.go
package ports

import (
	"context"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/google/uuid"
)

// InventoryService defines the application service port for inventory.
// This interface is implemented by the application service.
type InventoryService interface {
	SaveItem(ctx context.Context, item *domain.InventoryItem) error
	SaveItems(ctx context.Context, items []domain.InventoryItem) error
	BulkUpsert(ctx context.Context, items []domain.InventoryItem) error
	GetByID(ctx context.Context, lotID uuid.UUID) (*domain.InventoryItem, error)
	UpdateItem(ctx context.Context, lotID uuid.UUID, item *domain.InventoryItem) error
	DeleteItem(ctx context.Context, lotID uuid.UUID, permanent bool) error
	// Note: We need to define ListParams and ListResult here to avoid circular dependencies.
	List(ctx context.Context, params ListParams) (*ListResult, error)
}

// ListParams holds parameters for listing inventory
type ListParams struct {
	Search          string
	Category        string
	Condition       string
	StorageLocation string
	StorageBin      string
	InvoiceID       string
	NeedsRepair     *bool
	SortBy          string
	SortOrder       string
	Page            int
	PageSize        int
}

// ListResult holds the result of listing inventory
type ListResult struct {
	Items      []*domain.InventoryItem `json:"items"`
	Page       int                     `json:"page"`
	PageSize   int                     `json:"page_size"`
	TotalCount int64                   `json:"total_count"`
	TotalPages int                     `json:"total_pages"`
}
