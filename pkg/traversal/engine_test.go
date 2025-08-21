package traversal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

func TestDefaultTraversalConfig(t *testing.T) {
	config := NewDefaultTraversalConfig()
	
	assert.NotNil(t, config)
	assert.Equal(t, 3, config.MaxDepth)
	assert.Equal(t, 100, config.MaxResources)
	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.True(t, config.ScopeFilter.PlatformOnly)
	assert.Contains(t, config.ScopeFilter.IncludeAPIGroups, "*.kubecore.io")
	assert.True(t, config.BatchConfig.Enabled)
	assert.True(t, config.CacheConfig.Enabled)
	assert.True(t, config.ReferenceResolution.EnableDynamicCRDs)
	assert.True(t, config.CycleHandling.DetectionEnabled)
	assert.True(t, config.Performance.EnableMetrics)
}

func TestResourceTracker(t *testing.T) {
	tracker := NewResourceTracker()
	
	// Test initial state
	assert.Equal(t, 0, tracker.Size())
	assert.False(t, tracker.IsProcessed("test-resource"))
	
	// Test marking processed
	tracker.MarkProcessed("test-resource", 1)
	assert.Equal(t, 1, tracker.Size())
	assert.True(t, tracker.IsProcessed("test-resource"))
	
	// Test getting processed resource
	info := tracker.GetProcessedResource("test-resource")
	assert.NotNil(t, info)
	assert.Equal(t, "test-resource", info.ResourceID)
	assert.Equal(t, 1, info.Depth)
	assert.Equal(t, 1, info.ProcessingCount)
	
	// Test stats
	stats := tracker.GetStats()
	assert.Equal(t, 1, stats.TotalResources)
	assert.Equal(t, 1, stats.ResourcesByDepth[1])
	assert.Equal(t, 1, stats.MaxDepth)
}

func TestLRUCache(t *testing.T) {
	cache := NewLRUCache(2, 1*time.Minute)
	defer cache.Close()
	
	// Test basic operations
	cache.Set("key1", "value1", 1*time.Minute)
	cache.Set("key2", "value2", 1*time.Minute)
	
	value, found := cache.Get("key1")
	assert.True(t, found)
	assert.Equal(t, "value1", value)
	
	// Test eviction
	cache.Set("key3", "value3", 1*time.Minute)
	
	// key2 should be evicted since key1 was accessed more recently
	_, found = cache.Get("key2")
	assert.False(t, found)
	
	// Test stats
	stats := cache.Stats()
	assert.Equal(t, 2, stats.Size)
	assert.Equal(t, 2, stats.Capacity)
}

func TestDefaultPlatformChecker(t *testing.T) {
	checker := NewDefaultPlatformChecker([]string{"*.kubecore.io"})
	
	// Test platform API group detection
	assert.True(t, checker.IsPlatformAPIGroup("platform.kubecore.io"))
	assert.True(t, checker.IsPlatformAPIGroup("github.kubecore.io"))
	assert.False(t, checker.IsPlatformAPIGroup("apps"))
	assert.False(t, checker.IsPlatformAPIGroup("v1"))
	
	// Create test resources
	platformResource := &unstructured.Unstructured{}
	platformResource.SetAPIVersion("platform.kubecore.io/v1")
	platformResource.SetKind("KubeCluster")
	
	coreResource := &unstructured.Unstructured{}
	coreResource.SetAPIVersion("v1")
	coreResource.SetKind("Pod")
	
	// Test platform resource detection
	assert.True(t, checker.IsPlatformResource(platformResource))
	assert.False(t, checker.IsPlatformResource(coreResource))
}

func TestDefaultScopeFilter(t *testing.T) {
	platformChecker := NewDefaultPlatformChecker([]string{"*.kubecore.io"})
	filter := NewDefaultScopeFilter(platformChecker, logging.NewNopLogger())
	
	// Create test resources
	platformResource := &unstructured.Unstructured{}
	platformResource.SetAPIVersion("platform.kubecore.io/v1")
	platformResource.SetKind("KubeCluster")
	platformResource.SetName("test-cluster")
	
	coreResource := &unstructured.Unstructured{}
	coreResource.SetAPIVersion("v1")
	coreResource.SetKind("Pod")
	coreResource.SetName("test-pod")
	
	resources := []*unstructured.Unstructured{platformResource, coreResource}
	
	// Test platform-only filtering
	config := &ScopeFilterConfig{
		PlatformOnly: true,
	}
	
	filtered := filter.FilterResources(resources, config)
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "KubeCluster", filtered[0].GetKind())
	
	// Test include API groups
	config.PlatformOnly = false
	config.IncludeAPIGroups = []string{"platform.kubecore.io"}
	
	filtered = filter.FilterResources(resources, config)
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "KubeCluster", filtered[0].GetKind())
	
	// Test exclude kinds
	config.IncludeAPIGroups = nil
	config.ExcludeKinds = []string{"Pod"}
	
	filtered = filter.FilterResources(resources, config)
	assert.Equal(t, 1, len(filtered))
	assert.Equal(t, "KubeCluster", filtered[0].GetKind())
}

func TestBatchOptimizer(t *testing.T) {
	optimizer := NewDefaultBatchOptimizer(logging.NewNopLogger())
	
	// Create test resources
	var resources []*unstructured.Unstructured
	for i := 0; i < 25; i++ {
		resource := &unstructured.Unstructured{}
		resource.SetAPIVersion("platform.kubecore.io/v1")
		resource.SetKind("KubeCluster")
		resource.SetName(fmt.Sprintf("cluster-%d", i))
		resources = append(resources, resource)
	}
	
	config := &BatchConfig{
		Enabled:           true,
		BatchSize:         10,
		SameDepthBatching: false,
	}
	
	batches, err := optimizer.OptimizeBatches(context.Background(), resources, config)
	require.NoError(t, err)
	
	// Should create 3 batches (10, 10, 5)
	assert.Equal(t, 3, len(batches))
	assert.Equal(t, 10, len(batches[0].Resources))
	assert.Equal(t, 10, len(batches[1].Resources))
	assert.Equal(t, 5, len(batches[2].Resources))
	
	// Test statistics
	stats := optimizer.GetOptimizationStatistics()
	assert.Equal(t, 25, stats.TotalResources)
	assert.Equal(t, 3, stats.TotalBatches)
	assert.InDelta(t, 8.33, stats.AverageBatchSize, 0.1)
}

func TestMetricsCollector(t *testing.T) {
	collector := NewMetricsCollector(true)
	
	// Test initial state
	assert.True(t, collector.IsEnabled())
	assert.Equal(t, int64(0), collector.GetTotalAPIRequests())
	
	// Record some metrics
	collector.RecordAPIRequestLatency(100 * time.Millisecond)
	collector.RecordAPIRequestLatency(200 * time.Millisecond)
	collector.RecordAPIRequestLatency(150 * time.Millisecond)
	
	collector.RecordResourceProcessed()
	collector.RecordResourceProcessed()
	
	// Test metrics
	assert.Equal(t, int64(3), collector.GetTotalAPIRequests())
	assert.Equal(t, int64(2), collector.GetTotalResourcesProcessed())
	
	metrics := collector.GetMetrics()
	assert.NotNil(t, metrics.APIRequestLatency)
	assert.Equal(t, 100*time.Millisecond, metrics.APIRequestLatency.Min)
	assert.Equal(t, 200*time.Millisecond, metrics.APIRequestLatency.Max)
	assert.Equal(t, 150*time.Millisecond, metrics.APIRequestLatency.Average)
	
	// Test summary
	summary := collector.GetSummary()
	assert.True(t, summary.Enabled)
	assert.Equal(t, int64(3), summary.TotalAPIRequests)
	assert.Equal(t, int64(2), summary.TotalResourcesProcessed)
}

// Mock implementations for testing

type mockRegistry struct{}

func (mr *mockRegistry) GetResourceType(apiVersion, kind string) (*registry.ResourceType, error) {
	return &registry.ResourceType{
		APIVersion: apiVersion,
		Kind:       kind,
		Group:      "platform.kubecore.io",
		Version:    "v1",
		Namespaced: true,
	}, nil
}

func (mr *mockRegistry) ListResourceTypes() ([]registry.ResourceType, error) {
	return []registry.ResourceType{
		{
			APIVersion: "platform.kubecore.io/v1",
			Kind:       "KubeCluster",
			Group:      "platform.kubecore.io",
			Version:    "v1",
			Namespaced: true,
		},
	}, nil
}

func (mr *mockRegistry) GetGroupVersionResource(apiVersion, kind string) (schema.GroupVersionResource, error) {
	return schema.GroupVersionResource{
		Group:    "platform.kubecore.io",
		Version:  "v1",
		Resource: "kubeclusters",
	}, nil
}

func (mr *mockRegistry) IsNamespaced(apiVersion, kind string) (bool, error) {
	return true, nil
}

func (mr *mockRegistry) GetReferences(apiVersion, kind string) ([]registry.ReferenceField, error) {
	return []registry.ReferenceField{}, nil
}

// Integration test for traversal engine (would require actual Kubernetes cluster)
func TestTraversalEngineIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	
	// This would require a real Kubernetes cluster with test resources
	// For now, we'll just test that the engine can be created
	
	// Mock Kubernetes config (would use real config in integration test)
	config := &rest.Config{}
	registry := &mockRegistry{}
	logger := logging.NewNopLogger()
	
	// This would fail in unit tests due to missing cluster, but shows the API
	_, err := NewDefaultTraversalEngine(config, registry, logger)
	
	// In unit tests, we expect this to fail
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create")
}

// Benchmark tests
func BenchmarkLRUCache(b *testing.B) {
	cache := NewLRUCache(1000, 1*time.Minute)
	defer cache.Close()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%500)
		cache.Set(key, fmt.Sprintf("value-%d", i), 1*time.Minute)
		cache.Get(key)
	}
}

func BenchmarkResourceTracker(b *testing.B) {
	tracker := NewResourceTracker()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resourceID := fmt.Sprintf("resource-%d", i)
		tracker.MarkProcessed(resourceID, i%5)
		tracker.IsProcessed(resourceID)
	}
}