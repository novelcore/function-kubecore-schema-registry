package interfaces

import (
	"context"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
)

// SchemaRepository defines the contract for CRD schema operations
type SchemaRepository interface {
	// GetCRDSchema retrieves a CRD schema by kind and API version
	GetCRDSchema(ctx context.Context, kind, apiVersion string) (*domain.SchemaInfo, error)
	
	// ListCRDs returns available CRDs matching the given criteria
	ListCRDs(ctx context.Context, labelSelector string) ([]*domain.CRDInfo, error)
	
	// ValidateSchema validates a schema structure
	ValidateSchema(schema *domain.SchemaInfo) error
}

// CacheProvider defines the contract for caching operations
type CacheProvider interface {
	// Get retrieves a cached item
	Get(key string) (*domain.SchemaInfo, bool)
	
	// Set stores an item in cache
	Set(key string, value *domain.SchemaInfo)
	
	// Size returns current cache size
	Size() int
	
	// Clear removes all cached items
	Clear()
}

// SchemaDiscoveryService defines the contract for schema discovery
type SchemaDiscoveryService interface {
	// DiscoverSchemas discovers schemas for given execution context
	DiscoverSchemas(ctx context.Context, execCtx *domain.ExecutionContext, opts *domain.DiscoveryOptions) (*domain.DiscoveryResult, error)
	
	// DiscoverTransitiveReferences discovers transitive schema dependencies
	DiscoverTransitiveReferences(ctx context.Context, schema *domain.SchemaInfo, visited map[string]bool, opts *domain.DiscoveryOptions) error
}

// ReferenceExtractor defines the contract for reference field extraction
type ReferenceExtractor interface {
	// ExtractReferences extracts reference fields from a resource spec
	ExtractReferences(spec map[string]interface{}) map[string]domain.ResourceReference
	
	// IsReferenceField determines if a field name indicates a reference
	IsReferenceField(fieldName string) bool
	
	// InferReferenceTarget infers target kind and API version from field patterns
	InferReferenceTarget(fieldName string, parentSchema *domain.SchemaInfo) *domain.ResourceReference
}

// SchemaFactory defines the contract for creating schemas
type SchemaFactory interface {
	// CreateSchema creates a schema from CRD data
	CreateSchema(crd interface{}, includeFullSchema bool) (*domain.SchemaInfo, error)
	
	// CreateFallbackSchema creates a basic fallback schema
	CreateFallbackSchema(ref domain.ResourceReference, includeFullSchema bool) *domain.SchemaInfo
}

// ContextExtractor defines the contract for extracting execution context
type ContextExtractor interface {
	// ExtractExecutionContext extracts context from XR
	ExtractExecutionContext(ctx context.Context, xr interface{}, correlationID string) (*domain.ExecutionContext, error)
}

// Logger defines the contract for logging operations
type Logger interface {
	Debug(msg string, keysAndValues ...interface{})
	Info(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
}