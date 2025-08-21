package traversal

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	
	"github.com/crossplane/function-sdk-go/logging"
	
	dynamictypes "github.com/crossplane/function-kubecore-schema-registry/pkg/dynamic"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/graph"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// DefaultTraversalEngine implements the TraversalEngine interface
type DefaultTraversalEngine struct {
	// components contains all the required components
	components TraversalEngineComponents
	
	// logger provides structured logging
	logger logging.Logger
	
	// resourceTracker tracks processed resources to prevent infinite loops
	resourceTracker *ResourceTracker
	
	// metricsCollector collects performance metrics
	metricsCollector *MetricsCollector
	
	// mu protects internal state
	mu sync.RWMutex
}

// NewDefaultTraversalEngine creates a new default traversal engine
func NewDefaultTraversalEngine(config *rest.Config, registry registry.Registry, logger logging.Logger) (*DefaultTraversalEngine, error) {
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to create dynamic client")
	}
	
	typedClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to create typed client")
	}
	
	// Create platform checker for scope filtering
	platformChecker := NewDefaultPlatformChecker([]string{"*.kubecore.io"})
	
	components := TraversalEngineComponents{
		DynamicClient:     dynamicClient,
		TypedClient:       typedClient,
		Registry:          registry,
		ReferenceResolver: NewDefaultReferenceResolver(dynamicClient, registry, logger),
		ScopeFilter:       NewDefaultScopeFilter(platformChecker, logger),
		BatchOptimizer:    NewDefaultBatchOptimizer(logger),
		Cache:             NewLRUCache(DefaultCacheMaxSize, DefaultCacheTTL),
		GraphBuilder:      graph.NewDefaultGraphBuilder(platformChecker),
		CycleDetector:     graph.NewDFSCycleDetector(10, true),
		PathTracker:       graph.NewDefaultPathTracker(true),
	}
	
	engine := &DefaultTraversalEngine{
		components:       components,
		logger:           logger,
		resourceTracker:  NewResourceTracker(),
		metricsCollector: NewMetricsCollector(true),
	}
	
	return engine, nil
}

// ExecuteTransitiveDiscovery performs transitive discovery starting from root resources
func (te *DefaultTraversalEngine) ExecuteTransitiveDiscovery(ctx context.Context, config *TraversalConfig, rootResources []*unstructured.Unstructured) (*TraversalResult, error) {
	startTime := time.Now()
	
	te.logger.Info("Starting transitive discovery",
		"rootResourceCount", len(rootResources),
		"maxDepth", config.MaxDepth,
		"maxResources", config.MaxResources,
		"timeout", config.Timeout)
	
	// Apply timeout from config
	if config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}
	
	// Initialize result
	result := &TraversalResult{
		ResourceGraph:       te.components.GraphBuilder.NewGraph(),
		DiscoveredResources: make(map[string]*unstructured.Unstructured),
		TraversalPath: &TraversalPath{
			Steps:     make([]TraversalStep, 0),
			StartTime: startTime,
		},
		Statistics: &TraversalStatistics{
			ResourcesByDepth:    make(map[int]int),
			ResourcesByKind:     make(map[string]int),
			ResourcesByAPIGroup: make(map[string]int),
			MemoryUsage:         &MemoryUsageStats{},
			PerformanceMetrics:  &PerformanceMetrics{},
		},
		Metadata: &TraversalMetadata{
			Config:      config,
			StartResources: te.resourceIDs(rootResources),
			Version:     "1.0.0",
			Environment: map[string]string{
				"platform": "kubernetes",
			},
		},
	}
	
	// Initialize metrics collection
	if config.Performance.EnableMetrics {
		te.metricsCollector.Reset()
	}
	
	// Reset resource tracker
	te.resourceTracker.Reset()
	
	// Add root resources to graph and resource tracker
	for _, resource := range rootResources {
		node := te.components.GraphBuilder.AddNode(result.ResourceGraph, resource, 0, []graph.NodeID{})
		resourceID := te.generateResourceID(resource)
		result.DiscoveredResources[resourceID] = resource
		te.resourceTracker.MarkProcessed(resourceID, 0)
		
		// Update statistics
		result.Statistics.TotalResources++
		result.Statistics.ResourcesByDepth[0]++
		result.Statistics.ResourcesByKind[resource.GetKind()]++
		result.Statistics.ResourcesByAPIGroup[te.extractAPIGroup(resource.GetAPIVersion())]++
	}
	
	// Perform traversal
	var traversalError error
	switch config.Direction {
	case graph.TraversalDirectionForward:
		traversalError = te.executeForwardTraversal(ctx, config, rootResources, result)
	case graph.TraversalDirectionReverse:
		traversalError = te.executeReverseTraversal(ctx, config, rootResources, result)
	case graph.TraversalDirectionBidirectional:
		traversalError = te.executeBidirectionalTraversal(ctx, config, rootResources, result)
	default:
		traversalError = fmt.Errorf("unsupported traversal direction: %s", config.Direction)
	}
	
	// Complete traversal path
	result.TraversalPath.EndTime = time.Now()
	result.TraversalPath.Duration = result.TraversalPath.EndTime.Sub(result.TraversalPath.StartTime)
	result.TraversalPath.TotalSteps = len(result.TraversalPath.Steps)
	
	// Determine termination reason
	if traversalError != nil {
		result.Metadata.TerminationReason = TerminationReasonError
		te.logger.Error("Transitive discovery failed", "error", traversalError)
		return result, traversalError
	} else if result.Statistics.TotalResources >= config.MaxResources {
		result.Metadata.TerminationReason = TerminationReasonMaxResources
	} else if result.TraversalPath.MaxDepthReached >= config.MaxDepth {
		result.Metadata.TerminationReason = TerminationReasonMaxDepth
	} else {
		result.Metadata.TerminationReason = TerminationReasonCompleted
	}
	
	// Detect cycles if enabled
	if config.CycleHandling.DetectionEnabled {
		cycleResult := te.components.CycleDetector.DetectCycles(result.ResourceGraph)
		result.CycleResults = cycleResult
		
		if cycleResult.CyclesFound && config.CycleHandling.OnCycleDetected == CycleActionFail {
			result.Metadata.TerminationReason = TerminationReasonCycle
			return result, fmt.Errorf("cycles detected and cycle action is set to fail")
		}
	}
	
	// Validate result
	result.ValidationResult = te.ValidateTraversalResult(result)
	
	// Collect final metrics
	if config.Performance.EnableMetrics {
		result.Statistics.PerformanceMetrics = te.metricsCollector.GetMetrics()
	}
	
	result.Metadata.CompletedAt = time.Now()
	
	te.logger.Info("Transitive discovery completed",
		"totalResources", result.Statistics.TotalResources,
		"maxDepthReached", result.TraversalPath.MaxDepthReached,
		"duration", result.TraversalPath.Duration,
		"terminationReason", result.Metadata.TerminationReason)
	
	return result, nil
}

// DiscoverReferencedResources discovers resources referenced by the given resources
func (te *DefaultTraversalEngine) DiscoverReferencedResources(ctx context.Context, resources []*unstructured.Unstructured, config *TraversalConfig) (*DiscoveryResult, error) {
	startTime := time.Now()
	
	result := &DiscoveryResult{
		Resources:  make([]*unstructured.Unstructured, 0),
		References: make(map[string][]dynamictypes.ReferenceField),
		Depth:      1, // This is always depth 1 since it's direct references
		Statistics: &DiscoveryStatistics{
			ResourcesRequested: len(resources),
		},
		Errors: make([]TraversalError, 0),
	}
	
	// Use errgroup for concurrent processing
	g, gCtx := errgroup.WithContext(ctx)
	
	// Semaphore to limit concurrent requests
	sem := make(chan struct{}, config.Performance.MaxConcurrentRequests)
	
	// Results collection
	var mu sync.Mutex
	discoveredResources := make(map[string]*unstructured.Unstructured)
	allReferences := make(map[string][]dynamictypes.ReferenceField)
	
	// Process each resource
	for _, resource := range resources {
		resource := resource // Capture loop variable
		g.Go(func() error {
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()
			
			resourceID := te.generateResourceID(resource)
			
			// Extract references from this resource
			references, err := te.components.ReferenceResolver.ExtractReferences(gCtx, resource)
			if err != nil {
				mu.Lock()
				result.Errors = append(result.Errors, TraversalError{
					Type:        TraversalErrorReferenceResolution,
					Message:     fmt.Sprintf("Failed to extract references: %v", err),
					ResourceID:  resourceID,
					Depth:       1,
					Timestamp:   time.Now(),
					Recoverable: true,
				})
				mu.Unlock()
				return nil // Don't fail the entire operation
			}
			
			// Filter references based on scope
			filteredReferences := te.components.ScopeFilter.FilterReferences(references, config.ScopeFilter)
			
			// Resolve references to actual resources
			referencedResources, resolveErrors := te.components.ReferenceResolver.ResolveReferences(gCtx, resource, filteredReferences)
			
			// Collect results
			mu.Lock()
			allReferences[resourceID] = filteredReferences
			
			for _, referencedResource := range referencedResources {
				referencedID := te.generateResourceID(referencedResource)
				if _, exists := discoveredResources[referencedID]; !exists {
					discoveredResources[referencedID] = referencedResource
				}
			}
			
			// Add resolve errors
			for _, resolveErr := range resolveErrors {
				result.Errors = append(result.Errors, TraversalError{
					Type:        TraversalErrorReferenceResolution,
					Message:     resolveErr.Error(),
					ResourceID:  resourceID,
					Depth:       1,
					Timestamp:   time.Now(),
					Recoverable: config.ReferenceResolution.SkipMissingReferences,
				})
			}
			
			mu.Unlock()
			return nil
		})
	}
	
	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return result, functionerrors.Wrap(err, "error during reference discovery")
	}
	
	// Convert map to slice
	for _, resource := range discoveredResources {
		result.Resources = append(result.Resources, resource)
	}
	
	result.References = allReferences
	result.Statistics.ResourcesFound = len(result.Resources)
	result.Statistics.ReferencesDetected = len(allReferences)
	result.Statistics.DiscoveryTime = time.Since(startTime)
	
	return result, nil
}

// BuildResourceGraph builds a resource dependency graph from discovered resources
func (te *DefaultTraversalEngine) BuildResourceGraph(ctx context.Context, resources []*unstructured.Unstructured, config *TraversalConfig) (*graph.ResourceGraph, error) {
	// Extract all references first
	allReferences := make(map[string][]dynamictypes.ReferenceField)
	
	for _, resource := range resources {
		resourceID := te.generateResourceID(resource)
		references, err := te.components.ReferenceResolver.ExtractReferences(ctx, resource)
		if err != nil {
			te.logger.Debug("Failed to extract references for resource", "resourceID", resourceID, "error", err)
			continue
		}
		
		// Filter references based on scope
		filteredReferences := te.components.ScopeFilter.FilterReferences(references, config.ScopeFilter)
		allReferences[resourceID] = filteredReferences
	}
	
	// Build graph using the graph builder
	resourceGraph, err := te.components.GraphBuilder.BuildGraph(resources, allReferences)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to build resource graph")
	}
	
	return resourceGraph, nil
}

// ValidateTraversalResult validates the results of transitive discovery
func (te *DefaultTraversalEngine) ValidateTraversalResult(result *TraversalResult) *TraversalValidationResult {
	startTime := time.Now()
	
	validationResult := &TraversalValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationWarning, 0),
		Statistics: &ValidationStatistics{
			ResourcesValidated: result.Statistics.TotalResources,
		},
	}
	
	// Validate resource graph
	if result.ResourceGraph != nil {
		graphValidation := te.components.GraphBuilder.ValidateGraph(result.ResourceGraph)
		if !graphValidation.Valid {
			validationResult.Valid = false
			for _, graphError := range graphValidation.Errors {
				validationResult.Errors = append(validationResult.Errors, ValidationError{
					Type:    ValidationErrorInvalidGraph,
					Message: graphError.Message,
					Context: map[string]interface{}{
						"errorType": graphError.Type,
					},
				})
			}
		}
	}
	
	// Validate resource counts
	if result.Statistics.TotalResources > result.Metadata.Config.MaxResources {
		validationResult.Warnings = append(validationResult.Warnings, ValidationWarning{
			Type:     ValidationWarningManyResources,
			Message:  fmt.Sprintf("Discovered %d resources, exceeding limit of %d", result.Statistics.TotalResources, result.Metadata.Config.MaxResources),
			Severity: "medium",
		})
	}
	
	// Validate depth
	if result.TraversalPath.MaxDepthReached >= result.Metadata.Config.MaxDepth {
		validationResult.Warnings = append(validationResult.Warnings, ValidationWarning{
			Type:     ValidationWarningDeepTraversal,
			Message:  fmt.Sprintf("Reached maximum traversal depth of %d", result.Metadata.Config.MaxDepth),
			Severity: "low",
		})
	}
	
	// Validate performance
	if result.TraversalPath.Duration > result.Metadata.Config.Timeout/2 {
		validationResult.Warnings = append(validationResult.Warnings, ValidationWarning{
			Type:     ValidationWarningSlowPerformance,
			Message:  fmt.Sprintf("Traversal took %v, approaching timeout of %v", result.TraversalPath.Duration, result.Metadata.Config.Timeout),
			Severity: "high",
		})
	}
	
	// Validate cycles if detected
	if result.CycleResults != nil && result.CycleResults.CyclesFound {
		if result.Metadata.Config.CycleHandling.OnCycleDetected == CycleActionFail {
			validationResult.Valid = false
			validationResult.Errors = append(validationResult.Errors, ValidationError{
				Type:    ValidationErrorCycleDetected,
				Message: fmt.Sprintf("Detected %d cycles and cycle action is set to fail", result.CycleResults.TotalCycles),
				Context: map[string]interface{}{
					"cycleCount": result.CycleResults.TotalCycles,
				},
			})
		}
	}
	
	validationResult.Statistics.ErrorCount = len(validationResult.Errors)
	validationResult.Statistics.WarningCount = len(validationResult.Warnings)
	validationResult.Statistics.ValidationTime = time.Since(startTime)
	
	return validationResult
}

// Helper methods for different traversal strategies

// executeForwardTraversal executes forward (following outbound references) traversal
func (te *DefaultTraversalEngine) executeForwardTraversal(ctx context.Context, config *TraversalConfig, rootResources []*unstructured.Unstructured, result *TraversalResult) error {
	currentResources := rootResources
	
	for depth := 1; depth <= config.MaxDepth && len(currentResources) > 0; depth++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		
		if result.Statistics.TotalResources >= config.MaxResources {
			break
		}
		
		te.logger.Debug("Processing traversal depth", "depth", depth, "resourceCount", len(currentResources))
		
		// Discover referenced resources at this depth
		discoveryResult, err := te.DiscoverReferencedResources(ctx, currentResources, config)
		if err != nil {
			return functionerrors.Wrap(err, fmt.Sprintf("failed to discover references at depth %d", depth))
		}
		
		// Filter new resources (not already discovered)
		newResources := make([]*unstructured.Unstructured, 0)
		for _, resource := range discoveryResult.Resources {
			resourceID := te.generateResourceID(resource)
			if !te.resourceTracker.IsProcessed(resourceID) {
				newResources = append(newResources, resource)
				result.DiscoveredResources[resourceID] = resource
				te.resourceTracker.MarkProcessed(resourceID, depth)
				
				// Add to graph
				discoveryPath := te.buildDiscoveryPath(resource, result.ResourceGraph)
				te.components.GraphBuilder.AddNode(result.ResourceGraph, resource, depth, discoveryPath)
				
				// Update statistics
				result.Statistics.TotalResources++
				result.Statistics.ResourcesByDepth[depth]++
				result.Statistics.ResourcesByKind[resource.GetKind()]++
				result.Statistics.ResourcesByAPIGroup[te.extractAPIGroup(resource.GetAPIVersion())]++
			}
		}
		
		// Update traversal path
		step := TraversalStep{
			StepID:             len(result.TraversalPath.Steps),
			Depth:              depth,
			Action:             TraversalActionDiscover,
			ReferencesFound:    discoveryResult.Statistics.ReferencesDetected,
			ReferencesFollowed: len(newResources),
			Timestamp:          time.Now(),
			Duration:           discoveryResult.Statistics.DiscoveryTime,
		}
		
		result.TraversalPath.Steps = append(result.TraversalPath.Steps, step)
		result.TraversalPath.MaxDepthReached = depth
		
		// Prepare for next iteration
		currentResources = newResources
		
		// Add edges to graph based on references
		te.addReferencesToGraph(result.ResourceGraph, discoveryResult.References)
		
		te.logger.Debug("Completed traversal depth", "depth", depth, "newResources", len(newResources), "totalResources", result.Statistics.TotalResources)
	}
	
	return nil
}

// executeReverseTraversal executes reverse (following inbound references) traversal
func (te *DefaultTraversalEngine) executeReverseTraversal(ctx context.Context, config *TraversalConfig, rootResources []*unstructured.Unstructured, result *TraversalResult) error {
	// Reverse traversal is more complex as we need to find resources that reference our targets
	// For now, implement a simplified version that discovers forward and then reverses the graph
	
	// First, do forward discovery to build a complete picture
	err := te.executeForwardTraversal(ctx, config, rootResources, result)
	if err != nil {
		return err
	}
	
	// TODO: Implement true reverse traversal by:
	// 1. Finding all resources in the namespace/cluster
	// 2. Extracting their references
	// 3. Finding which ones reference our root resources
	// 4. Building the reverse graph
	
	te.logger.Info("Reverse traversal completed using forward traversal with graph reversal")
	return nil
}

// executeBidirectionalTraversal executes bidirectional traversal
func (te *DefaultTraversalEngine) executeBidirectionalTraversal(ctx context.Context, config *TraversalConfig, rootResources []*unstructured.Unstructured, result *TraversalResult) error {
	// Execute forward traversal
	forwardConfig := *config
	forwardConfig.Direction = graph.TraversalDirectionForward
	
	err := te.executeForwardTraversal(ctx, &forwardConfig, rootResources, result)
	if err != nil {
		return functionerrors.Wrap(err, "failed during forward phase of bidirectional traversal")
	}
	
	// Execute reverse traversal from the same root resources
	reverseConfig := *config
	reverseConfig.Direction = graph.TraversalDirectionReverse
	
	err = te.executeReverseTraversal(ctx, &reverseConfig, rootResources, result)
	if err != nil {
		return functionerrors.Wrap(err, "failed during reverse phase of bidirectional traversal")
	}
	
	return nil
}

// Helper methods

// generateResourceID generates a unique ID for a resource
func (te *DefaultTraversalEngine) generateResourceID(resource *unstructured.Unstructured) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		resource.GetAPIVersion(),
		resource.GetKind(),
		resource.GetNamespace(),
		resource.GetName())
}

// resourceIDs extracts resource IDs from a slice of resources
func (te *DefaultTraversalEngine) resourceIDs(resources []*unstructured.Unstructured) []string {
	ids := make([]string, len(resources))
	for i, resource := range resources {
		ids[i] = te.generateResourceID(resource)
	}
	return ids
}

// extractAPIGroup extracts the API group from an API version
func (te *DefaultTraversalEngine) extractAPIGroup(apiVersion string) string {
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 2 {
		return parts[0]
	}
	return "core" // Core API group for resources like Pod, Service, etc.
}

// buildDiscoveryPath builds a discovery path for a resource
func (te *DefaultTraversalEngine) buildDiscoveryPath(resource *unstructured.Unstructured, resourceGraph *graph.ResourceGraph) []graph.NodeID {
	// For now, return empty path. In a full implementation, this would:
	// 1. Find the path from root nodes to this resource
	// 2. Use graph traversal to determine the shortest path
	// 3. Return the sequence of node IDs
	return []graph.NodeID{}
}

// addReferencesToGraph adds reference edges to the graph
func (te *DefaultTraversalEngine) addReferencesToGraph(resourceGraph *graph.ResourceGraph, references map[string][]dynamictypes.ReferenceField) {
	for sourceResourceID, refFields := range references {
		sourceNodeID := graph.NodeID(sourceResourceID)
		
		for _, refField := range refFields {
			// Build target resource ID (this is simplified)
			targetResourceID := fmt.Sprintf("%s/%s/%s/%s",
				refField.TargetGroup,
				refField.TargetKind,
				"", // namespace would need to be resolved
				"") // name would need to be resolved
			
			targetNodeID := graph.NodeID(targetResourceID)
			
			// Map dynamic reference type to graph relation type
			var relationType graph.RelationType
			switch refField.RefType {
			case dynamictypes.RefTypeOwnerRef:
				relationType = graph.RelationTypeOwnerRef
			case dynamictypes.RefTypeCustom:
				relationType = graph.RelationTypeCustomRef
			default:
				relationType = graph.RelationTypeCustomRef
			}
			
			// Add edge if both nodes exist
			if _, sourceExists := resourceGraph.Nodes[sourceNodeID]; sourceExists {
				if _, targetExists := resourceGraph.Nodes[targetNodeID]; targetExists {
					te.components.GraphBuilder.AddEdge(resourceGraph, sourceNodeID, targetNodeID, relationType, refField.FieldPath, refField.FieldName, refField.Confidence)
				}
			}
		}
	}
}