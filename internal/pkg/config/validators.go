// internal/pkg/config/validators.go
package config

import (
	"fmt"
	"reflect"
	"strings"
)

// BasicValidator performs basic configuration validation
type BasicValidator struct{}

// Validate performs basic validation
func (v *BasicValidator) Validate(cfg *Config) error {
	// Validate required fields using reflection
	if err := validateRequiredFields(cfg); err != nil {
		return err
	}

	// Validate numeric ranges
	if cfg.Database.MaxConnections < cfg.Database.MinConnections {
		return fmt.Errorf("database max_connections must be >= min_connections")
	}

	if cfg.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis pool_size must be positive")
	}

	if cfg.Security.RateLimitRequests <= 0 {
		return fmt.Errorf("rate_limit_requests must be positive")
	}

	return nil
}

// ProductionValidator performs strict validation for production environments
type ProductionValidator struct{}

// Validate performs production-specific validation
func (v *ProductionValidator) Validate(cfg *Config) error {
	// Check for placeholder values
	if strings.Contains(cfg.Database.Password, "MISSING_") {
		return fmt.Errorf("%w: database password", ErrMissingRequiredConfig)
	}

	if strings.Contains(cfg.Security.JWTSecret, "MISSING_") {
		return fmt.Errorf("%w: JWT secret", ErrMissingRequiredConfig)
	}

	// Ensure secure defaults in production
	if cfg.Database.SSLMode == "disable" {
		return fmt.Errorf("database SSL must be enabled in production")
	}

	if !cfg.Security.SecureHeaders {
		return fmt.Errorf("secure headers must be enabled in production")
	}

	if !cfg.Security.CSRFProtection {
		return fmt.Errorf("CSRF protection must be enabled in production")
	}

	if len(cfg.Security.AllowedOrigins) == 0 {
		return fmt.Errorf("allowed origins must be configured in production")
	}

	// Check for insecure defaults
	if cfg.Security.JWTSecret == "development-secret-change-in-production" {
		return fmt.Errorf("default JWT secret cannot be used in production")
	}

	// Ensure proper TLS configuration
	if cfg.Server.TLSEnabled {
		if cfg.Server.TLSCertFile == "" || cfg.Server.TLSKeyFile == "" {
			return fmt.Errorf("TLS cert and key files must be provided when TLS is enabled")
		}
	}

	return nil
}

// SecurityValidator validates security-related configuration
type SecurityValidator struct{}

// Validate performs security validation
func (v *SecurityValidator) Validate(cfg *Config) error {
	// JWT secret strength
	if len(cfg.Security.JWTSecret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters")
	}

	// Bcrypt cost validation
	if cfg.Security.BcryptCost < 10 {
		return fmt.Errorf("bcrypt cost must be at least 10")
	}
	if cfg.Security.BcryptCost > 15 {
		return fmt.Errorf("bcrypt cost should not exceed 15 for performance reasons")
	}

	// Validate allowed origins format
	for _, origin := range cfg.Security.AllowedOrigins {
		if origin == "*" && cfg.IsProduction() {
			return fmt.Errorf("wildcard origin (*) not allowed in production")
		}
	}

	return nil
}

// validateRequiredFields uses reflection to check required struct tags
func validateRequiredFields(cfg interface{}) error {
	v := reflect.ValueOf(cfg)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	return validateStruct(v, "")
}

func validateStruct(v reflect.Value, prefix string) error {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		fieldName := fieldType.Name

		if prefix != "" {
			fieldName = prefix + "." + fieldName
		}

		// Check for required tag
		if required := fieldType.Tag.Get("required"); required == "true" {
			if isZeroValue(field) {
				return fmt.Errorf("%w: %s", ErrMissingRequiredConfig, fieldName)
			}
		}

		// Recursively check nested structs
		if field.Kind() == reflect.Struct {
			if err := validateStruct(field, fieldName); err != nil {
				return err
			}
		}
	}

	return nil
}

func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == "" || strings.HasPrefix(v.String(), "MISSING_")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	case reflect.Ptr, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
