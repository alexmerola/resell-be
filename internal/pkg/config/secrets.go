// internal/pkg/config/secrets.go
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// AWSSecretsManager implements AWS Secrets Manager integration
type AWSSecretsManager struct {
	client     *secretsmanager.Client
	secretName string
	cache      map[string]string
	cacheMu    sync.RWMutex
	lastFetch  time.Time
	ttl        time.Duration
	logger     *slog.Logger
}

// NewAWSSecretsManager creates a new AWS Secrets Manager client
func NewAWSSecretsManager(region, secretName string, logger *slog.Logger) (*AWSSecretsManager, error) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	client := secretsmanager.NewFromConfig(cfg)

	return &AWSSecretsManager{
		client:     client,
		secretName: secretName,
		cache:      make(map[string]string),
		ttl:        5 * time.Minute,
		logger:     logger,
	}, nil
}

// GetSecret retrieves a single secret
func (sm *AWSSecretsManager) GetSecret(ctx context.Context, key string) (string, error) {
	secrets, err := sm.GetSecrets(ctx, []string{key})
	if err != nil {
		return "", err
	}

	val, ok := secrets[key]
	if !ok {
		return "", fmt.Errorf("secret key %s not found", key)
	}

	return val, nil
}

// GetSecrets retrieves multiple secrets
func (sm *AWSSecretsManager) GetSecrets(ctx context.Context, keys []string) (map[string]string, error) {
	// Check cache first
	sm.cacheMu.RLock()
	if time.Since(sm.lastFetch) < sm.ttl && len(sm.cache) > 0 {
		cached := make(map[string]string)
		for _, key := range keys {
			if val, ok := sm.cache[key]; ok {
				cached[key] = val
			}
		}
		sm.cacheMu.RUnlock()

		if len(cached) == len(keys) {
			sm.logger.Debug("returning cached secrets")
			return cached, nil
		}
	}
	sm.cacheMu.RUnlock()

	// Fetch from AWS
	sm.logger.Info("fetching secrets from AWS Secrets Manager",
		slog.String("secret_name", sm.secretName))

	input := &secretsmanager.GetSecretValueInput{
		SecretId:     aws.String(sm.secretName),
		VersionStage: aws.String("AWSCURRENT"),
	}

	result, err := sm.client.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}

	// Parse the secret
	var secretData map[string]string
	if err := json.Unmarshal([]byte(*result.SecretString), &secretData); err != nil {
		return nil, fmt.Errorf("failed to parse secret JSON: %w", err)
	}

	// Update cache
	sm.cacheMu.Lock()
	sm.cache = secretData
	sm.lastFetch = time.Now()
	sm.cacheMu.Unlock()

	// Filter requested keys
	filtered := make(map[string]string)
	for _, key := range keys {
		if val, ok := secretData[key]; ok {
			filtered[key] = val
		} else {
			sm.logger.Warn("secret key not found in AWS Secrets Manager",
				slog.String("key", key))
		}
	}

	return filtered, nil
}

// RefreshSecrets refreshes the secrets cache
func (sm *AWSSecretsManager) RefreshSecrets(ctx context.Context) error {
	sm.cacheMu.Lock()
	sm.cache = make(map[string]string)
	sm.lastFetch = time.Time{}
	sm.cacheMu.Unlock()

	_, err := sm.GetSecrets(ctx, []string{})
	return err
}

// EnvSecretsManager implements secrets management using environment variables
type EnvSecretsManager struct{}

// NewEnvSecretsManager creates a new environment-based secrets manager
func NewEnvSecretsManager() *EnvSecretsManager {
	return &EnvSecretsManager{}
}

// GetSecret retrieves a secret from environment variables
func (em *EnvSecretsManager) GetSecret(ctx context.Context, key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", fmt.Errorf("environment variable %s not set", key)
	}
	return val, nil
}

// GetSecrets retrieves multiple secrets from environment variables
func (em *EnvSecretsManager) GetSecrets(ctx context.Context, keys []string) (map[string]string, error) {
	secrets := make(map[string]string)
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			secrets[key] = val
		}
	}
	return secrets, nil
}

// RefreshSecrets is a no-op for environment variables
func (em *EnvSecretsManager) RefreshSecrets(ctx context.Context) error {
	return nil
}

// VaultSecretsManager would implement HashiCorp Vault integration
// This is a stub for future implementation
type VaultSecretsManager struct {
	addr   string
	token  string
	path   string
	logger *slog.Logger
	// Add Vault client here
}

// NewVaultSecretsManager creates a new Vault secrets manager
func NewVaultSecretsManager(addr, token, path string, logger *slog.Logger) (*VaultSecretsManager, error) {
	// Implementation would go here
	return &VaultSecretsManager{
		addr:   addr,
		token:  token,
		path:   path,
		logger: logger,
	}, nil
}

// Implement SecretsManager interface methods...
func (vm *VaultSecretsManager) GetSecret(ctx context.Context, key string) (string, error) {
	// Vault implementation
	return "", fmt.Errorf("vault integration not yet implemented")
}

func (vm *VaultSecretsManager) GetSecrets(ctx context.Context, keys []string) (map[string]string, error) {
	// Vault implementation
	return nil, fmt.Errorf("vault integration not yet implemented")
}

func (vm *VaultSecretsManager) RefreshSecrets(ctx context.Context) error {
	// Vault implementation
	return fmt.Errorf("vault integration not yet implemented")
}
