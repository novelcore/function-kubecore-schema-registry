package repository

import (
	"context"
	"fmt"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// KubernetesRepository implements SchemaRepository using Kubernetes API
type KubernetesRepository struct {
	client clientset.Interface
	logger interfaces.Logger
}

// NewKubernetesRepository creates a new Kubernetes-based repository
func NewKubernetesRepository(logger interfaces.Logger) (interfaces.SchemaRepository, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := clientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &KubernetesRepository{
		client: client,
		logger: logger,
	}, nil
}

// NewKubernetesRepositoryWithClient creates a repository with an existing client
func NewKubernetesRepositoryWithClient(client clientset.Interface, logger interfaces.Logger) interfaces.SchemaRepository {
	return &KubernetesRepository{
		client: client,
		logger: logger,
	}
}

// GetCRDSchema retrieves a CRD schema by kind and API version
func (k *KubernetesRepository) GetCRDSchema(ctx context.Context, kind, apiVersion string) (*domain.SchemaInfo, error) {
	if k.client == nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeConfiguration,
			"kubernetes client not available",
			nil,
			&domain.ResourceReference{Kind: kind, APIVersion: apiVersion},
			"",
		)
	}

	// Parse the API version to get group
	group, version, err := k.parseAPIVersion(apiVersion)
	if err != nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"failed to parse API version",
			err,
			&domain.ResourceReference{Kind: kind, APIVersion: apiVersion},
			"",
		)
	}

	// Construct CRD name
	crdName := k.constructCRDName(kind, group)

	k.logger.Debug("Fetching CRD",
		"crdName", crdName,
		"group", group,
		"version", version)

	// Get the CRD
	crd, err := k.client.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeNotFound,
			fmt.Sprintf("failed to get CRD %s", crdName),
			err,
			&domain.ResourceReference{Kind: kind, APIVersion: apiVersion},
			"",
		)
	}

	// Find the correct version in the CRD
	var versionSchema *apiextensionsv1.CustomResourceValidation
	for _, v := range crd.Spec.Versions {
		if v.Name == version {
			versionSchema = v.Schema
			break
		}
	}

	if versionSchema == nil || versionSchema.OpenAPIV3Schema == nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeNotFound,
			fmt.Sprintf("no schema found for version %s in CRD %s", version, crdName),
			nil,
			&domain.ResourceReference{Kind: kind, APIVersion: apiVersion},
			"",
		)
	}

	// Extract reference fields from the schema
	referenceFields := k.extractReferenceFieldsFromSchema(versionSchema.OpenAPIV3Schema)

	// Build SchemaInfo
	schema := &domain.SchemaInfo{
		Kind:            kind,
		APIVersion:      apiVersion,
		ReferenceFields: referenceFields,
		RequiredFields:  k.extractRequiredFields(versionSchema.OpenAPIV3Schema),
		OpenAPIV3Schema: versionSchema.OpenAPIV3Schema,
		Source:          string(domain.SourceKubernetesAPI),
	}

	k.logger.Debug("Successfully fetched CRD schema",
		"crdName", crdName,
		"referenceFields", len(referenceFields))

	return schema, nil
}

// ListCRDs returns available CRDs matching the given criteria
func (k *KubernetesRepository) ListCRDs(ctx context.Context, labelSelector string) ([]*domain.CRDInfo, error) {
	if k.client == nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypeConfiguration,
			"kubernetes client not available",
			nil,
			nil,
			"",
		)
	}

	options := metav1.ListOptions{}
	if labelSelector != "" {
		options.LabelSelector = labelSelector
	}

	crdList, err := k.client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, options)
	if err != nil {
		return nil, domain.NewDiscoveryError(
			domain.ErrorTypePermission,
			"failed to list CRDs",
			err,
			nil,
			"",
		)
	}

	var crdInfos []*domain.CRDInfo
	for _, crd := range crdList.Items {
		crdInfo := &domain.CRDInfo{
			Name:     crd.Name,
			Group:    crd.Spec.Group,
			Kind:     crd.Spec.Names.Kind,
			Plural:   crd.Spec.Names.Plural,
			Singular: crd.Spec.Names.Singular,
			Scope:    string(crd.Spec.Scope),
		}
		
		// Get the served version
		for _, version := range crd.Spec.Versions {
			if version.Served && version.Storage {
				crdInfo.Version = version.Name
				break
			}
		}
		
		crdInfos = append(crdInfos, crdInfo)
	}

	return crdInfos, nil
}

// ValidateSchema validates a schema structure
func (k *KubernetesRepository) ValidateSchema(schema *domain.SchemaInfo) error {
	if schema == nil {
		return domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"schema is nil",
			nil,
			nil,
			"",
		)
	}

	if schema.Kind == "" {
		return domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"schema kind is empty",
			nil,
			nil,
			"",
		)
	}

	if schema.APIVersion == "" {
		return domain.NewDiscoveryError(
			domain.ErrorTypeValidation,
			"schema API version is empty",
			nil,
			nil,
			"",
		)
	}

	return nil
}

// parseAPIVersion splits apiVersion into group and version
func (k *KubernetesRepository) parseAPIVersion(apiVersion string) (group, version string, err error) {
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

// constructCRDName constructs the CRD name from kind and group
func (k *KubernetesRepository) constructCRDName(kind, group string) string {
	// Convert kind to lowercase plural (simplified approach)
	kindLower := strings.ToLower(kind)
	plural := k.makeKindPlural(kindLower)

	if group == "" {
		return plural
	}
	return plural + "." + group
}

// makeKindPlural creates plural form of a kind (simplified)
func (k *KubernetesRepository) makeKindPlural(kind string) string {
	// Simple pluralization rules
	if strings.HasSuffix(kind, "s") {
		return kind + "es"
	}
	if strings.HasSuffix(kind, "y") {
		return strings.TrimSuffix(kind, "y") + "ies"
	}
	return kind + "s"
}

// extractRequiredFields extracts required fields from OpenAPI schema
func (k *KubernetesRepository) extractRequiredFields(schema *apiextensionsv1.JSONSchemaProps) []string {
	if schema == nil {
		return []string{"metadata", "spec"}
	}
	return schema.Required
}

// extractReferenceFieldsFromSchema extracts reference field names from OpenAPI schema
func (k *KubernetesRepository) extractReferenceFieldsFromSchema(schema *apiextensionsv1.JSONSchemaProps) []string {
	var refFields []string

	if schema == nil || schema.Properties == nil {
		return refFields
	}

	// Look for reference fields in spec
	if specSchema, exists := schema.Properties["spec"]; exists {
		refFields = append(refFields, k.findReferenceFields(specSchema.Properties, "")...)
	}

	return refFields
}

// findReferenceFields recursively finds reference fields in schema properties
func (k *KubernetesRepository) findReferenceFields(properties map[string]apiextensionsv1.JSONSchemaProps, prefix string) []string {
	var refFields []string

	for fieldName, fieldSchema := range properties {
		fullFieldName := fieldName
		if prefix != "" {
			fullFieldName = prefix + "." + fieldName
		}

		// Check if this field is a reference field
		if k.isReferenceField(fieldName) {
			refFields = append(refFields, fullFieldName)
		}

		// Recursively check nested objects
		if fieldSchema.Type == "object" && fieldSchema.Properties != nil {
			nestedRefs := k.findReferenceFields(fieldSchema.Properties, fullFieldName)
			refFields = append(refFields, nestedRefs...)
		}

		// Check array items
		if fieldSchema.Type == "array" && fieldSchema.Items != nil && fieldSchema.Items.Schema != nil {
			if fieldSchema.Items.Schema.Properties != nil {
				arrayRefs := k.findReferenceFields(fieldSchema.Items.Schema.Properties, fullFieldName+"[]")
				refFields = append(refFields, arrayRefs...)
			}
		}
	}

	return refFields
}

// isReferenceField determines if a field name indicates a reference
func (k *KubernetesRepository) isReferenceField(fieldName string) bool {
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
		if matched := strings.HasSuffix(fieldName, strings.TrimSuffix(pattern, "$")); matched {
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