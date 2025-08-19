package factory

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// SchemaFactory implements the SchemaFactory interface
type SchemaFactory struct {
	logger interfaces.Logger
}

// NewSchemaFactory creates a new schema factory
func NewSchemaFactory(logger interfaces.Logger) interfaces.SchemaFactory {
	return &SchemaFactory{
		logger: logger,
	}
}

// CreateSchema creates a schema from CRD data
func (s *SchemaFactory) CreateSchema(crd interface{}, includeFullSchema bool) (*domain.SchemaInfo, error) {
	// Type assert to CRD
	crdObj, ok := crd.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"invalid CRD type",
			nil,
			nil,
			"",
		)
	}

	// Get the served version
	var servedVersion *apiextensionsv1.CustomResourceDefinitionVersion
	for _, version := range crdObj.Spec.Versions {
		if version.Served && version.Storage {
			servedVersion = &version
			break
		}
	}

	if servedVersion == nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"no served version found in CRD",
			nil,
			nil,
			"",
		)
	}

	apiVersion := crdObj.Spec.Group + "/" + servedVersion.Name
	if crdObj.Spec.Group == "" {
		apiVersion = servedVersion.Name
	}

	schema := &domain.SchemaInfo{
		Kind:            crdObj.Spec.Names.Kind,
		APIVersion:      apiVersion,
		ReferenceFields: []string{}, // Will be populated by reference extraction
		RequiredFields:  []string{"metadata", "spec"},
		Source:          string(domain.SourceKubernetesAPI),
	}

	if includeFullSchema && servedVersion.Schema != nil && servedVersion.Schema.OpenAPIV3Schema != nil {
		schema.OpenAPIV3Schema = servedVersion.Schema.OpenAPIV3Schema
		schema.RequiredFields = s.extractRequiredFields(servedVersion.Schema.OpenAPIV3Schema)
		schema.ReferenceFields = s.extractReferenceFields(servedVersion.Schema.OpenAPIV3Schema)
	}

	s.logger.Debug("Created schema from CRD",
		"kind", schema.Kind,
		"apiVersion", schema.APIVersion,
		"referenceFields", len(schema.ReferenceFields))

	return schema, nil
}

// CreateFallbackSchema creates a basic fallback schema
func (s *SchemaFactory) CreateFallbackSchema(ref domain.ResourceReference, includeFullSchema bool) *domain.SchemaInfo {
	schema := &domain.SchemaInfo{
		Kind:            ref.Kind,
		APIVersion:      ref.APIVersion,
		ReferenceFields: []string{}, // Will be populated by enhanced reference detection
		RequiredFields:  []string{"metadata", "spec"},
		Source:          string(domain.SourceFallback),
	}

	if includeFullSchema {
		schema.OpenAPIV3Schema = &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"metadata": {Type: "object"},
				"spec":     {Type: "object"},
				"status":   {Type: "object"},
			},
			Required: []string{"metadata", "spec"},
		}
	}

	s.logger.Debug("Created fallback schema",
		"kind", schema.Kind,
		"apiVersion", schema.APIVersion)

	return schema
}

// extractRequiredFields extracts required fields from OpenAPI schema
func (s *SchemaFactory) extractRequiredFields(schema *apiextensionsv1.JSONSchemaProps) []string {
	if schema == nil {
		return []string{"metadata", "spec"}
	}
	if len(schema.Required) == 0 {
		return []string{"metadata", "spec"}
	}
	return schema.Required
}

// extractReferenceFields extracts reference fields from OpenAPI schema
func (s *SchemaFactory) extractReferenceFields(schema *apiextensionsv1.JSONSchemaProps) []string {
	var refFields []string

	if schema == nil || schema.Properties == nil {
		return refFields
	}

	// Look for reference fields in spec
	if specSchema, exists := schema.Properties["spec"]; exists {
		refFields = append(refFields, s.findReferenceFieldsInProperties(specSchema.Properties, "")...)
	}

	return refFields
}

// findReferenceFieldsInProperties recursively finds reference fields in schema properties
func (s *SchemaFactory) findReferenceFieldsInProperties(properties map[string]apiextensionsv1.JSONSchemaProps, prefix string) []string {
	var refFields []string

	for fieldName, fieldSchema := range properties {
		fullFieldName := fieldName
		if prefix != "" {
			fullFieldName = prefix + "." + fieldName
		}

		// Check if this field is a reference field
		if s.isReferenceField(fieldName) {
			refFields = append(refFields, fullFieldName)
		}

		// Recursively check nested objects
		if fieldSchema.Type == "object" && fieldSchema.Properties != nil {
			nestedRefs := s.findReferenceFieldsInProperties(fieldSchema.Properties, fullFieldName)
			refFields = append(refFields, nestedRefs...)
		}

		// Check array items
		if fieldSchema.Type == "array" && fieldSchema.Items != nil && fieldSchema.Items.Schema != nil {
			if fieldSchema.Items.Schema.Properties != nil {
				arrayRefs := s.findReferenceFieldsInProperties(fieldSchema.Items.Schema.Properties, fullFieldName+"[]")
				refFields = append(refFields, arrayRefs...)
			}
		}
	}

	return refFields
}

// isReferenceField determines if a field name indicates a reference
func (s *SchemaFactory) isReferenceField(fieldName string) bool {
	// Enhanced patterns beyond just *Ref
	referencePatterns := []string{
		"Ref", "Refs", "Reference", "References",
		"Config", "Provider", "Secret",
	}

	for _, pattern := range referencePatterns {
		if len(fieldName) >= len(pattern) {
			if fieldName[len(fieldName)-len(pattern):] == pattern {
				return true
			}
		}
	}

	// Additional logic for nested reference patterns
	containsPatterns := []string{
		"providerConfig", "secretRef", "configMapRef",
	}
	
	fieldLower := fieldName
	for _, pattern := range containsPatterns {
		if len(fieldLower) >= len(pattern) {
			for i := 0; i <= len(fieldLower)-len(pattern); i++ {
				if fieldLower[i:i+len(pattern)] == pattern {
					return true
				}
			}
		}
	}

	return false
}