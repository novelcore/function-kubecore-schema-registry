package dynamic

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/pkg/errors"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// SchemaParser interface for parsing OpenAPI schemas
type SchemaParser interface {
	ParseOpenAPISchema(schema *apiextv1.JSONSchemaProps) (*ResourceSchema, error)
	ExtractFieldDefinitions(schema *apiextv1.JSONSchemaProps, path string) map[string]*FieldDefinition
	ValidateSchema(schema *apiextv1.JSONSchemaProps) error
}

// DefaultSchemaParser implements schema parsing with caching
type DefaultSchemaParser struct {
	logger      logging.Logger
	fieldCache  map[string]*FieldDefinition
	schemaCache map[string]*ResourceSchema
	mu          sync.RWMutex
}

// NewSchemaParser creates a new schema parser
func NewSchemaParser(logger logging.Logger) *DefaultSchemaParser {
	return &DefaultSchemaParser{
		logger:      logger,
		fieldCache:  make(map[string]*FieldDefinition),
		schemaCache: make(map[string]*ResourceSchema),
	}
}

// ParseOpenAPISchema parses an OpenAPI v3 schema into ResourceSchema format
func (p *DefaultSchemaParser) ParseOpenAPISchema(schema *apiextv1.JSONSchemaProps) (*ResourceSchema, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}

	// Generate cache key based on schema content
	cacheKey := p.generateSchemaCacheKey(schema)

	// Check cache first
	if cached := p.getCachedSchema(cacheKey); cached != nil {
		return cached, nil
	}

	// Parse the schema
	resourceSchema, err := p.parseSchemaRecursive(schema, "")
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse schema")
	}

	// Cache the result
	p.cacheSchema(cacheKey, resourceSchema)

	return resourceSchema, nil
}

// parseSchemaRecursive recursively parses a schema and its nested properties
func (p *DefaultSchemaParser) parseSchemaRecursive(schema *apiextv1.JSONSchemaProps, path string) (*ResourceSchema, error) {
	resourceSchema := &ResourceSchema{
		Fields:      make(map[string]*FieldDefinition),
		Description: schema.Description,
		Required:    schema.Required,
		Properties:  make(map[string]*ResourceSchema),
	}

	// Parse properties
	if schema.Properties != nil {
		for propName, propSchema := range schema.Properties {
			fieldPath := p.buildFieldPath(path, propName)

			field, err := p.parseFieldDefinitionWithValidation(propName, &propSchema, fieldPath)
			if err != nil {
				p.logger.Debug("Failed to parse field", "field", propName, "path", fieldPath, "error", err)
				// Continue parsing other fields instead of failing completely
				continue
			}

			resourceSchema.Fields[propName] = field

			// If this field has nested properties, create a nested ResourceSchema
			if field.Properties != nil && len(field.Properties) > 0 {
				nestedSchema, err := p.parseSchemaRecursive(&propSchema, fieldPath)
				if err != nil {
					p.logger.Debug("Failed to parse nested schema", "field", propName, "error", err)
					continue
				}
				resourceSchema.Properties[propName] = nestedSchema
			}
		}
	}

	return resourceSchema, nil
}

// parseFieldDefinitionWithValidation parses a field definition with validation
func (p *DefaultSchemaParser) parseFieldDefinitionWithValidation(name string, schema *apiextv1.JSONSchemaProps, path string) (*FieldDefinition, error) {
	// Validate schema first
	if err := p.ValidateSchema(schema); err != nil {
		return nil, errors.Wrapf(err, "invalid schema for field %s", name)
	}

	// Check cache
	cacheKey := p.generateFieldCacheKey(path, schema)
	if cached := p.getCachedField(cacheKey); cached != nil {
		return cached, nil
	}

	field := &FieldDefinition{
		Type:        p.inferFieldType(schema),
		Format:      schema.Format,
		Description: schema.Description,
		Required:    false, // Will be set by parent based on required array
		Pattern:     schema.Pattern,
		Default:     p.extractDefault(schema),
	}

	// Handle enum values
	if schema.Enum != nil {
		field.Enum = p.parseEnumValues(schema.Enum)
	}

	// Handle nested properties recursively
	if schema.Properties != nil {
		field.Properties = make(map[string]*FieldDefinition)
		for propName, propSchema := range schema.Properties {
			nestedPath := p.buildFieldPath(path, propName)
			nestedField, err := p.parseFieldDefinitionWithValidation(propName, &propSchema, nestedPath)
			if err != nil {
				p.logger.Debug("Failed to parse nested field", "field", propName, "parent", name, "error", err)
				continue
			}
			field.Properties[propName] = nestedField
		}
	}

	// Handle array items
	if schema.Items != nil && schema.Items.Schema != nil {
		itemPath := p.buildFieldPath(path, "[]")
		itemField, err := p.parseFieldDefinitionWithValidation("", schema.Items.Schema, itemPath)
		if err != nil {
			p.logger.Debug("Failed to parse array items", "field", name, "error", err)
		} else {
			field.Items = itemField
		}
	}

	// Cache the result
	p.cacheField(cacheKey, field)

	return field, nil
}

// ExtractFieldDefinitions extracts field definitions from a schema at a specific path
func (p *DefaultSchemaParser) ExtractFieldDefinitions(schema *apiextv1.JSONSchemaProps, path string) map[string]*FieldDefinition {
	if schema == nil || schema.Properties == nil {
		return make(map[string]*FieldDefinition)
	}

	fields := make(map[string]*FieldDefinition)

	for propName, propSchema := range schema.Properties {
		fieldPath := p.buildFieldPath(path, propName)

		field, err := p.parseFieldDefinitionWithValidation(propName, &propSchema, fieldPath)
		if err != nil {
			p.logger.Debug("Failed to extract field definition", "field", propName, "path", fieldPath, "error", err)
			continue
		}

		fields[propName] = field
	}

	return fields
}

// ValidateSchema validates that a schema is well-formed
func (p *DefaultSchemaParser) ValidateSchema(schema *apiextv1.JSONSchemaProps) error {
	if schema == nil {
		return fmt.Errorf("schema is nil")
	}

	// Basic validation checks
	if schema.Type == "" && schema.Properties == nil && schema.Items == nil {
		// This might be a reference or a complex schema without explicit type
		// Don't treat as error, but log for debugging
		p.logger.Debug("Schema has no explicit type, properties, or items")
	}

	// Validate enum values if present
	if schema.Enum != nil && len(schema.Enum) == 0 {
		return fmt.Errorf("enum array is empty")
	}

	// Validate array items if type is array
	if schema.Type == "array" && schema.Items == nil {
		return fmt.Errorf("array type must have items definition")
	}

	// Validate object properties if type is object
	if schema.Type == "object" && schema.Properties == nil && schema.AdditionalProperties == nil {
		// This is acceptable - object without properties
		p.logger.Debug("Object type without properties or additionalProperties")
	}

	return nil
}

// inferFieldType infers the field type from schema
func (p *DefaultSchemaParser) inferFieldType(schema *apiextv1.JSONSchemaProps) string {
	// Explicit type
	if schema.Type != "" {
		return schema.Type
	}

	// Infer from properties
	if schema.Properties != nil {
		return "object"
	}

	// Infer from items
	if schema.Items != nil {
		return "array"
	}

	// Infer from enum
	if schema.Enum != nil {
		return "string" // Most enums are string-based
	}

	// Infer from format
	if schema.Format != "" {
		switch schema.Format {
		case "date", "date-time", "email", "hostname", "ipv4", "ipv6", "uri", "uuid":
			return "string"
		case "int32", "int64":
			return "integer"
		case "float", "double":
			return "number"
		case "byte", "binary":
			return "string"
		}
	}

	// Default fallback
	return "string"
}

// extractDefault safely extracts default value
func (p *DefaultSchemaParser) extractDefault(schema *apiextv1.JSONSchemaProps) interface{} {
	if schema.Default == nil {
		return nil
	}

	// Unmarshal JSON to get the actual value
	var defaultValue interface{}
	if err := json.Unmarshal(schema.Default.Raw, &defaultValue); err != nil {
		p.logger.Debug("Failed to unmarshal default value", "error", err)
		return nil
	}

	return defaultValue
}

// parseEnumValues parses enum values from JSON
func (p *DefaultSchemaParser) parseEnumValues(enum []apiextv1.JSON) []string {
	var values []string
	for _, val := range enum {
		// Unmarshal JSON to get the actual value
		var enumValue interface{}
		if err := json.Unmarshal(val.Raw, &enumValue); err != nil {
			p.logger.Debug("Failed to unmarshal enum value", "error", err)
			continue
		}

		// Try to convert to string
		switch v := enumValue.(type) {
		case string:
			values = append(values, v)
		case int, int32, int64, float32, float64:
			values = append(values, fmt.Sprintf("%v", v))
		case bool:
			values = append(values, fmt.Sprintf("%t", v))
		default:
			values = append(values, fmt.Sprintf("%v", v))
		}
	}
	return values
}

// buildFieldPath builds a JSON path for a field
func (p *DefaultSchemaParser) buildFieldPath(parentPath, fieldName string) string {
	if parentPath == "" {
		return fieldName
	}
	if fieldName == "" {
		return parentPath
	}
	return fmt.Sprintf("%s.%s", parentPath, fieldName)
}

// Cache management methods

func (p *DefaultSchemaParser) generateSchemaCacheKey(schema *apiextv1.JSONSchemaProps) string {
	// Generate a simple hash-like key based on schema content
	key := fmt.Sprintf("schema_%s_%d_%d",
		schema.Type,
		len(schema.Properties),
		len(schema.Required))

	if schema.Description != "" {
		key += "_" + fmt.Sprintf("%d", len(schema.Description))
	}

	return key
}

func (p *DefaultSchemaParser) generateFieldCacheKey(path string, schema *apiextv1.JSONSchemaProps) string {
	return fmt.Sprintf("field_%s_%s_%s", path, schema.Type, schema.Format)
}

func (p *DefaultSchemaParser) getCachedSchema(key string) *ResourceSchema {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.schemaCache[key]
}

func (p *DefaultSchemaParser) cacheSchema(key string, schema *ResourceSchema) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.schemaCache[key] = schema
}

func (p *DefaultSchemaParser) getCachedField(key string) *FieldDefinition {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.fieldCache[key]
}

func (p *DefaultSchemaParser) cacheField(key string, field *FieldDefinition) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fieldCache[key] = field
}

// ClearCache clears all cached schemas and fields
func (p *DefaultSchemaParser) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.fieldCache = make(map[string]*FieldDefinition)
	p.schemaCache = make(map[string]*ResourceSchema)
}

// GetCacheStats returns cache statistics
func (p *DefaultSchemaParser) GetCacheStats() map[string]int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]int{
		"schemas": len(p.schemaCache),
		"fields":  len(p.fieldCache),
	}
}

// IsKnownType checks if a type is a known OpenAPI type
func IsKnownType(typeName string) bool {
	knownTypes := []string{
		"string", "number", "integer", "boolean", "array", "object", "null",
	}

	for _, known := range knownTypes {
		if strings.EqualFold(typeName, known) {
			return true
		}
	}

	return false
}

// GetTypeHierarchy returns the type hierarchy for a field
func GetTypeHierarchy(field *FieldDefinition) []string {
	var hierarchy []string

	current := field
	for current != nil {
		hierarchy = append(hierarchy, current.Type)

		// For arrays, descend into items
		if current.Type == "array" && current.Items != nil {
			current = current.Items
		} else {
			break
		}
	}

	return hierarchy
}
