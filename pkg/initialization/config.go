package initialization

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/types"
)

// LoadConfigFromEnvironment loads registry configuration from environment variables
func LoadConfigFromEnvironment() *types.RegistryConfig {
	config := &types.RegistryConfig{
		Mode:              types.RegistryModeHybrid, // Default
		APIGroupPatterns:  types.DefaultAPIGroupPatterns,
		Timeout:           types.DefaultDiscoveryTimeout,
		FallbackEnabled:   true,
		RefPatterns:       []string{},
		CacheEnabled:      true,
		CacheTTL:          types.DefaultCacheTTL,
		LogLevel:          "info",
	}
	
	// Registry mode
	if mode := os.Getenv("REGISTRY_MODE"); mode != "" {
		config.Mode = types.RegistryMode(mode)
	}
	
	// API group patterns
	if patterns := os.Getenv("API_GROUP_PATTERNS"); patterns != "" {
		config.APIGroupPatterns = strings.Split(patterns, ",")
		// Trim whitespace
		for i, pattern := range config.APIGroupPatterns {
			config.APIGroupPatterns[i] = strings.TrimSpace(pattern)
		}
	}
	
	// Discovery timeout
	if timeout := os.Getenv("DISCOVERY_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			config.Timeout = duration
		}
	}
	
	// Fallback enabled
	if fallback := os.Getenv("FALLBACK_ENABLED"); fallback != "" {
		if enabled, err := strconv.ParseBool(fallback); err == nil {
			config.FallbackEnabled = enabled
		}
	}
	
	// Reference patterns
	if patterns := os.Getenv("REF_PATTERNS"); patterns != "" {
		config.RefPatterns = strings.Split(patterns, ",")
		// Trim whitespace
		for i, pattern := range config.RefPatterns {
			config.RefPatterns[i] = strings.TrimSpace(pattern)
		}
	}
	
	// Cache enabled
	if cache := os.Getenv("CACHE_ENABLED"); cache != "" {
		if enabled, err := strconv.ParseBool(cache); err == nil {
			config.CacheEnabled = enabled
		}
	}
	
	// Cache TTL
	if ttl := os.Getenv("CACHE_TTL"); ttl != "" {
		if duration, err := time.ParseDuration(ttl); err == nil {
			config.CacheTTL = duration
		}
	}
	
	// Log level
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		config.LogLevel = level
	}
	
	return config
}