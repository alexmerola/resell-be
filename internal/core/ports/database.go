// internal/core/ports/database.go
package ports

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database defines the port for database operations, abstracting away the
// concrete pgxpool implementation from handlers that need basic DB access.
type Database interface {
	Pool() *pgxpool.Pool
	Close()
	Ping(ctx context.Context) error
	Health(ctx context.Context) map[string]interface{}
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}
