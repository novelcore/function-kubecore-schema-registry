package traversal

import (
	"context"
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	
	"github.com/crossplane/function-kubecore-schema-registry/pkg/graph"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// TraversalEngine orchestrates transitive discovery operations
type TraversalEngine interface {
	// ExecuteTransitiveDiscovery performs transitive discovery starting from root resources
	ExecuteTransitiveDiscovery(ctx context.Context, config *TraversalConfig, rootResources []*unstructured.Unstructured) (*TraversalResult, error)
	
	// DiscoverReferencedResources discovers resources referenced by the given resources
	DiscoverReferencedResources(ctx context.Context, resources []*unstructured.Unstructured, config *TraversalConfig) (*DiscoveryResult, error)
	
	// BuildResourceGraph builds a resource dependency graph from discovered resources
	BuildResourceGraph(ctx context.Context, resources []*unstructured.Unstructured, config *TraversalConfig) (*graph.ResourceGraph, error)
	
	// ValidateTraversalResult validates the results of transitive discovery
	ValidateTraversalResult(result *TraversalResult) *TraversalValidationResult
}

// TraversalConfig contains configuration for transitive discovery
type TraversalConfig struct {
	// MaxDepth limits the depth of transitive discovery
	MaxDepth int
	
	// MaxResources limits the total number of resources to discover
	MaxResources int
	
	// Timeout limits the total time for traversal
	Timeout time.Duration
	
	// Direction specifies the direction of traversal
	Direction graph.TraversalDirection
	
	// ScopeFilter determines which resources to include in traversal
	ScopeFilter *ScopeFilterConfig
	
	// BatchConfig controls batch processing optimization
	BatchConfig *BatchConfig
	
	// CacheConfig controls execution-scoped caching
	CacheConfig *CacheConfig
	
	// ReferenceResolution controls how references are resolved
	ReferenceResolution *ReferenceResolutionConfig
	
	// CycleHandling controls how cycles are handled
	CycleHandling *CycleHandlingConfig
	
	// Performance controls performance optimization
	Performance *PerformanceConfig
}

// ScopeFilterConfig controls which resources are included in traversal
type ScopeFilterConfig struct {
	// IncludeAPIGroups specifies which API groups to include (allowlist)
	IncludeAPIGroups []string
	
	// ExcludeAPIGroups specifies which API groups to exclude (blocklist)
	ExcludeAPIGroups []string
	
	// IncludeKinds specifies which resource kinds to include
	IncludeKinds []string
	
	// ExcludeKinds specifies which resource kinds to exclude
	ExcludeKinds []string
	
	// PlatformOnly limits traversal to platform resources only
	PlatformOnly bool
	
	// CrossNamespaceEnabled allows traversal across namespace boundaries
	CrossNamespaceEnabled bool
	
	// IncludeNamespaces specifies which namespaces to include
	IncludeNamespaces []string
	
	// ExcludeNamespaces specifies which namespaces to exclude
	ExcludeNamespaces []string
}

// BatchConfig controls batch processing optimization
type BatchConfig struct {
	// Enabled indicates if batch processing is enabled
	Enabled bool
	
	// BatchSize is the number of resources to process per batch
	BatchSize int
	
	// MaxConcurrentBatches limits the number of concurrent batches
	MaxConcurrentBatches int
	
	// SameDepthBatching enables batching of resources at the same depth
	SameDepthBatching bool
	
	// BatchTimeout is the timeout for each batch operation
	BatchTimeout time.Duration
}

// CacheConfig controls execution-scoped caching
type CacheConfig struct {
	// Enabled indicates if caching is enabled
	Enabled bool
	
	// TTL is the time-to-live for cached entries
	TTL time.Duration
	
	// MaxSize is the maximum number of cached entries
	MaxSize int
	
	// CacheStrategy determines how resources are cached
	CacheStrategy CacheStrategy
}

// CacheStrategy defines caching strategies
type CacheStrategy string

const (
	// CacheStrategyLRU uses least-recently-used eviction
	CacheStrategyLRU CacheStrategy = "lru"
	// CacheStrategyLFU uses least-frequently-used eviction
	CacheStrategyLFU CacheStrategy = "lfu"
	// CacheStrategyTTL uses time-to-live eviction
	CacheStrategyTTL CacheStrategy = "ttl"
)

// ReferenceResolutionConfig controls how references are resolved
type ReferenceResolutionConfig struct {
	// EnableDynamicCRDs allows resolution of references in dynamically discovered CRDs
	EnableDynamicCRDs bool
	
	// FollowOwnerReferences enables following owner reference chains
	FollowOwnerReferences bool
	
	// FollowCustomReferences enables following custom reference fields
	FollowCustomReferences bool
	
	// SkipMissingReferences continues traversal when referenced resources are missing
	SkipMissingReferences bool
	
	// ReferencePatterns additional patterns for detecting reference fields
	ReferencePatterns []ReferencePattern
	
	// MinConfidenceThreshold is the minimum confidence required for following references
	MinConfidenceThreshold float64
}

// CycleHandlingConfig controls how cycles are handled
type CycleHandlingConfig struct {
	// DetectionEnabled enables cycle detection during traversal
	DetectionEnabled bool
	
	// OnCycleDetected defines the action when a cycle is detected
	OnCycleDetected CycleAction
	
	// MaxCycles limits the number of cycles to track
	MaxCycles int
	
	// ReportCycles includes cycle information in results
	ReportCycles bool
}

// CycleAction defines actions to take when cycles are detected
type CycleAction string

const (
	// CycleActionContinue continues traversal ignoring the cycle
	CycleActionContinue CycleAction = "continue"
	// CycleActionStop stops traversal at the cycle point
	CycleActionStop CycleAction = "stop"
	// CycleActionFail fails the entire traversal operation
	CycleActionFail CycleAction = "fail"
)

// PerformanceConfig controls performance optimization
type PerformanceConfig struct {
	// MaxConcurrentRequests limits concurrent Kubernetes API requests
	MaxConcurrentRequests int
	
	// RequestTimeout is the timeout for individual API requests
	RequestTimeout time.Duration
	
	// EnableMetrics enables collection of performance metrics
	EnableMetrics bool
	
	// ResourceDeduplication enables resource deduplication by UID
	ResourceDeduplication bool
	
	// MemoryLimits sets memory usage limits
	MemoryLimits *MemoryLimits
}

// MemoryLimits defines memory usage constraints
type MemoryLimits struct {
	// MaxGraphSize limits the maximum size of the resource graph
	MaxGraphSize int64
	
	// MaxCacheSize limits the maximum size of caches
	MaxCacheSize int64
	
	// GCThreshold triggers garbage collection when memory usage exceeds this threshold
	GCThreshold int64
}

// TraversalResult contains the complete result of transitive discovery
type TraversalResult struct {
	// ResourceGraph contains the discovered resource dependency graph
	ResourceGraph *graph.ResourceGraph
	
	// DiscoveredResources contains all discovered resources
	DiscoveredResources map[string]*unstructured.Unstructured
	
	// TraversalPath contains the path taken during traversal
	TraversalPath *TraversalPath
	
	// Statistics contains performance and execution statistics
	Statistics *TraversalStatistics
	
	// ValidationResult contains validation results
	ValidationResult *TraversalValidationResult
	
	// CycleResults contains information about detected cycles
	CycleResults *graph.CycleDetectionResult
	
	// Metadata contains additional traversal metadata
	Metadata *TraversalMetadata
}

// DiscoveryResult contains the result of resource discovery at a specific level
type DiscoveryResult struct {
	// Resources contains the discovered resources
	Resources []*unstructured.Unstructured
	
	// References contains the reference fields found in the resources
	References map[string][]dynamic.ReferenceField
	
	// Depth is the depth at which these resources were discovered
	Depth int
	
	// Statistics contains discovery statistics for this level
	Statistics *DiscoveryStatistics
	
	// Errors contains any errors encountered during discovery
	Errors []TraversalError
}

// TraversalPath represents the path taken during traversal
type TraversalPath struct {
	// Steps contains each step of the traversal process
	Steps []TraversalStep
	
	// TotalSteps is the number of steps taken
	TotalSteps int
	
	// MaxDepthReached is the maximum depth reached during traversal
	MaxDepthReached int
	
	// StartTime is when traversal began
	StartTime time.Time
	
	// EndTime is when traversal completed
	EndTime time.Time
	
	// Duration is the total time taken for traversal
	Duration time.Duration
}

// TraversalStep represents a single step in the traversal process
type TraversalStep struct {
	// StepID is a unique identifier for this step
	StepID int
	
	// Depth is the depth at which this step occurred
	Depth int
	
	// Action describes what action was taken
	Action TraversalAction
	
	// ResourceID identifies the resource being processed
	ResourceID string
	
	// ReferencesFound is the number of references found in this step
	ReferencesFound int
	
	// ReferencesFollowed is the number of references followed
	ReferencesFollowed int
	
	// Timestamp is when this step occurred
	Timestamp time.Time
	
	// Duration is how long this step took
	Duration time.Duration
	
	// Errors contains any errors encountered in this step
	Errors []TraversalError
}

// TraversalAction represents an action taken during traversal
type TraversalAction string

const (
	// TraversalActionDiscover indicates resource discovery
	TraversalActionDiscover TraversalAction = "discover"
	// TraversalActionAnalyze indicates reference analysis
	TraversalActionAnalyze TraversalAction = "analyze"
	// TraversalActionResolve indicates reference resolution
	TraversalActionResolve TraversalAction = "resolve"
	// TraversalActionSkip indicates resource was skipped
	TraversalActionSkip TraversalAction = "skip"
	// TraversalActionCache indicates resource was cached
	TraversalActionCache TraversalAction = "cache"
)

// TraversalStatistics contains statistics about the traversal process
type TraversalStatistics struct {
	// TotalResources is the total number of resources processed
	TotalResources int
	
	// ResourcesByDepth groups resources by their discovery depth
	ResourcesByDepth map[int]int
	
	// ResourcesByKind groups resources by their Kubernetes kind
	ResourcesByKind map[string]int
	
	// ResourcesByAPIGroup groups resources by their API group
	ResourcesByAPIGroup map[string]int
	
	// TotalReferences is the total number of references found
	TotalReferences int
	
	// ReferencesFollowed is the number of references that were followed
	ReferencesFollowed int
	
	// ReferencesSkipped is the number of references that were skipped
	ReferencesSkipped int
	
	// APICallCount is the total number of Kubernetes API calls made
	APICallCount int
	
	// CacheHits is the number of cache hits
	CacheHits int
	
	// CacheMisses is the number of cache misses
	CacheMisses int
	
	// MemoryUsage contains memory usage statistics
	MemoryUsage *MemoryUsageStats
	
	// PerformanceMetrics contains detailed performance metrics
	PerformanceMetrics *PerformanceMetrics
}

// DiscoveryStatistics contains statistics for a single discovery operation
type DiscoveryStatistics struct {
	// ResourcesRequested is the number of resources requested for discovery
	ResourcesRequested int
	
	// ResourcesFound is the number of resources actually found
	ResourcesFound int
	
	// ResourcesFiltered is the number of resources filtered out by scope
	ResourcesFiltered int
	
	// ReferencesDetected is the number of references detected
	ReferencesDetected int
	
	// APICallsToThisDepth is the number of API calls made at this depth
	APICallsToThisDepth int
	
	// DiscoveryTime is the time taken for discovery at this depth
	DiscoveryTime time.Duration
}

// MemoryUsageStats contains memory usage statistics
type MemoryUsageStats struct {
	// CurrentUsage is the current memory usage in bytes
	CurrentUsage int64
	
	// PeakUsage is the peak memory usage in bytes
	PeakUsage int64
	
	// GraphSize is the size of the resource graph in bytes
	GraphSize int64
	
	// CacheSize is the size of caches in bytes
	CacheSize int64
	
	// GCCount is the number of garbage collection cycles performed
	GCCount int
}

// PerformanceMetrics contains detailed performance metrics
type PerformanceMetrics struct {
	// APIRequestLatency contains API request latency statistics
	APIRequestLatency *LatencyStats
	
	// ReferenceResolutionLatency contains reference resolution latency
	ReferenceResolutionLatency *LatencyStats
	
	// GraphBuildingTime is the time taken to build the resource graph
	GraphBuildingTime time.Duration
	
	// CycleDetectionTime is the time taken for cycle detection
	CycleDetectionTime time.Duration
	
	// FilteringTime is the time taken for scope filtering
	FilteringTime time.Duration
	
	// ThroughputMetrics contains throughput statistics
	ThroughputMetrics *ThroughputStats
}

// LatencyStats contains latency statistics
type LatencyStats struct {
	// Average is the average latency
	Average time.Duration
	
	// Median is the median latency
	Median time.Duration
	
	// P95 is the 95th percentile latency
	P95 time.Duration
	
	// P99 is the 99th percentile latency
	P99 time.Duration
	
	// Min is the minimum latency
	Min time.Duration
	
	// Max is the maximum latency
	Max time.Duration
}

// ThroughputStats contains throughput statistics
type ThroughputStats struct {
	// ResourcesPerSecond is the rate of resource processing
	ResourcesPerSecond float64
	
	// ReferencesPerSecond is the rate of reference processing
	ReferencesPerSecond float64
	
	// APICallsPerSecond is the rate of API calls
	APICallsPerSecond float64
}

// TraversalValidationResult contains validation results
type TraversalValidationResult struct {
	// Valid indicates if the traversal result is valid
	Valid bool
	
	// Errors contains validation errors
	Errors []ValidationError
	
	// Warnings contains validation warnings
	Warnings []ValidationWarning
	
	// Statistics contains validation statistics
	Statistics *ValidationStatistics
}

// ValidationError represents a validation error
type ValidationError struct {
	// Type is the type of validation error
	Type ValidationErrorType
	
	// Message is the error message
	Message string
	
	// ResourceID identifies the problematic resource
	ResourceID string
	
	// Context provides additional context about the error
	Context map[string]interface{}
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	// Type is the type of validation warning
	Type ValidationWarningType
	
	// Message is the warning message
	Message string
	
	// ResourceID identifies the resource with the warning
	ResourceID string
	
	// Severity indicates the severity of the warning
	Severity string
}

// ValidationErrorType defines types of validation errors
type ValidationErrorType string

const (
	// ValidationErrorInvalidGraph indicates an invalid resource graph
	ValidationErrorInvalidGraph ValidationErrorType = "invalid_graph"
	// ValidationErrorMissingResource indicates a missing referenced resource
	ValidationErrorMissingResource ValidationErrorType = "missing_resource"
	// ValidationErrorCycleDetected indicates a problematic cycle
	ValidationErrorCycleDetected ValidationErrorType = "cycle_detected"
	// ValidationErrorScopeViolation indicates a scope boundary violation
	ValidationErrorScopeViolation ValidationErrorType = "scope_violation"
)

// ValidationWarningType defines types of validation warnings
type ValidationWarningType string

const (
	// ValidationWarningLowConfidence indicates low confidence references
	ValidationWarningLowConfidence ValidationWarningType = "low_confidence"
	// ValidationWarningDeepTraversal indicates traversal reached maximum depth
	ValidationWarningDeepTraversal ValidationWarningType = "deep_traversal"
	// ValidationWarningManyResources indicates many resources were discovered
	ValidationWarningManyResources ValidationWarningType = "many_resources"
	// ValidationWarningSlowPerformance indicates slow traversal performance
	ValidationWarningSlowPerformance ValidationWarningType = "slow_performance"
)

// ValidationStatistics contains validation statistics
type ValidationStatistics struct {
	// ResourcesValidated is the number of resources validated
	ResourcesValidated int
	
	// ReferencesValidated is the number of references validated
	ReferencesValidated int
	
	// ErrorCount is the total number of validation errors
	ErrorCount int
	
	// WarningCount is the total number of validation warnings
	WarningCount int
	
	// ValidationTime is the time taken for validation
	ValidationTime time.Duration
}

// TraversalError represents an error that occurred during traversal
type TraversalError struct {
	// Type is the type of error
	Type TraversalErrorType
	
	// Message is the error message
	Message string
	
	// ResourceID identifies the resource associated with the error
	ResourceID string
	
	// Depth is the depth at which the error occurred
	Depth int
	
	// Timestamp is when the error occurred
	Timestamp time.Time
	
	// Recoverable indicates if the error is recoverable
	Recoverable bool
	
	// Context provides additional context about the error
	Context map[string]interface{}
}

// TraversalErrorType defines types of traversal errors
type TraversalErrorType string

const (
	// TraversalErrorAPICall indicates an error making a Kubernetes API call
	TraversalErrorAPICall TraversalErrorType = "api_call"
	// TraversalErrorReferenceResolution indicates an error resolving a reference
	TraversalErrorReferenceResolution TraversalErrorType = "reference_resolution"
	// TraversalErrorScopeFilter indicates an error applying scope filters
	TraversalErrorScopeFilter TraversalErrorType = "scope_filter"
	// TraversalErrorTimeout indicates a timeout error
	TraversalErrorTimeout TraversalErrorType = "timeout"
	// TraversalErrorMemoryLimit indicates a memory limit was exceeded
	TraversalErrorMemoryLimit TraversalErrorType = "memory_limit"
)

// TraversalMetadata contains additional metadata about the traversal
type TraversalMetadata struct {
	// Config is the configuration used for traversal
	Config *TraversalConfig
	
	// StartResources contains the initial resources that started traversal
	StartResources []string
	
	// CompletedAt is when the traversal completed
	CompletedAt time.Time
	
	// TerminationReason indicates why traversal stopped
	TerminationReason TerminationReason
	
	// Version is the version of the traversal engine used
	Version string
	
	// Environment contains information about the execution environment
	Environment map[string]string
}

// TerminationReason indicates why traversal was terminated
type TerminationReason string

const (
	// TerminationReasonCompleted indicates normal completion
	TerminationReasonCompleted TerminationReason = "completed"
	// TerminationReasonMaxDepth indicates maximum depth was reached
	TerminationReasonMaxDepth TerminationReason = "max_depth"
	// TerminationReasonMaxResources indicates maximum resource count was reached
	TerminationReasonMaxResources TerminationReason = "max_resources"
	// TerminationReasonTimeout indicates timeout was reached
	TerminationReasonTimeout TerminationReason = "timeout"
	// TerminationReasonError indicates an error caused termination
	TerminationReasonError TerminationReason = "error"
	// TerminationReasonCycle indicates a cycle caused termination
	TerminationReasonCycle TerminationReason = "cycle"
)

// TraversalEngineComponents contains the components needed by the traversal engine
type TraversalEngineComponents struct {
	// DynamicClient provides access to Kubernetes dynamic API
	DynamicClient dynamic.Interface
	
	// TypedClient provides access to Kubernetes typed API
	TypedClient kubernetes.Interface
	
	// Registry provides access to the resource type registry
	Registry registry.Registry
	
	// ReferenceResolver resolves references in resources
	ReferenceResolver ReferenceResolver
	
	// ScopeFilter filters resources based on scope criteria
	ScopeFilter ScopeFilter
	
	// BatchOptimizer optimizes batch processing
	BatchOptimizer BatchOptimizer
	
	// Cache provides execution-scoped caching
	Cache Cache
	
	// GraphBuilder builds resource dependency graphs
	GraphBuilder graph.GraphBuilder
	
	// CycleDetector detects cycles in graphs
	CycleDetector graph.CycleDetector
	
	// PathTracker tracks discovery paths
	PathTracker graph.PathTracker
}

// Default configuration values
const (
	DefaultMaxDepth        = 3
	DefaultMaxResources    = 100
	DefaultTimeout         = 10 * time.Second
	DefaultBatchSize       = 10
	DefaultCacheTTL        = 5 * time.Minute
	DefaultCacheMaxSize    = 1000
	DefaultMaxConcurrent   = 10
	DefaultRequestTimeout  = 2 * time.Second
	DefaultConfidenceThreshold = 0.5
)

// Default traversal configuration
func NewDefaultTraversalConfig() *TraversalConfig {
	return &TraversalConfig{
		MaxDepth:     DefaultMaxDepth,
		MaxResources: DefaultMaxResources,
		Timeout:      DefaultTimeout,
		Direction:    graph.TraversalDirectionForward,
		ScopeFilter: &ScopeFilterConfig{
			IncludeAPIGroups:      []string{"*.kubecore.io"},
			PlatformOnly:          true,
			CrossNamespaceEnabled: false,
		},
		BatchConfig: &BatchConfig{
			Enabled:              true,
			BatchSize:            DefaultBatchSize,
			MaxConcurrentBatches: 3,
			SameDepthBatching:    true,
			BatchTimeout:         DefaultTimeout / 3,
		},
		CacheConfig: &CacheConfig{
			Enabled:       true,
			TTL:           DefaultCacheTTL,
			MaxSize:       DefaultCacheMaxSize,
			CacheStrategy: CacheStrategyLRU,
		},
		ReferenceResolution: &ReferenceResolutionConfig{
			EnableDynamicCRDs:       true,
			FollowOwnerReferences:   true,
			FollowCustomReferences:  true,
			SkipMissingReferences:   true,
			MinConfidenceThreshold:  DefaultConfidenceThreshold,
		},
		CycleHandling: &CycleHandlingConfig{
			DetectionEnabled: true,
			OnCycleDetected:  CycleActionContinue,
			MaxCycles:        10,
			ReportCycles:     true,
		},
		Performance: &PerformanceConfig{
			MaxConcurrentRequests: DefaultMaxConcurrent,
			RequestTimeout:        DefaultRequestTimeout,
			EnableMetrics:         true,
			ResourceDeduplication: true,
			MemoryLimits: &MemoryLimits{
				MaxGraphSize:  50 * 1024 * 1024, // 50MB
				MaxCacheSize:  10 * 1024 * 1024, // 10MB
				GCThreshold:   80 * 1024 * 1024, // 80MB
			},
		},
	}
}

// ReferencePattern defines patterns for detecting reference fields
type ReferencePattern struct {
	Pattern     string
	TargetKind  string
	TargetGroup string
	RefType     RefType
	Confidence  float64
}

// RefType represents the type of reference relationship
type RefType string

const (
	RefTypeOwnerRef   RefType = "ownerRef"   // metadata.ownerReferences
	RefTypeConfigMap  RefType = "configMap"  // Reference to ConfigMap
	RefTypeSecret     RefType = "secret"     // Reference to Secret
	RefTypeService    RefType = "service"    // Reference to Service
	RefTypePVC        RefType = "pvc"        // Reference to PersistentVolumeClaim
	RefTypeCustom     RefType = "custom"     // Custom reference (platform-specific)
)

// ReferenceField represents a field that references another resource
type ReferenceField struct {
	FieldPath       string
	FieldName       string
	TargetKind      string
	TargetGroup     string
	TargetVersion   string
	RefType         RefType
	Confidence      float64
	DetectionMethod string
}