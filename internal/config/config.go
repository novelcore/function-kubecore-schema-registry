package config

import (
	"os"
	"strconv"
	"time"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
)

// Config holds configuration for the schema registry function
type Config struct {
	// Cache settings
	CacheTTL time.Duration
	
	// Default discovery options
	DefaultTraversalDepth     int
	DefaultEnableTransitive   bool
	DefaultIncludeFullSchema  bool
	
	// Kubernetes client settings
	InClusterConfig bool
	KubeConfigPath  string
	
	// Timeout settings
	DiscoveryTimeout time.Duration
	APICallTimeout   time.Duration
	
	// Logging settings
	LogLevel string
	
	// Reference patterns
	KnownReferencePatterns map[string]domain.ReferencePattern
}

// New creates a new configuration with defaults
func New() *Config {
	return &Config{
		CacheTTL:                 getEnvDuration("CACHE_TTL", 5*time.Minute),
		DefaultTraversalDepth:    getEnvInt("DEFAULT_TRAVERSAL_DEPTH", 3),
		DefaultEnableTransitive:  getEnvBool("DEFAULT_ENABLE_TRANSITIVE", true),
		DefaultIncludeFullSchema: getEnvBool("DEFAULT_INCLUDE_FULL_SCHEMA", true),
		InClusterConfig:          getEnvBool("IN_CLUSTER_CONFIG", true),
		KubeConfigPath:           getEnv("KUBECONFIG_PATH", ""),
		DiscoveryTimeout:         getEnvDuration("DISCOVERY_TIMEOUT", 30*time.Second),
		APICallTimeout:           getEnvDuration("API_CALL_TIMEOUT", 10*time.Second),
		LogLevel:                 getEnv("LOG_LEVEL", "info"),
		KnownReferencePatterns:   getDefaultReferencePatterns(),
	}
}

// getDefaultReferencePatterns returns the default reference patterns
func getDefaultReferencePatterns() map[string]domain.ReferencePattern {
	return map[string]domain.ReferencePattern{
		"githubProjectRef": {
			KindHint:   "GitHubProject",
			APIVersion: "github.platform.kubecore.io/v1alpha1",
		},
		"githubProviderRef": {
			KindHint:   "GithubProvider",
			APIVersion: "github.platform.kubecore.io/v1alpha1",
		},
		"providerConfigRef": {
			KindHint:   "ProviderConfig",
			APIVersion: "pkg.crossplane.io/v1",
		},
		"secretRef": {
			KindHint:   "Secret",
			APIVersion: "v1",
		},
		"configMapRef": {
			KindHint:   "ConfigMap",
			APIVersion: "v1",
		},
		"serviceAccountRef": {
			KindHint:   "ServiceAccount",
			APIVersion: "v1",
		},
		"qualityGateRef": {
			KindHint:   "QualityGate",
			APIVersion: "ci.platform.kubecore.io/v1alpha1",
		},
		"qualityGateRefs": {
			KindHint:   "QualityGate",
			APIVersion: "ci.platform.kubecore.io/v1alpha1",
			IsArray:    true,
		},
	}
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}