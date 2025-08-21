package traversal

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/function-sdk-go/logging"
)

// BatchOptimizer optimizes batch processing of resources during traversal
type BatchOptimizer interface {
	// OptimizeBatches optimizes resource processing by batching related operations
	OptimizeBatches(ctx context.Context, resources []*unstructured.Unstructured, config *BatchConfig) ([]ResourceBatch, error)

	// ProcessBatch processes a single batch of resources
	ProcessBatch(ctx context.Context, batch ResourceBatch, processor BatchProcessor) (*BatchResult, error)

	// ProcessBatches processes multiple batches concurrently
	ProcessBatches(ctx context.Context, batches []ResourceBatch, processor BatchProcessor) ([]*BatchResult, error)

	// GetOptimizationStatistics returns statistics about batch optimization
	GetOptimizationStatistics() *BatchOptimizationStats
}

// BatchProcessor processes a batch of resources
type BatchProcessor interface {
	// ProcessResource processes a single resource
	ProcessResource(ctx context.Context, resource *unstructured.Unstructured) (*ResourceProcessingResult, error)

	// ProcessBatch processes an entire batch (allows for batch-specific optimizations)
	ProcessBatch(ctx context.Context, resources []*unstructured.Unstructured) ([]*ResourceProcessingResult, error)

	// GetProcessorName returns the name of the processor
	GetProcessorName() string
}

// ResourceBatch represents a batch of resources to be processed together
type ResourceBatch struct {
	// ID is a unique identifier for this batch
	ID string

	// Resources contains the resources in this batch
	Resources []*unstructured.Unstructured

	// BatchType indicates the type of batching used
	BatchType BatchType

	// Priority indicates the processing priority of this batch
	Priority int

	// Depth is the traversal depth of resources in this batch
	Depth int

	// Metadata contains additional batch information
	Metadata *BatchMetadata

	// CreatedAt indicates when the batch was created
	CreatedAt time.Time
}

// BatchType represents different batching strategies
type BatchType string

const (
	// BatchTypeSize groups resources by batch size
	BatchTypeSize BatchType = "size"
	// BatchTypeDepth groups resources by traversal depth
	BatchTypeDepth BatchType = "depth"
	// BatchTypeAPIGroup groups resources by API group
	BatchTypeAPIGroup BatchType = "api_group"
	// BatchTypeKind groups resources by Kubernetes kind
	BatchTypeKind BatchType = "kind"
	// BatchTypeNamespace groups resources by namespace
	BatchTypeNamespace BatchType = "namespace"
	// BatchTypeMixed uses multiple batching criteria
	BatchTypeMixed BatchType = "mixed"
)

// BatchMetadata contains metadata about a batch
type BatchMetadata struct {
	// APIGroups contains unique API groups in the batch
	APIGroups []string

	// Kinds contains unique kinds in the batch
	Kinds []string

	// Namespaces contains unique namespaces in the batch
	Namespaces []string

	// EstimatedProcessingTime is the estimated time to process this batch
	EstimatedProcessingTime time.Duration

	// Dependencies contains other batches this batch depends on
	Dependencies []string

	// OptimizationHints contains hints for processing optimization
	OptimizationHints map[string]interface{}
}

// BatchResult contains the result of processing a batch
type BatchResult struct {
	// BatchID is the ID of the processed batch
	BatchID string

	// Results contains the processing results for each resource
	Results []*ResourceProcessingResult

	// ProcessingTime is the total time taken to process the batch
	ProcessingTime time.Duration

	// Success indicates if the batch was processed successfully
	Success bool

	// Errors contains any errors that occurred during processing
	Errors []error

	// Statistics contains processing statistics
	Statistics *BatchProcessingStats

	// CompletedAt indicates when batch processing completed
	CompletedAt time.Time
}

// ResourceProcessingResult contains the result of processing a single resource
type ResourceProcessingResult struct {
	// ResourceID identifies the processed resource
	ResourceID string

	// ProcessedResource is the processed resource (may be modified)
	ProcessedResource *unstructured.Unstructured

	// ProcessingTime is the time taken to process this resource
	ProcessingTime time.Duration

	// Success indicates if the resource was processed successfully
	Success bool

	// Error contains any error that occurred during processing
	Error error

	// Metadata contains additional processing metadata
	Metadata map[string]interface{}
}

// BatchOptimizationStats contains statistics about batch optimization
type BatchOptimizationStats struct {
	// TotalResources is the total number of resources processed
	TotalResources int

	// TotalBatches is the total number of batches created
	TotalBatches int

	// AverageBatchSize is the average size of batches
	AverageBatchSize float64

	// OptimizationTime is the time taken for batch optimization
	OptimizationTime time.Duration

	// ProcessingTime is the total time taken for batch processing
	ProcessingTime time.Duration

	// ConcurrencyUtilization is the average concurrency utilization
	ConcurrencyUtilization float64

	// BatchTypes contains counts of each batch type used
	BatchTypes map[BatchType]int

	// DepthDistribution shows the distribution of resources by depth
	DepthDistribution map[int]int
}

// BatchProcessingStats contains statistics about batch processing
type BatchProcessingStats struct {
	// ResourcesProcessed is the number of resources processed
	ResourcesProcessed int

	// ResourcesSucceeded is the number of resources processed successfully
	ResourcesSucceeded int

	// ResourcesFailed is the number of resources that failed processing
	ResourcesFailed int

	// AverageProcessingTime is the average time per resource
	AverageProcessingTime time.Duration

	// ThroughputPerSecond is the processing throughput
	ThroughputPerSecond float64
}

// DefaultBatchOptimizer implements BatchOptimizer interface
type DefaultBatchOptimizer struct {
	// logger provides structured logging
	logger logging.Logger

	// stats tracks optimization statistics
	stats *BatchOptimizationStats

	// mu protects access to statistics
	mu sync.RWMutex
}

// NewDefaultBatchOptimizer creates a new default batch optimizer
func NewDefaultBatchOptimizer(logger logging.Logger) *DefaultBatchOptimizer {
	return &DefaultBatchOptimizer{
		logger: logger,
		stats: &BatchOptimizationStats{
			BatchTypes:        make(map[BatchType]int),
			DepthDistribution: make(map[int]int),
		},
	}
}

// OptimizeBatches optimizes resource processing by batching related operations
func (bo *DefaultBatchOptimizer) OptimizeBatches(ctx context.Context, resources []*unstructured.Unstructured, config *BatchConfig) ([]ResourceBatch, error) {
	startTime := time.Now()

	bo.logger.Debug("Starting batch optimization",
		"totalResources", len(resources),
		"batchSize", config.BatchSize,
		"sameDepthBatching", config.SameDepthBatching)

	var batches []ResourceBatch

	if !config.Enabled || len(resources) <= config.BatchSize {
		// Create single batch if batching is disabled or resources fit in one batch
		batch := ResourceBatch{
			ID:        "batch-0",
			Resources: resources,
			BatchType: BatchTypeSize,
			Priority:  1,
			Depth:     bo.getAverageDepth(resources),
			Metadata:  bo.createBatchMetadata(resources),
			CreatedAt: time.Now(),
		}
		batches = append(batches, batch)
		bo.stats.BatchTypes[BatchTypeSize]++
	} else {
		// Create optimized batches
		if config.SameDepthBatching {
			batches = bo.batchByDepth(resources, config)
		} else {
			batches = bo.batchBySize(resources, config)
		}
	}

	// Update statistics
	bo.mu.Lock()
	bo.stats.TotalResources += len(resources)
	bo.stats.TotalBatches += len(batches)
	bo.stats.OptimizationTime += time.Since(startTime)

	if bo.stats.TotalBatches > 0 {
		bo.stats.AverageBatchSize = float64(bo.stats.TotalResources) / float64(bo.stats.TotalBatches)
	}

	// Update depth distribution
	for _, resource := range resources {
		depth := bo.getResourceDepth(resource)
		bo.stats.DepthDistribution[depth]++
	}
	bo.mu.Unlock()

	bo.logger.Debug("Batch optimization completed",
		"totalBatches", len(batches),
		"averageBatchSize", float64(len(resources))/float64(len(batches)),
		"optimizationTime", time.Since(startTime))

	return batches, nil
}

// ProcessBatch processes a single batch of resources
func (bo *DefaultBatchOptimizer) ProcessBatch(ctx context.Context, batch ResourceBatch, processor BatchProcessor) (*BatchResult, error) {
	startTime := time.Now()

	bo.logger.Debug("Processing batch",
		"batchID", batch.ID,
		"resourceCount", len(batch.Resources),
		"batchType", batch.BatchType,
		"processor", processor.GetProcessorName())

	result := &BatchResult{
		BatchID:     batch.ID,
		Results:     make([]*ResourceProcessingResult, 0, len(batch.Resources)),
		Success:     true,
		Errors:      make([]error, 0),
		Statistics:  &BatchProcessingStats{},
		CompletedAt: time.Now(),
	}

	// Try batch processing first
	batchResults, err := processor.ProcessBatch(ctx, batch.Resources)
	if err == nil && len(batchResults) == len(batch.Resources) {
		// Batch processing succeeded
		result.Results = batchResults

		// Calculate statistics
		for _, res := range batchResults {
			result.Statistics.ResourcesProcessed++
			if res.Success {
				result.Statistics.ResourcesSucceeded++
			} else {
				result.Statistics.ResourcesFailed++
				if res.Error != nil {
					result.Errors = append(result.Errors, res.Error)
				}
			}
		}
	} else {
		// Fall back to individual resource processing
		bo.logger.Debug("Batch processing failed, falling back to individual processing", "error", err)

		for _, resource := range batch.Resources {
			resourceResult, err := processor.ProcessResource(ctx, resource)
			if err != nil {
				result.Success = false
				result.Errors = append(result.Errors, err)

				// Create error result
				resourceResult = &ResourceProcessingResult{
					ResourceID:        bo.generateResourceID(resource),
					ProcessedResource: resource,
					Success:           false,
					Error:             err,
				}
			}

			result.Results = append(result.Results, resourceResult)
			result.Statistics.ResourcesProcessed++

			if resourceResult.Success {
				result.Statistics.ResourcesSucceeded++
			} else {
				result.Statistics.ResourcesFailed++
			}
		}
	}

	// Calculate final statistics
	result.ProcessingTime = time.Since(startTime)

	if result.Statistics.ResourcesProcessed > 0 {
		avgTime := result.ProcessingTime / time.Duration(result.Statistics.ResourcesProcessed)
		result.Statistics.AverageProcessingTime = avgTime

		if result.ProcessingTime > 0 {
			result.Statistics.ThroughputPerSecond = float64(result.Statistics.ResourcesProcessed) / result.ProcessingTime.Seconds()
		}
	}

	if len(result.Errors) > 0 {
		result.Success = false
	}

	bo.logger.Debug("Batch processing completed",
		"batchID", batch.ID,
		"resourcesProcessed", result.Statistics.ResourcesProcessed,
		"resourcesSucceeded", result.Statistics.ResourcesSucceeded,
		"resourcesFailed", result.Statistics.ResourcesFailed,
		"processingTime", result.ProcessingTime,
		"success", result.Success)

	return result, nil
}

// ProcessBatches processes multiple batches concurrently
func (bo *DefaultBatchOptimizer) ProcessBatches(ctx context.Context, batches []ResourceBatch, processor BatchProcessor) ([]*BatchResult, error) {
	startTime := time.Now()

	bo.logger.Debug("Starting concurrent batch processing",
		"totalBatches", len(batches),
		"processor", processor.GetProcessorName())

	// Sort batches by priority and depth
	sortedBatches := make([]ResourceBatch, len(batches))
	copy(sortedBatches, batches)
	sort.Slice(sortedBatches, func(i, j int) bool {
		if sortedBatches[i].Priority != sortedBatches[j].Priority {
			return sortedBatches[i].Priority > sortedBatches[j].Priority // Higher priority first
		}
		return sortedBatches[i].Depth < sortedBatches[j].Depth // Lower depth first
	})

	// Process batches with limited concurrency
	results := make([]*BatchResult, len(batches))
	g, gCtx := errgroup.WithContext(ctx)

	// Semaphore to limit concurrent batch processing
	maxConcurrency := 3 // Default to 3 concurrent batches
	if len(batches) < maxConcurrency {
		maxConcurrency = len(batches)
	}
	sem := make(chan struct{}, maxConcurrency)

	for i, batch := range sortedBatches {
		i, batch := i, batch // Capture loop variables
		g.Go(func() error {
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := bo.ProcessBatch(gCtx, batch, processor)
			if err != nil {
				return fmt.Errorf("failed to process batch %s: %w", batch.ID, err)
			}

			results[i] = result
			return nil
		})
	}

	// Wait for all batches to complete
	if err := g.Wait(); err != nil {
		return results, err
	}

	// Update statistics
	bo.mu.Lock()
	processingTime := time.Since(startTime)
	bo.stats.ProcessingTime += processingTime

	totalResources := 0
	for _, batch := range batches {
		totalResources += len(batch.Resources)
	}

	if processingTime > 0 && maxConcurrency > 0 {
		// Calculate concurrency utilization (simplified)
		bo.stats.ConcurrencyUtilization = float64(totalResources) / (processingTime.Seconds() * float64(maxConcurrency))
	}
	bo.mu.Unlock()

	bo.logger.Debug("Concurrent batch processing completed",
		"totalBatches", len(batches),
		"totalResources", totalResources,
		"processingTime", processingTime,
		"concurrency", maxConcurrency)

	return results, nil
}

// GetOptimizationStatistics returns statistics about batch optimization
func (bo *DefaultBatchOptimizer) GetOptimizationStatistics() *BatchOptimizationStats {
	bo.mu.RLock()
	defer bo.mu.RUnlock()

	// Return a copy to prevent concurrent modification
	statsCopy := *bo.stats
	statsCopy.BatchTypes = make(map[BatchType]int)
	statsCopy.DepthDistribution = make(map[int]int)

	for k, v := range bo.stats.BatchTypes {
		statsCopy.BatchTypes[k] = v
	}

	for k, v := range bo.stats.DepthDistribution {
		statsCopy.DepthDistribution[k] = v
	}

	return &statsCopy
}

// Helper methods

// batchBySize creates batches based on size limits
func (bo *DefaultBatchOptimizer) batchBySize(resources []*unstructured.Unstructured, config *BatchConfig) []ResourceBatch {
	var batches []ResourceBatch

	for i := 0; i < len(resources); i += config.BatchSize {
		end := i + config.BatchSize
		if end > len(resources) {
			end = len(resources)
		}

		batchResources := resources[i:end]
		batch := ResourceBatch{
			ID:        fmt.Sprintf("size-batch-%d", len(batches)),
			Resources: batchResources,
			BatchType: BatchTypeSize,
			Priority:  1,
			Depth:     bo.getAverageDepth(batchResources),
			Metadata:  bo.createBatchMetadata(batchResources),
			CreatedAt: time.Now(),
		}

		batches = append(batches, batch)
		bo.stats.BatchTypes[BatchTypeSize]++
	}

	return batches
}

// batchByDepth creates batches grouped by traversal depth
func (bo *DefaultBatchOptimizer) batchByDepth(resources []*unstructured.Unstructured, config *BatchConfig) []ResourceBatch {
	// Group resources by depth
	resourcesByDepth := make(map[int][]*unstructured.Unstructured)
	for _, resource := range resources {
		depth := bo.getResourceDepth(resource)
		resourcesByDepth[depth] = append(resourcesByDepth[depth], resource)
	}

	var batches []ResourceBatch

	// Create batches for each depth level
	for depth, depthResources := range resourcesByDepth {
		// Further divide by batch size if needed
		for i := 0; i < len(depthResources); i += config.BatchSize {
			end := i + config.BatchSize
			if end > len(depthResources) {
				end = len(depthResources)
			}

			batchResources := depthResources[i:end]
			batch := ResourceBatch{
				ID:        fmt.Sprintf("depth-%d-batch-%d", depth, len(batches)),
				Resources: batchResources,
				BatchType: BatchTypeDepth,
				Priority:  bo.calculateDepthPriority(depth),
				Depth:     depth,
				Metadata:  bo.createBatchMetadata(batchResources),
				CreatedAt: time.Now(),
			}

			batches = append(batches, batch)
			bo.stats.BatchTypes[BatchTypeDepth]++
		}
	}

	return batches
}

// getResourceDepth extracts the traversal depth from resource annotations or metadata
func (bo *DefaultBatchOptimizer) getResourceDepth(resource *unstructured.Unstructured) int {
	// Try to get depth from annotations first
	annotations := resource.GetAnnotations()
	if annotations != nil {
		if depthStr, exists := annotations["kubecore.io/traversal-depth"]; exists {
			if depth, err := fmt.Sscanf(depthStr, "%d", new(int)); err == nil && depth == 1 {
				var d int
				fmt.Sscanf(depthStr, "%d", &d)
				return d
			}
		}
	}

	// Fallback: try to infer from labels or return 0
	return 0
}

// getAverageDepth calculates the average depth of resources in a slice
func (bo *DefaultBatchOptimizer) getAverageDepth(resources []*unstructured.Unstructured) int {
	if len(resources) == 0 {
		return 0
	}

	totalDepth := 0
	for _, resource := range resources {
		totalDepth += bo.getResourceDepth(resource)
	}

	return totalDepth / len(resources)
}

// calculateDepthPriority calculates processing priority based on depth
func (bo *DefaultBatchOptimizer) calculateDepthPriority(depth int) int {
	// Lower depth = higher priority (process root resources first)
	return 10 - depth
}

// createBatchMetadata creates metadata for a batch of resources
func (bo *DefaultBatchOptimizer) createBatchMetadata(resources []*unstructured.Unstructured) *BatchMetadata {
	apiGroups := make(map[string]bool)
	kinds := make(map[string]bool)
	namespaces := make(map[string]bool)

	for _, resource := range resources {
		// Extract API group
		apiVersion := resource.GetAPIVersion()
		if strings.Contains(apiVersion, "/") {
			parts := strings.Split(apiVersion, "/")
			apiGroups[parts[0]] = true
		} else {
			apiGroups["core"] = true
		}

		// Extract kind
		kinds[resource.GetKind()] = true

		// Extract namespace (if any)
		if ns := resource.GetNamespace(); ns != "" {
			namespaces[ns] = true
		}
	}

	metadata := &BatchMetadata{
		APIGroups:               make([]string, 0, len(apiGroups)),
		Kinds:                   make([]string, 0, len(kinds)),
		Namespaces:              make([]string, 0, len(namespaces)),
		EstimatedProcessingTime: time.Duration(len(resources)) * 100 * time.Millisecond, // Rough estimate
		OptimizationHints:       make(map[string]interface{}),
	}

	for apiGroup := range apiGroups {
		metadata.APIGroups = append(metadata.APIGroups, apiGroup)
	}
	for kind := range kinds {
		metadata.Kinds = append(metadata.Kinds, kind)
	}
	for namespace := range namespaces {
		metadata.Namespaces = append(metadata.Namespaces, namespace)
	}

	// Add optimization hints
	if len(metadata.APIGroups) == 1 {
		metadata.OptimizationHints["single_api_group"] = metadata.APIGroups[0]
	}
	if len(metadata.Kinds) == 1 {
		metadata.OptimizationHints["single_kind"] = metadata.Kinds[0]
	}
	if len(metadata.Namespaces) == 1 {
		metadata.OptimizationHints["single_namespace"] = metadata.Namespaces[0]
	}

	return metadata
}

// generateResourceID generates a unique ID for a resource
func (bo *DefaultBatchOptimizer) generateResourceID(resource *unstructured.Unstructured) string {
	return fmt.Sprintf("%s/%s/%s/%s",
		resource.GetAPIVersion(),
		resource.GetKind(),
		resource.GetNamespace(),
		resource.GetName())
}
