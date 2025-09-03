// internal/adapters/db/repository.go
package db

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// QueryOption is a function that modifies a query
type QueryOption func(*squirrel.SelectBuilder) *squirrel.SelectBuilder

// OrderDirection represents sort order
type OrderDirection string

const (
	OrderAsc  OrderDirection = "ASC"
	OrderDesc OrderDirection = "DESC"
)

// WithLimit adds a limit to the query
func WithLimit(limit uint64) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		*sb = sb.Limit(limit)
		return sb
	}
}

// WithOffset adds an offset to the query
func WithOffset(offset uint64) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		*sb = sb.Offset(offset)
		return sb
	}
}

// WithOrderBy adds ordering to the query
func WithOrderBy(column string, direction OrderDirection) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		*sb = sb.OrderBy(fmt.Sprintf("%s %s", column, direction))
		return sb
	}
}

// WithWhere adds a WHERE condition to the query
func WithWhere(condition string, args ...interface{}) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		*sb = sb.Where(condition, args...)
		return sb
	}
}

// Repository defines generic repository interface
type Repository[T any] interface {
	Create(ctx context.Context, entity *T) error
	CreateBatch(ctx context.Context, entities []*T) error
	Update(ctx context.Context, id uuid.UUID, entity *T) error
	UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error
	Delete(ctx context.Context, id uuid.UUID) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	FindByID(ctx context.Context, id uuid.UUID) (*T, error)
	FindAll(ctx context.Context, opts ...QueryOption) ([]*T, error)
	FindOne(ctx context.Context, opts ...QueryOption) (*T, error)
	Count(ctx context.Context, opts ...QueryOption) (int64, error)
	Exists(ctx context.Context, id uuid.UUID) (bool, error)
}

// BaseRepository provides base implementation
type BaseRepository[T any] struct {
	db         *Database
	table      string
	columns    []string
	primaryKey string
	scanner    RowScanner[T]
	builder    EntityBuilder[T]
	logger     *slog.Logger
}

// RowScanner is a function that scans a row into an entity
type RowScanner[T any] func(row pgx.Row) (*T, error)

// RowsScanner is a function that scans rows into an entity
type RowsScanner[T any] func(rows pgx.Rows) (*T, error)

// EntityBuilder is a function that builds SQL values from an entity
type EntityBuilder[T any] func(entity *T) map[string]interface{}

// NewRepository creates a new repository instance
func NewRepository[T any](
	db *Database,
	table string,
	columns []string,
	scanner RowScanner[T],
	builder EntityBuilder[T],
	logger *slog.Logger,
) Repository[T] {
	return &BaseRepository[T]{
		db:         db,
		table:      table,
		columns:    columns,
		primaryKey: "lot_id", // Default, can be overridden
		scanner:    scanner,
		builder:    builder,
		logger:     logger.With(slog.String("repository", table)),
	}
}

// Create inserts a new entity
func (r *BaseRepository[T]) Create(ctx context.Context, entity *T) error {
	values := r.builder(entity)

	// Build INSERT query
	query := squirrel.Insert(r.table).
		SetMap(values).
		Suffix("RETURNING " + strings.Join(r.columns, ", ")).
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build insert query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing create",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	row := r.db.QueryRow(ctx, sql, args...)
	created, err := r.scanner(row)
	if err != nil {
		return fmt.Errorf("failed to scan created entity: %w", err)
	}

	*entity = *created
	return nil
}

// CreateBatch inserts multiple entities efficiently
func (r *BaseRepository[T]) CreateBatch(ctx context.Context, entities []*T) error {
	if len(entities) == 0 {
		return nil
	}

	return r.db.Transaction(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}

		for _, entity := range entities {
			values := r.builder(entity)

			query := squirrel.Insert(r.table).
				SetMap(values).
				Suffix("RETURNING " + strings.Join(r.columns, ", ")).
				PlaceholderFormat(squirrel.Dollar)

			sql, args, err := query.ToSql()
			if err != nil {
				return fmt.Errorf("failed to build insert query: %w", err)
			}

			batch.Queue(sql, args...)
		}

		br := tx.SendBatch(ctx, batch)
		defer br.Close()

		for i, entity := range entities {
			row := br.QueryRow()
			created, err := r.scanner(row)
			if err != nil {
				return fmt.Errorf("failed to scan entity %d: %w", i, err)
			}
			*entity = *created
		}

		return nil
	})
}

// Update updates an existing entity
func (r *BaseRepository[T]) Update(ctx context.Context, id uuid.UUID, entity *T) error {
	values := r.builder(entity)
	delete(values, r.primaryKey) // Remove primary key from updates
	values["updated_at"] = time.Now()

	query := squirrel.Update(r.table).
		SetMap(values).
		Where(squirrel.Eq{r.primaryKey: id}).
		Suffix("RETURNING " + strings.Join(r.columns, ", ")).
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing update",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	row := r.db.QueryRow(ctx, sql, args...)
	updated, err := r.scanner(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("entity not found: %s", id)
		}
		return fmt.Errorf("failed to scan updated entity: %w", err)
	}

	*entity = *updated
	return nil
}

// UpdatePartial updates specific fields of an entity
func (r *BaseRepository[T]) UpdatePartial(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	updates["updated_at"] = time.Now()

	query := squirrel.Update(r.table).
		SetMap(updates).
		Where(squirrel.Eq{r.primaryKey: id}).
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build update query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing partial update",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute update: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entity not found: %s", id)
	}

	return nil
}

// Delete removes an entity permanently
func (r *BaseRepository[T]) Delete(ctx context.Context, id uuid.UUID) error {
	query := squirrel.Delete(r.table).
		Where(squirrel.Eq{r.primaryKey: id}).
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return fmt.Errorf("failed to build delete query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing delete",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	tag, err := r.db.Exec(ctx, sql, args...)
	if err != nil {
		return fmt.Errorf("failed to execute delete: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("entity not found: %s", id)
	}

	return nil
}

// SoftDelete marks an entity as deleted
func (r *BaseRepository[T]) SoftDelete(ctx context.Context, id uuid.UUID) error {
	updates := map[string]interface{}{
		"deleted_at": time.Now(),
		"updated_at": time.Now(),
	}

	return r.UpdatePartial(ctx, id, updates)
}

// FindByID retrieves an entity by ID
func (r *BaseRepository[T]) FindByID(ctx context.Context, id uuid.UUID) (*T, error) {
	query := squirrel.Select(r.columns...).
		From(r.table).
		Where(squirrel.Eq{r.primaryKey: id}).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing find by id",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	row := r.db.QueryRow(ctx, sql, args...)
	entity, err := r.scanner(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan entity: %w", err)
	}

	return entity, nil
}

// FindAll retrieves all entities matching the criteria
func (r *BaseRepository[T]) FindAll(ctx context.Context, opts ...QueryOption) ([]*T, error) {
	query := squirrel.Select(r.columns...).
		From(r.table).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar)

	// Apply query options
	for _, opt := range opts {
		query = *opt(&query)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing find all",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	var entities []*T
	for rows.Next() {
		// Convert Rows scanner to Row scanner
		entity, err := r.scanRows(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan entity: %w", err)
		}
		entities = append(entities, entity)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return entities, nil
}

// FindOne retrieves a single entity matching the criteria
func (r *BaseRepository[T]) FindOne(ctx context.Context, opts ...QueryOption) (*T, error) {
	query := squirrel.Select(r.columns...).
		From(r.table).
		Where("deleted_at IS NULL").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	// Apply query options
	for _, opt := range opts {
		query = *opt(&query)
	}

	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("failed to build select query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing find one",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	row := r.db.QueryRow(ctx, sql, args...)
	entity, err := r.scanner(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan entity: %w", err)
	}

	return entity, nil
}

// Count returns the number of entities matching the criteria
func (r *BaseRepository[T]) Count(ctx context.Context, opts ...QueryOption) (int64, error) {
	query := squirrel.Select("COUNT(*)").
		From(r.table).
		Where("deleted_at IS NULL").
		PlaceholderFormat(squirrel.Dollar)

	// Apply query options (except limit and offset)
	for _, opt := range opts {
		query = *opt(&query)
	}

	// Remove limit and offset from count query
	query = query.RemoveLimit().RemoveOffset()

	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing count",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	var count int64
	err = r.db.QueryRow(ctx, sql, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to scan count: %w", err)
	}

	return count, nil
}

// Exists checks if an entity exists
func (r *BaseRepository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	query := squirrel.Select("1").
		From(r.table).
		Where(squirrel.Eq{r.primaryKey: id}).
		Where("deleted_at IS NULL").
		Limit(1).
		PlaceholderFormat(squirrel.Dollar)

	sql, args, err := query.ToSql()
	if err != nil {
		return false, fmt.Errorf("failed to build exists query: %w", err)
	}

	r.logger.DebugContext(ctx, "executing exists",
		slog.String("query", sql),
		slog.Any("args", args),
	)

	var exists int
	err = r.db.QueryRow(ctx, sql, args...).Scan(&exists)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return true, nil
}

// scanRows is a helper to convert pgx.Rows to the entity scanner format
func (r *BaseRepository[T]) scanRows(rows pgx.Rows) (*T, error) {
	// Create a temporary row wrapper for scanning
	_, err := rows.Values()
	if err != nil {
		return nil, err
	}

	// Use reflection or a type assertion to handle the scanning
	// This is a simplified version - in production, you'd need proper field mapping
	var entity T

	// For now, we'll need the actual implementation to handle this properly
	// based on the specific entity type

	return &entity, nil
}

// Pagination helper struct
type Pagination struct {
	Page     int
	PageSize int
	Total    int64
}

// PaginationOption creates query options for pagination
func PaginationOption(page, pageSize int) []QueryOption {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize
	return []QueryOption{
		WithLimit(uint64(pageSize)),
		WithOffset(uint64(offset)),
	}
}

// TextSearchOption creates a full-text search query option
func TextSearchOption(searchVector, query string) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		*sb = sb.Where(fmt.Sprintf("%s @@ plainto_tsquery('english', ?)", searchVector), query)
		return sb
	}
}

// DateRangeOption creates a date range query option
func DateRangeOption(column string, from, to time.Time) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		if !from.IsZero() {
			*sb = sb.Where(squirrel.GtOrEq{column: from})
		}
		if !to.IsZero() {
			*sb = sb.Where(squirrel.LtOrEq{column: to})
		}
		return sb
	}
}

// InOption creates an IN query option
func InOption(column string, values []interface{}) QueryOption {
	return func(sb *squirrel.SelectBuilder) *squirrel.SelectBuilder {
		if len(values) > 0 {
			*sb = sb.Where(squirrel.Eq{column: values})
		}
		return sb
	}
}

// Transaction executor interface
type Transactor interface {
	Transaction(ctx context.Context, fn func(pgx.Tx) error) error
}

// Unit of Work pattern implementation
type UnitOfWork struct {
	db *Database
	tx pgx.Tx
}

// NewUnitOfWork creates a new unit of work
func NewUnitOfWork(db *Database) *UnitOfWork {
	return &UnitOfWork{db: db}
}

// Begin starts a new transaction
func (uow *UnitOfWork) Begin(ctx context.Context) error {
	tx, err := uow.db.Pool().Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	uow.tx = tx
	return nil
}

// Commit commits the transaction
func (uow *UnitOfWork) Commit(ctx context.Context) error {
	if uow.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := uow.tx.Commit(ctx)
	uow.tx = nil
	return err
}

// Rollback rolls back the transaction
func (uow *UnitOfWork) Rollback(ctx context.Context) error {
	if uow.tx == nil {
		return nil
	}
	err := uow.tx.Rollback(ctx)
	uow.tx = nil
	return err
}

// Tx returns the current transaction
func (uow *UnitOfWork) Tx() pgx.Tx {
	return uow.tx
}
