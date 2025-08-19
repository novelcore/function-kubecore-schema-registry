package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/crossplane/function-kubecore-schema-registry/internal/config"
	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// ReferenceExtractor implements the ReferenceExtractor interface
type ReferenceExtractor struct {
	config *config.Config
	logger interfaces.Logger
}

// NewReferenceExtractor creates a new reference extractor
func NewReferenceExtractor(config *config.Config, logger interfaces.Logger) interfaces.ReferenceExtractor {
	return &ReferenceExtractor{
		config: config,
		logger: logger,
	}
}

// ExtractReferences extracts reference fields from a resource spec
func (r *ReferenceExtractor) ExtractReferences(spec map[string]interface{}) map[string]domain.ResourceReference {
	refs := make(map[string]domain.ResourceReference)
	r.extractReferencesFromSpec(spec, refs, "")
	return refs
}

// IsReferenceField determines if a field name indicates a reference
func (r *ReferenceExtractor) IsReferenceField(fieldName string) bool {
	// Enhanced patterns beyond just *Ref
	patterns := []string{
		"Ref$",           // ends with Ref
		"Refs$",          // ends with Refs (arrays)
		"Reference$",     // ends with Reference
		"References$",    // ends with References
		"Config$",        // configuration references
		"Provider$",      // provider references
		"Secret$",        // secret references
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, fieldName); matched {
			return true
		}
	}

	// Additional logic for nested reference patterns
	if strings.Contains(fieldName, "providerConfig") ||
		strings.Contains(fieldName, "secretRef") ||
		strings.Contains(fieldName, "configMapRef") {
		return true
	}

	return false
}

// InferReferenceTarget infers target kind and API version from field patterns
func (r *ReferenceExtractor) InferReferenceTarget(fieldName string, parentSchema *domain.SchemaInfo) *domain.ResourceReference {
	// Check known patterns first
	if pattern, exists := r.config.KnownReferencePatterns[fieldName]; exists {
		return &domain.ResourceReference{
			Kind:       pattern.KindHint,
			APIVersion: pattern.APIVersion,
		}
	}

	// Try pattern matching with regex
	if kind := r.extractKindFromRefField(fieldName); kind != "" {
		// Try to infer apiVersion from parent schema or use common patterns
		apiVersion := r.inferAPIVersionFromKind(kind, parentSchema)
		return &domain.ResourceReference{
			Kind:       kind,
			APIVersion: apiVersion,
		}
	}

	return nil
}

// extractReferencesFromSpec recursively extracts reference fields from spec with enhanced patterns
func (r *ReferenceExtractor) extractReferencesFromSpec(spec map[string]interface{}, refs map[string]domain.ResourceReference, correlationID string) {
	for key, value := range spec {
		// Enhanced reference pattern detection
		if r.IsReferenceField(key) {
			r.processReferenceField(key, value, refs, correlationID)
		} else if nested, ok := value.(map[string]interface{}); ok {
			r.extractReferencesFromSpec(nested, refs, correlationID)
		}
	}
}

// processReferenceField processes a reference field and extracts reference info
func (r *ReferenceExtractor) processReferenceField(fieldName string, value interface{}, refs map[string]domain.ResourceReference, correlationID string) {
	switch v := value.(type) {
	case map[string]interface{}:
		ref := domain.ResourceReference{}
		if name, ok := v["name"].(string); ok {
			ref.Name = name
		}
		if namespace, ok := v["namespace"].(string); ok {
			ref.Namespace = namespace
		}
		if kind, ok := v["kind"].(string); ok {
			ref.Kind = kind
		}
		if apiVersion, ok := v["apiVersion"].(string); ok {
			ref.APIVersion = apiVersion
		}

		// If kind/apiVersion not provided in the reference, infer from field name
		if ref.Kind == "" || ref.APIVersion == "" {
			inferred := r.InferReferenceTarget(fieldName, nil)
			if inferred != nil {
				if ref.Kind == "" {
					ref.Kind = inferred.Kind
				}
				if ref.APIVersion == "" {
					ref.APIVersion = inferred.APIVersion
				}
			}
		}

		if ref.Name != "" {
			refs[fieldName] = ref
			r.logger.Debug("Reference field processed",
				"correlationId", correlationID,
				"fieldName", fieldName,
				"refName", ref.Name,
				"kind", ref.Kind,
				"apiVersion", ref.APIVersion)
		}
	case []interface{}:
		// Handle array of references
		for i, item := range v {
			if refMap, ok := item.(map[string]interface{}); ok {
				arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)
				r.processReferenceField(arrayFieldName, refMap, refs, correlationID)
			}
		}
	}
}

// extractKindFromRefField extracts kind from reference field name
func (r *ReferenceExtractor) extractKindFromRefField(refField string) string {
	// Remove common suffixes and convert to PascalCase
	patterns := []string{"Ref", "Refs", "Reference", "References"}
	kind := refField

	for _, suffix := range patterns {
		if strings.HasSuffix(kind, suffix) {
			kind = strings.TrimSuffix(kind, suffix)
			break
		}
	}

	// Convert camelCase to PascalCase
	if len(kind) > 0 {
		return strings.ToUpper(kind[:1]) + kind[1:]
	}

	return ""
}

// inferAPIVersionFromKind tries to infer API version from kind
func (r *ReferenceExtractor) inferAPIVersionFromKind(kind string, parentSchema *domain.SchemaInfo) string {
	// Common kind to API version mappings
	mappings := map[string]string{
		"Secret":             "v1",
		"ConfigMap":          "v1",
		"ServiceAccount":     "v1",
		"ProviderConfig":     "pkg.crossplane.io/v1",
		"GitHubProject":      "github.platform.kubecore.io/v1alpha1",
		"GithubProvider":     "github.platform.kubecore.io/v1alpha1",
		"QualityGate":        "ci.platform.kubecore.io/v1alpha1",
	}

	if apiVersion, exists := mappings[kind]; exists {
		return apiVersion
	}

	// If no mapping found, try to use parent's group with v1alpha1
	if parentSchema != nil && parentSchema.APIVersion != "" {
		if group, _, err := r.parseAPIVersion(parentSchema.APIVersion); err == nil && group != "" {
			return group + "/v1alpha1"
		}
	}

	// Default fallback
	return "v1alpha1"
}

// parseAPIVersion splits apiVersion into group and version
func (r *ReferenceExtractor) parseAPIVersion(apiVersion string) (group, version string, err error) {
	if apiVersion == "" {
		return "", "", fmt.Errorf("empty API version")
	}

	// Handle core API (e.g., "v1")
	if !strings.Contains(apiVersion, "/") {
		return "", apiVersion, nil
	}

	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid API version format: %s", apiVersion)
	}

	return parts[0], parts[1], nil
}