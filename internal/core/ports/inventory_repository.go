// internal/core/ports/inventory_repository.go
package ports

import (
	"context"

	"github.com/ammerola/resell-be/internal/core/domain"
	"github.com/google/uuid"
)

// InventoryRepository defines the persistence port for inventory.
// This interface is implemented by the database adapter.
type InventoryRepository interface {
	Save(ctx context.Context, item *domain.InventoryItem) error
	SaveBatch(ctx context.Context, items []domain.InventoryItem) error
	Update(ctx context.Context, item *domain.InventoryItem) error
	FindByID(ctx context.Context, lotID uuid.UUID) (*domain.InventoryItem, error)
	FindByInvoiceID(ctx context.Context, invoiceID string) ([]domain.InventoryItem, error)
	Delete(ctx context.Context, lotID uuid.UUID) error
	SoftDelete(ctx context.Context, lotID uuid.UUID) error
	Count(ctx context.Context) (int64, error)
	Exists(ctx context.Context, lotID uuid.UUID) (bool, error)
}
