// Package v1beta1 contains Phase 3 traversal configuration types
// +kubebuilder:object:generate=true
package v1beta1

// TraversalConfig contains configuration for Phase 3 transitive discovery
type TraversalConfig struct {
	// Enabled indicates if Phase 3 transitive discovery is enabled
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// MaxDepth limits the depth of transitive discovery
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxDepth int `json:"maxDepth,omitempty"`

	// MaxResources limits the total number of resources to discover
	// +kubebuilder:default=100
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=1000
	MaxResources int `json:"maxResources,omitempty"`

	// Timeout limits the total time for traversal
	// +kubebuilder:default="10s"
	// +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
	Timeout *string `json:"timeout,omitempty"`

	// Direction specifies the direction of traversal
	// +kubebuilder:validation:Enum=forward;reverse;bidirectional
	// +kubebuilder:default="forward"
	Direction TraversalDirection `json:"direction,omitempty"`

	// ScopeFilter determines which resources to include in traversal
	ScopeFilter *ScopeFilterConfig `json:"scopeFilter,omitempty"`

	// BatchConfig controls batch processing optimization
	BatchConfig *BatchConfig `json:"batchConfig,omitempty"`

	// CacheConfig controls execution-scoped caching
	CacheConfig *CacheConfig `json:"cacheConfig,omitempty"`

	// ReferenceResolution controls how references are resolved
	ReferenceResolution *ReferenceResolutionConfig `json:"referenceResolution,omitempty"`

	// CycleHandling controls how cycles are handled
	CycleHandling *CycleHandlingConfig `json:"cycleHandling,omitempty"`

	// Performance controls performance optimization
	Performance *PerformanceConfig `json:"performance,omitempty"`
}

// TraversalDirection defines the direction of graph traversal
type TraversalDirection string

const (
	// TraversalDirectionForward follows references from source to target
	TraversalDirectionForward TraversalDirection = "forward"
	// TraversalDirectionReverse follows back-references from target to source
	TraversalDirectionReverse TraversalDirection = "reverse"
	// TraversalDirectionBidirectional follows both forward and reverse references
	TraversalDirectionBidirectional TraversalDirection = "bidirectional"
)

// ScopeFilterConfig controls which resources are included in traversal
type ScopeFilterConfig struct {
	// PlatformOnly limits traversal to platform resources only
	// +kubebuilder:default=true
	PlatformOnly bool `json:"platformOnly,omitempty"`

	// IncludeAPIGroups specifies which API groups to include (allowlist)
	// +kubebuilder:default={"*.kubecore.io"}
	IncludeAPIGroups []string `json:"includeAPIGroups,omitempty"`

	// ExcludeAPIGroups specifies which API groups to exclude (blocklist)
	ExcludeAPIGroups []string `json:"excludeAPIGroups,omitempty"`

	// IncludeKinds specifies which resource kinds to include
	IncludeKinds []string `json:"includeKinds,omitempty"`

	// ExcludeKinds specifies which resource kinds to exclude
	ExcludeKinds []string `json:"excludeKinds,omitempty"`

	// CrossNamespaceEnabled allows traversal across namespace boundaries
	// +kubebuilder:default=false
	CrossNamespaceEnabled bool `json:"crossNamespaceEnabled,omitempty"`

	// IncludeNamespaces specifies which namespaces to include
	IncludeNamespaces []string `json:"includeNamespaces,omitempty"`

	// ExcludeNamespaces specifies which namespaces to exclude
	ExcludeNamespaces []string `json:"excludeNamespaces,omitempty"`
}

// BatchConfig controls batch processing optimization
type BatchConfig struct {
	// Enabled indicates if batch processing is enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// BatchSize is the number of resources to process per batch
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	BatchSize int `json:"batchSize,omitempty"`

	// MaxConcurrentBatches limits the number of concurrent batches
	// +kubebuilder:default=3
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	MaxConcurrentBatches int `json:"maxConcurrentBatches,omitempty"`

	// SameDepthBatching enables batching of resources at the same depth
	// +kubebuilder:default=true
	SameDepthBatching bool `json:"sameDepthBatching,omitempty"`

	// BatchTimeout is the timeout for each batch operation
	// +kubebuilder:default="3s"
	// +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
	BatchTimeout *string `json:"batchTimeout,omitempty"`
}

// CacheConfig controls execution-scoped caching
type CacheConfig struct {
	// Enabled indicates if caching is enabled
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// TTL is the time-to-live for cached entries
	// +kubebuilder:default="5m"
	// +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
	TTL *string `json:"ttl,omitempty"`

	// MaxSize is the maximum number of cached entries
	// +kubebuilder:default=1000
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=10000
	MaxSize int `json:"maxSize,omitempty"`

	// Strategy determines how resources are cached
	// +kubebuilder:validation:Enum=lru;lfu;ttl
	// +kubebuilder:default="lru"
	Strategy CacheStrategy `json:"strategy,omitempty"`
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
	// +kubebuilder:default=true
	EnableDynamicCRDs bool `json:"enableDynamicCRDs,omitempty"`

	// FollowOwnerReferences enables following owner reference chains
	// +kubebuilder:default=true
	FollowOwnerReferences bool `json:"followOwnerReferences,omitempty"`

	// FollowCustomReferences enables following custom reference fields
	// +kubebuilder:default=true
	FollowCustomReferences bool `json:"followCustomReferences,omitempty"`

	// SkipMissingReferences continues traversal when referenced resources are missing
	// +kubebuilder:default=true
	SkipMissingReferences bool `json:"skipMissingReferences,omitempty"`

	// MinConfidenceThreshold is the minimum confidence required for following references
	// +kubebuilder:default=0.5
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	MinConfidenceThreshold float64 `json:"minConfidenceThreshold,omitempty"`

	// AdditionalPatterns contains additional patterns for detecting reference fields
	AdditionalPatterns []ReferencePattern `json:"additionalPatterns,omitempty"`
}

// ReferencePattern defines a pattern for detecting reference fields
type ReferencePattern struct {
	// Pattern is the field name pattern to match
	// +kubebuilder:validation:Required
	Pattern string `json:"pattern"`

	// TargetKind is the expected target resource kind
	TargetKind string `json:"targetKind,omitempty"`

	// TargetGroup is the expected target API group
	TargetGroup string `json:"targetGroup,omitempty"`

	// Confidence is the confidence level of this pattern
	// +kubebuilder:default=0.8
	// +kubebuilder:validation:Minimum=0.0
	// +kubebuilder:validation:Maximum=1.0
	Confidence float64 `json:"confidence,omitempty"`
}

// CycleHandlingConfig controls how cycles are handled
type CycleHandlingConfig struct {
	// DetectionEnabled enables cycle detection during traversal
	// +kubebuilder:default=true
	DetectionEnabled bool `json:"detectionEnabled,omitempty"`

	// OnCycleDetected defines the action when a cycle is detected
	// +kubebuilder:validation:Enum=continue;stop;fail
	// +kubebuilder:default="continue"
	OnCycleDetected CycleAction `json:"onCycleDetected,omitempty"`

	// MaxCycles limits the number of cycles to track
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxCycles int `json:"maxCycles,omitempty"`

	// ReportCycles includes cycle information in results
	// +kubebuilder:default=true
	ReportCycles bool `json:"reportCycles,omitempty"`
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
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	MaxConcurrentRequests int `json:"maxConcurrentRequests,omitempty"`

	// RequestTimeout is the timeout for individual API requests
	// +kubebuilder:default="2s"
	// +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
	RequestTimeout *string `json:"requestTimeout,omitempty"`

	// EnableMetrics enables collection of performance metrics
	// +kubebuilder:default=true
	EnableMetrics bool `json:"enableMetrics,omitempty"`

	// ResourceDeduplication enables resource deduplication by UID
	// +kubebuilder:default=true
	ResourceDeduplication bool `json:"resourceDeduplication,omitempty"`

	// MemoryLimits sets memory usage limits
	MemoryLimits *MemoryLimits `json:"memoryLimits,omitempty"`
}

// MemoryLimits defines memory usage constraints
type MemoryLimits struct {
	// MaxGraphSize limits the maximum size of the resource graph (in bytes)
	// +kubebuilder:default=52428800
	// +kubebuilder:validation:Minimum=1048576
	// +kubebuilder:validation:Maximum=1073741824
	MaxGraphSize int64 `json:"maxGraphSize,omitempty"`

	// MaxCacheSize limits the maximum size of caches (in bytes)
	// +kubebuilder:default=10485760
	// +kubebuilder:validation:Minimum=1048576
	// +kubebuilder:validation:Maximum=104857600
	MaxCacheSize int64 `json:"maxCacheSize,omitempty"`

	// GCThreshold triggers garbage collection when memory usage exceeds this threshold (in bytes)
	// +kubebuilder:default=83886080
	// +kubebuilder:validation:Minimum=10485760
	// +kubebuilder:validation:Maximum=2147483648
	GCThreshold int64 `json:"gcThreshold,omitempty"`
}
