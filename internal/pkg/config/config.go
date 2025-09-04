// internal/pkg/config/config.go
package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ammerola/resell-be/internal/pkg/logger"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// ErrMissingRequiredConfig indicates a required configuration value is missing
var ErrMissingRequiredConfig = errors.New("missing required configuration")

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

	// Secrets Management
	Secrets SecretsConfig

	// Logger
	Logging LoggingConfig
}

// SecretsConfig holds secrets management configuration
type SecretsConfig struct {
	Provider        string // aws-secrets-manager, vault, env
	AWSRegion       string
	SecretName      string
	VaultAddr       string
	VaultToken      string
	VaultPath       string
	RefreshInterval time.Duration
}

// AppConfig holds application-specific configuration
type AppConfig struct {
	Name        string
	Environment string `required:"true" validate:"oneof=development staging production"`
	Version     string
	LogLevel    string
	LogFormat   string `validate:"oneof=json text"`
	Debug       bool
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Host               string `required:"true" validate:"required"`
	Port               string `required:"true" validate:"required,numeric"`
	User               string `required:"true" validate:"required"`
	Password           string `required:"true" validate:"required" sensitive:"true"`
	Name               string `required:"true" validate:"required"`
	SSLMode            string `validate:"oneof=disable require verify-ca verify-full"`
	MaxConnections     int32  `validate:"min=1,max=100"`
	MinConnections     int32  `validate:"min=1,max=100"`
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
	Host            string `required:"true" validate:"required"`
	Port            string `required:"true" validate:"required,numeric"`
	Password        string `sensitive:"true"`
	DB              int    `validate:"min=0,max=15"`
	MaxRetries      int    `validate:"min=0,max=10"`
	MinRetryBackoff time.Duration
	MaxRetryBackoff time.Duration
	DialTimeout     time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	PoolSize        int `validate:"min=1,max=100"`
	MinIdleConns    int `validate:"min=0,max=100"`
	MaxConnAge      time.Duration
	PoolTimeout     time.Duration
	IdleTimeout     time.Duration
	TTL             time.Duration
}

// SecurityConfig holds security configuration
type SecurityConfig struct {
	JWTSecret            string `required:"true" validate:"required,min=32" sensitive:"true"`
	JWTExpiration        time.Duration
	JWTRefreshExpiration time.Duration
	BcryptCost           int `validate:"min=10,max=15"`
	RateLimitRequests    int `validate:"min=1"`
	RateLimitDuration    time.Duration
	AllowedOrigins       []string
	TrustedProxies       []string
	SecureHeaders        bool
	CSRFProtection       bool
	RequestIDHeader      string
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

// OutputConfig defines logging output destinations
type OutputConfig struct {
	Type    string         `json:"type"` // console, file, elasticsearch, datadog, etc.
	Level   string         `json:"level"`
	Format  string         `json:"format"`
	Options map[string]any `json:"options"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level            string           `json:"level"`
	Format           string           `json:"format"`
	EnableSampling   bool             `json:"enable_sampling"`
	SampleRate       float64          `json:"sample_rate"`
	EnableELK        bool             `json:"enable_elk"`
	ELKConfig        logger.ELKConfig `json:"elk"`
	EnableStackTrace bool             `json:"enable_stack_trace"`
	Outputs          []OutputConfig   `json:"outputs"`
}

// ConfigLoader handles configuration loading with secrets management
type ConfigLoader struct {
	logger         *slog.Logger
	secretsManager SecretsManager
	validators     []Validator
}

// SecretsManager interface for different secret providers
type SecretsManager interface {
	GetSecret(ctx context.Context, key string) (string, error)
	GetSecrets(ctx context.Context, keys []string) (map[string]string, error)
	RefreshSecrets(ctx context.Context) error
}

// Validator interface for configuration validation
type Validator interface {
	Validate(cfg *Config) error
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(logger *slog.Logger) *ConfigLoader {
	return &ConfigLoader{
		logger:     logger,
		validators: []Validator{},
	}
}

// Load loads configuration from environment and secrets
func Load(logger *slog.Logger) (*Config, error) {
	loader := NewConfigLoader(logger)
	return loader.Load(context.Background())
}

// Load loads configuration with context
func (cl *ConfigLoader) Load(ctx context.Context) (*Config, error) {
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	// Load .env file only in development
	if env == "development" || env == "local" {
		if err := cl.loadEnvFile(); err != nil {
			cl.logger.Warn("failed to load .env file",
				slog.String("error", err.Error()))
		}
	}

	// Initialize viper
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Build base configuration
	cfg := cl.buildConfig(env)

	// Initialize secrets manager based on environment
	if err := cl.initializeSecretsManager(ctx, cfg); err != nil {
		return nil, fmt.Errorf("failed to initialize secrets manager: %w", err)
	}

	// Load secrets if in production/staging
	if env != "development" && env != "local" {
		if err := cl.loadSecrets(ctx, cfg); err != nil {
			return nil, fmt.Errorf("failed to load secrets: %w", err)
		}
	}

	// Add validators based on environment
	cl.addValidators(env)

	// Validate configuration
	if err := cl.validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	// Log configuration summary (without sensitive data)
	cl.logConfigSummary(cfg)

	return cfg, nil
}

// loadEnvFile loads .env file for development
func (cl *ConfigLoader) loadEnvFile() error {
	if err := godotenv.Load(); err != nil {
		return err
	}
	cl.logger.Info(".env file loaded successfully")
	return nil
}

// buildConfig builds the configuration struct
func (cl *ConfigLoader) buildConfig(env string) *Config {
	return &Config{
		App: AppConfig{
			Name:        getEnv("APP_NAME", "resell-api"),
			Environment: env,
			Version:     getEnv("APP_VERSION", "dev"),
			LogLevel:    getEnv("LOG_LEVEL", cl.getDefaultLogLevel(env)),
			LogFormat:   getEnv("LOG_FORMAT", "json"),
			Debug:       getBoolEnv("APP_DEBUG", env == "development"),
		},
		Database: DatabaseConfig{
			Host:               getEnvRequired("DB_HOST", env),
			Port:               getEnvRequired("DB_PORT", env),
			User:               getEnvRequired("DB_USER", env),
			Password:           getEnvRequired("DB_PASSWORD", env),
			Name:               getEnvRequired("DB_NAME", env),
			SSLMode:            getEnv("DB_SSL_MODE", cl.getDefaultSSLMode(env)),
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
			Host:            getEnvRequired("REDIS_HOST", env),
			Port:            getEnvRequired("REDIS_PORT", env),
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
		Security: SecurityConfig{
			JWTSecret:            getEnvRequired("JWT_SECRET", env),
			JWTExpiration:        getDurationEnv("JWT_EXPIRATION", 24*time.Hour),
			JWTRefreshExpiration: getDurationEnv("JWT_REFRESH_EXPIRATION", 7*24*time.Hour),
			BcryptCost:           getIntEnv("BCRYPT_COST", cl.getDefaultBcryptCost(env)),
			RateLimitRequests:    getIntEnv("RATE_LIMIT_REQUESTS", 100),
			RateLimitDuration:    getDurationEnv("RATE_LIMIT_DURATION", time.Minute),
			AllowedOrigins:       getSliceEnv("ALLOWED_ORIGINS", cl.getDefaultAllowedOrigins(env)),
			TrustedProxies:       getSliceEnv("TRUSTED_PROXIES", []string{}),
			SecureHeaders:        getBoolEnv("SECURE_HEADERS", env == "production"),
			CSRFProtection:       getBoolEnv("CSRF_PROTECTION", env == "production"),
			RequestIDHeader:      getEnv("REQUEST_ID_HEADER", "X-Request-ID"),
		},
		Secrets: SecretsConfig{
			Provider:        getEnv("SECRETS_PROVIDER", cl.getDefaultSecretsProvider(env)),
			AWSRegion:       getEnv("AWS_REGION", "us-east-1"),
			SecretName:      getEnv("AWS_SECRET_NAME", fmt.Sprintf("resell-api/%s", env)),
			VaultAddr:       getEnv("VAULT_ADDR", ""),
			VaultToken:      getEnv("VAULT_TOKEN", ""),
			VaultPath:       getEnv("VAULT_PATH", fmt.Sprintf("secret/data/resell/%s", env)),
			RefreshInterval: getDurationEnv("SECRETS_REFRESH_INTERVAL", 5*time.Minute),
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
		Logging: LoggingConfig{
			Level:            getEnv("LOG_LEVEL", cl.getDefaultLogLevel(env)),
			Format:           getEnv("LOG_FORMAT", "json"),
			EnableSampling:   getBoolEnv("LOG_SAMPLING_ENABLE", false),
			SampleRate:       getFloatEnv("LOG_SAMPLING_RATE", 0.1),
			EnableELK:        getBoolEnv("LOG_ELK_ENABLE", false),
			EnableStackTrace: getBoolEnv("LOG_STACKTRACE_ENABLE", env == "development"),
			ELKConfig: logger.ELKConfig{
				ElasticsearchURL: getEnv("LOG_ELK_URL", ""),
				IndexPattern:     getEnv("LOG_ELK_INDEX", "resell-logs"),
				BatchSize:        getIntEnv("LOG_ELK_BATCH_SIZE", 100),
				FlushInterval:    getDurationEnv("LOG_ELK_FLUSH_INTERVAL", 5*time.Second),
				Username:         getEnv("LOG_ELK_USER", ""),
				Password:         getEnv("LOG_ELK_PASS", ""),
				EnableBatching:   getBoolEnv("LOG_ELK_BATCHING_ENABLE", true),
			},
		},
	}
}

// initializeSecretsManager initializes the appropriate secrets manager
func (cl *ConfigLoader) initializeSecretsManager(ctx context.Context, cfg *Config) error {
	switch cfg.Secrets.Provider {
	case "aws-secrets-manager":
		sm, err := NewAWSSecretsManager(cfg.Secrets.AWSRegion, cfg.Secrets.SecretName, cl.logger)
		if err != nil {
			return err
		}
		cl.secretsManager = sm
	case "vault":
		sm, err := NewVaultSecretsManager(cfg.Secrets.VaultAddr, cfg.Secrets.VaultToken, cfg.Secrets.VaultPath, cl.logger)
		if err != nil {
			return err
		}
		cl.secretsManager = sm
	case "env", "":
		cl.secretsManager = NewEnvSecretsManager()
	default:
		return fmt.Errorf("unknown secrets provider: %s", cfg.Secrets.Provider)
	}
	return nil
}

// loadSecrets loads secrets from the configured provider
func (cl *ConfigLoader) loadSecrets(ctx context.Context, cfg *Config) error {
	if cl.secretsManager == nil {
		return nil
	}

	// Define which configuration fields need to be loaded from secrets
	secretKeys := []string{
		"DB_PASSWORD",
		"JWT_SECRET",
		"REDIS_PASSWORD",
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
	}

	secrets, err := cl.secretsManager.GetSecrets(ctx, secretKeys)
	if err != nil {
		return fmt.Errorf("failed to load secrets: %w", err)
	}

	// Apply secrets to configuration
	if val, ok := secrets["DB_PASSWORD"]; ok && val != "" {
		cfg.Database.Password = val
	}
	if val, ok := secrets["JWT_SECRET"]; ok && val != "" {
		cfg.Security.JWTSecret = val
	}
	if val, ok := secrets["REDIS_PASSWORD"]; ok && val != "" {
		cfg.Redis.Password = val
	}
	if val, ok := secrets["AWS_ACCESS_KEY_ID"]; ok && val != "" {
		cfg.AWS.AccessKeyID = val
	}
	if val, ok := secrets["AWS_SECRET_ACCESS_KEY"]; ok && val != "" {
		cfg.AWS.SecretAccessKey = val
	}

	return nil
}

// addValidators adds appropriate validators based on environment
func (cl *ConfigLoader) addValidators(env string) {
	// Always add basic validator
	cl.validators = append(cl.validators, &BasicValidator{})

	// Add production validator for production/staging
	if env == "production" || env == "staging" {
		cl.validators = append(cl.validators, &ProductionValidator{})
	}

	// Add security validator
	cl.validators = append(cl.validators, &SecurityValidator{})
}

// validateConfig runs all validators
func (cl *ConfigLoader) validateConfig(cfg *Config) error {
	for _, validator := range cl.validators {
		if err := validator.Validate(cfg); err != nil {
			return err
		}
	}
	return nil
}

// logConfigSummary logs configuration summary without sensitive data
func (cl *ConfigLoader) logConfigSummary(cfg *Config) {
	cl.logger.Info("configuration loaded",
		slog.Group("app",
			slog.String("name", cfg.App.Name),
			slog.String("environment", cfg.App.Environment),
			slog.String("version", cfg.App.Version),
		),
		slog.Group("database",
			slog.String("host", cfg.Database.Host),
			slog.String("port", cfg.Database.Port),
			slog.String("name", cfg.Database.Name),
			slog.Bool("ssl", cfg.Database.SSLMode != "disable"),
		),
		slog.Group("redis",
			slog.String("host", cfg.Redis.Host),
			slog.String("port", cfg.Redis.Port),
			slog.Int("db", cfg.Redis.DB),
		),
		slog.Group("security",
			slog.Bool("secure_headers", cfg.Security.SecureHeaders),
			slog.Bool("csrf_protection", cfg.Security.CSRFProtection),
			slog.Int("rate_limit", cfg.Security.RateLimitRequests),
		),
	)
}

// Helper methods for default values based on environment
func (cl *ConfigLoader) getDefaultLogLevel(env string) string {
	if env == "production" {
		return "info"
	}
	return "debug"
}

func (cl *ConfigLoader) getDefaultSSLMode(env string) string {
	if env == "production" {
		return "require"
	}
	return "disable"
}

func (cl *ConfigLoader) getDefaultBcryptCost(env string) int {
	if env == "production" {
		return 12
	}
	return 10
}

func (cl *ConfigLoader) getDefaultAllowedOrigins(env string) []string {
	if env == "production" {
		return []string{} // Must be explicitly configured
	}
	return []string{"http://localhost:3000", "http://localhost:5173"}
}

func (cl *ConfigLoader) getDefaultSecretsProvider(env string) string {
	if env == "production" || env == "staging" {
		return "aws-secrets-manager"
	}
	return "env"
}

// Helper functions remain the same but with better handling for required values
func getEnvRequired(key string, env string) string {
	value := os.Getenv(key)
	if value == "" && (env == "production" || env == "staging") {
		// In production/staging, this will cause validation to fail
		return ""
	}
	// In development, return a placeholder that will trigger validation warning
	if value == "" {
		return fmt.Sprintf("MISSING_%s", key)
	}
	return value
}

// Rest of the helper functions remain similar...
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

func getFloatEnv(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		f, err := strconv.ParseFloat(value, 64)
		if err == nil {
			return f
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

// Configuration methods remain the same
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

func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}

func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development" || c.App.Environment == "local"
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
