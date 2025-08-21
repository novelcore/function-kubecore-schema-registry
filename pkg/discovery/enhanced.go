package discovery

import (
	"context"
	"fmt"
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	
	"github.com/crossplane/function-sdk-go/logging"
	
	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/dynamic"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/graph"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/traversal"
)

// EnhancedDiscoveryEngine extends the discovery engine with Phase 3 capabilities
type EnhancedDiscoveryEngine struct {
	// base is the base discovery engine (Phase 1 & 2)
	base Engine
	
	// traversalEngine provides Phase 3 transitive discovery capabilities
	traversalEngine traversal.TraversalEngine
	
	// logger provides structured logging
	logger logging.Logger
	
	// config contains the discovery context
	config DiscoveryContext
}

// NewEnhancedDiscoveryEngine creates a new enhanced discovery engine with Phase 3 capabilities
func NewEnhancedDiscoveryEngine(config *rest.Config, registry registry.Registry, context DiscoveryContext, logger logging.Logger) (*EnhancedDiscoveryEngine, error) {
	// Create base engine for Phase 1 & 2
	baseEngine, err := NewEnhancedEngine(config, registry, context)
	if err != nil {
		return nil, fmt.Errorf("failed to create base engine: %w", err)
	}
	
	// Create traversal engine for Phase 3
	traversalEngine, err := traversal.NewDefaultTraversalEngine(config, registry, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create traversal engine: %w", err)
	}
	
	return &EnhancedDiscoveryEngine{
		base:            baseEngine,
		traversalEngine: traversalEngine,
		logger:          logger,
		config:          context,
	}, nil
}

// FetchResources fetches resources using Phase 1, 2, or 3 based on configuration
func (ede *EnhancedDiscoveryEngine) FetchResources(requests []v1beta1.ResourceRequest) (*FetchResult, error) {
	// Check if any request has Phase 3 configuration
	hasPhase3Config := false
	for _, req := range requests {
		if hasPhase3ResourceRequest(req) {
			hasPhase3Config = true
			break
		}
	}
	
	if !hasPhase3Config {
		// Use base engine for Phase 1 & 2 functionality
		return ede.base.FetchResources(requests)
	}
	
	// Execute Phase 3 transitive discovery
	return ede.executePhase3Discovery(requests)
}

// executePhase3Discovery executes Phase 3 transitive discovery
func (ede *EnhancedDiscoveryEngine) executePhase3Discovery(requests []v1beta1.ResourceRequest) (*FetchResult, error) {
	ctx := context.Background()
	
	ede.logger.Info("Starting Phase 3 transitive discovery", "requestCount", len(requests))
	
	// Step 1: Perform Phase 1 & 2 discovery to get initial resources
	baseResult, err := ede.base.FetchResources(requests)
	if err != nil {
		return nil, fmt.Errorf("Phase 1/2 discovery failed: %w", err)
	}
	
	// Step 2: Extract root resources for traversal
	var rootResources []*unstructured.Unstructured
	for _, resource := range baseResult.Resources {
		if resource.Resource != nil {
			rootResources = append(rootResources, resource.Resource)
		}
	}
	
	for _, resources := range baseResult.MultiResources {
		for _, resource := range resources {
			if resource.Resource != nil {
				rootResources = append(rootResources, resource.Resource)
			}
		}
	}
	
	if len(rootResources) == 0 {
		ede.logger.Info("No root resources found for Phase 3 traversal")
		return baseResult, nil
	}
	
	// Step 3: Build traversal configuration
	traversalConfig := ede.buildTraversalConfig(requests)
	
	// Step 4: Execute transitive discovery
	traversalResult, err := ede.traversalEngine.ExecuteTransitiveDiscovery(ctx, traversalConfig, rootResources)
	if err != nil {
		return nil, fmt.Errorf("transitive discovery failed: %w", err)
	}
	
	// Step 5: Merge results
	mergedResult := ede.mergeResults(baseResult, traversalResult)
	
	ede.logger.Info("Phase 3 transitive discovery completed",
		"rootResources", len(rootResources),
		"discoveredResources", len(traversalResult.DiscoveredResources),
		"traversalDuration", traversalResult.TraversalPath.Duration,
		"terminationReason", traversalResult.Metadata.TerminationReason)
	
	return mergedResult, nil
}

// buildTraversalConfig builds traversal configuration from resource requests
func (ede *EnhancedDiscoveryEngine) buildTraversalConfig(requests []v1beta1.ResourceRequest) *traversal.TraversalConfig {
	// Start with default configuration
	config := traversal.NewDefaultTraversalConfig()
	
	// Extract Phase 3 configuration from requests
	for _, req := range requests {
		if phase3Config := extractPhase3Config(req); phase3Config != nil {
			// Apply Phase 3 configuration
			ede.applyPhase3Config(config, phase3Config)
			break // Use first found Phase 3 config
		}
	}
	
	// Apply discovery context settings
	config.Performance.MaxConcurrentRequests = ede.config.MaxConcurrentRequests
	
	// Set timeout from context
	if ede.config.TimeoutPerRequest > 0 {
		config.Timeout = ede.config.TimeoutPerRequest * 5 // Allow 5x per-request timeout for full traversal
	}
	
	return config
}

// applyPhase3Config applies Phase 3 configuration to the traversal config
func (ede *EnhancedDiscoveryEngine) applyPhase3Config(config *traversal.TraversalConfig, phase3Config *Phase3Config) {
	if phase3Config.MaxDepth > 0 {
		config.MaxDepth = phase3Config.MaxDepth
	}
	
	if phase3Config.MaxResources > 0 {
		config.MaxResources = phase3Config.MaxResources
	}
	
	if phase3Config.Timeout != nil {
		if timeout, err := time.ParseDuration(*phase3Config.Timeout); err == nil {
			config.Timeout = timeout
		}
	}
	
	if phase3Config.Direction != "" {
		switch phase3Config.Direction {
		case "forward":
			config.Direction = graph.TraversalDirectionForward
		case "reverse":
			config.Direction = graph.TraversalDirectionReverse
		case "bidirectional":
			config.Direction = graph.TraversalDirectionBidirectional
		}
	}
	
	// Apply scope filter configuration
	if phase3Config.ScopeFilter != nil {
		config.ScopeFilter.PlatformOnly = phase3Config.ScopeFilter.PlatformOnly
		config.ScopeFilter.CrossNamespaceEnabled = phase3Config.ScopeFilter.CrossNamespaceEnabled
		
		if len(phase3Config.ScopeFilter.IncludeAPIGroups) > 0 {
			config.ScopeFilter.IncludeAPIGroups = phase3Config.ScopeFilter.IncludeAPIGroups
		}
		
		if len(phase3Config.ScopeFilter.ExcludeAPIGroups) > 0 {
			config.ScopeFilter.ExcludeAPIGroups = phase3Config.ScopeFilter.ExcludeAPIGroups
		}
		
		if len(phase3Config.ScopeFilter.IncludeKinds) > 0 {
			config.ScopeFilter.IncludeKinds = phase3Config.ScopeFilter.IncludeKinds
		}
		
		if len(phase3Config.ScopeFilter.ExcludeKinds) > 0 {
			config.ScopeFilter.ExcludeKinds = phase3Config.ScopeFilter.ExcludeKinds
		}
		
		if len(phase3Config.ScopeFilter.IncludeNamespaces) > 0 {
			config.ScopeFilter.IncludeNamespaces = phase3Config.ScopeFilter.IncludeNamespaces
		}
		
		if len(phase3Config.ScopeFilter.ExcludeNamespaces) > 0 {
			config.ScopeFilter.ExcludeNamespaces = phase3Config.ScopeFilter.ExcludeNamespaces
		}
	}
	
	// Apply performance configuration
	if phase3Config.Performance != nil {
		if phase3Config.Performance.MaxConcurrentRequests > 0 {
			config.Performance.MaxConcurrentRequests = phase3Config.Performance.MaxConcurrentRequests
		}
		
		if phase3Config.Performance.RequestTimeout != nil {
			if timeout, err := time.ParseDuration(*phase3Config.Performance.RequestTimeout); err == nil {
				config.Performance.RequestTimeout = timeout
			}
		}
		
		config.Performance.EnableMetrics = phase3Config.Performance.EnableMetrics
		config.Performance.ResourceDeduplication = phase3Config.Performance.ResourceDeduplication
		
		if phase3Config.Performance.MemoryLimits != nil {
			if config.Performance.MemoryLimits == nil {
				config.Performance.MemoryLimits = &traversal.MemoryLimits{}
			}
			
			config.Performance.MemoryLimits.MaxGraphSize = phase3Config.Performance.MemoryLimits.MaxGraphSize
			config.Performance.MemoryLimits.MaxCacheSize = phase3Config.Performance.MemoryLimits.MaxCacheSize
			config.Performance.MemoryLimits.GCThreshold = phase3Config.Performance.MemoryLimits.GCThreshold
		}
	}
	
	// Apply reference resolution configuration
	if phase3Config.ReferenceResolution != nil {
		config.ReferenceResolution.EnableDynamicCRDs = phase3Config.ReferenceResolution.EnableDynamicCRDs
		config.ReferenceResolution.FollowOwnerReferences = phase3Config.ReferenceResolution.FollowOwnerReferences
		config.ReferenceResolution.FollowCustomReferences = phase3Config.ReferenceResolution.FollowCustomReferences
		config.ReferenceResolution.SkipMissingReferences = phase3Config.ReferenceResolution.SkipMissingReferences
		config.ReferenceResolution.MinConfidenceThreshold = phase3Config.ReferenceResolution.MinConfidenceThreshold
		
		// Convert additional patterns
		for _, pattern := range phase3Config.ReferenceResolution.AdditionalPatterns {
			config.ReferenceResolution.ReferencePatterns = append(
				config.ReferenceResolution.ReferencePatterns,
				traversal.ReferencePattern{
					Pattern:     pattern.Pattern,
					TargetKind:  pattern.TargetKind,
					TargetGroup: pattern.TargetGroup,
					Confidence:  pattern.Confidence,
					RefType:     traversal.RefTypeCustom,
				},
			)
		}
	}
	
	// Apply cycle handling configuration
	if phase3Config.CycleHandling != nil {
		config.CycleHandling.DetectionEnabled = phase3Config.CycleHandling.DetectionEnabled
		config.CycleHandling.MaxCycles = phase3Config.CycleHandling.MaxCycles
		config.CycleHandling.ReportCycles = phase3Config.CycleHandling.ReportCycles
		
		switch phase3Config.CycleHandling.OnCycleDetected {
		case "continue":
			config.CycleHandling.OnCycleDetected = traversal.CycleActionContinue
		case "stop":
			config.CycleHandling.OnCycleDetected = traversal.CycleActionStop
		case "fail":
			config.CycleHandling.OnCycleDetected = traversal.CycleActionFail
		}
	}
	
	// Apply batch configuration
	if phase3Config.BatchConfig != nil {
		config.BatchConfig.Enabled = phase3Config.BatchConfig.Enabled
		config.BatchConfig.BatchSize = phase3Config.BatchConfig.BatchSize
		config.BatchConfig.MaxConcurrentBatches = phase3Config.BatchConfig.MaxConcurrentBatches
		config.BatchConfig.SameDepthBatching = phase3Config.BatchConfig.SameDepthBatching
		
		if phase3Config.BatchConfig.BatchTimeout != nil {
			if timeout, err := time.ParseDuration(*phase3Config.BatchConfig.BatchTimeout); err == nil {
				config.BatchConfig.BatchTimeout = timeout
			}
		}
	}
	
	// Apply cache configuration
	if phase3Config.CacheConfig != nil {
		config.CacheConfig.Enabled = phase3Config.CacheConfig.Enabled
		config.CacheConfig.MaxSize = phase3Config.CacheConfig.MaxSize
		
		if phase3Config.CacheConfig.TTL != nil {
			if ttl, err := time.ParseDuration(*phase3Config.CacheConfig.TTL); err == nil {
				config.CacheConfig.TTL = ttl
			}
		}
		
		switch phase3Config.CacheConfig.Strategy {
		case "lru":
			config.CacheConfig.CacheStrategy = traversal.CacheStrategyLRU
		case "lfu":
			config.CacheConfig.CacheStrategy = traversal.CacheStrategyLFU
		case "ttl":
			config.CacheConfig.CacheStrategy = traversal.CacheStrategyTTL
		}
	}
}

// mergeResults merges Phase 1/2 results with Phase 3 traversal results
func (ede *EnhancedDiscoveryEngine) mergeResults(baseResult *FetchResult, traversalResult *traversal.TraversalResult) *FetchResult {
	// Start with base result
	mergedResult := *baseResult
	
	// Add Phase 3 metadata
	if mergedResult.Phase2Results == nil {
		mergedResult.Phase2Results = &Phase2Results{}
	}
	
	// Add traversal statistics to performance metrics
	if mergedResult.Phase2Results.Performance == nil {
		mergedResult.Phase2Results.Performance = &PerformanceMetrics{}
	}
	
	// Convert traversal statistics to discovery performance metrics
	if traversalResult.Statistics.PerformanceMetrics != nil {
		mergedResult.Phase2Results.Performance.KubernetesAPITime += traversalResult.Statistics.PerformanceMetrics.APIRequestLatency.Average
		mergedResult.Phase2Results.Performance.TotalResourcesScanned += traversalResult.Statistics.TotalResources
	}
	
	// Add discovered resources to the result
	for resourceID, resource := range traversalResult.DiscoveredResources {
		// Convert to FetchedResource format
		namespace := resource.GetNamespace()
		fetchedResource := &FetchedResource{
			Request: v1beta1.ResourceRequest{
				Into:       resourceID,
				APIVersion: resource.GetAPIVersion(),
				Kind:       resource.GetKind(),
				Name:       resource.GetName(),
				Namespace:  &namespace,
				MatchType:  v1beta1.MatchTypeDirect,
			},
			Resource:  resource,
			FetchedAt: time.Now(),
			Metadata: ResourceMetadata{
				FetchStatus:    FetchStatusSuccess,
				FetchDuration:  0, // TODO: Get actual fetch duration
				ResourceExists: true,
				Phase2Metadata: &Phase2Metadata{
					MatchedBy: "phase3_transitive_discovery",
				},
			},
		}
		
		// Add to multi-resources (Phase 3 can discover multiple resources)
		key := fmt.Sprintf("phase3_%s", resourceID)
		mergedResult.MultiResources[key] = []*FetchedResource{fetchedResource}
		
		// Update summary
		mergedResult.Summary.Successful++
	}
	
	// Update summary with Phase 3 statistics
	mergedResult.Summary.TotalRequested += len(traversalResult.DiscoveredResources)
	
	// Add cycle information if available
	if traversalResult.CycleResults != nil && traversalResult.CycleResults.CyclesFound {
		// Add cycle information to Phase2Results
		// This would require extending Phase2Results to include cycle information
		ede.logger.Info("Cycles detected during Phase 3 traversal",
			"cycleCount", traversalResult.CycleResults.TotalCycles)
	}
	
	return &mergedResult
}

// Helper functions

// hasPhase3ResourceRequest checks if any resource request has Phase 3 configuration
func hasPhase3ResourceRequest(req v1beta1.ResourceRequest) bool {
	// Check for Phase 3 indicators in the request
	// This would be implemented based on how Phase 3 configuration is embedded in requests
	// For now, we'll look for specific patterns or metadata
	
	// Check if request has transitive discovery indicators
	if req.Selector != nil && len(req.Selector.Expressions) > 0 {
		for _, expr := range req.Selector.Expressions {
			if expr.Field == "kubecore.io/phase3-enabled" && expr.Operator == v1beta1.ExpressionOpEquals {
				return expr.Value != nil && *expr.Value == "true"
			}
		}
	}
	
	return false
}

// Phase3Config represents Phase 3 configuration extracted from requests
type Phase3Config struct {
	MaxDepth            int
	MaxResources        int
	Timeout             *string
	Direction           string
	ScopeFilter         *Phase3ScopeFilter
	Performance         *Phase3Performance
	ReferenceResolution *Phase3ReferenceResolution
	CycleHandling       *Phase3CycleHandling
	BatchConfig         *Phase3BatchConfig
	CacheConfig         *Phase3CacheConfig
}

// Phase3ScopeFilter represents Phase 3 scope filter configuration
type Phase3ScopeFilter struct {
	PlatformOnly          bool
	CrossNamespaceEnabled bool
	IncludeAPIGroups      []string
	ExcludeAPIGroups      []string
	IncludeKinds          []string
	ExcludeKinds          []string
	IncludeNamespaces     []string
	ExcludeNamespaces     []string
}

// Phase3Performance represents Phase 3 performance configuration
type Phase3Performance struct {
	MaxConcurrentRequests int
	RequestTimeout        *string
	EnableMetrics         bool
	ResourceDeduplication bool
	MemoryLimits          *Phase3MemoryLimits
}

// Phase3MemoryLimits represents Phase 3 memory limits
type Phase3MemoryLimits struct {
	MaxGraphSize int64
	MaxCacheSize int64
	GCThreshold  int64
}

// Phase3ReferenceResolution represents Phase 3 reference resolution configuration
type Phase3ReferenceResolution struct {
	EnableDynamicCRDs       bool
	FollowOwnerReferences   bool
	FollowCustomReferences  bool
	SkipMissingReferences   bool
	MinConfidenceThreshold  float64
	AdditionalPatterns      []Phase3ReferencePattern
}

// Phase3ReferencePattern represents a reference pattern
type Phase3ReferencePattern struct {
	Pattern     string
	TargetKind  string
	TargetGroup string
	Confidence  float64
}

// Phase3CycleHandling represents Phase 3 cycle handling configuration
type Phase3CycleHandling struct {
	DetectionEnabled  bool
	OnCycleDetected   string
	MaxCycles         int
	ReportCycles      bool
}

// Phase3BatchConfig represents Phase 3 batch configuration
type Phase3BatchConfig struct {
	Enabled              bool
	BatchSize            int
	MaxConcurrentBatches int
	SameDepthBatching    bool
	BatchTimeout         *string
}

// Phase3CacheConfig represents Phase 3 cache configuration
type Phase3CacheConfig struct {
	Enabled  bool
	TTL      *string
	MaxSize  int
	Strategy string
}

// extractPhase3Config extracts Phase 3 configuration from a resource request
func extractPhase3Config(req v1beta1.ResourceRequest) *Phase3Config {
	// This is a placeholder implementation
	// In a real implementation, this would extract Phase 3 configuration
	// from the request's selector expressions or metadata
	
	config := &Phase3Config{
		MaxDepth:     3,
		MaxResources: 100,
		Direction:    "forward",
		ScopeFilter: &Phase3ScopeFilter{
			PlatformOnly:          true,
			CrossNamespaceEnabled: false,
			IncludeAPIGroups:      []string{"*.kubecore.io"},
		},
		Performance: &Phase3Performance{
			MaxConcurrentRequests: 10,
			EnableMetrics:         true,
			ResourceDeduplication: true,
		},
		ReferenceResolution: &Phase3ReferenceResolution{
			EnableDynamicCRDs:      true,
			FollowOwnerReferences:  true,
			FollowCustomReferences: true,
			SkipMissingReferences:  true,
			MinConfidenceThreshold: 0.5,
		},
		CycleHandling: &Phase3CycleHandling{
			DetectionEnabled: true,
			OnCycleDetected:  "continue",
			MaxCycles:        10,
			ReportCycles:     true,
		},
		BatchConfig: &Phase3BatchConfig{
			Enabled:              true,
			BatchSize:            10,
			MaxConcurrentBatches: 3,
			SameDepthBatching:    true,
		},
		CacheConfig: &Phase3CacheConfig{
			Enabled:  true,
			MaxSize:  1000,
			Strategy: "lru",
		},
	}
	
	return config
}