// internal/pkg/config/config.go
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	// Application
	App AppConfig

	// Database
	Database DatabaseConfig

	// Redis
	Redis RedisConfig

	// Asynq
	Asynq AsynqConfig

	// AWS
	AWS AWSConfig

	// File Processing
	FileProcessing FileProcessingConfig

	// Security
	Security SecurityConfig

	// Server
	Server ServerConfig
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	Name        string
	Environment string // development, staging, production
	Version     string
	LogLevel    string
	LogFormat   string // json, text
	Debug       bool
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host               string
	Port               string
	User               string
	Password           string
	Name               string
	SSLMode            string
	MaxConnections     int32
	MinConnections     int32
	MaxConnLifetime    time.Duration
	MaxConnIdleTime    time.Duration
	HealthCheckPeriod  time.Duration
	ConnectTimeout     time.Duration
	StatementCacheMode string
	EnableQueryLogging bool
	MigrationPath      string
}

// RedisConfig holds Redis configuration
type RedisConfig struct {
	Host            string
	Port            string
	Password        string
	DB              int
	MaxRetries      int
	MinRetryBackoff time.Duration
	MaxRetryBackoff time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PoolSize        int
	MinIdleConns    int
	MaxConnAge      time.Duration
	PoolTimeout     time.Duration
	IdleTimeout     time.Duration
	TTL             time.Duration
}

// AsynqConfig holds Asynq configuration
type AsynqConfig struct {
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	Concurrency          int
	Queues               map[string]int // queue name -> priority
	StrictPriority       bool
	RetryMax             int
	ShutdownTimeout      time.Duration
	HealthCheckInterval  time.Duration
	DelayedTaskCheckTime time.Duration
}

// AWSConfig holds AWS configuration
type AWSConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	S3Bucket        string
	S3Endpoint      string // For MinIO in development
	UsePathStyle    bool   // For MinIO compatibility
}

// FileProcessingConfig holds file processing configuration
type FileProcessingConfig struct {
	PDFMaxSizeMB      int
	ExcelMaxSizeMB    int
	ProcessingTimeout time.Duration
	TempDir           string
	CleanupInterval   time.Duration
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	JWTSecret            string
	JWTExpiration        time.Duration
	JWTRefreshExpiration time.Duration
	BcryptCost           int
	RateLimitRequests    int
	RateLimitDuration    time.Duration
	AllowedOrigins       []string
	TrustedProxies       []string
	SecureHeaders        bool
	CSRFProtection       bool
	RequestIDHeader      string
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host              string
	Port              string
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	GracefulTimeout   time.Duration
	EnablePprof       bool
	EnableMetrics     bool
	EnableHealthCheck bool
	TLSEnabled        bool
	TLSCertFile       string
	TLSKeyFile        string
}

// Load loads configuration from environment variables
func Load(logger *slog.Logger) (*Config, error) {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	// Load .env file in development
	if env == "development" || env == "local" {
		if err := godotenv.Load(); err != nil {
			logger.Warn("no .env file found, using environment variables",
				slog.String("error", err.Error()))
		} else {
			logger.Info(".env file loaded successfully")
		}
	}

	// Initialize viper
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.SetTypeByDefaultValue(true)

	// Set defaults
	setDefaults()

	// Build config struct
	cfg := &Config{
		App: AppConfig{
			Name:        getEnv("APP_NAME", "resell-api"),
			Environment: env,
			Version:     getEnv("APP_VERSION", "dev"),
			LogLevel:    getEnv("LOG_LEVEL", "debug"),
			LogFormat:   getEnv("LOG_FORMAT", "json"),
			Debug:       getBoolEnv("APP_DEBUG", env == "development"),
		},
		Database: DatabaseConfig{
			Host:               getEnv("DB_HOST", "localhost"),
			Port:               getEnv("DB_PORT", "5432"),
			User:               getEnv("DB_USER", "resell"),
			Password:           getEnv("DB_PASSWORD", "resell_dev_2025"),
			Name:               getEnv("DB_NAME", "resell_inventory"),
			SSLMode:            getEnv("DB_SSL_MODE", "disable"),
			MaxConnections:     int32(getIntEnv("DB_MAX_CONNECTIONS", 25)),
			MinConnections:     int32(getIntEnv("DB_MIN_CONNECTIONS", 5)),
			MaxConnLifetime:    getDurationEnv("DB_CONNECTION_LIFETIME", time.Hour),
			MaxConnIdleTime:    getDurationEnv("DB_IDLE_TIME", 30*time.Minute),
			HealthCheckPeriod:  getDurationEnv("DB_HEALTH_CHECK_PERIOD", time.Minute),
			ConnectTimeout:     getDurationEnv("DB_CONNECT_TIMEOUT", 10*time.Second),
			StatementCacheMode: getEnv("DB_STATEMENT_CACHE_MODE", "describe"),
			EnableQueryLogging: getBoolEnv("DB_QUERY_LOGGING", env == "development"),
			MigrationPath:      getEnv("DB_MIGRATION_PATH", "migrations"),
		},
		Redis: RedisConfig{
			Host:            getEnv("REDIS_HOST", "localhost"),
			Port:            getEnv("REDIS_PORT", "6379"),
			Password:        getEnv("REDIS_PASSWORD", ""),
			DB:              getIntEnv("REDIS_DB", 0),
			MaxRetries:      getIntEnv("REDIS_MAX_RETRIES", 3),
			MinRetryBackoff: getDurationEnv("REDIS_MIN_RETRY_BACKOFF", 8*time.Millisecond),
			MaxRetryBackoff: getDurationEnv("REDIS_MAX_RETRY_BACKOFF", 512*time.Millisecond),
			DialTimeout:     getDurationEnv("REDIS_DIAL_TIMEOUT", 5*time.Second),
			ReadTimeout:     getDurationEnv("REDIS_READ_TIMEOUT", 3*time.Second),
			WriteTimeout:    getDurationEnv("REDIS_WRITE_TIMEOUT", 3*time.Second),
			PoolSize:        getIntEnv("REDIS_POOL_SIZE", 10),
			MinIdleConns:    getIntEnv("REDIS_MIN_IDLE_CONNS", 2),
			MaxConnAge:      getDurationEnv("REDIS_MAX_CONN_AGE", 0),
			PoolTimeout:     getDurationEnv("REDIS_POOL_TIMEOUT", 4*time.Second),
			IdleTimeout:     getDurationEnv("REDIS_IDLE_TIMEOUT", 5*time.Minute),
			TTL:             getDurationEnv("REDIS_TTL", time.Hour),
		},
		Asynq: AsynqConfig{
			RedisAddr:            fmt.Sprintf("%s:%s", getEnv("REDIS_HOST", "localhost"), getEnv("REDIS_PORT", "6379")),
			RedisPassword:        getEnv("REDIS_PASSWORD", ""),
			RedisDB:              getIntEnv("ASYNQ_REDIS_DB", 0),
			Concurrency:          getIntEnv("ASYNQ_CONCURRENCY", 10),
			Queues:               parseQueues(getEnv("ASYNQ_QUEUES", "critical:6,default:3,low:1")),
			StrictPriority:       getBoolEnv("ASYNQ_STRICT_PRIORITY", false),
			RetryMax:             getIntEnv("ASYNQ_RETRY_MAX", 3),
			ShutdownTimeout:      getDurationEnv("ASYNQ_SHUTDOWN_TIMEOUT", 30*time.Second),
			HealthCheckInterval:  getDurationEnv("ASYNQ_HEALTH_CHECK_INTERVAL", 30*time.Second),
			DelayedTaskCheckTime: getDurationEnv("ASYNQ_DELAYED_TASK_CHECK", 5*time.Second),
		},
		AWS: AWSConfig{
			Region:          getEnv("AWS_REGION", "us-east-1"),
			AccessKeyID:     getEnv("AWS_ACCESS_KEY_ID", "minioadmin"),
			SecretAccessKey: getEnv("AWS_SECRET_ACCESS_KEY", "minioadmin123"),
			S3Bucket:        getEnv("AWS_S3_BUCKET", "resell-uploads"),
			S3Endpoint:      getEnv("AWS_S3_ENDPOINT", ""),
			UsePathStyle:    getBoolEnv("AWS_S3_PATH_STYLE", env == "development"),
		},
		FileProcessing: FileProcessingConfig{
			PDFMaxSizeMB:      getIntEnv("PDF_MAX_SIZE_MB", 50),
			ExcelMaxSizeMB:    getIntEnv("EXCEL_MAX_SIZE_MB", 100),
			ProcessingTimeout: getDurationEnv("PROCESSING_TIMEOUT", 5*time.Minute),
			TempDir:           getEnv("TEMP_DIR", "/tmp"),
			CleanupInterval:   getDurationEnv("CLEANUP_INTERVAL", time.Hour),
		},
		Security: SecurityConfig{
			JWTSecret:            getEnv("JWT_SECRET", generateDefaultSecret(env)),
			JWTExpiration:        getDurationEnv("JWT_EXPIRATION", 24*time.Hour),
			JWTRefreshExpiration: getDurationEnv("JWT_REFRESH_EXPIRATION", 7*24*time.Hour),
			BcryptCost:           getIntEnv("BCRYPT_COST", 10),
			RateLimitRequests:    getIntEnv("RATE_LIMIT_REQUESTS", 100),
			RateLimitDuration:    getDurationEnv("RATE_LIMIT_DURATION", time.Minute),
			AllowedOrigins:       getSliceEnv("ALLOWED_ORIGINS", []string{"*"}),
			TrustedProxies:       getSliceEnv("TRUSTED_PROXIES", []string{}),
			SecureHeaders:        getBoolEnv("SECURE_HEADERS", env == "production"),
			CSRFProtection:       getBoolEnv("CSRF_PROTECTION", env == "production"),
			RequestIDHeader:      getEnv("REQUEST_ID_HEADER", "X-Request-ID"),
		},
		Server: ServerConfig{
			Host:              getEnv("SERVER_HOST", "0.0.0.0"),
			Port:              getEnv("SERVER_PORT", "8080"),
			ReadTimeout:       getDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:      getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:       getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),
			MaxHeaderBytes:    getIntEnv("SERVER_MAX_HEADER_BYTES", 1<<20), // 1 MB
			GracefulTimeout:   getDurationEnv("SERVER_GRACEFUL_TIMEOUT", 30*time.Second),
			EnablePprof:       getBoolEnv("ENABLE_PPROF", env == "development"),
			EnableMetrics:     getBoolEnv("ENABLE_METRICS", true),
			EnableHealthCheck: getBoolEnv("ENABLE_HEALTH_CHECK", true),
			TLSEnabled:        getBoolEnv("TLS_ENABLED", false),
			TLSCertFile:       getEnv("TLS_CERT_FILE", ""),
			TLSKeyFile:        getEnv("TLS_KEY_FILE", ""),
		},
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate required fields
	if c.Database.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Database.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if c.Security.JWTSecret == "" || c.Security.JWTSecret == "change-me-in-production" {
		if c.App.Environment == "production" {
			return fmt.Errorf("JWT secret must be set in production")
		}
	}
	if c.Server.Port == "" {
		return fmt.Errorf("server port is required")
	}

	// Validate numeric ranges
	if c.Database.MaxConnections < c.Database.MinConnections {
		return fmt.Errorf("max connections must be >= min connections")
	}
	if c.Security.RateLimitRequests <= 0 {
		return fmt.Errorf("rate limit requests must be positive")
	}

	return nil
}

// GetDatabaseURL returns the formatted database connection string
func (c *Config) GetDatabaseURL() string {
	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%s/%s?sslmode=%s",
		c.Database.User,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Name,
		c.Database.SSLMode,
	)
}

// GetServerAddress returns the formatted server address
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// IsProduction returns true if running in production
func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}

// IsDevelopment returns true if running in development
func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development" || c.App.Environment == "local"
}

// Helper functions

func setDefaults() {
	viper.SetDefault("app.name", "resell-api")
	viper.SetDefault("app.environment", "development")
	viper.SetDefault("log.level", "info")
	viper.SetDefault("log.format", "json")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		i, err := strconv.Atoi(value)
		if err == nil {
			return i
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		d, err := time.ParseDuration(value)
		if err == nil {
			return d
		}
	}
	return defaultValue
}

func getSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

func parseQueues(queuesStr string) map[string]int {
	queues := make(map[string]int)
	pairs := strings.Split(queuesStr, ",")
	for _, pair := range pairs {
		parts := strings.Split(pair, ":")
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			priority, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err == nil {
				queues[name] = priority
			}
		}
	}
	if len(queues) == 0 {
		queues["default"] = 1
	}
	return queues
}

func generateDefaultSecret(env string) string {
	if env == "production" {
		return "" // Force error in production if not set
	}
	return "development-secret-change-in-production"
}
