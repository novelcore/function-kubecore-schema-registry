package resolver

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// Resolver defines the interface for different resource matching strategies
type Resolver interface {
	// Resolve resolves resources based on a specific matching strategy
	Resolve(ctx context.Context, request v1beta1.ResourceRequest) ([]*FetchedResource, error)
	
	// SupportsMatchType checks if this resolver supports the given match type
	SupportsMatchType(matchType v1beta1.MatchType) bool
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