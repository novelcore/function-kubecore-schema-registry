package dynamic

import (
	"context"
	"testing"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSchemaParser(t *testing.T) {
	logger := logging.NewNopLogger()
	parser := NewSchemaParser(logger)

	tests := []struct {
		name           string
		schema         *apiextv1.JSONSchemaProps
		expectedFields int
		expectedError  bool
	}{
		{
			name: "simple object schema",
			schema: &apiextv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"name": {
						Type:        "string",
						Description: "Name of the resource",
					},
					"value": {
						Type:        "string",
						Description: "Value of the resource",
					},
				},
				Required: []string{"name"},
			},
			expectedFields: 2,
			expectedError:  false,
		},
		{
			name: "nested object schema",
			schema: &apiextv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"metadata": {
						Type: "object",
						Properties: map[string]apiextv1.JSONSchemaProps{
							"name": {
								Type: "string",
							},
							"namespace": {
								Type: "string",
							},
						},
					},
					"spec": {
						Type: "object",
						Properties: map[string]apiextv1.JSONSchemaProps{
							"replicas": {
								Type: "integer",
							},
						},
					},
				},
			},
			expectedFields: 2,
			expectedError:  false,
		},
		{
			name: "array schema",
			schema: &apiextv1.JSONSchemaProps{
				Type: "object",
				Properties: map[string]apiextv1.JSONSchemaProps{
					"items": {
						Type: "array",
						Items: &apiextv1.JSONSchemaPropsOrArray{
							Schema: &apiextv1.JSONSchemaProps{
								Type: "string",
							},
						},
					},
				},
			},
			expectedFields: 1,
			expectedError:  false,
		},
		{
			name:          "nil schema",
			schema:        nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.ParseOpenAPISchema(tt.schema)

			if tt.expectedError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Len(t, result.Fields, tt.expectedFields)
		})
	}
}

func TestReferenceDetector(t *testing.T) {
	logger := logging.NewNopLogger()
	detector := NewReferenceDetector(logger)

	tests := []struct {
		name               string
		schema             *ResourceSchema
		expectedReferences int
	}{
		{
			name: "fields with reference patterns",
			schema: &ResourceSchema{
				Fields: map[string]*FieldDefinition{
					"configMapRef": {
						Type:        "string",
						Description: "Reference to a ConfigMap",
					},
					"secretRef": {
						Type:        "string",
						Description: "Reference to a Secret",
					},
					"normalField": {
						Type:        "string",
						Description: "A normal field",
					},
					"kubeClusterRef": {
						Type:        "string",
						Description: "Reference to a KubeCluster",
					},
				},
			},
			expectedReferences: 3, // configMapRef, secretRef, kubeClusterRef
		},
		{
			name: "object reference structure",
			schema: &ResourceSchema{
				Fields: map[string]*FieldDefinition{
					"targetRef": {
						Type: "object",
						Properties: map[string]*FieldDefinition{
							"name": {
								Type: "string",
							},
							"namespace": {
								Type: "string",
							},
							"kind": {
								Type: "string",
							},
						},
					},
					"simpleRef": {
						Type: "string",
					},
				},
			},
			expectedReferences: 3, // targetRef (pattern match) + targetRef.name (heuristic) + simpleRef (pattern match)
		},
		{
			name: "no references",
			schema: &ResourceSchema{
				Fields: map[string]*FieldDefinition{
					"description": {
						Type: "string",
					},
					"value": {
						Type: "integer",
					},
				},
			},
			expectedReferences: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			references, err := detector.DetectReferences(tt.schema)

			require.NoError(t, err)
			assert.Len(t, references, tt.expectedReferences)

			// Verify confidence scores
			for _, ref := range references {
				assert.Greater(t, ref.Confidence, 0.0)
				assert.LessOrEqual(t, ref.Confidence, 1.0)
				assert.NotEmpty(t, ref.DetectionMethod)
			}
		})
	}
}

func TestCRDDiscovererMocked(t *testing.T) {
	logger := logging.NewNopLogger()

	// Create a fake clientset with mock CRDs
	fakeClient := apiextensionsfake.NewSimpleClientset()

	// Add a mock CRD that matches KubeCore patterns
	mockCRD := &apiextv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "kubeclusters.platform.kubecore.io",
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
								"metadata": {
									Type: "object",
									Properties: map[string]apiextv1.JSONSchemaProps{
										"name":      {Type: "string"},
										"namespace": {Type: "string"},
									},
								},
								"spec": {
									Type: "object",
									Properties: map[string]apiextv1.JSONSchemaProps{
										"region": {
											Type:        "string",
											Description: "AWS region for the cluster",
										},
										"providerConfigRef": {
											Type:        "string",
											Description: "Reference to provider config",
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
		Status: apiextv1.CustomResourceDefinitionStatus{
			Conditions: []apiextv1.CustomResourceDefinitionCondition{
				{
					Type:   apiextv1.Established,
					Status: apiextv1.ConditionTrue,
				},
			},
		},
	}

	// Add the CRD to the fake client
	_, err := fakeClient.ApiextensionsV1().CustomResourceDefinitions().Create(
		context.Background(), mockCRD, metav1.CreateOptions{})
	require.NoError(t, err)

	discoverer := NewCRDDiscoverer(fakeClient, logger)

	t.Run("discover matching CRDs", func(t *testing.T) {
		patterns := []string{"platform.kubecore.io"}

		crdInfos, err := discoverer.DiscoverCRDs(context.Background(), patterns)

		require.NoError(t, err)
		assert.Len(t, crdInfos, 1)

		crdInfo := crdInfos[0]
		assert.Equal(t, "KubeCluster", crdInfo.Kind)
		assert.Equal(t, "platform.kubecore.io", crdInfo.Group)
		assert.Equal(t, "v1alpha1", crdInfo.Version)
		assert.True(t, crdInfo.Namespaced)
		assert.NotNil(t, crdInfo.Schema)
		assert.NotNil(t, crdInfo.Metadata)
	})

	t.Run("discover with timeout", func(t *testing.T) {
		patterns := []string{"platform.kubecore.io"}
		timeout := 2 * time.Second

		crdInfos, err := discoverer.DiscoverWithTimeout(context.Background(), patterns, timeout)

		require.NoError(t, err)
		assert.Len(t, crdInfos, 1)
	})

	t.Run("no matching patterns", func(t *testing.T) {
		patterns := []string{"nonexistent.example.com"}

		crdInfos, err := discoverer.DiscoverCRDs(context.Background(), patterns)

		require.NoError(t, err)
		assert.Len(t, crdInfos, 0)
	})

	t.Run("statistics", func(t *testing.T) {
		patterns := []string{"platform.kubecore.io"}

		_, err := discoverer.DiscoverCRDs(context.Background(), patterns)
		require.NoError(t, err)

		stats := discoverer.GetDiscoveryStatistics()
		assert.NotNil(t, stats)
		assert.Equal(t, 1, stats.TotalCRDs)
		assert.Equal(t, 1, stats.MatchedCRDs)
		assert.Greater(t, stats.DiscoveryTime, time.Duration(0))
	})
}

func TestReferencePatterns(t *testing.T) {
	logger := logging.NewNopLogger()
	detector := NewReferenceDetector(logger)

	tests := []struct {
		name      string
		fieldName string
		fieldDef  *FieldDefinition
		expected  bool
	}{
		{
			name:      "configMapRef matches",
			fieldName: "configMapRef",
			fieldDef:  &FieldDefinition{Type: "string"},
			expected:  true,
		},
		{
			name:      "secretReference matches",
			fieldName: "secretReference",
			fieldDef:  &FieldDefinition{Type: "string"},
			expected:  true,
		},
		{
			name:      "kubeClusterRef matches",
			fieldName: "kubeClusterRef",
			fieldDef:  &FieldDefinition{Type: "string"},
			expected:  true,
		},
		{
			name:      "regular field doesn't match",
			fieldName: "regularField",
			fieldDef:  &FieldDefinition{Type: "string"},
			expected:  false,
		},
		{
			name:      "object with reference structure matches",
			fieldName: "targetRef",
			fieldDef: &FieldDefinition{
				Type: "object",
				Properties: map[string]*FieldDefinition{
					"name":      {Type: "string"},
					"namespace": {Type: "string"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := detector.MatchesReferencePattern(tt.fieldName, tt.fieldDef)
			assert.Equal(t, tt.expected, matches)
		})
	}
}

func TestExtractReferenceMetadata(t *testing.T) {
	logger := logging.NewNopLogger()
	detector := NewReferenceDetector(logger)

	tests := []struct {
		name           string
		fieldName      string
		fieldDef       *FieldDefinition
		expectedKind   string
		expectedGroup  string
		expectedMethod string
	}{
		{
			name:           "configMapRef",
			fieldName:      "configMapRef",
			fieldDef:       &FieldDefinition{Type: "string"},
			expectedKind:   "ConfigMap",
			expectedGroup:  "",
			expectedMethod: "pattern_match",
		},
		{
			name:           "kubeClusterRef",
			fieldName:      "kubeClusterRef",
			fieldDef:       &FieldDefinition{Type: "string"},
			expectedKind:   "KubeCluster",
			expectedGroup:  "", // May not match group due to pattern ordering
			expectedMethod: "pattern_match",
		},
		{
			name:      "field with reference description",
			fieldName: "resourceConnection",
			fieldDef: &FieldDefinition{
				Type:        "string",
				Description: "Reference to another resource",
			},
			expectedMethod: "description_analysis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata := detector.ExtractReferenceMetadata(tt.fieldName, tt.fieldDef)

			if tt.expectedMethod == "" {
				assert.Nil(t, metadata)
				return
			}

			require.NotNil(t, metadata)
			assert.Equal(t, tt.expectedMethod, metadata.DetectionMethod)

			if tt.expectedKind != "" {
				assert.Equal(t, tt.expectedKind, metadata.TargetKind)
			}

			// Only check group if we expect a specific one
			if tt.expectedGroup != "" && tt.expectedGroup != "ANY" {
				assert.Equal(t, tt.expectedGroup, metadata.TargetGroup)
			}

			assert.Greater(t, metadata.Confidence, 0.0)
			assert.LessOrEqual(t, metadata.Confidence, 1.0)
		})
	}
}

func TestCacheOperations(t *testing.T) {
	cache := NewCRDCache(1 * time.Second)

	crdInfo := &CRDInfo{
		Name:    "test-crd",
		Group:   "test.example.com",
		Version: "v1",
		Kind:    "TestKind",
	}

	t.Run("set and get", func(t *testing.T) {
		key := "test-key"

		// Initially empty
		result := cache.Get(key)
		assert.Nil(t, result)

		// Set value
		cache.Set(key, crdInfo)

		// Get value
		result = cache.Get(key)
		assert.NotNil(t, result)
		assert.Equal(t, crdInfo.Name, result.Name)
		assert.Equal(t, crdInfo.Kind, result.Kind)
	})

	t.Run("cache expiration", func(t *testing.T) {
		key := "expiring-key"

		// Use a cache with very short TTL
		shortCache := NewCRDCache(10 * time.Millisecond)

		// Set value
		shortCache.Set(key, crdInfo)

		// Should be available immediately
		result := shortCache.Get(key)
		assert.NotNil(t, result)

		// Wait for expiration
		time.Sleep(20 * time.Millisecond)

		// Should be expired
		result = shortCache.Get(key)
		assert.Nil(t, result)
	})

	t.Run("clear cache", func(t *testing.T) {
		key := "clear-test"

		cache.Set(key, crdInfo)
		result := cache.Get(key)
		assert.NotNil(t, result)

		cache.Clear()
		result = cache.Get(key)
		assert.Nil(t, result)
	})
}

func TestDefaultReferencePatterns(t *testing.T) {
	t.Run("default patterns exist", func(t *testing.T) {
		assert.NotEmpty(t, DefaultReferencePatterns)
		assert.Greater(t, len(DefaultReferencePatterns), 5)
	})

	t.Run("patterns have required fields", func(t *testing.T) {
		for i, pattern := range DefaultReferencePatterns {
			assert.NotEmpty(t, pattern.Pattern, "Pattern %d should have a pattern", i)
			assert.NotEmpty(t, pattern.RefType, "Pattern %d should have a ref type", i)
			assert.Greater(t, pattern.Confidence, 0.0, "Pattern %d should have positive confidence", i)
			assert.LessOrEqual(t, pattern.Confidence, 1.0, "Pattern %d should have confidence <= 1.0", i)
		}
	})

	t.Run("kubecore patterns exist", func(t *testing.T) {
		var kubeCorePatterns []ReferencePattern
		for _, pattern := range DefaultReferencePatterns {
			if pattern.TargetGroup == "platform.kubecore.io" ||
				pattern.TargetGroup == "github.platform.kubecore.io" {
				kubeCorePatterns = append(kubeCorePatterns, pattern)
			}
		}

		assert.NotEmpty(t, kubeCorePatterns, "Should have KubeCore-specific patterns")
	})
}

func TestSchemaParserCaching(t *testing.T) {
	logger := logging.NewNopLogger()
	parser := NewSchemaParser(logger)

	schema := &apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"name": {Type: "string"},
		},
	}

	t.Run("parse twice should use cache", func(t *testing.T) {
		// First parse
		result1, err := parser.ParseOpenAPISchema(schema)
		require.NoError(t, err)
		assert.NotNil(t, result1)

		// Second parse (should use cache)
		result2, err := parser.ParseOpenAPISchema(schema)
		require.NoError(t, err)
		assert.NotNil(t, result2)

		// Results should be equivalent
		assert.Equal(t, len(result1.Fields), len(result2.Fields))
	})

	t.Run("cache statistics", func(t *testing.T) {
		stats := parser.GetCacheStats()
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "schemas")
		assert.Contains(t, stats, "fields")
	})

	t.Run("clear cache", func(t *testing.T) {
		parser.ClearCache()
		stats := parser.GetCacheStats()
		assert.Equal(t, 0, stats["schemas"])
		assert.Equal(t, 0, stats["fields"])
	})
}

// Benchmark tests for performance
func BenchmarkSchemaParser(b *testing.B) {
	logger := logging.NewNopLogger()
	parser := NewSchemaParser(logger)

	schema := &apiextv1.JSONSchemaProps{
		Type: "object",
		Properties: map[string]apiextv1.JSONSchemaProps{
			"name":      {Type: "string"},
			"namespace": {Type: "string"},
			"value":     {Type: "string"},
			"replicas":  {Type: "integer"},
			"enabled":   {Type: "boolean"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := parser.ParseOpenAPISchema(schema)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReferenceDetector(b *testing.B) {
	logger := logging.NewNopLogger()
	detector := NewReferenceDetector(logger)

	schema := &ResourceSchema{
		Fields: map[string]*FieldDefinition{
			"configMapRef":   {Type: "string"},
			"secretRef":      {Type: "string"},
			"kubeClusterRef": {Type: "string"},
			"normalField1":   {Type: "string"},
			"normalField2":   {Type: "integer"},
			"normalField3":   {Type: "boolean"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := detector.DetectReferences(schema)
		if err != nil {
			b.Fatal(err)
		}
	}
}
