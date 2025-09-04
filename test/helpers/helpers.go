// test/helpers/test_helpers.go
package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/ammerola/resell-be/internal/adapters/db"
	"github.com/ammerola/resell-be/internal/pkg/config"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/ammerola/resell-be/internal/core/domain"
)

// TestDB represents a test database instance
type TestDB struct {
	PgxPool  *pgxpool.Pool
	Database *db.Database
	Resource *dockertest.Resource
	Pool     *dockertest.Pool
	Config   *db.Config
}

// TestRedis represents a test Redis instance
type TestRedis struct {
	Client *redis.Client
	Server *miniredis.Miniredis
}

// TestLogger returns a test logger
func TestLogger() *slog.Logger {
	if testing.Verbose() {
		return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// SetupTestDB creates a PostgreSQL container for integration tests
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	pool, err := dockertest.NewPool("")
	require.NoError(t, err, "Could not connect to Docker")

	// Pull PostgreSQL image
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=test_inventory",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	require.NoError(t, err, "Could not start PostgreSQL container")

	// Clean up on test completion
	t.Cleanup(func() {
		if err := pool.Purge(resource); err != nil {
			t.Logf("Could not purge resource: %s", err)
		}
	})

	// Get connection details
	dbConfig := &db.Config{
		Host:               "localhost",
		Port:               resource.GetPort("5432/tcp"),
		User:               "test",
		Password:           "test",
		Database:           "test_inventory",
		SSLMode:            "disable",
		MaxConnections:     5,
		MinConnections:     1,
		MaxConnLifetime:    time.Hour,
		MaxConnIdleTime:    time.Minute * 30,
		HealthCheckPeriod:  time.Minute,
		ConnectTimeout:     time.Second * 10,
		StatementCacheMode: "describe",
		EnableQueryLogging: testing.Verbose(),
	}

	// Wait for database to be ready
	var database *db.Database
	err = pool.Retry(func() error {
		ctx := context.Background()
		var err error
		database, err = db.NewDatabase(ctx, dbConfig, TestLogger())
		if err != nil {
			return err
		}
		return database.Ping(ctx)
	})
	require.NoError(t, err, "Could not connect to PostgreSQL")

	// Run migrations
	ctx := context.Background()
	migrationConfig := &db.MigrationConfig{
		DatabaseURL: fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=%s",
			dbConfig.User, dbConfig.Password, dbConfig.Host, dbConfig.Port,
			dbConfig.Database, dbConfig.SSLMode),
		SourcePath: "../../migrations",
		TableName:  "schema_migrations",
		SchemaName: "public",
	}

	err = db.RunMigrationsWithRetry(ctx, migrationConfig, TestLogger(), 3)
	require.NoError(t, err, "Could not run migrations")

	return &TestDB{
		PgxPool:  database.Pool(),
		Database: database,
		Resource: resource,
		Pool:     pool,
		Config:   dbConfig,
	}
}

// SetupTestRedis creates a mock Redis instance for testing
func SetupTestRedis(t *testing.T) *TestRedis {
	t.Helper()

	mr := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	t.Cleanup(func() {
		client.Close()
		mr.Close()
	})

	return &TestRedis{
		Client: client,
		Server: mr,
	}
}

// SetupMockDB creates a mock database for unit testing
func SetupMockDB(t *testing.T) (sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	db, mock, err := sqlmock.New()
	require.NoError(t, err, "Failed to create mock DB")

	t.Cleanup(func() {
		db.Close()
	})

	return mock, db
}

// LoadTestConfig returns a test configuration
func LoadTestConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Name:        "test-api",
			Environment: "test",
			Version:     "test",
			LogLevel:    "debug",
			LogFormat:   "text",
			Debug:       true,
		},
		Database: config.DatabaseConfig{
			Host:               "localhost",
			Port:               "5432",
			User:               "test",
			Password:           "test",
			Name:               "test_inventory",
			SSLMode:            "disable",
			MaxConnections:     10,
			MinConnections:     2,
			EnableQueryLogging: true,
		},
		Redis: config.RedisConfig{
			Host:     "localhost",
			Port:     "6379",
			DB:       0,
			TTL:      time.Hour,
			PoolSize: 10,
		},
		FileProcessing: config.FileProcessingConfig{
			PDFMaxSizeMB:      50,
			ExcelMaxSizeMB:    100,
			ProcessingTimeout: 5 * time.Minute,
			TempDir:           "/tmp",
		},
		Security: config.SecurityConfig{
			JWTSecret:         "test-secret",
			JWTExpiration:     24 * time.Hour,
			RateLimitRequests: 100,
			RateLimitDuration: time.Minute,
			AllowedOrigins:    []string{"*"},
			SecureHeaders:     false,
		},
		Server: config.ServerConfig{
			Host:         "localhost",
			Port:         "8080",
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
		},
	}
}

// CreateTestInventoryItem creates a test inventory item
func CreateTestInventoryItem(overrides ...func(*domain.InventoryItem)) *domain.InventoryItem {
	item := &domain.InventoryItem{
		LotID:           uuid.New(),
		InvoiceID:       "TEST-001",
		AuctionID:       12345,
		ItemName:        "Test Victorian Tea Set",
		Description:     "Antique porcelain tea set, circa 1890, excellent condition",
		Category:        domain.CategoryAntiques,
		Condition:       domain.ConditionExcellent,
		Quantity:        1,
		BidAmount:       decimal.NewFromFloat(150.00),
		BuyersPremium:   decimal.NewFromFloat(27.00),
		SalesTax:        decimal.NewFromFloat(15.31),
		ShippingCost:    decimal.NewFromFloat(10.00),
		TotalCost:       decimal.NewFromFloat(202.31),
		CostPerItem:     decimal.NewFromFloat(202.31),
		AcquisitionDate: time.Now().AddDate(0, -1, 0),
		MarketDemand:    domain.DemandMedium,
		Keywords:        []string{"antique", "porcelain", "tea", "victorian"},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	for _, override := range overrides {
		override(item)
	}

	return item
}

// CreateTestInventoryItems creates multiple test inventory items
func CreateTestInventoryItems(count int) []domain.InventoryItem {
	items := make([]domain.InventoryItem, count)

	categories := []domain.ItemCategory{
		domain.CategoryAntiques,
		domain.CategoryArt,
		domain.CategoryFurniture,
		domain.CategoryJewelry,
		domain.CategoryGlass,
	}

	conditions := []domain.ItemCondition{
		domain.ConditionMint,
		domain.ConditionExcellent,
		domain.ConditionGood,
		domain.ConditionFair,
	}

	for i := 0; i < count; i++ {
		items[i] = *CreateTestInventoryItem(func(item *domain.InventoryItem) {
			item.InvoiceID = fmt.Sprintf("TEST-%03d", i+1)
			item.ItemName = fmt.Sprintf("Test Item %d", i+1)
			item.Category = categories[i%len(categories)]
			item.Condition = conditions[i%len(conditions)]
			item.BidAmount = decimal.NewFromFloat(float64(100 + (i * 50)))
		})
	}

	return items
}

// CompareInventoryItems compares two inventory items for testing
func CompareInventoryItems(t *testing.T, expected, actual *domain.InventoryItem) {
	t.Helper()

	require.Equal(t, expected.InvoiceID, actual.InvoiceID)
	require.Equal(t, expected.AuctionID, actual.AuctionID)
	require.Equal(t, expected.ItemName, actual.ItemName)
	require.Equal(t, expected.Description, actual.Description)
	require.Equal(t, expected.Category, actual.Category)
	require.Equal(t, expected.Condition, actual.Condition)
	require.Equal(t, expected.Quantity, actual.Quantity)
	require.True(t, expected.BidAmount.Equal(actual.BidAmount))
	require.True(t, expected.BuyersPremium.Equal(actual.BuyersPremium))
	require.True(t, expected.SalesTax.Equal(actual.SalesTax))
	require.True(t, expected.ShippingCost.Equal(actual.ShippingCost))
}

// LoadFixture loads a test fixture file
func LoadFixture(t *testing.T, filename string) []byte {
	t.Helper()

	path := fmt.Sprintf("../../test/fixtures/%s", filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to load fixture: %s", filename)

	return data
}

// AssertEventuallyWithTimeout asserts that a condition is met within a timeout
func AssertEventuallyWithTimeout(t *testing.T, condition func() bool, timeout time.Duration, msg string) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Errorf("Condition not met within %v: %s", timeout, msg)
}

// TruncateAllTables truncates all tables in the test database
func TruncateAllTables(t *testing.T, db *pgxpool.Pool) {
	t.Helper()

	ctx := context.Background()
	tables := []string{
		"activity_logs",
		"async_jobs",
		"platform_listings",
		"inventory",
	}

	for _, table := range tables {
		_, err := db.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		require.NoError(t, err, "Failed to truncate table: %s", table)
	}
}

// SeedTestData seeds the database with test data
func SeedTestData(t *testing.T, db *pgxpool.Pool, items []domain.InventoryItem) {
	t.Helper()

	ctx := context.Background()

	for _, item := range items {
		query := `
			INSERT INTO inventory (
				lot_id, invoice_id, auction_id, item_name, description,
				category, condition, quantity, bid_amount, buyers_premium,
				sales_tax, shipping_cost, acquisition_date, keywords, 
				created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		`

		_, err := db.Exec(ctx, query,
			item.LotID, item.InvoiceID, item.AuctionID, item.ItemName, item.Description,
			item.Category, item.Condition, item.Quantity, item.BidAmount, item.BuyersPremium,
			item.SalesTax, item.ShippingCost, item.AcquisitionDate,
			strings.Join(item.Keywords, ","), item.CreatedAt, item.UpdatedAt,
		)
		require.NoError(t, err, "Failed to seed test data")
	}
}

// CreateTempFile creates a temporary file for testing
func CreateTempFile(t *testing.T, content []byte, extension string) string {
	t.Helper()

	file, err := os.CreateTemp("", fmt.Sprintf("test-*%s", extension))
	require.NoError(t, err, "Failed to create temp file")

	_, err = file.Write(content)
	require.NoError(t, err, "Failed to write to temp file")

	file.Close()

	t.Cleanup(func() {
		os.Remove(file.Name())
	})

	return file.Name()
}
