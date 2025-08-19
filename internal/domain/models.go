package domain

import (
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// ResourceReference represents a reference to another resource
type ResourceReference struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Kind       string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// SchemaInfo holds schema metadata and structure
type SchemaInfo struct {
	Kind               string                            `json:"kind"`
	APIVersion         string                            `json:"apiVersion"`
	OpenAPIV3Schema    *apiextensionsv1.JSONSchemaProps `json:"openAPIV3Schema,omitempty"`
	ReferenceFields    []string                          `json:"referenceFields"`
	RequiredFields     []string                          `json:"requiredFields"`
	TransitiveRefs     map[string]*SchemaInfo            `json:"transitiveReferences,omitempty"`
	Depth              int                               `json:"depth,omitempty"`
	ReferencePath      string                            `json:"referencePath,omitempty"`
	Source             string                            `json:"source,omitempty"` // "kubernetes-api" | "cache" | "fallback"
}

// ExecutionContext holds extracted context from the XResource
type ExecutionContext struct {
	SourceXResource  string                         `json:"sourceXResource"`
	ClaimName        string                         `json:"claimName"`
	ClaimNamespace   string                         `json:"claimNamespace"`
	DirectReferences map[string]ResourceReference  `json:"directReferences"`
}

// DiscoveryStats holds metrics about the discovery process
type DiscoveryStats struct {
	TotalReferencesFound int   `json:"totalReferencesFound"`
	MaxDepthReached      int   `json:"maxDepthReached"`
	SchemasRetrieved     int   `json:"schemasRetrieved"`
	RealSchemasFound     int   `json:"realSchemasFound"`
	CacheHits            int   `json:"cacheHits"`
	APICallsMade         int   `json:"apiCalls"`
	ExecutionTimeMs      int64 `json:"executionTimeMs"`
}

// DiscoveryOptions holds options for schema discovery
type DiscoveryOptions struct {
	EnableTransitive   bool
	TraversalDepth     int
	IncludeFullSchema  bool
	CorrelationID      string
}

// DiscoveryResult holds the result of a schema discovery operation
type DiscoveryResult struct {
	Schemas map[string]*SchemaInfo `json:"schemas"`
	Stats   *DiscoveryStats        `json:"stats"`
	Error   error                  `json:"error,omitempty"`
}

// CachedSchema represents a cached schema with timestamp
type CachedSchema struct {
	Schema    *SchemaInfo
	Timestamp time.Time
}

// CRDInfo holds information about a CustomResourceDefinition
type CRDInfo struct {
	Name       string
	Group      string
	Version    string
	Kind       string
	Plural     string
	Singular   string
	Scope      string
}

// ReferencePattern represents different types of reference patterns
type ReferencePattern struct {
	FieldName   string
	KindHint    string
	APIVersion  string
	IsArray     bool
}

// SchemaSource represents the source of schema information
type SchemaSource string

const (
	SourceKubernetesAPI SchemaSource = "kubernetes-api"
	SourceCache         SchemaSource = "cache"
	SourceFallback      SchemaSource = "fallback"
)

// ErrorType represents different types of errors that can occur
type ErrorType string

const (
	ErrorTypeValidation      ErrorType = "validation"
	ErrorTypeNotFound        ErrorType = "not_found"
	ErrorTypePermission      ErrorType = "permission"
	ErrorTypeTimeout         ErrorType = "timeout"
	ErrorTypeConfiguration   ErrorType = "configuration"
)

// DiscoveryError represents an error during discovery with context
type DiscoveryError struct {
	Type         ErrorType
	Message      string
	Cause        error
	ResourceRef  *ResourceReference
	CorrelationID string
}

func (e *DiscoveryError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *DiscoveryError) Unwrap() error {
	return e.Cause
}

// NewDiscoveryError creates a new DiscoveryError
func NewDiscoveryError(errType ErrorType, message string, cause error, ref *ResourceReference, correlationID string) *DiscoveryError {
	return &DiscoveryError{
		Type:          errType,
		Message:       message,
		Cause:         cause,
		ResourceRef:   ref,
		CorrelationID: correlationID,
	}
}