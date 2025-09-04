// test/mocks/mocks.go

// Package mocks contains generated mocks for the application's interfaces.
// To regenerate mocks, run `make mocks` from the root directory.
package mocks

//go:generate mockgen -source=../../internal/core/ports/inventory_repository.go -destination=inventory_repository_mock.go -package=mocks
//go:generate mockgen -source=../../internal/core/ports/inventory_service.go -destination=inventory_service_mock.go -package=mocks
//go:generate mockgen -source=../../internal/core/services/inventory.go -destination=pgxpool_mock.go -package=mocks PgxPool
//go:generate mockgen -source=../../internal/core/ports/cache.go -destination=cache_repository_mock.go -package=mocks
//go:generate mockgen -source=../../internal/core/ports/database.go -destination=database_mock.go -package=mocks
