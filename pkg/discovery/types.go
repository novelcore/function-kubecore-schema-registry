package discovery

import (
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	
	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// Engine defines the interface for resource discovery
type Engine interface {
	// FetchResources fetches resources based on the provided requests
	FetchResources(requests []v1beta1.ResourceRequest) (*FetchResult, error)
}

// Note: Resolver interface moved to resolver package to avoid import cycles

// DiscoveryContext provides context for discovery operations
type DiscoveryContext struct {
	// FunctionNamespace is the namespace where the function is running
	FunctionNamespace string
	
	// TimeoutPerRequest is the maximum time to wait for each request
	TimeoutPerRequest time.Duration
	
	// MaxConcurrentRequests limits concurrent operations
	MaxConcurrentRequests int
	
	// Phase2Enabled indicates if Phase 2 features are enabled
	Phase2Enabled bool
}

// FetchResult represents the result of a resource fetch operation
type FetchResult struct {
	// Resources contains the successfully fetched resources by 'into' field name
	// For Phase 2, this may contain multiple resources per 'into' key
	Resources map[string]*FetchedResource `json:"resources"`
	
	// MultiResources contains multiple resources for Phase 2 discovery results
	// Key is 'into' field, value is array of resources
	MultiResources map[string][]*FetchedResource `json:"multiResources,omitempty"`
	
	// Summary contains statistics about the fetch operation
	Summary FetchSummary `json:"fetchSummary"`
	
	// Phase2Results contains Phase 2 specific metadata
	Phase2Results *Phase2Results `json:"phase2Results,omitempty"`
}

// FetchedResource represents a single fetched resource with metadata
type FetchedResource struct {
	// Request is the original request that led to fetching this resource
	Request v1beta1.ResourceRequest `json:"request"`
	
	// Resource is the fetched Kubernetes resource
	Resource *unstructured.Unstructured `json:"resource"`
	
	// Metadata contains additional information about the resource
	Metadata ResourceMetadata `json:"_kubecore"`
	
	// FetchedAt is when the resource was retrieved
	FetchedAt time.Time `json:"fetchedAt"`
}

// ResourceMetadata contains KubeCore-specific metadata
type ResourceMetadata struct {
	// FetchStatus indicates the result of the fetch operation
	FetchStatus FetchStatus `json:"fetchStatus"`
	
	// Error contains error details if the fetch failed
	Error *errors.FunctionError `json:"error,omitempty"`
	
	// FetchDuration is how long it took to fetch this resource
	FetchDuration time.Duration `json:"fetchDuration"`
	
	// ResourceExists indicates whether the resource exists
	ResourceExists bool `json:"resourceExists"`
	
	// Permissions contains information about access permissions
	Permissions *PermissionInfo `json:"permissions,omitempty"`
	
	// Phase2Metadata contains Phase 2 specific metadata
	Phase2Metadata *Phase2Metadata `json:"phase2,omitempty"`
}

// Phase2Metadata contains Phase 2 specific resource metadata
type Phase2Metadata struct {
	// MatchedBy indicates how this resource was matched (direct, label, expression)
	MatchedBy string `json:"matchedBy"`
	
	// MatchDetails provides details about the match
	MatchDetails *MatchDetails `json:"matchDetails,omitempty"`
	
	// SearchNamespaces lists the namespaces searched to find this resource
	SearchNamespaces []string `json:"searchNamespaces,omitempty"`
	
	// SortPosition indicates the position in sorted results (for ordered discovery)
	SortPosition *int `json:"sortPosition,omitempty"`
}

// MatchDetails provides details about how a resource was matched
type MatchDetails struct {
	// MatchedLabels contains the labels that matched (for label-based discovery)
	MatchedLabels map[string]string `json:"matchedLabels,omitempty"`
	
	// MatchedExpressions contains details about matched expressions
	MatchedExpressions []ExpressionMatch `json:"matchedExpressions,omitempty"`
	
	// MatchScore is a numeric score indicating match quality (higher is better)
	MatchScore *float64 `json:"matchScore,omitempty"`
}

// ExpressionMatch represents a matched expression
type ExpressionMatch struct {
	// Field is the field that was evaluated
	Field string `json:"field"`
	
	// Operator is the operator that was used
	Operator string `json:"operator"`
	
	// ExpectedValue is what was expected
	ExpectedValue interface{} `json:"expectedValue,omitempty"`
	
	// ActualValue is what was found
	ActualValue interface{} `json:"actualValue,omitempty"`
	
	// Matched indicates if this expression matched
	Matched bool `json:"matched"`
}

// Phase2Results contains Phase 2 discovery metadata
type Phase2Results struct {
	// QueryPlan contains the execution plan for discovery operations
	QueryPlan *QueryPlan `json:"queryPlan,omitempty"`
	
	// Performance contains performance metrics
	Performance *PerformanceMetrics `json:"performance,omitempty"`
	
	// ConstraintResults contains results of constraint evaluation
	ConstraintResults map[string]*ConstraintResult `json:"constraintResults,omitempty"`
}

// QueryPlan represents the execution plan for discovery
type QueryPlan struct {
	// TotalQueries is the number of queries planned
	TotalQueries int `json:"totalQueries"`
	
	// BatchedQueries is the number of queries that were batched
	BatchedQueries int `json:"batchedQueries"`
	
	// OptimizedQueries is the number of queries after optimization
	OptimizedQueries int `json:"optimizedQueries"`
	
	// ExecutionSteps describes the execution steps
	ExecutionSteps []string `json:"executionSteps,omitempty"`
}

// PerformanceMetrics contains performance information
type PerformanceMetrics struct {
	// QueryPlanningTime is time spent planning queries
	QueryPlanningTime time.Duration `json:"queryPlanningTime"`
	
	// KubernetesAPITime is time spent in Kubernetes API calls
	KubernetesAPITime time.Duration `json:"kubernetesAPITime"`
	
	// FilteringTime is time spent filtering results
	FilteringTime time.Duration `json:"filteringTime"`
	
	// SortingTime is time spent sorting results
	SortingTime time.Duration `json:"sortingTime"`
	
	// TotalResourcesScanned is the total number of resources scanned
	TotalResourcesScanned int `json:"totalResourcesScanned"`
	
	// CacheHitRate is the percentage of cache hits
	CacheHitRate *float64 `json:"cacheHitRate,omitempty"`
}

// ConstraintResult represents the result of constraint evaluation
type ConstraintResult struct {
	// RequestName is the 'into' field of the request
	RequestName string `json:"requestName"`
	
	// Expected contains expected constraints
	Expected ConstraintValues `json:"expected"`
	
	// Actual contains actual results
	Actual ConstraintValues `json:"actual"`
	
	// Satisfied indicates if constraints were satisfied
	Satisfied bool `json:"satisfied"`
	
	// Message provides details about constraint evaluation
	Message string `json:"message,omitempty"`
}

// ConstraintValues represents constraint values
type ConstraintValues struct {
	// MinMatches is the minimum expected matches
	MinMatches *int `json:"minMatches,omitempty"`
	
	// MaxMatches is the maximum expected matches
	MaxMatches *int `json:"maxMatches,omitempty"`
	
	// ActualMatches is the actual number of matches found
	ActualMatches int `json:"actualMatches"`
}

// FetchStatus represents the status of a resource fetch operation
type FetchStatus string

const (
	FetchStatusSuccess     FetchStatus = "success"
	FetchStatusNotFound    FetchStatus = "not_found"
	FetchStatusForbidden   FetchStatus = "forbidden"
	FetchStatusTimeout     FetchStatus = "timeout"
	FetchStatusError       FetchStatus = "error"
	FetchStatusSkipped     FetchStatus = "skipped" // For optional resources that failed
)

// PermissionInfo contains information about resource access permissions
type PermissionInfo struct {
	CanGet    bool `json:"canGet"`
	CanList   bool `json:"canList"`
	CanWatch  bool `json:"canWatch"`
	CanCreate bool `json:"canCreate"`
	CanUpdate bool `json:"canUpdate"`
	CanDelete bool `json:"canDelete"`
}

// FetchSummary contains statistics about the fetch operation
type FetchSummary struct {
	// Total number of resources requested
	TotalRequested int `json:"totalRequested"`
	
	// Number of resources successfully fetched
	Successful int `json:"successful"`
	
	// Number of resources that failed to fetch
	Failed int `json:"failed"`
	
	// Number of optional resources that were skipped due to errors
	Skipped int `json:"skipped"`
	
	// Number of resources not found
	NotFound int `json:"notFound"`
	
	// Number of resources forbidden (access denied)
	Forbidden int `json:"forbidden"`
	
	// Number of resources that timed out
	Timeout int `json:"timeout"`
	
	// Total execution time for all fetch operations
	TotalDuration time.Duration `json:"totalDuration"`
	
	// Average fetch time per resource
	AverageDuration time.Duration `json:"averageDuration"`
	
	// Errors that occurred during fetching (for failed resources)
	Errors []*FetchError `json:"errors,omitempty"`
}

// FetchError represents an error that occurred during resource fetching
type FetchError struct {
	// ResourceRequest is the request that failed
	ResourceRequest v1beta1.ResourceRequest `json:"resourceRequest"`
	
	// Error is the detailed error information
	Error *errors.FunctionError `json:"error"`
	
	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`
}