package service

import (
	"sort"
	"strings"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// ConverterService handles conversion from complex schema discovery results to Go template-friendly output
type ConverterService struct {
	logger interfaces.Logger
}

// NewConverterService creates a new converter service
func NewConverterService(logger interfaces.Logger) *ConverterService {
	return &ConverterService{
		logger: logger,
	}
}

// ConvertToSchemaRegistryOutput converts complex discovery results to simplified Go template-friendly format
func (c *ConverterService) ConvertToSchemaRegistryOutput(
	discoveryResult *domain.DiscoveryResult,
	execCtx *domain.ExecutionContext,
	correlationID string,
) (*domain.SchemaRegistryOutput, error) {
	c.logger.Debug("Converting discovery results to schema registry output",
		"correlationId", correlationID,
		"schemasCount", len(discoveryResult.Schemas))

	// Build discovered resources list
	discoveredResources := c.buildDiscoveredResources(discoveryResult.Schemas, execCtx, correlationID)
	
	// Build simplified schemas
	resourceSchemas := c.buildResourceSchemas(discoveryResult.Schemas, correlationID)
	
	// Build reference chains
	referenceChains := c.buildReferenceChains(discoveredResources, correlationID)
	
	// Group resources by kind
	resourcesByKind := c.groupResourcesByKind(discoveredResources)

	// Update stats with new format
	stats := c.convertDiscoveryStats(discoveryResult.Stats, len(discoveredResources), len(resourceSchemas))

	output := &domain.SchemaRegistryOutput{
		DiscoveredResources: discoveredResources,
		ResourceSchemas:     resourceSchemas,
		ReferenceChains:     referenceChains,
		ResourcesByKind:     resourcesByKind,
		DiscoveryStats:      *stats,
	}

	c.logger.Info("Schema registry output conversion completed",
		"correlationId", correlationID,
		"discoveredResourcesCount", len(discoveredResources),
		"resourceSchemasCount", len(resourceSchemas),
		"referenceChainsCount", len(referenceChains),
		"resourceKindsCount", len(resourcesByKind))

	return output, nil
}

// buildDiscoveredResources creates a flat list of discovered resources from the complex schema map
func (c *ConverterService) buildDiscoveredResources(
	schemas map[string]*domain.SchemaInfo,
	execCtx *domain.ExecutionContext,
	correlationID string,
) []domain.DiscoveredResource {
	var resources []domain.DiscoveredResource
	visited := make(map[string]bool)

	// Process direct references first
	for refKey, ref := range execCtx.DirectReferences {
		if visited[ref.Name] {
			continue
		}
		visited[ref.Name] = true

		resource := domain.DiscoveredResource{
			Name:         ref.Name,
			Kind:         ref.Kind,
			APIVersion:   ref.APIVersion,
			Namespace:    ref.Namespace,
			ReferencedBy: refKey,
			Depth:        0,
			Source:       "direct",
		}
		resources = append(resources, resource)

		// Process transitive references for this resource
		if schema, exists := schemas[refKey]; exists && schema.TransitiveRefs != nil {
			c.processTransitiveReferences(schema.TransitiveRefs, ref.Name, &resources, visited, 1)
		}
	}

	c.logger.Debug("Built discovered resources list",
		"correlationId", correlationID,
		"totalResources", len(resources))

	return resources
}

// processTransitiveReferences recursively processes transitive references
func (c *ConverterService) processTransitiveReferences(
	transitiveRefs map[string]*domain.SchemaInfo,
	parentResource string,
	resources *[]domain.DiscoveredResource,
	visited map[string]bool,
	depth int,
) {
	for refKey, schema := range transitiveRefs {
		// Extract resource name from reference path or use Kind as fallback
		resourceName := c.extractResourceNameFromSchema(schema, refKey)
		
		if visited[resourceName] {
			continue
		}
		visited[resourceName] = true

		resource := domain.DiscoveredResource{
			Name:           resourceName,
			Kind:           schema.Kind,
			APIVersion:     schema.APIVersion,
			ReferencedBy:   refKey,
			Depth:          depth,
			Source:         "transitive",
			ParentResource: parentResource,
		}
		*resources = append(*resources, resource)

		// Recursively process nested transitive references
		if schema.TransitiveRefs != nil && depth < 5 { // Prevent infinite recursion
			c.processTransitiveReferences(schema.TransitiveRefs, resourceName, resources, visited, depth+1)
		}
	}
}

// extractResourceNameFromSchema extracts a resource name from schema information
func (c *ConverterService) extractResourceNameFromSchema(schema *domain.SchemaInfo, refKey string) string {
	// Try to extract from reference path if available
	if schema.ReferencePath != "" {
		parts := strings.Split(schema.ReferencePath, " -> ")
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			// Remove common prefixes/suffixes to get clean name
			cleanName := strings.TrimSuffix(lastPart, "Ref")
			cleanName = strings.TrimSuffix(cleanName, "Config")
			if cleanName != "" {
				return cleanName
			}
		}
	}

	// Fallback to using Kind with some cleanup
	if schema.Kind != "" {
		// Convert CamelCase to kebab-case for common naming patterns
		kindName := schema.Kind
		// Add common naming patterns for well-known resources
		switch kindName {
		case "ProviderConfig":
			return "kubesys-enhanced" // Common default name
		case "GithubProvider":
			return "gh-default" // Common default name
		case "SecretStore":
			return "secret-store-default"
		default:
			// Convert to lowercase for generic naming
			return strings.ToLower(kindName)
		}
	}

	// Final fallback to reference key
	return refKey
}

// buildResourceSchemas creates simplified schemas from complex OpenAPI schemas
func (c *ConverterService) buildResourceSchemas(
	schemas map[string]*domain.SchemaInfo,
	correlationID string,
) map[string]domain.SimplifiedSchema {
	resourceSchemas := make(map[string]domain.SimplifiedSchema)

	for _, schema := range schemas {
		if schema.Kind == "" {
			continue
		}

		simplified := domain.SimplifiedSchema{
			APIVersion:      schema.APIVersion,
			ReferenceFields: c.extractReferenceFields(schema),
			RequiredFields:  schema.RequiredFields,
		}

		resourceSchemas[schema.Kind] = simplified
	}

	c.logger.Debug("Built simplified resource schemas",
		"correlationId", correlationID,
		"schemasCount", len(resourceSchemas))

	return resourceSchemas
}

// extractReferenceFields extracts reference field information from schema
func (c *ConverterService) extractReferenceFields(schema *domain.SchemaInfo) []domain.ReferenceField {
	var refFields []domain.ReferenceField

	for _, fieldName := range schema.ReferenceFields {
		// Try to determine target kind from field name patterns
		targetKind := c.inferTargetKindFromFieldName(fieldName)
		required := c.isFieldRequired(fieldName, schema.RequiredFields)

		refField := domain.ReferenceField{
			Name:       fieldName,
			TargetKind: targetKind,
			Required:   required,
		}
		refFields = append(refFields, refField)
	}

	return refFields
}

// inferTargetKindFromFieldName tries to determine the target Kind from field name patterns
func (c *ConverterService) inferTargetKindFromFieldName(fieldName string) string {
	// Remove common suffixes to get base name
	baseName := strings.TrimSuffix(fieldName, "Ref")
	baseName = strings.TrimSuffix(baseName, "Config")
	baseName = strings.TrimSuffix(baseName, "Store")

	// Common patterns
	switch {
	case strings.Contains(fieldName, "github"):
		if strings.Contains(fieldName, "provider") {
			return "GithubProvider"
		}
		return "GitHubProject"
	case strings.Contains(fieldName, "kubernetes") || strings.Contains(fieldName, "k8s"):
		return "ProviderConfig"
	case strings.Contains(fieldName, "secret"):
		return "SecretStore"
	case strings.Contains(fieldName, "composition"):
		if strings.Contains(fieldName, "revision") {
			return "CompositionRevision"
		}
		return "Composition"
	case strings.Contains(fieldName, "provider"):
		return "ProviderConfig"
	default:
		// Convert to PascalCase as best guess
		return strings.Title(baseName)
	}
}

// isFieldRequired checks if a field is in the required fields list
func (c *ConverterService) isFieldRequired(fieldName string, requiredFields []string) bool {
	for _, required := range requiredFields {
		if required == fieldName {
			return true
		}
	}
	return false
}

// buildReferenceChains creates reference chains showing resource relationships
func (c *ConverterService) buildReferenceChains(
	resources []domain.DiscoveredResource,
	correlationID string,
) []domain.ReferenceChain {
	var chains []domain.ReferenceChain
	chainMap := make(map[string]*domain.ReferenceChain)

	// Build chains by following parent relationships
	for _, resource := range resources {
		if resource.Source == "direct" {
			// Start a new chain for direct resources
			chain := &domain.ReferenceChain{
				Path:      resource.ReferencedBy,
				Resources: []string{resource.Name},
				Kinds:     []string{resource.Kind},
			}
			chainMap[resource.Name] = chain
		} else if resource.Source == "transitive" && resource.ParentResource != "" {
			// Extend existing chain
			if parentChain, exists := chainMap[resource.ParentResource]; exists {
				parentChain.Path += " -> " + resource.ReferencedBy
				parentChain.Resources = append(parentChain.Resources, resource.Name)
				parentChain.Kinds = append(parentChain.Kinds, resource.Kind)
				chainMap[resource.Name] = parentChain
			}
		}
	}

	// Convert map to slice and deduplicate
	seen := make(map[string]bool)
	for _, chain := range chainMap {
		if !seen[chain.Path] {
			chains = append(chains, *chain)
			seen[chain.Path] = true
		}
	}

	c.logger.Debug("Built reference chains",
		"correlationId", correlationID,
		"chainsCount", len(chains))

	return chains
}

// groupResourcesByKind groups discovered resources by their Kind
func (c *ConverterService) groupResourcesByKind(resources []domain.DiscoveredResource) map[string][]domain.DiscoveredResource {
	grouped := make(map[string][]domain.DiscoveredResource)

	for _, resource := range resources {
		grouped[resource.Kind] = append(grouped[resource.Kind], resource)
	}

	// Sort resources within each group by depth then name for consistent output
	for kind := range grouped {
		sort.Slice(grouped[kind], func(i, j int) bool {
			if grouped[kind][i].Depth != grouped[kind][j].Depth {
				return grouped[kind][i].Depth < grouped[kind][j].Depth
			}
			return grouped[kind][i].Name < grouped[kind][j].Name
		})
	}

	return grouped
}

// convertDiscoveryStats converts old stats format to new format
func (c *ConverterService) convertDiscoveryStats(
	oldStats *domain.DiscoveryStats,
	resourcesCount, schemasCount int,
) *domain.DiscoveryStats {
	newStats := &domain.DiscoveryStats{
		TotalResourcesFound:   resourcesCount,
		TotalSchemasRetrieved: schemasCount,
		MaxDepthReached:       oldStats.MaxDepthReached,
		ExecutionTimeMs:       oldStats.ExecutionTimeMs,
		// Preserve legacy fields for backward compatibility
		TotalReferencesFound: oldStats.TotalReferencesFound,
		SchemasRetrieved:     oldStats.SchemasRetrieved,
		RealSchemasFound:     oldStats.RealSchemasFound,
		CacheHits:            oldStats.CacheHits,
		APICallsMade:         oldStats.APICallsMade,
	}

	return newStats
}