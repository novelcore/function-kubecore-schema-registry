package discovery

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-sdk-go/logging"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
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

	// traversalConfig contains Phase 3 traversal configuration
	traversalConfig *v1beta1.TraversalConfig
}

// NewEnhancedDiscoveryEngine creates a new enhanced discovery engine with Phase 3 capabilities
func NewEnhancedDiscoveryEngine(config *rest.Config, registry registry.Registry, context DiscoveryContext, traversalConfig *v1beta1.TraversalConfig, logger logging.Logger) (*EnhancedDiscoveryEngine, error) {
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
		traversalConfig: traversalConfig,
	}, nil
}

// FetchResources fetches resources using Phase 1, 2, or 3 based on configuration
func (ede *EnhancedDiscoveryEngine) FetchResources(requests []v1beta1.ResourceRequest) (*FetchResult, error) {
	// Check if Phase 3 configuration is provided and enabled
	hasPhase3Config := ede.traversalConfig != nil && ede.traversalConfig.Enabled

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

	// Step 3: Build traversal configuration from input
	traversalConfig := ede.buildTraversalConfigFromInput()

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

// buildTraversalConfigFromInput builds traversal configuration from the input TraversalConfig
func (ede *EnhancedDiscoveryEngine) buildTraversalConfigFromInput() *traversal.TraversalConfig {
	// Start with default configuration
	config := traversal.NewDefaultTraversalConfig()

	// Apply input traversal configuration
	if ede.traversalConfig != nil {
		ede.applyInputTraversalConfig(config, ede.traversalConfig)
	}

	// Apply discovery context settings
	config.Performance.MaxConcurrentRequests = ede.config.MaxConcurrentRequests

	// Set timeout from context
	if ede.config.TimeoutPerRequest > 0 {
		config.Timeout = ede.config.TimeoutPerRequest * 5 // Allow 5x per-request timeout for full traversal
	}

	return config
}

// applyInputTraversalConfig applies input traversal configuration to the traversal config
func (ede *EnhancedDiscoveryEngine) applyInputTraversalConfig(config *traversal.TraversalConfig, inputConfig *v1beta1.TraversalConfig) {
	// Apply basic settings
	if inputConfig.MaxDepth > 0 {
		config.MaxDepth = inputConfig.MaxDepth
	}

	if inputConfig.MaxResources > 0 {
		config.MaxResources = inputConfig.MaxResources
	}

	if inputConfig.Timeout != nil {
		if timeout, err := time.ParseDuration(*inputConfig.Timeout); err == nil {
			config.Timeout = timeout
		}
	}

	if inputConfig.Direction != "" {
		switch inputConfig.Direction {
		case v1beta1.TraversalDirectionForward:
			config.Direction = graph.TraversalDirectionForward
		case v1beta1.TraversalDirectionReverse:
			config.Direction = graph.TraversalDirectionReverse
		case v1beta1.TraversalDirectionBidirectional:
			config.Direction = graph.TraversalDirectionBidirectional
		}
	}

	// Apply scope filter configuration
	if inputConfig.ScopeFilter != nil {
		config.ScopeFilter.PlatformOnly = inputConfig.ScopeFilter.PlatformOnly
		config.ScopeFilter.CrossNamespaceEnabled = inputConfig.ScopeFilter.CrossNamespaceEnabled

		if len(inputConfig.ScopeFilter.IncludeAPIGroups) > 0 {
			config.ScopeFilter.IncludeAPIGroups = inputConfig.ScopeFilter.IncludeAPIGroups
		}

		if len(inputConfig.ScopeFilter.ExcludeAPIGroups) > 0 {
			config.ScopeFilter.ExcludeAPIGroups = inputConfig.ScopeFilter.ExcludeAPIGroups
		}

		if len(inputConfig.ScopeFilter.IncludeKinds) > 0 {
			config.ScopeFilter.IncludeKinds = inputConfig.ScopeFilter.IncludeKinds
		}

		if len(inputConfig.ScopeFilter.ExcludeKinds) > 0 {
			config.ScopeFilter.ExcludeKinds = inputConfig.ScopeFilter.ExcludeKinds
		}

		if len(inputConfig.ScopeFilter.IncludeNamespaces) > 0 {
			config.ScopeFilter.IncludeNamespaces = inputConfig.ScopeFilter.IncludeNamespaces
		}

		if len(inputConfig.ScopeFilter.ExcludeNamespaces) > 0 {
			config.ScopeFilter.ExcludeNamespaces = inputConfig.ScopeFilter.ExcludeNamespaces
		}
	}

	// Apply performance configuration
	if inputConfig.Performance != nil {
		if inputConfig.Performance.MaxConcurrentRequests > 0 {
			config.Performance.MaxConcurrentRequests = inputConfig.Performance.MaxConcurrentRequests
		}

		if inputConfig.Performance.RequestTimeout != nil {
			if timeout, err := time.ParseDuration(*inputConfig.Performance.RequestTimeout); err == nil {
				config.Performance.RequestTimeout = timeout
			}
		}

		config.Performance.EnableMetrics = inputConfig.Performance.EnableMetrics
		config.Performance.ResourceDeduplication = inputConfig.Performance.ResourceDeduplication

		if inputConfig.Performance.MemoryLimits != nil {
			if config.Performance.MemoryLimits == nil {
				config.Performance.MemoryLimits = &traversal.MemoryLimits{}
			}

			config.Performance.MemoryLimits.MaxGraphSize = inputConfig.Performance.MemoryLimits.MaxGraphSize
			config.Performance.MemoryLimits.MaxCacheSize = inputConfig.Performance.MemoryLimits.MaxCacheSize
			config.Performance.MemoryLimits.GCThreshold = inputConfig.Performance.MemoryLimits.GCThreshold
		}
	}

	// Apply reference resolution configuration
	if inputConfig.ReferenceResolution != nil {
		config.ReferenceResolution.EnableDynamicCRDs = inputConfig.ReferenceResolution.EnableDynamicCRDs
		config.ReferenceResolution.FollowOwnerReferences = inputConfig.ReferenceResolution.FollowOwnerReferences
		config.ReferenceResolution.FollowCustomReferences = inputConfig.ReferenceResolution.FollowCustomReferences
		config.ReferenceResolution.SkipMissingReferences = inputConfig.ReferenceResolution.SkipMissingReferences
		config.ReferenceResolution.MinConfidenceThreshold = inputConfig.ReferenceResolution.MinConfidenceThreshold

		// Convert additional patterns
		for _, pattern := range inputConfig.ReferenceResolution.AdditionalPatterns {
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
	if inputConfig.CycleHandling != nil {
		config.CycleHandling.DetectionEnabled = inputConfig.CycleHandling.DetectionEnabled
		config.CycleHandling.MaxCycles = inputConfig.CycleHandling.MaxCycles
		config.CycleHandling.ReportCycles = inputConfig.CycleHandling.ReportCycles

		switch inputConfig.CycleHandling.OnCycleDetected {
		case v1beta1.CycleActionContinue:
			config.CycleHandling.OnCycleDetected = traversal.CycleActionContinue
		case v1beta1.CycleActionStop:
			config.CycleHandling.OnCycleDetected = traversal.CycleActionStop
		case v1beta1.CycleActionFail:
			config.CycleHandling.OnCycleDetected = traversal.CycleActionFail
		}
	}

	// Apply batch configuration
	if inputConfig.BatchConfig != nil {
		config.BatchConfig.Enabled = inputConfig.BatchConfig.Enabled
		config.BatchConfig.BatchSize = inputConfig.BatchConfig.BatchSize
		config.BatchConfig.MaxConcurrentBatches = inputConfig.BatchConfig.MaxConcurrentBatches
		config.BatchConfig.SameDepthBatching = inputConfig.BatchConfig.SameDepthBatching

		if inputConfig.BatchConfig.BatchTimeout != nil {
			if timeout, err := time.ParseDuration(*inputConfig.BatchConfig.BatchTimeout); err == nil {
				config.BatchConfig.BatchTimeout = timeout
			}
		}
	}

	// Apply cache configuration
	if inputConfig.CacheConfig != nil {
		config.CacheConfig.Enabled = inputConfig.CacheConfig.Enabled
		config.CacheConfig.MaxSize = inputConfig.CacheConfig.MaxSize

		if inputConfig.CacheConfig.TTL != nil {
			if ttl, err := time.ParseDuration(*inputConfig.CacheConfig.TTL); err == nil {
				config.CacheConfig.TTL = ttl
			}
		}

		switch inputConfig.CacheConfig.Strategy {
		case v1beta1.CacheStrategyLRU:
			config.CacheConfig.CacheStrategy = traversal.CacheStrategyLRU
		case v1beta1.CacheStrategyLFU:
			config.CacheConfig.CacheStrategy = traversal.CacheStrategyLFU
		case v1beta1.CacheStrategyTTL:
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

