package dynamic

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestEndToEndDynamicDiscovery demonstrates the complete workflow
func TestEndToEndDynamicDiscovery(t *testing.T) {
	logger := logging.NewNopLogger()
	ctx := context.Background()

	// Create fake CRDs that represent KubeCore platform resources
	fakeClient := apiextensionsfake.NewSimpleClientset()

	// Add multiple KubeCore CRDs
	kubeCoreCSDs := []*apiextv1.CustomResourceDefinition{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kubeclusters.platform.kubecore.io",
				Labels: map[string]string{
					"kubecore.io/managed": "true",
				},
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "platform.kubecore.io",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:     "KubeCluster",
					Plural:   "kubeclusters",
					Singular: "kubecluster",
				},
				Scope: apiextv1.NamespaceScoped,
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Storage: true,
						Served:  true,
						Schema: &apiextv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]apiextv1.JSONSchemaProps{
									"spec": {
										Type: "object",
										Properties: map[string]apiextv1.JSONSchemaProps{
											"region": {
												Type:        "string",
												Description: "AWS region for the cluster",
											},
											"providerConfigRef": {
												Type:        "string",
												Description: "Reference to provider configuration",
											},
											"githubProviderRef": {
												Type:        "string",
												Description: "Reference to GitHub provider",
											},
											"kubEnvRef": {
												Type:        "string",
												Description: "Reference to KubEnv",
											},
											"metadata": {
												Type: "object",
												Properties: map[string]apiextv1.JSONSchemaProps{
													"name":      {Type: "string"},
													"namespace": {Type: "string"},
												},
											},
										},
										Required: []string{"region"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "githubprojects.github.platform.kubecore.io",
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "github.platform.kubecore.io",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:     "GitHubProject",
					Plural:   "githubprojects",
					Singular: "githubproject",
				},
				Scope: apiextv1.NamespaceScoped,
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Storage: true,
						Served:  true,
						Schema: &apiextv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]apiextv1.JSONSchemaProps{
									"spec": {
										Type: "object",
										Properties: map[string]apiextv1.JSONSchemaProps{
											"repository": {
												Type:        "string",
												Description: "GitHub repository name",
											},
											"organization": {
												Type:        "string",
												Description: "GitHub organization",
											},
											"configMapRef": {
												Type:        "string",
												Description: "Reference to configuration ConfigMap",
											},
											"secretRef": {
												Type:        "string",
												Description: "Reference to secret for credentials",
											},
										},
										Required: []string{"repository", "organization"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Add CRDs to fake client
	for _, crd := range kubeCoreCSDs {
		_, err := fakeClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
		require.NoError(t, err)
	}

	t.Run("complete discovery workflow", func(t *testing.T) {
		// 1. Create discovery components
		discoverer := NewCRDDiscoverer(fakeClient, logger)
		detector := NewReferenceDetector(logger)

		// 2. Discover CRDs matching KubeCore patterns
		patterns := []string{"*.kubecore.io"}
		crdInfos, err := discoverer.DiscoverCRDs(ctx, patterns)

		require.NoError(t, err)
		assert.Len(t, crdInfos, 2) // Should find both CRDs

		// 3. Verify CRD discovery results
		var kubeClusterCRD, githubProjectCRD *CRDInfo
		for _, crd := range crdInfos {
			switch crd.Kind {
			case "KubeCluster":
				kubeClusterCRD = crd
			case "GitHubProject":
				githubProjectCRD = crd
			}
		}

		require.NotNil(t, kubeClusterCRD, "Should discover KubeCluster CRD")
		require.NotNil(t, githubProjectCRD, "Should discover GitHubProject CRD")

		// 4. Validate KubeCluster schema parsing
		assert.Equal(t, "platform.kubecore.io", kubeClusterCRD.Group)
		assert.Equal(t, "v1alpha1", kubeClusterCRD.Version)
		assert.True(t, kubeClusterCRD.Namespaced)
		require.NotNil(t, kubeClusterCRD.Schema)

		// Check that spec fields were parsed
		assert.Contains(t, kubeClusterCRD.Schema.Fields, "spec")
		specField := kubeClusterCRD.Schema.Fields["spec"]
		assert.Equal(t, "object", specField.Type)

		// 5. Detect reference fields in KubeCluster
		references, err := detector.DetectReferences(kubeClusterCRD.Schema)
		require.NoError(t, err)

		// Should detect multiple reference fields
		assert.Greater(t, len(references), 2, "Should detect reference fields")

		// Check for specific reference fields
		referenceNames := make(map[string]bool)
		for _, ref := range references {
			referenceNames[ref.FieldName] = true
		}

		// These should be detected based on naming patterns
		expectedRefs := []string{"providerConfigRef", "githubProviderRef", "kubEnvRef"}
		for _, expectedRef := range expectedRefs {
			assert.True(t, referenceNames[expectedRef], "Should detect %s as reference", expectedRef)
		}

		// 6. Validate GitHubProject reference detection
		githubReferences, err := detector.DetectReferences(githubProjectCRD.Schema)
		require.NoError(t, err)

		githubRefNames := make(map[string]bool)
		for _, ref := range githubReferences {
			githubRefNames[ref.FieldName] = true
		}

		// Should detect configMapRef and secretRef
		assert.True(t, githubRefNames["configMapRef"], "Should detect configMapRef")
		assert.True(t, githubRefNames["secretRef"], "Should detect secretRef")

		// 7. Verify reference metadata extraction
		for _, ref := range references {
			// Skip if this is a nested reference path that doesn't directly map to spec fields
			if strings.Contains(ref.FieldPath, ".") {
				continue
			}

			specField := kubeClusterCRD.Schema.Fields["spec"]
			if specField != nil && specField.Properties != nil {
				if fieldDef, exists := specField.Properties[ref.FieldName]; exists {
					metadata := detector.ExtractReferenceMetadata(ref.FieldName, fieldDef)

					if metadata != nil {
						assert.Greater(t, metadata.Confidence, 0.0)
						assert.LessOrEqual(t, metadata.Confidence, 1.0)
						assert.NotEmpty(t, metadata.DetectionMethod)
					}
				}
			}
		}
	})

	t.Run("performance characteristics", func(t *testing.T) {
		discoverer := NewCRDDiscoverer(fakeClient, logger)

		start := time.Now()
		patterns := []string{"*.kubecore.io"}

		// Measure discovery time
		_, err := discoverer.DiscoverCRDs(ctx, patterns)
		require.NoError(t, err)

		duration := time.Since(start)

		// Should complete quickly for mock CRDs
		assert.Less(t, duration, 1*time.Second, "Discovery should be fast for mock data")

		// Check statistics
		stats := discoverer.GetDiscoveryStatistics()
		assert.Equal(t, 2, stats.TotalCRDs)
		assert.Equal(t, 2, stats.MatchedCRDs)
		assert.Greater(t, stats.DiscoveryTime, time.Duration(0))
	})

	t.Run("caching effectiveness", func(t *testing.T) {
		discoverer := NewCRDDiscoverer(fakeClient, logger)
		patterns := []string{"*.kubecore.io"}

		// First discovery - populate cache
		start1 := time.Now()
		_, err := discoverer.DiscoverCRDs(ctx, patterns)
		require.NoError(t, err)
		duration1 := time.Since(start1)

		// Second discovery - should use cache
		start2 := time.Now()
		_, err = discoverer.DiscoverCRDs(ctx, patterns)
		require.NoError(t, err)
		duration2 := time.Since(start2)

		// Second run should be faster due to caching
		assert.LessOrEqual(t, duration2, duration1, "Cached discovery should be faster")

		stats := discoverer.GetDiscoveryStatistics()
		assert.Greater(t, stats.DiscoveryTime, time.Duration(0))
	})

	t.Run("error handling", func(t *testing.T) {
		discoverer := NewCRDDiscoverer(fakeClient, logger)

		// Test with timeout
		shortCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		patterns := []string{"*.kubecore.io"}
		_, err := discoverer.DiscoverCRDs(shortCtx, patterns)

		// Should handle timeout gracefully
		if err != nil {
			assert.Contains(t, err.Error(), "context")
		}
	})
}

// TestRealWorldScenario simulates a more realistic scenario
func TestRealWorldScenario(t *testing.T) {
	logger := logging.NewNopLogger()

	// Test configuration loading
	t.Run("configuration integration", func(t *testing.T) {
		// Test default patterns
		assert.NotEmpty(t, DefaultAPIGroupPatterns)
		assert.Contains(t, DefaultAPIGroupPatterns, "*.kubecore.io")
		assert.Contains(t, DefaultAPIGroupPatterns, "platform.kubecore.io")
		assert.Contains(t, DefaultAPIGroupPatterns, "github.platform.kubecore.io")

		// Test default reference patterns
		assert.NotEmpty(t, DefaultReferencePatterns)

		// Verify KubeCore-specific patterns exist
		kubeCorePatterns := 0
		for _, pattern := range DefaultReferencePatterns {
			if pattern.TargetGroup == "platform.kubecore.io" ||
				pattern.TargetGroup == "github.platform.kubecore.io" {
				kubeCorePatterns++
			}
		}
		assert.Greater(t, kubeCorePatterns, 0, "Should have KubeCore-specific patterns")
	})

	t.Run("schema complexity handling", func(t *testing.T) {
		parser := NewSchemaParser(logger)

		// Test complex nested schema
		complexSchema := &apiextv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextv1.JSONSchemaProps{
				"metadata": {
					Type: "object",
					Properties: map[string]apiextv1.JSONSchemaProps{
						"name":      {Type: "string"},
						"namespace": {Type: "string"},
						"labels": {
							Type: "object",
							AdditionalProperties: &apiextv1.JSONSchemaPropsOrBool{
								Allows: true,
								Schema: &apiextv1.JSONSchemaProps{Type: "string"},
							},
						},
					},
				},
				"spec": {
					Type: "object",
					Properties: map[string]apiextv1.JSONSchemaProps{
						"replicas": {
							Type:    "integer",
							Minimum: &[]float64{1}[0],
							Maximum: &[]float64{100}[0],
						},
						"strategy": {
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"type": {
									Type: "string",
									Enum: []apiextv1.JSON{
										{Raw: []byte(`"RollingUpdate"`)},
										{Raw: []byte(`"Recreate"`)},
									},
								},
							},
						},
						"template": {
							Type: "object",
							Properties: map[string]apiextv1.JSONSchemaProps{
								"containers": {
									Type: "array",
									Items: &apiextv1.JSONSchemaPropsOrArray{
										Schema: &apiextv1.JSONSchemaProps{
											Type: "object",
											Properties: map[string]apiextv1.JSONSchemaProps{
												"name":  {Type: "string"},
												"image": {Type: "string"},
												"configMapRef": {
													Type:        "string",
													Description: "Reference to ConfigMap",
												},
											},
											Required: []string{"name", "image"},
										},
									},
								},
							},
						},
					},
					Required: []string{"replicas", "template"},
				},
			},
			Required: []string{"metadata", "spec"},
		}

		// Parse the complex schema
		result, err := parser.ParseOpenAPISchema(complexSchema)
		require.NoError(t, err)
		assert.NotNil(t, result)

		// Verify it handles nested structures
		assert.Contains(t, result.Fields, "metadata")
		assert.Contains(t, result.Fields, "spec")

		metadataField := result.Fields["metadata"]
		assert.Equal(t, "object", metadataField.Type)
		assert.Contains(t, metadataField.Properties, "name")
		assert.Contains(t, metadataField.Properties, "namespace")

		specField := result.Fields["spec"]
		assert.Equal(t, "object", specField.Type)
		assert.Contains(t, specField.Properties, "replicas")
		assert.Contains(t, specField.Properties, "template")
	})

	t.Run("reference detection in complex schemas", func(t *testing.T) {
		detector := NewReferenceDetector(logger)

		// Create a schema with various reference patterns
		schema := &ResourceSchema{
			Fields: map[string]*FieldDefinition{
				"spec": {
					Type: "object",
					Properties: map[string]*FieldDefinition{
						"configMapRef":      {Type: "string"},
						"secretRef":         {Type: "string"},
						"kubeClusterRef":    {Type: "string"},
						"providerConfigRef": {Type: "string"},
						"templateRef": {
							Type: "object",
							Properties: map[string]*FieldDefinition{
								"name":      {Type: "string"},
								"namespace": {Type: "string"},
							},
						},
						"containers": {
							Type: "array",
							Items: &FieldDefinition{
								Type: "object",
								Properties: map[string]*FieldDefinition{
									"name":         {Type: "string"},
									"image":        {Type: "string"},
									"configMapRef": {Type: "string"},
								},
							},
						},
					},
				},
			},
		}

		// Detect references
		references, err := detector.DetectReferences(schema)
		require.NoError(t, err)

		// Should detect multiple references at different levels
		assert.Greater(t, len(references), 4, "Should detect references at multiple levels")

		// Check that nested references are detected
		foundNestedRef := false
		for _, ref := range references {
			if ref.FieldPath == "spec.containers[*].configMapRef" {
				foundNestedRef = true
				break
			}
		}
		assert.True(t, foundNestedRef, "Should detect nested array references")
	})
}

// Benchmark the performance of discovery operations
func BenchmarkDynamicDiscovery(b *testing.B) {
	logger := logging.NewNopLogger()
	ctx := context.Background()

	// Create fake client with multiple CRDs
	fakeClient := apiextensionsfake.NewSimpleClientset()

	// Add several test CRDs
	for i := 0; i < 10; i++ {
		crd := &apiextv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("testresource%d.platform.kubecore.io", i),
			},
			Spec: apiextv1.CustomResourceDefinitionSpec{
				Group: "platform.kubecore.io",
				Names: apiextv1.CustomResourceDefinitionNames{
					Kind:     fmt.Sprintf("TestResource%d", i),
					Plural:   fmt.Sprintf("testresource%ds", i),
					Singular: fmt.Sprintf("testresource%d", i),
				},
				Scope: apiextv1.NamespaceScoped,
				Versions: []apiextv1.CustomResourceDefinitionVersion{
					{
						Name:    "v1alpha1",
						Storage: true,
						Served:  true,
						Schema: &apiextv1.CustomResourceValidation{
							OpenAPIV3Schema: &apiextv1.JSONSchemaProps{
								Type: "object",
								Properties: map[string]apiextv1.JSONSchemaProps{
									"spec": {
										Type: "object",
										Properties: map[string]apiextv1.JSONSchemaProps{
											"configMapRef": {Type: "string"},
											"secretRef":    {Type: "string"},
											"value":        {Type: "string"},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		_, err := fakeClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{})
		if err != nil {
			b.Fatal(err)
		}
	}

	discoverer := NewCRDDiscoverer(fakeClient, logger)
	patterns := []string{"*.kubecore.io"}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := discoverer.DiscoverCRDs(ctx, patterns)
		if err != nil {
			b.Fatal(err)
		}
	}
}
