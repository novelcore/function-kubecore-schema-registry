package traversal

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	
	"github.com/crossplane/function-sdk-go/logging"
	
	"github.com/crossplane/function-kubecore-schema-registry/pkg/dynamic"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// ReferenceResolver resolves references in Kubernetes resources
type ReferenceResolver interface {
	// ExtractReferences extracts reference fields from a resource
	ExtractReferences(ctx context.Context, resource *unstructured.Unstructured) ([]dynamic.ReferenceField, error)
	
	// ResolveReferences resolves reference fields to actual resources
	ResolveReferences(ctx context.Context, source *unstructured.Unstructured, references []dynamic.ReferenceField) ([]*unstructured.Unstructured, []error)
	
	// ResolveReference resolves a single reference field
	ResolveReference(ctx context.Context, source *unstructured.Unstructured, reference dynamic.ReferenceField) (*unstructured.Unstructured, error)
	
	// ValidateReference validates if a reference can be resolved
	ValidateReference(reference dynamic.ReferenceField) error
}

// DefaultReferenceResolver implements ReferenceResolver interface
type DefaultReferenceResolver struct {
	// dynamicClient provides access to Kubernetes dynamic API
	dynamicClient dynamic.Interface
	
	// registry provides resource type information
	registry registry.Registry
	
	// referenceDetector detects reference fields in resources
	referenceDetector dynamic.ReferenceDetector
	
	// logger provides structured logging
	logger logging.Logger
	
	// cache stores resolved references
	cache Cache
}

// ReferenceResolutionResult contains the result of reference resolution
type ReferenceResolutionResult struct {
	// Reference is the reference field that was resolved
	Reference dynamic.ReferenceField
	
	// ResolvedResource is the resolved resource (nil if not found)
	ResolvedResource *unstructured.Unstructured
	
	// Error contains any error that occurred during resolution
	Error error
	
	// Cached indicates if the result was retrieved from cache
	Cached bool
	
	// ResolutionTime is the time taken to resolve this reference
	ResolutionTime time.Duration
}

// NewDefaultReferenceResolver creates a new default reference resolver
func NewDefaultReferenceResolver(dynamicClient dynamic.Interface, registry registry.Registry, logger logging.Logger) *DefaultReferenceResolver {
	return &DefaultReferenceResolver{
		dynamicClient:     dynamicClient,
		registry:          registry,
		referenceDetector: dynamic.NewReferenceDetector(logger),
		logger:            logger,
		cache:             NewLRUCache(1000, 5*time.Minute),
	}
}

// ExtractReferences extracts reference fields from a resource
func (rr *DefaultReferenceResolver) ExtractReferences(ctx context.Context, resource *unstructured.Unstructured) ([]dynamic.ReferenceField, error) {
	// Get resource type information
	resourceType, err := rr.registry.GetResourceType(resource.GetAPIVersion(), resource.GetKind())
	if err != nil {
		rr.logger.Debug("Resource type not found in registry, using heuristic detection",
			"apiVersion", resource.GetAPIVersion(),
			"kind", resource.GetKind())
	}
	
	// Extract references using multiple methods
	var allReferences []dynamic.ReferenceField
	
	// Method 1: Registry-based detection (if available)
	if resourceType != nil {
		registryRefs, err := rr.extractReferencesFromRegistry(resource, resourceType)
		if err == nil {
			allReferences = append(allReferences, registryRefs...)
		}
	}
	
	// Method 2: Pattern-based detection
	patternRefs, err := rr.extractReferencesFromPatterns(resource)
	if err == nil {
		allReferences = append(allReferences, patternRefs...)
	}
	
	// Method 3: Owner reference extraction
	ownerRefs, err := rr.extractOwnerReferences(resource)
	if err == nil {
		allReferences = append(allReferences, ownerRefs...)
	}
	
	// Deduplicate references
	deduplicatedRefs := rr.deduplicateReferences(allReferences)
	
	rr.logger.Debug("Extracted references from resource",
		"resource", fmt.Sprintf("%s/%s", resource.GetNamespace(), resource.GetName()),
		"kind", resource.GetKind(),
		"totalReferences", len(deduplicatedRefs),
		"registryRefs", len(allReferences)-len(patternRefs)-len(ownerRefs),
		"patternRefs", len(patternRefs),
		"ownerRefs", len(ownerRefs))
	
	return deduplicatedRefs, nil
}

// ResolveReferences resolves reference fields to actual resources
func (rr *DefaultReferenceResolver) ResolveReferences(ctx context.Context, source *unstructured.Unstructured, references []dynamic.ReferenceField) ([]*unstructured.Unstructured, []error) {
	var resolvedResources []*unstructured.Unstructured
	var errors []error
	
	// Process references concurrently for better performance
	results := make(chan *ReferenceResolutionResult, len(references))
	
	// Start goroutines for each reference
	for _, ref := range references {
		go func(ref dynamic.ReferenceField) {
			startTime := time.Now()
			
			resolved, err := rr.ResolveReference(ctx, source, ref)
			
			results <- &ReferenceResolutionResult{
				Reference:        ref,
				ResolvedResource: resolved,
				Error:            err,
				ResolutionTime:   time.Since(startTime),
			}
		}(ref)
	}
	
	// Collect results
	for i := 0; i < len(references); i++ {
		result := <-results
		
		if result.Error != nil {
			errors = append(errors, result.Error)
		} else if result.ResolvedResource != nil {
			resolvedResources = append(resolvedResources, result.ResolvedResource)
		}
	}
	
	return resolvedResources, errors
}

// ResolveReference resolves a single reference field
func (rr *DefaultReferenceResolver) ResolveReference(ctx context.Context, source *unstructured.Unstructured, reference dynamic.ReferenceField) (*unstructured.Unstructured, error) {
	// Generate cache key
	cacheKey := rr.generateCacheKey(source, reference)
	
	// Check cache first
	if cached, found := rr.cache.Get(cacheKey); found {
		if cachedResource, ok := cached.(*unstructured.Unstructured); ok {
			rr.logger.Debug("Reference resolved from cache", "reference", reference.FieldPath)
			return cachedResource, nil
		}
	}
	
	// Validate reference
	if err := rr.ValidateReference(reference); err != nil {
		return nil, functionerrors.Wrap(err, "reference validation failed")
	}
	
	// Extract reference value from source resource
	refValue, err := rr.extractReferenceValue(source, reference.FieldPath)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to extract reference value")
	}
	
	// Parse reference value to get target resource details
	targetName, targetNamespace, err := rr.parseReferenceValue(refValue, reference, source.GetNamespace())
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to parse reference value")
	}
	
	// Build GroupVersionResource for the target
	gvr, err := rr.buildGVR(reference.TargetGroup, reference.TargetVersion, reference.TargetKind)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to build GroupVersionResource")
	}
	
	// Resolve the reference
	var resolvedResource *unstructured.Unstructured
	
	if targetNamespace != "" {
		// Namespaced resource
		resolvedResource, err = rr.dynamicClient.Resource(gvr).Namespace(targetNamespace).Get(ctx, targetName, metav1.GetOptions{})
	} else {
		// Cluster-scoped resource
		resolvedResource, err = rr.dynamicClient.Resource(gvr).Get(ctx, targetName, metav1.GetOptions{})
	}
	
	if err != nil {
		return nil, functionerrors.Wrap(err, fmt.Sprintf("failed to resolve reference to %s/%s", reference.TargetKind, targetName))
	}
	
	// Cache the result
	rr.cache.Set(cacheKey, resolvedResource, 5*time.Minute)
	
	rr.logger.Debug("Reference resolved successfully",
		"reference", reference.FieldPath,
		"targetKind", reference.TargetKind,
		"targetName", targetName,
		"targetNamespace", targetNamespace)
	
	return resolvedResource, nil
}

// ValidateReference validates if a reference can be resolved
func (rr *DefaultReferenceResolver) ValidateReference(reference dynamic.ReferenceField) error {
	// Validate required fields
	if reference.FieldPath == "" {
		return fmt.Errorf("reference field path is empty")
	}
	
	if reference.TargetKind == "" {
		return fmt.Errorf("reference target kind is empty")
	}
	
	// Validate confidence threshold
	if reference.Confidence < 0.1 {
		return fmt.Errorf("reference confidence too low: %f", reference.Confidence)
	}
	
	return nil
}

// Helper methods

// extractReferencesFromRegistry extracts references using registry information
func (rr *DefaultReferenceResolver) extractReferencesFromRegistry(resource *unstructured.Unstructured, resourceType registry.ResourceType) ([]dynamic.ReferenceField, error) {
	// This would use the registry's schema information to identify reference fields
	// For now, return empty slice as the registry interface would need extension
	return []dynamic.ReferenceField{}, nil
}

// extractReferencesFromPatterns extracts references using pattern matching
func (rr *DefaultReferenceResolver) extractReferencesFromPatterns(resource *unstructured.Unstructured) ([]dynamic.ReferenceField, error) {
	// Use the reference detector to find references based on patterns
	resourceSchema := rr.convertToResourceSchema(resource)
	if resourceSchema == nil {
		return []dynamic.ReferenceField{}, nil
	}
	
	return rr.referenceDetector.DetectReferences(resourceSchema)
}

// extractOwnerReferences extracts owner references
func (rr *DefaultReferenceResolver) extractOwnerReferences(resource *unstructured.Unstructured) ([]dynamic.ReferenceField, error) {
	var references []dynamic.ReferenceField
	
	ownerRefs := resource.GetOwnerReferences()
	for i, ownerRef := range ownerRefs {
		ref := dynamic.ReferenceField{
			FieldPath:       fmt.Sprintf("metadata.ownerReferences[%d]", i),
			FieldName:       "ownerReference",
			TargetKind:      ownerRef.Kind,
			TargetGroup:     ownerRef.APIVersion, // This contains group/version
			RefType:         dynamic.RefTypeOwnerRef,
			Confidence:      1.0, // Owner references are always accurate
			DetectionMethod: "ownerReference",
		}
		
		// Extract group and version from APIVersion
		if strings.Contains(ownerRef.APIVersion, "/") {
			parts := strings.Split(ownerRef.APIVersion, "/")
			ref.TargetGroup = parts[0]
			ref.TargetVersion = parts[1]
		} else {
			ref.TargetGroup = ""
			ref.TargetVersion = ownerRef.APIVersion
		}
		
		references = append(references, ref)
	}
	
	return references, nil
}

// convertToResourceSchema converts an unstructured resource to a ResourceSchema
func (rr *DefaultReferenceResolver) convertToResourceSchema(resource *unstructured.Unstructured) *dynamic.ResourceSchema {
	// This is a simplified conversion
	// In a full implementation, this would:
	// 1. Extract the spec/status fields
	// 2. Analyze their structure
	// 3. Build field definitions with types
	// 4. Return a proper ResourceSchema
	
	fields := make(map[string]*dynamic.FieldDefinition)
	
	// Analyze spec fields
	if spec, found, _ := unstructured.NestedMap(resource.Object, "spec"); found {
		rr.analyzeFields(spec, "spec", fields)
	}
	
	// Analyze status fields
	if status, found, _ := unstructured.NestedMap(resource.Object, "status"); found {
		rr.analyzeFields(status, "status", fields)
	}
	
	return &dynamic.ResourceSchema{
		Fields:      fields,
		Description: fmt.Sprintf("Schema for %s", resource.GetKind()),
	}
}

// analyzeFields recursively analyzes fields to build field definitions
func (rr *DefaultReferenceResolver) analyzeFields(obj map[string]interface{}, basePath string, fields map[string]*dynamic.FieldDefinition) {
	for key, value := range obj {
		fieldPath := fmt.Sprintf("%s.%s", basePath, key)
		
		fieldDef := &dynamic.FieldDefinition{
			Type: rr.determineFieldType(value),
		}
		
		// Recursively analyze nested objects
		if nestedMap, ok := value.(map[string]interface{}); ok {
			properties := make(map[string]*dynamic.FieldDefinition)
			rr.analyzeNestedFields(nestedMap, properties)
			fieldDef.Properties = properties
		}
		
		fields[fieldPath] = fieldDef
	}
}

// analyzeNestedFields analyzes nested fields
func (rr *DefaultReferenceResolver) analyzeNestedFields(obj map[string]interface{}, properties map[string]*dynamic.FieldDefinition) {
	for key, value := range obj {
		properties[key] = &dynamic.FieldDefinition{
			Type: rr.determineFieldType(value),
		}
	}
}

// determineFieldType determines the type of a field value
func (rr *DefaultReferenceResolver) determineFieldType(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case int, int32, int64:
		return "integer"
	case float32, float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "string"
	}
}

// deduplicateReferences removes duplicate references
func (rr *DefaultReferenceResolver) deduplicateReferences(references []dynamic.ReferenceField) []dynamic.ReferenceField {
	seen := make(map[string]bool)
	var result []dynamic.ReferenceField
	
	for _, ref := range references {
		key := fmt.Sprintf("%s:%s:%s", ref.FieldPath, ref.TargetKind, ref.TargetGroup)
		if !seen[key] {
			seen[key] = true
			result = append(result, ref)
		}
	}
	
	return result
}

// extractReferenceValue extracts the value of a reference field from a resource
func (rr *DefaultReferenceResolver) extractReferenceValue(resource *unstructured.Unstructured, fieldPath string) (interface{}, error) {
	pathParts := strings.Split(fieldPath, ".")
	
	// Handle owner references specially
	if len(pathParts) >= 2 && pathParts[0] == "metadata" && strings.HasPrefix(pathParts[1], "ownerReferences") {
		ownerRefs := resource.GetOwnerReferences()
		if len(ownerRefs) > 0 {
			// Return the name of the first owner reference
			return ownerRefs[0].Name, nil
		}
		return nil, fmt.Errorf("no owner references found")
	}
	
	// Use unstructured.NestedFieldCopy to extract the field value
	value, found, err := unstructured.NestedFieldCopy(resource.Object, pathParts...)
	if err != nil {
		return nil, functionerrors.Wrap(err, "failed to extract field value")
	}
	
	if !found {
		return nil, fmt.Errorf("field not found: %s", fieldPath)
	}
	
	return value, nil
}

// parseReferenceValue parses a reference value to extract target name and namespace
func (rr *DefaultReferenceResolver) parseReferenceValue(refValue interface{}, reference dynamic.ReferenceField, sourceNamespace string) (name, namespace string, err error) {
	switch v := refValue.(type) {
	case string:
		// Simple string reference (just the name)
		name = v
		namespace = sourceNamespace // Default to source namespace
		
	case map[string]interface{}:
		// Object reference with name and optionally namespace
		if nameVal, found := v["name"]; found {
			if nameStr, ok := nameVal.(string); ok {
				name = nameStr
			} else {
				return "", "", fmt.Errorf("reference name is not a string")
			}
		} else {
			return "", "", fmt.Errorf("reference object missing 'name' field")
		}
		
		// Check for namespace
		if nsVal, found := v["namespace"]; found {
			if nsStr, ok := nsVal.(string); ok {
				namespace = nsStr
			}
		} else {
			namespace = sourceNamespace // Default to source namespace
		}
		
	default:
		return "", "", fmt.Errorf("unsupported reference value type: %T", refValue)
	}
	
	// Validate that we have a name
	if name == "" {
		return "", "", fmt.Errorf("empty reference name")
	}
	
	return name, namespace, nil
}

// buildGVR builds a GroupVersionResource from the reference information
func (rr *DefaultReferenceResolver) buildGVR(group, version, kind string) (schema.GroupVersionResource, error) {
	// Default version if not specified
	if version == "" {
		version = "v1"
	}
	
	// Convert kind to resource name (pluralize and lowercase)
	resource := rr.kindToResource(kind)
	
	return schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}, nil
}

// kindToResource converts a Kubernetes Kind to a resource name
func (rr *DefaultReferenceResolver) kindToResource(kind string) string {
	// Simple pluralization rules
	lower := strings.ToLower(kind)
	
	// Special cases
	specialCases := map[string]string{
		"pod":                           "pods",
		"service":                      "services",
		"configmap":                    "configmaps",
		"secret":                       "secrets",
		"persistentvolumeclaim":        "persistentvolumeclaims",
		"persistentvolume":             "persistentvolumes",
		"storageclass":                 "storageclasses",
		"deployment":                   "deployments",
		"replicaset":                   "replicasets",
		"daemonset":                    "daemonsets",
		"statefulset":                  "statefulsets",
		"job":                          "jobs",
		"cronjob":                      "cronjobs",
		"ingress":                      "ingresses",
		"networkpolicy":                "networkpolicies",
		"poddisruptionbudget":          "poddisruptionbudgets",
		"horizontalpodautoscaler":      "horizontalpodautoscalers",
		"verticalpodautoscaler":        "verticalpodautoscalers",
		
		// KubeCore platform resources
		"kubecluster":                  "kubeclusters",
		"kubenv":                       "kubenvs",
		"kubeapp":                      "kubeapps",
		"kubesystem":                   "kubesystems",
		"kubenet":                      "kubenets",
		"qualitygate":                  "qualitygates",
		"githubproject":                "githubprojects",
		"githubinfra":                  "githubinfras",
		"githubsystem":                 "githubsystems",
		"githubprovider":               "githubproviders",
	}
	
	if resource, found := specialCases[lower]; found {
		return resource
	}
	
	// Default pluralization: add 's'
	return lower + "s"
}

// generateCacheKey generates a cache key for a reference resolution
func (rr *DefaultReferenceResolver) generateCacheKey(source *unstructured.Unstructured, reference dynamic.ReferenceField) string {
	return fmt.Sprintf("%s/%s/%s:%s:%s:%s",
		source.GetAPIVersion(),
		source.GetKind(),
		source.GetName(),
		reference.FieldPath,
		reference.TargetKind,
		reference.TargetGroup)
}