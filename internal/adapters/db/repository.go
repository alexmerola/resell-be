// internal/adapters/db/repository.go
package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Generic repository interface
type Repository[T any] interface {
	Create(ctx context.Context, entity *T) error
	Update(ctx context.Context, id uuid.UUID, entity *T) error
	Delete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*T, error)
	FindAll(ctx context.Context, opts ...QueryOption) ([]*T, error)
}

// Base repository implementation
type BaseRepository[T any] struct {
	db    *pgxpool.Pool
	table string
}

func NewRepository[T any](db *pgxpool.Pool, table string) Repository[T] {
	return &BaseRepository[T]{
		db:    db,
		table: table,
	}
}
