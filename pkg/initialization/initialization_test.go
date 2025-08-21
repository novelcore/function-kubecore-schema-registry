package initialization

import (
	"os"
	"testing"
	"time"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/types"
)

func TestLoadConfigFromEnvironment(t *testing.T) {
	// Test default configuration
	config := LoadConfigFromEnvironment()

	if config.Mode != types.RegistryModeHybrid {
		t.Errorf("Expected default mode to be hybrid, got %s", config.Mode)
	}

	if config.Timeout != types.DefaultDiscoveryTimeout {
		t.Errorf("Expected default timeout to be %v, got %v", types.DefaultDiscoveryTimeout, config.Timeout)
	}

	if !config.FallbackEnabled {
		t.Error("Expected fallback to be enabled by default")
	}

	if len(config.APIGroupPatterns) == 0 {
		t.Error("Expected default API group patterns to be set")
	}
}

func TestLoadConfigFromEnvironmentWithCustomValues(t *testing.T) {
	// Set environment variables
	os.Setenv("REGISTRY_MODE", "dynamic")
	os.Setenv("API_GROUP_PATTERNS", "test.kubecore.io,another.kubecore.io")
	os.Setenv("DISCOVERY_TIMEOUT", "10s")
	os.Setenv("FALLBACK_ENABLED", "false")
	os.Setenv("CACHE_ENABLED", "false")

	defer func() {
		// Clean up environment variables
		os.Unsetenv("REGISTRY_MODE")
		os.Unsetenv("API_GROUP_PATTERNS")
		os.Unsetenv("DISCOVERY_TIMEOUT")
		os.Unsetenv("FALLBACK_ENABLED")
		os.Unsetenv("CACHE_ENABLED")
	}()

	config := LoadConfigFromEnvironment()

	if config.Mode != types.RegistryModeDynamic {
		t.Errorf("Expected mode to be dynamic, got %s", config.Mode)
	}

	expectedPatterns := []string{"test.kubecore.io", "another.kubecore.io"}
	if len(config.APIGroupPatterns) != len(expectedPatterns) {
		t.Errorf("Expected %d API group patterns, got %d", len(expectedPatterns), len(config.APIGroupPatterns))
	}

	for i, pattern := range expectedPatterns {
		if config.APIGroupPatterns[i] != pattern {
			t.Errorf("Expected pattern %s, got %s", pattern, config.APIGroupPatterns[i])
		}
	}

	if config.Timeout != 10*time.Second {
		t.Errorf("Expected timeout to be 10s, got %v", config.Timeout)
	}

	if config.FallbackEnabled {
		t.Error("Expected fallback to be disabled")
	}

	if config.CacheEnabled {
		t.Error("Expected cache to be disabled")
	}
}

func TestLoadConfigInvalidValues(t *testing.T) {
	// Set invalid environment variables
	os.Setenv("DISCOVERY_TIMEOUT", "invalid")
	os.Setenv("FALLBACK_ENABLED", "not-a-bool")
	os.Setenv("CACHE_ENABLED", "not-a-bool")

	defer func() {
		// Clean up environment variables
		os.Unsetenv("DISCOVERY_TIMEOUT")
		os.Unsetenv("FALLBACK_ENABLED")
		os.Unsetenv("CACHE_ENABLED")
	}()

	config := LoadConfigFromEnvironment()

	// Should fall back to defaults for invalid values
	if config.Timeout != types.DefaultDiscoveryTimeout {
		t.Errorf("Expected timeout to fall back to default %v, got %v", types.DefaultDiscoveryTimeout, config.Timeout)
	}

	if !config.FallbackEnabled {
		t.Error("Expected fallback to fall back to default (true)")
	}

	if !config.CacheEnabled {
		t.Error("Expected cache to fall back to default (true)")
	}
}
