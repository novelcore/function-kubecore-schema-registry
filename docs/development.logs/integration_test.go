package main

import (
	"os"
	"testing"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/types"
)

func TestFunctionWithDynamicConfiguration(t *testing.T) {
	// Test that function can be created with dynamic configuration
	os.Setenv("REGISTRY_MODE", "dynamic")
	os.Setenv("API_GROUP_PATTERNS", "test.kubecore.io")
	os.Setenv("DISCOVERY_TIMEOUT", "10s")
	
	defer func() {
		os.Unsetenv("REGISTRY_MODE")
		os.Unsetenv("API_GROUP_PATTERNS")
		os.Unsetenv("DISCOVERY_TIMEOUT")
	}()
	
	// Create function
	logger := logging.NewNopLogger()
	function := NewFunction(logger)
	
	// Verify configuration was loaded
	if function.config.Mode != types.RegistryModeDynamic {
		t.Errorf("Expected dynamic mode, got %s", function.config.Mode)
	}
	
	if len(function.config.APIGroupPatterns) != 1 || function.config.APIGroupPatterns[0] != "test.kubecore.io" {
		t.Errorf("Expected [test.kubecore.io], got %v", function.config.APIGroupPatterns)
	}
	
	if function.config.Timeout.String() != "10s" {
		t.Errorf("Expected 10s timeout, got %v", function.config.Timeout)
	}
}

func TestFunctionDefaultConfiguration(t *testing.T) {
	// Ensure environment is clean
	os.Unsetenv("REGISTRY_MODE")
	os.Unsetenv("API_GROUP_PATTERNS")
	os.Unsetenv("DISCOVERY_TIMEOUT")
	
	// Create function
	logger := logging.NewNopLogger()
	function := NewFunction(logger)
	
	// Verify defaults were applied
	if function.config.Mode != types.RegistryModeHybrid {
		t.Errorf("Expected hybrid mode by default, got %s", function.config.Mode)
	}
	
	if !function.config.FallbackEnabled {
		t.Error("Expected fallback to be enabled by default")
	}
	
	if !function.config.CacheEnabled {
		t.Error("Expected cache to be enabled by default")
	}
	
	// Should contain default KubeCore patterns
	foundKubeCore := false
	for _, pattern := range function.config.APIGroupPatterns {
		if pattern == "*.kubecore.io" {
			foundKubeCore = true
			break
		}
	}
	
	if !foundKubeCore {
		t.Errorf("Expected default patterns to include *.kubecore.io, got %v", function.config.APIGroupPatterns)
	}
}

func TestFunctionRegistryInitialization(t *testing.T) {
	// Test that registry is properly initialized
	logger := logging.NewNopLogger()
	function := NewFunction(logger)
	
	// Registry should be available
	if function.registry == nil {
		t.Fatal("Registry should be initialized")
	}
	
	// Should be able to list resource types
	types, err := function.registry.ListResourceTypes()
	if err != nil {
		t.Fatalf("Should be able to list resource types: %v", err)
	}
	
	if len(types) == 0 {
		t.Error("Registry should contain resource types")
	}
	
	// Check for some expected KubeCore types
	foundKubEnv := false
	foundKubeCluster := false
	
	for _, rt := range types {
		if rt.Kind == "KubEnv" {
			foundKubEnv = true
		}
		if rt.Kind == "KubeCluster" {
			foundKubeCluster = true
		}
	}
	
	if !foundKubEnv {
		t.Error("Expected to find KubEnv resource type")
	}
	
	if !foundKubeCluster {
		t.Error("Expected to find KubeCluster resource type")
	}
}