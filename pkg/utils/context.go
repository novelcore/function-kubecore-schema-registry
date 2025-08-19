package utils

import (
	"context"
	"fmt"

	"github.com/crossplane/function-sdk-go/resource"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// ContextExtractor implements the ContextExtractor interface
type ContextExtractor struct {
	refExtractor interfaces.ReferenceExtractor
	logger       interfaces.Logger
}

// NewContextExtractor creates a new context extractor
func NewContextExtractor(refExtractor interfaces.ReferenceExtractor, logger interfaces.Logger) interfaces.ContextExtractor {
	return &ContextExtractor{
		refExtractor: refExtractor,
		logger:       logger,
	}
}

// ExtractExecutionContext extracts context from XR
func (c *ContextExtractor) ExtractExecutionContext(ctx context.Context, xr interface{}, correlationID string) (*domain.ExecutionContext, error) {
	c.logger.Debug("Extracting execution context", "correlationId", correlationID)

	// Type assert to Composite resource
	composite, ok := xr.(*resource.Composite)
	if !ok {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"invalid composite resource type",
			nil,
			nil,
			correlationID,
		)
	}

	if composite == nil || composite.Resource == nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"composite resource is nil",
			nil,
			nil,
			correlationID,
		)
	}

	// Get the underlying object from the composite
	xrObj := composite.Resource.Object

	execCtx := &domain.ExecutionContext{
		DirectReferences: make(map[string]domain.ResourceReference),
	}

	// Extract metadata
	if metadata, ok := xrObj["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			execCtx.SourceXResource = name
		}

		// Extract claim information from labels
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			if claimName, ok := labels["crossplane.io/claim-name"].(string); ok {
				execCtx.ClaimName = claimName
			}
			if claimNamespace, ok := labels["crossplane.io/claim-namespace"].(string); ok {
				execCtx.ClaimNamespace = claimNamespace
			}
		}
	}

	// Extract direct references from spec
	if spec, ok := xrObj["spec"].(map[string]interface{}); ok {
		execCtx.DirectReferences = c.refExtractor.ExtractReferences(spec)
	}

	c.logger.Debug("Execution context extracted successfully",
		"correlationId", correlationID,
		"sourceXResource", execCtx.SourceXResource,
		"directReferences", len(execCtx.DirectReferences))

	return execCtx, nil
}

// ValidateExecutionContext validates an execution context
func (c *ContextExtractor) ValidateExecutionContext(execCtx *domain.ExecutionContext) error {
	if execCtx == nil {
		return fmt.Errorf("execution context is nil")
	}

	if execCtx.SourceXResource == "" {
		return fmt.Errorf("source XResource name is empty")
	}

	if execCtx.DirectReferences == nil {
		return fmt.Errorf("direct references map is nil")
	}

	return nil
}