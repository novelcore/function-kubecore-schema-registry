package service

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// DiscoveryService implements the SchemaDiscoveryService interface
type DiscoveryService struct {
	repository    interfaces.SchemaRepository
	cache         interfaces.CacheProvider
	factory       interfaces.SchemaFactory
	refExtractor  interfaces.ReferenceExtractor
	logger        interfaces.Logger
}

// NewDiscoveryService creates a new discovery service
func NewDiscoveryService(
	repository interfaces.SchemaRepository,
	cache interfaces.CacheProvider,
	factory interfaces.SchemaFactory,
	refExtractor interfaces.ReferenceExtractor,
	logger interfaces.Logger,
) interfaces.SchemaDiscoveryService {
	return &DiscoveryService{
		repository:   repository,
		cache:        cache,
		factory:      factory,
		refExtractor: refExtractor,
		logger:       logger,
	}
}

// DiscoverSchemas discovers schemas for given execution context
func (d *DiscoveryService) DiscoverSchemas(ctx context.Context, execCtx *domain.ExecutionContext, opts *domain.DiscoveryOptions) (*domain.DiscoveryResult, error) {
	startTime := time.Now()
	
	d.logger.Debug("Starting schema discovery",
		"correlationId", opts.CorrelationID,
		"enableTransitive", opts.EnableTransitive,
		"traversalDepth", opts.TraversalDepth)

	result := &domain.DiscoveryResult{
		Schemas: make(map[string]*domain.SchemaInfo),
		Stats: &domain.DiscoveryStats{
			TotalReferencesFound: len(execCtx.DirectReferences),
		},
	}

	visited := make(map[string]bool)

	// Discover schemas for direct references
	for fieldName, ref := range execCtx.DirectReferences {
		if ref.Name == "" {
			continue
		}

		// Check if schema is in cache before discovery
		schemaKey := d.getSchemaKey(ref)
		if _, exists := d.cache.Get(schemaKey); exists {
			result.Stats.CacheHits++
		} else {
			result.Stats.APICallsMade++
		}

		schema, err := d.discoverSchema(ctx, ref, opts)
		if err != nil {
			d.logger.Warn("Failed to discover schema for reference",
				"correlationId", opts.CorrelationID,
				"fieldName", fieldName,
				"refName", ref.Name,
				"error", err)
			continue
		}

		if schema != nil {
			// Set initial metadata
			schema.Depth = 0
			schema.ReferencePath = fieldName

			result.Schemas[fieldName] = schema
			result.Stats.SchemasRetrieved++

			// Count real vs fallback schemas
			if schema.Source == string(domain.SourceKubernetesAPI) || schema.Source == string(domain.SourceCache) {
				result.Stats.RealSchemasFound++
			}

			visited[schemaKey] = true

			// Perform transitive discovery if enabled
			if opts.EnableTransitive && opts.TraversalDepth > 0 {
				newOpts := *opts
				newOpts.TraversalDepth = opts.TraversalDepth - 1
				err := d.DiscoverTransitiveReferences(ctx, schema, visited, &newOpts)
				if err != nil {
					d.logger.Warn("Failed transitive discovery",
						"correlationId", opts.CorrelationID,
						"fieldName", fieldName,
						"error", err)
				}
				result.Stats.MaxDepthReached = max(result.Stats.MaxDepthReached, opts.TraversalDepth)
			}
		}
	}

	// Count all transitive schemas for accurate metrics
	d.countTransitiveSchemas(result.Schemas, result.Stats)

	// Calculate execution time
	result.Stats.ExecutionTimeMs = time.Since(startTime).Milliseconds()

	d.logger.Debug("Schema discovery completed",
		"correlationId", opts.CorrelationID,
		"schemasFound", len(result.Schemas),
		"statsRetrieved", result.Stats.SchemasRetrieved,
		"realSchemas", result.Stats.RealSchemasFound,
		"cacheHits", result.Stats.CacheHits,
		"apiCalls", result.Stats.APICallsMade)

	return result, nil
}

// DiscoverTransitiveReferences discovers transitive schema dependencies using real CRD analysis
func (d *DiscoveryService) DiscoverTransitiveReferences(ctx context.Context, schema *domain.SchemaInfo, visited map[string]bool, opts *domain.DiscoveryOptions) error {
	if opts.TraversalDepth <= 0 {
		return nil
	}

	d.logger.Debug("Performing transitive discovery",
		"correlationId", opts.CorrelationID,
		"schema", schema.Kind,
		"remainingDepth", opts.TraversalDepth,
		"referenceFields", len(schema.ReferenceFields))

	if schema.TransitiveRefs == nil {
		schema.TransitiveRefs = make(map[string]*domain.SchemaInfo)
	}

	// Process each reference field found in the schema
	for _, refField := range schema.ReferenceFields {
		d.logger.Debug("Processing reference field for transitive discovery",
			"correlationId", opts.CorrelationID,
			"refField", refField,
			"parentKind", schema.Kind)

		// Extract potential reference targets from the schema
		referenceTargets := d.extractReferenceTargets(schema, refField, opts.CorrelationID)

		for _, refTarget := range referenceTargets {
			transitiveKey := d.getSchemaKey(refTarget)
			if visited[transitiveKey] {
				d.logger.Debug("Reference already visited, skipping",
					"correlationId", opts.CorrelationID,
					"refTarget", refTarget.Kind)
				continue
			}

			visited[transitiveKey] = true

			transitiveSchema, err := d.discoverSchema(ctx, refTarget, opts)
			if err != nil {
				d.logger.Warn("Failed to discover transitive schema",
					"correlationId", opts.CorrelationID,
					"refField", refField,
					"refTarget", refTarget.Kind,
					"error", err)
				continue
			}

			if transitiveSchema != nil {
				// Set transitive metadata
				transitiveSchema.Depth = schema.Depth + 1
				if schema.ReferencePath != "" {
					transitiveSchema.ReferencePath = schema.ReferencePath + " -> " + refField
				} else {
					transitiveSchema.ReferencePath = refField
				}

				schema.TransitiveRefs[refField] = transitiveSchema

				d.logger.Debug("Successfully discovered transitive schema",
					"correlationId", opts.CorrelationID,
					"refField", refField,
					"transitiveKind", transitiveSchema.Kind,
					"depth", transitiveSchema.Depth)

				// Recursive call for next level
				newOpts := *opts
				newOpts.TraversalDepth = opts.TraversalDepth - 1
				err := d.DiscoverTransitiveReferences(ctx, transitiveSchema, visited, &newOpts)
				if err != nil {
					d.logger.Warn("Failed nested transitive discovery",
						"correlationId", opts.CorrelationID,
						"refField", refField,
						"error", err)
				}
			}
		}
	}

	return nil
}

// discoverSchema discovers schema for a given resource reference using real Kubernetes API
func (d *DiscoveryService) discoverSchema(ctx context.Context, ref domain.ResourceReference, opts *domain.DiscoveryOptions) (*domain.SchemaInfo, error) {
	schemaKey := d.getSchemaKey(ref)

	// Check cache first
	if cached, exists := d.cache.Get(schemaKey); exists {
		d.logger.Debug("Schema found in cache",
			"correlationId", opts.CorrelationID,
			"schemaKey", schemaKey)
		cached.Source = string(domain.SourceCache)
		return cached, nil
	}

	d.logger.Debug("Fetching schema from Kubernetes API",
		"correlationId", opts.CorrelationID,
		"schemaKey", schemaKey,
		"kind", ref.Kind,
		"apiVersion", ref.APIVersion)

	var schema *domain.SchemaInfo
	var err error

	// Try to get real CRD schema from Kubernetes API if repository is available
	if d.repository != nil {
		schema, err = d.repository.GetCRDSchema(ctx, ref.Kind, ref.APIVersion)
		if err != nil {
			d.logger.Warn("Failed to fetch real CRD schema, using fallback",
				"correlationId", opts.CorrelationID,
				"schemaKey", schemaKey,
				"error", err)
			schema = nil // Reset to trigger fallback creation
		}
	}

	// If repository is nil or schema retrieval failed, use fallback
	if schema == nil {
		d.logger.Debug("Using fallback schema",
			"correlationId", opts.CorrelationID,
			"schemaKey", schemaKey,
			"reason", "repository unavailable or schema not found")
		schema = d.factory.CreateFallbackSchema(ref, opts.IncludeFullSchema)
	}

	// Cache the schema
	d.cache.Set(schemaKey, schema)

	return schema, nil
}

// extractReferenceTargets extracts potential reference targets from a reference field
func (d *DiscoveryService) extractReferenceTargets(schema *domain.SchemaInfo, refField string, correlationID string) []domain.ResourceReference {
	var targets []domain.ResourceReference

	// Try to infer kind and apiVersion from the reference field name
	if refTarget := d.refExtractor.InferReferenceTarget(refField, schema); refTarget != nil {
		targets = append(targets, *refTarget)
	}

	return targets
}

// getSchemaKey generates a unique key for schema caching
func (d *DiscoveryService) getSchemaKey(ref domain.ResourceReference) string {
	// For caching, we only need APIVersion and Kind (not specific instance name)
	// since the schema is the same for all instances of a CRD
	return fmt.Sprintf("%s/%s", ref.APIVersion, ref.Kind)
}

// countTransitiveSchemas recursively counts all transitive schemas for accurate metrics
func (d *DiscoveryService) countTransitiveSchemas(schemas map[string]*domain.SchemaInfo, stats *domain.DiscoveryStats) {
	for _, schema := range schemas {
		d.countTransitiveInSchema(schema, stats)
	}
}

// countTransitiveInSchema counts transitive schemas within a single schema
func (d *DiscoveryService) countTransitiveInSchema(schema *domain.SchemaInfo, stats *domain.DiscoveryStats) {
	if schema.TransitiveRefs == nil {
		return
	}

	for _, transitiveSchema := range schema.TransitiveRefs {
		stats.SchemasRetrieved++
		if transitiveSchema.Source == string(domain.SourceKubernetesAPI) || transitiveSchema.Source == string(domain.SourceCache) {
			stats.RealSchemasFound++
		}
		// Recursively count nested transitive schemas
		d.countTransitiveInSchema(transitiveSchema, stats)
	}
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}