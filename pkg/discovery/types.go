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

// FetchResult represents the result of a resource fetch operation
type FetchResult struct {
	// Resources contains the successfully fetched resources by 'into' field name
	Resources map[string]*FetchedResource `json:"resources"`
	
	// Summary contains statistics about the fetch operation
	Summary FetchSummary `json:"fetchSummary"`
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