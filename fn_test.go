package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=hack/boilerplate.go.txt paths=./input/v1beta1/...

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestRunFunction(t *testing.T) {

	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}
	type want struct {
		rsp *fnv1.RunFunctionResponse
		err error
	}

	type mockSetup struct {
		crds []*apiextensionsv1.CustomResourceDefinition
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
		mock   *mockSetup
	}{
		"SchemaRegistryBasicTest": {
			reason: "The Function should process schema registry discovery successfully",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "schema-registry-test"},
					Input: resource.MustStructJSON(`{
						"apiVersion": "template.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"enableTransitiveDiscovery": true,
						"traversalDepth": 3,
						"includeFullSchema": true
					}`),
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "platform.kubecore.io/v1alpha1",
								"kind": "XSchemaRegistry", 
								"metadata": {
									"name": "test-schema-registry-abc123",
									"labels": {
										"crossplane.io/claim-name": "test-schema-registry",
										"crossplane.io/claim-namespace": "default"
									}
								},
								"spec": {
									"githubProjectRef": {
										"name": "test-project",
										"namespace": "default"
									},
									"enableTransitiveDiscovery": true,
									"traversalDepth": 3
								}
							}`),
						},
					},
				},
			},
			want: want{
				rsp: nil, // Will validate manually
				err: nil,
			},
			mock: nil, // Use fallback schemas in this test
		},
		"SchemaRegistryRealCRDTest": {
			reason: "The Function should discover real CRD schemas successfully",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "real-crd-test"},
					Input: resource.MustStructJSON(`{
						"apiVersion": "template.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"enableTransitiveDiscovery": true,
						"traversalDepth": 2,
						"includeFullSchema": true
					}`),
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "platform.kubecore.io/v1alpha1",
								"kind": "XSchemaRegistry", 
								"metadata": {
									"name": "real-crd-test-abc123",
									"labels": {
										"crossplane.io/claim-name": "real-crd-test",
										"crossplane.io/claim-namespace": "test"
									}
								},
								"spec": {
									"githubProjectRef": {
										"name": "demo-project",
										"namespace": "test"
									},
									"enableTransitiveDiscovery": true,
									"traversalDepth": 2
								}
							}`),
						},
					},
				},
			},
			want: want{
				rsp: nil, // Will validate manually
				err: nil,
			},
			mock: &mockSetup{
				crds: []*apiextensionsv1.CustomResourceDefinition{
					createGitHubProjectCRD(),
					createGithubProviderCRD(),
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(logging.NewNopLogger())

			// Setup mock Kubernetes client if provided
			if tc.mock != nil {
				fakeClient := fake.NewSimpleClientset()
				for _, crd := range tc.mock.crds {
					_, err := fakeClient.ApiextensionsV1().CustomResourceDefinitions().Create(
						context.Background(), crd, metav1.CreateOptions{})
					if err != nil {
						t.Fatalf("Failed to create mock CRD: %v", err)
					}
				}
				f.SetKubernetesClient(fakeClient)
			}

			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Basic error validation
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}

			// Manual validation for specific test cases
			if name == "SchemaRegistryBasicTest" || name == "SchemaRegistryRealCRDTest" {
				validateSchemaRegistryResponse(t, tc.reason, rsp, name)
			}
		})
	}
}

// validateSchemaRegistryResponse validates the response for schema registry tests
func validateSchemaRegistryResponse(t *testing.T, reason string, rsp *fnv1.RunFunctionResponse, testName string) {
	if rsp == nil {
		t.Errorf("%s\nExpected response, got nil", reason)
		return
	}

	// Verify we have results
	if len(rsp.Results) == 0 {
		t.Errorf("%s\nExpected results in response", reason)
	}

	// Verify we have conditions
	if len(rsp.Conditions) == 0 {
		t.Errorf("%s\nExpected conditions in response", reason)
	}

	// Verify no fatal errors (function should complete successfully)
	for _, result := range rsp.Results {
		if result.Severity == fnv1.Severity_SEVERITY_FATAL {
			t.Errorf("%s\nUnexpected fatal error: %s", reason, result.Message)
		}
	}

	// Check what context keys are available
	if rsp.Context != nil && rsp.Context.GetFields() != nil {
		contextFields := rsp.Context.GetFields()
		t.Logf("Available Context Keys (%d total):", len(contextFields))
		for key, value := range contextFields {
			if key == "kubecore.schemaRegistry.detailedStatus" {
				t.Logf("✅ Found legacy schema registry context key: %s", key)
				if value.GetStringValue() != "" {
					contextStr := value.GetStringValue()
					if len(contextStr) > 500 {
						contextStr = contextStr[:500] + "..."
					}
					t.Logf("Context Data: %s", contextStr)

					// Parse and validate structured data
					var contextData map[string]interface{}
					if err := json.Unmarshal([]byte(value.GetStringValue()), &contextData); err == nil {
						validateContextData(t, contextData, testName)
					} else {
						t.Errorf("Failed to parse context data as JSON: %v", err)
					}
				}
			} else if key == "schemaRegistryResults" {
				t.Logf("✅ Found NEW schema registry context key: %s", key)
				// Test structured access to the new format
				if structValue := value.GetStructValue(); structValue != nil {
					validateNewSchemaRegistryResults(t, structValue, testName)
				} else {
					t.Errorf("Expected structured data for schemaRegistryResults, got: %T", value)
				}
			} else {
				t.Logf("  %s: %s", key, value.GetStringValue())
			}
		}
	} else {
		t.Logf("No context data in response")
	}

	t.Logf("Schema registry test %s completed successfully with %d results", testName, len(rsp.Results))
}

// validateContextData validates the structure and content of context data
func validateContextData(t *testing.T, contextData map[string]interface{}, testName string) {
	if execCtx, ok := contextData["executionContext"]; ok {
		t.Logf("ExecutionContext: %+v", execCtx)
		if execMap, ok := execCtx.(map[string]interface{}); ok {
			if directRefs, exists := execMap["directReferences"]; exists {
				if refMap, ok := directRefs.(map[string]interface{}); ok {
					if len(refMap) == 0 {
						t.Errorf("Expected direct references to be found")
					}
					t.Logf("Found %d direct references", len(refMap))
				}
			}
		}
	} else {
		t.Errorf("Expected executionContext in context data")
	}

	if stats, ok := contextData["discoveryStats"]; ok {
		t.Logf("DiscoveryStats: %+v", stats)
		if statsMap, ok := stats.(map[string]interface{}); ok {
			// Validate enhanced metrics for Phase 2
			if realSchemas, exists := statsMap["realSchemasFound"]; exists {
				t.Logf("✅ Found realSchemasFound metric: %v", realSchemas)
			}
			if cacheHits, exists := statsMap["cacheHits"]; exists {
				t.Logf("✅ Found cacheHits metric: %v", cacheHits)
			}
			if apiCalls, exists := statsMap["apiCalls"]; exists {
				t.Logf("✅ Found apiCalls metric: %v", apiCalls)
			}
			// Execution time should be reasonable (< 1000ms)
			if execTimeMs, exists := statsMap["executionTimeMs"]; exists {
				if execTime, ok := execTimeMs.(float64); ok {
					if execTime > 1000 {
						t.Logf("⚠️ Execution time %vms is high", execTime)
					} else {
						t.Logf("✅ Good execution time: %vms", execTime)
					}
				}
			}
		}
	} else {
		t.Errorf("Expected discoveryStats in context data")
	}

	if schemas, ok := contextData["referencedResourceSchemas"]; ok {
		if schemasMap, ok := schemas.(map[string]interface{}); ok {
			t.Logf("Schema Count: %d", len(schemasMap))
			for refName, schemaData := range schemasMap {
				t.Logf("  - Reference: %s", refName)
				// Validate schema structure for Phase 2
				if schemaMap, ok := schemaData.(map[string]interface{}); ok {
					validateSchemaStructure(t, refName, schemaMap, testName)
				}
			}
			if len(schemasMap) == 0 {
				t.Errorf("Expected at least one schema to be discovered")
			}
		}
	} else {
		t.Errorf("Expected referencedResourceSchemas in context data")
	}
}

// validateSchemaStructure validates individual schema structure
func validateSchemaStructure(t *testing.T, refName string, schema map[string]interface{}, testName string) {
	// Validate required fields
	if kind, exists := schema["kind"]; exists {
		t.Logf("    Kind: %v", kind)
		// Phase 2: Should not be MockKind anymore
		if testName == "SchemaRegistryRealCRDTest" {
			if kindStr, ok := kind.(string); ok && kindStr == "MockKind" {
				t.Errorf("❌ Found MockKind in real CRD test - Phase 2 should use real schemas")
			} else if kindStr == "GitHubProject" || kindStr == "GithubProvider" {
				t.Logf("✅ Found real Kind: %s", kindStr)
			}
		}
	} else {
		t.Errorf("Schema missing 'kind' field")
	}

	if apiVersion, exists := schema["apiVersion"]; exists {
		t.Logf("    APIVersion: %v", apiVersion)
		// Phase 2: Should not be mock.kubecore.io/v1 anymore
		if testName == "SchemaRegistryRealCRDTest" {
			if apiStr, ok := apiVersion.(string); ok && apiStr == "mock.kubecore.io/v1" {
				t.Errorf("❌ Found mock API version in real CRD test - Phase 2 should use real API versions")
			} else if strings.Contains(apiStr, "github.platform.kubecore.io") {
				t.Logf("✅ Found real API version: %s", apiStr)
			}
		}
	} else {
		t.Errorf("Schema missing 'apiVersion' field")
	}

	if source, exists := schema["source"]; exists {
		t.Logf("    Source: %v", source)
		if testName == "SchemaRegistryRealCRDTest" && source == "kubernetes-api" {
			t.Logf("✅ Real Kubernetes API source detected")
		}
	}

	if refFields, exists := schema["referenceFields"]; exists {
		if fields, ok := refFields.([]interface{}); ok {
			t.Logf("    Reference Fields (%d): %v", len(fields), fields)
		}
	}

	// Check for transitive references
	if transitiveRefs, exists := schema["transitiveReferences"]; exists {
		if transMap, ok := transitiveRefs.(map[string]interface{}); ok {
			t.Logf("    Transitive References: %d", len(transMap))
			for transRef := range transMap {
				t.Logf("      -> %s", transRef)
			}
		}
	}
}

// validateNewSchemaRegistryResults validates the structured schema registry results
func validateNewSchemaRegistryResults(t *testing.T, structValue *structpb.Struct, testName string) {
	fields := structValue.GetFields()
	
	// Check for required fields
	requiredFields := []string{"discoveredResources", "resourceSchemas", "referenceChains", "resourcesByKind", "discoveryStats"}
	for _, field := range requiredFields {
		if _, exists := fields[field]; !exists {
			t.Errorf("Missing required field in schemaRegistryResults: %s", field)
		} else {
			t.Logf("✅ Found required field: %s", field)
		}
	}
	
	// Validate discoveryStats structure
	if discoveryStats, exists := fields["discoveryStats"]; exists {
		if statsStruct := discoveryStats.GetStructValue(); statsStruct != nil {
			statsFields := statsStruct.GetFields()
			expectedStats := []string{"totalResourcesFound", "totalSchemasRetrieved", "maxDepthReached", "executionTimeMs"}
			for _, stat := range expectedStats {
				if _, exists := statsFields[stat]; exists {
					t.Logf("✅ Found discovery stat: %s", stat)
				} else {
					t.Errorf("Missing discovery stat: %s", stat)
				}
			}
			
			// Test actual values
			if totalResources, exists := statsFields["totalResourcesFound"]; exists {
				if count := int(totalResources.GetNumberValue()); count > 0 {
					t.Logf("✅ Total resources found: %d", count)
				}
			}
			
			if executionTime, exists := statsFields["executionTimeMs"]; exists {
				if timeMs := int(executionTime.GetNumberValue()); timeMs >= 0 {
					t.Logf("✅ Execution time: %dms", timeMs)
				}
			}
		}
	}
	
	// Validate discoveredResources array
	if discoveredResources, exists := fields["discoveredResources"]; exists {
		if resourcesList := discoveredResources.GetListValue(); resourcesList != nil {
			values := resourcesList.GetValues()
			t.Logf("✅ Found %d discovered resources", len(values))
			
			// Validate first resource structure
			if len(values) > 0 {
				if firstResource := values[0].GetStructValue(); firstResource != nil {
					resourceFields := firstResource.GetFields()
					requiredResourceFields := []string{"name", "kind", "apiVersion", "referencedBy", "depth", "source"}
					for _, field := range requiredResourceFields {
						if _, exists := resourceFields[field]; exists {
							t.Logf("✅ Resource has field: %s", field)
						} else {
							t.Errorf("Missing resource field: %s", field)
						}
					}
				}
			}
		}
	}
	
	// Validate resourcesByKind structure
	if resourcesByKind, exists := fields["resourcesByKind"]; exists {
		if kindMap := resourcesByKind.GetStructValue(); kindMap != nil {
			kindFields := kindMap.GetFields()
			t.Logf("✅ Found resourcesByKind with %d kinds", len(kindFields))
			for kind := range kindFields {
				t.Logf("  - Kind: %s", kind)
			}
		}
	}
	
	t.Logf("✅ NEW schema registry format validation completed successfully for %s", testName)
}

// createGitHubProjectCRD creates a mock GitHubProject CRD for testing
func createGitHubProjectCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "githubprojects.github.platform.kubecore.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "github.platform.kubecore.io",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"metadata": {Type: "object"},
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"githubProviderRef": {
											Type: "object",
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"name": {Type: "string"},
											},
										},
										"repository": {Type: "string"},
										"branch": {Type: "string"},
									},
									Required: []string{"githubProviderRef", "repository"},
								},
								"status": {Type: "object"},
							},
							Required: []string{"metadata", "spec"},
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "githubprojects",
				Singular: "githubproject",
				Kind:     "GitHubProject",
			},
		},
	}
}

// createGithubProviderCRD creates a mock GithubProvider CRD for testing
func createGithubProviderCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "githubproviders.github.platform.kubecore.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "github.platform.kubecore.io",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1alpha1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"metadata": {Type: "object"},
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"providerConfigRef": {
											Type: "object",
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"name": {Type: "string"},
											},
										},
										"secretRef": {
											Type: "object",
											Properties: map[string]apiextensionsv1.JSONSchemaProps{
												"name":      {Type: "string"},
												"namespace": {Type: "string"},
											},
										},
										"baseUrl": {Type: "string"},
									},
									Required: []string{"providerConfigRef"},
								},
								"status": {Type: "object"},
							},
							Required: []string{"metadata", "spec"},
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "githubproviders",
				Singular: "githubprovider",
				Kind:     "GithubProvider",
			},
		},
	}
}
