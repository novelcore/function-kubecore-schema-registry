package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/discovery/resolver"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// EnhancedEngine implements the Engine interface with Phase 2 capabilities
type EnhancedEngine struct {
	dynamicClient dynamic.Interface
	typedClient   kubernetes.Interface
	registry      registry.Registry
	context       DiscoveryContext

	// Resolvers for different match types
	resolvers map[v1beta1.MatchType]resolver.Resolver

	// Performance tracking
	queryOptimizer *QueryOptimizer
}

// NewEnhancedEngine creates a new enhanced discovery engine with Phase 2 capabilities
func NewEnhancedEngine(config *rest.Config, registry registry.Registry, context DiscoveryContext) (*EnhancedEngine, error) {
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.KubernetesClientError(
			fmt.Sprintf("failed to create dynamic client: %v", err))
	}

	typedClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.KubernetesClientError(
			fmt.Sprintf("failed to create typed client: %v", err))
	}

	engine := &EnhancedEngine{
		dynamicClient:  dynamicClient,
		typedClient:    typedClient,
		registry:       registry,
		context:        context,
		resolvers:      make(map[v1beta1.MatchType]resolver.Resolver),
		queryOptimizer: NewQueryOptimizer(),
	}

	// Register resolvers
	engine.resolvers[v1beta1.MatchTypeDirect] = resolver.NewDirectResolver(dynamicClient, typedClient, registry)
	
	// Only register Phase 2 resolvers if enabled
	if context.Phase2Enabled {
		resolverContext := resolver.DiscoveryContext{
			FunctionNamespace:     context.FunctionNamespace,
			TimeoutPerRequest:     context.TimeoutPerRequest,
			MaxConcurrentRequests: context.MaxConcurrentRequests,
			Phase2Enabled:         context.Phase2Enabled,
		}
		engine.resolvers[v1beta1.MatchTypeLabel] = resolver.NewLabelResolver(dynamicClient, typedClient, registry, resolverContext)
		engine.resolvers[v1beta1.MatchTypeExpression] = resolver.NewExpressionResolver(dynamicClient, typedClient, registry, resolverContext)
	}

	return engine, nil
}

// FetchResources fetches resources based on the provided requests
func (e *EnhancedEngine) FetchResources(requests []v1beta1.ResourceRequest) (*FetchResult, error) {
	startTime := time.Now()

	result := &FetchResult{
		Resources:      make(map[string]*FetchedResource),
		MultiResources: make(map[string][]*FetchedResource),
		Summary: FetchSummary{
			TotalRequested: len(requests),
		},
	}

	// Initialize Phase 2 results if enabled
	if e.context.Phase2Enabled {
		result.Phase2Results = &Phase2Results{
			ConstraintResults: make(map[string]*ConstraintResult),
		}
	}

	// Optimize queries if Phase 2 is enabled
	var optimizedRequests []v1beta1.ResourceRequest
	if e.context.Phase2Enabled {
		planStart := time.Now()
		var err error
		optimizedRequests, result.Phase2Results.QueryPlan, err = e.queryOptimizer.OptimizeQueries(requests)
		if err != nil {
			return nil, functionerrors.QueryOptimizationError(fmt.Sprintf("query optimization failed: %v", err))
		}
		if result.Phase2Results.Performance == nil {
			result.Phase2Results.Performance = &PerformanceMetrics{}
		}
		result.Phase2Results.Performance.QueryPlanningTime = time.Since(planStart)
	} else {
		optimizedRequests = requests
	}

	// Create a semaphore to limit concurrent requests
	sem := make(chan struct{}, e.context.MaxConcurrentRequests)

	// Use errgroup for concurrent processing
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(context.Background())

	// Track performance metrics
	perfStart := time.Now()
	var totalResourcesScanned int

	for _, req := range optimizedRequests {
		req := req // Capture loop variable
		g.Go(func() error {
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Apply timeout per request
			reqCtx, cancel := context.WithTimeout(ctx, e.context.TimeoutPerRequest)
			defer cancel()

			resolverResources, err := e.resolveRequest(reqCtx, req)
			var resources []*FetchedResource
			for _, rr := range resolverResources {
				resources = append(resources, e.convertResolverResource(rr))
			}
			
			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				// Handle error for this request
				errorResource := &FetchedResource{
					Request:   req,
					FetchedAt: time.Now(),
					Metadata: ResourceMetadata{
						FetchStatus: FetchStatusError,
						Error:       err.(*functionerrors.FunctionError),
					},
				}
				
				if req.Optional {
					result.Summary.Skipped++
					result.Resources[req.Into] = errorResource
				} else {
					result.Summary.Failed++
					result.Resources[req.Into] = errorResource
					result.Summary.Errors = append(result.Summary.Errors, &FetchError{
						ResourceRequest: req,
						Error:          err.(*functionerrors.FunctionError),
						Timestamp:      time.Now(),
					})
				}
				return nil // Don't propagate individual fetch errors
			}

			// Process successful results
			if len(resources) == 0 {
				// No resources found
				if req.Optional {
					result.Summary.Skipped++
				} else {
					result.Summary.NotFound++
					result.Summary.Failed++
				}
			} else if len(resources) == 1 {
				// Single resource result (Phase 1 or Phase 2 with single match)
				result.Resources[req.Into] = resources[0]
				result.Summary.Successful++
				totalResourcesScanned++
			} else {
				// Multiple resources result (Phase 2 only)
				result.MultiResources[req.Into] = resources
				// For summary, count each resource
				result.Summary.Successful += len(resources)
				totalResourcesScanned += len(resources)

				// Also set first resource in single resources map for backward compatibility
				if len(resources) > 0 {
					result.Resources[req.Into] = resources[0]
				}
			}

			// Evaluate constraints for Phase 2
			if e.context.Phase2Enabled && req.Strategy != nil {
				constraintResult := e.evaluateConstraints(req, len(resources))
				result.Phase2Results.ConstraintResults[req.Into] = constraintResult
			}

			return nil
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, functionerrors.Wrap(err, "error during concurrent resource fetching")
	}

	// Calculate summary statistics
	result.Summary.TotalDuration = time.Since(startTime)
	if result.Summary.TotalRequested > 0 {
		result.Summary.AverageDuration = result.Summary.TotalDuration / time.Duration(result.Summary.TotalRequested)
	}

	// Update performance metrics for Phase 2
	if e.context.Phase2Enabled && result.Phase2Results.Performance != nil {
		result.Phase2Results.Performance.TotalResourcesScanned = totalResourcesScanned
		result.Phase2Results.Performance.KubernetesAPITime = time.Since(perfStart) - result.Phase2Results.Performance.QueryPlanningTime
	}

	return result, nil
}

// resolveRequest resolves a single request using the appropriate resolver
func (e *EnhancedEngine) resolveRequest(ctx context.Context, req v1beta1.ResourceRequest) ([]*resolver.FetchedResource, error) {
	// Determine match type (default to direct for backward compatibility)
	matchType := req.MatchType
	if matchType == "" {
		matchType = v1beta1.MatchTypeDirect
	}

	// Get appropriate resolver
	resolver, exists := e.resolvers[matchType]
	if !exists {
		return nil, functionerrors.UnsupportedMatchTypeError(string(matchType))
	}

	// Validate that resolver supports the match type
	if !resolver.SupportsMatchType(matchType) {
		return nil, functionerrors.UnsupportedMatchTypeError(
			fmt.Sprintf("resolver does not support match type: %s", matchType))
	}

	// Resolve using the appropriate resolver
	return resolver.Resolve(ctx, req)
}

// evaluateConstraints evaluates strategy constraints and returns results
func (e *EnhancedEngine) evaluateConstraints(req v1beta1.ResourceRequest, actualMatches int) *ConstraintResult {
	result := &ConstraintResult{
		RequestName: req.Into,
		Actual: ConstraintValues{
			ActualMatches: actualMatches,
		},
		Satisfied: true,
	}

	if req.Strategy == nil {
		return result
	}

	// Set expected values
	if req.Strategy.MinMatches != nil {
		result.Expected.MinMatches = req.Strategy.MinMatches
		if actualMatches < *req.Strategy.MinMatches {
			result.Satisfied = false
			result.Message = fmt.Sprintf("Expected minimum %d matches, got %d", *req.Strategy.MinMatches, actualMatches)
		}
	}

	if req.Strategy.MaxMatches != nil {
		result.Expected.MaxMatches = req.Strategy.MaxMatches
		if actualMatches > *req.Strategy.MaxMatches {
			result.Satisfied = false
			if result.Message != "" {
				result.Message += "; "
			}
			result.Message += fmt.Sprintf("Expected maximum %d matches, got %d", *req.Strategy.MaxMatches, actualMatches)
		}
	}

	if result.Satisfied {
		result.Message = "All constraints satisfied"
	}

	return result
}

// QueryOptimizer optimizes discovery queries for better performance
type QueryOptimizer struct {
	// Future: Add query optimization logic
}

// NewQueryOptimizer creates a new query optimizer
func NewQueryOptimizer() *QueryOptimizer {
	return &QueryOptimizer{}
}

// OptimizeQueries optimizes a set of requests for better performance
func (o *QueryOptimizer) OptimizeQueries(requests []v1beta1.ResourceRequest) ([]v1beta1.ResourceRequest, *QueryPlan, error) {
	// For now, return requests as-is
	// Future optimizations could include:
	// - Batching similar requests
	// - Reordering for cache efficiency
	// - Combining label selectors
	
	plan := &QueryPlan{
		TotalQueries:     len(requests),
		BatchedQueries:   0, // No batching implemented yet
		OptimizedQueries: len(requests),
		ExecutionSteps:   []string{"Direct execution (no optimization)"},
	}

	return requests, plan, nil
}

// convertResolverResource converts resolver.FetchedResource to discovery.FetchedResource
func (e *EnhancedEngine) convertResolverResource(rr *resolver.FetchedResource) *FetchedResource {
	fr := &FetchedResource{
		Request:   rr.Request,
		Resource:  rr.Resource,
		FetchedAt: rr.FetchedAt,
		Metadata: ResourceMetadata{
			FetchStatus:    FetchStatus(rr.Metadata.FetchStatus),
			Error:          rr.Metadata.Error,
			FetchDuration:  rr.Metadata.FetchDuration,
			ResourceExists: rr.Metadata.ResourceExists,
			Permissions:    (*PermissionInfo)(rr.Metadata.Permissions),
		},
	}

	// Convert Phase2Metadata if present
	if rr.Metadata.Phase2Metadata != nil {
		fr.Metadata.Phase2Metadata = &Phase2Metadata{
			MatchedBy:        rr.Metadata.Phase2Metadata.MatchedBy,
			SearchNamespaces: rr.Metadata.Phase2Metadata.SearchNamespaces,
			SortPosition:     rr.Metadata.Phase2Metadata.SortPosition,
		}

		// Convert MatchDetails if present
		if rr.Metadata.Phase2Metadata.MatchDetails != nil {
			fr.Metadata.Phase2Metadata.MatchDetails = &MatchDetails{
				MatchedLabels: rr.Metadata.Phase2Metadata.MatchDetails.MatchedLabels,
				MatchScore:    rr.Metadata.Phase2Metadata.MatchDetails.MatchScore,
			}

			// Convert ExpressionMatches
			for _, em := range rr.Metadata.Phase2Metadata.MatchDetails.MatchedExpressions {
				fr.Metadata.Phase2Metadata.MatchDetails.MatchedExpressions = append(
					fr.Metadata.Phase2Metadata.MatchDetails.MatchedExpressions,
					ExpressionMatch{
						Field:         em.Field,
						Operator:      em.Operator,
						ExpectedValue: em.ExpectedValue,
						ActualValue:   em.ActualValue,
						Matched:       em.Matched,
					},
				)
			}
		}
	}

	return fr
}