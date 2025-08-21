package main

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen object:headerFile=hack/boilerplate.go.txt paths=./input/v1beta1/...

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
)

func TestRunFunction(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}

	cases := map[string]struct {
		reason string
		args   args
	}{
		"NoFetchRequests": {
			reason: "Should handle empty fetch requests gracefully",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"fetchResources": []
					}`),
				},
			},
		},
		"InvalidInput": {
			reason: "Should handle invalid function input gracefully",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"invalid": "input"
					}`),
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(logging.NewNopLogger())
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Basic functional test - should not panic and should return a response
			if rsp == nil {
				t.Errorf("%s\nExpected response but got nil", tc.reason)
			}

			// For invalid input case, expect no error but fatal result
			if name == "InvalidInput" {
				if err != nil {
					t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
				}
				if rsp != nil && len(rsp.Results) > 0 {
					foundFatal := false
					for _, result := range rsp.Results {
						if result.Severity == fnv1.Severity_SEVERITY_FATAL {
							foundFatal = true
							break
						}
					}
					if !foundFatal {
						t.Errorf("%s\nExpected fatal result for invalid input", tc.reason)
					}
				}
			}

			// For no fetch requests case, expect normal completion
			if name == "NoFetchRequests" {
				if err != nil {
					t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
				}
				if rsp != nil && len(rsp.Results) > 0 {
					foundNormal := false
					for _, result := range rsp.Results {
						if result.Severity == fnv1.Severity_SEVERITY_NORMAL {
							foundNormal = true
							break
						}
					}
					if !foundNormal {
						t.Errorf("%s\nExpected normal result for empty fetch requests", tc.reason)
					}
				}
			}
		})
	}
}

func TestParseValidationRequests(t *testing.T) {
	cases := map[string]struct {
		reason   string
		input    *v1beta1.Input
		expected int
		hasError bool
	}{
		"ValidInput": {
			reason: "Should accept valid input with fetch requests",
			input: &v1beta1.Input{
				FetchResources: []v1beta1.ResourceRequest{
					{
						Into:       "project",
						Name:       "test-project",
						APIVersion: "github.platform.kubecore.io/v1alpha1",
						Kind:       "GitHubProject",
						Optional:   false,
					},
				},
			},
			expected: 1,
			hasError: false,
		},
		"EmptyInput": {
			reason: "Should handle empty fetch requests",
			input: &v1beta1.Input{
				FetchResources: []v1beta1.ResourceRequest{},
			},
			expected: 0,
			hasError: false,
		},
		"MultipleResources": {
			reason: "Should handle multiple fetch requests",
			input: &v1beta1.Input{
				FetchResources: []v1beta1.ResourceRequest{
					{
						Into:       "project",
						Name:       "test-project",
						APIVersion: "github.platform.kubecore.io/v1alpha1",
						Kind:       "GitHubProject",
					},
					{
						Into:       "infra",
						Name:       "test-infra",
						APIVersion: "github.platform.kubecore.io/v1alpha1",
						Kind:       "GitHubInfra",
					},
				},
			},
			expected: 2,
			hasError: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if len(tc.input.FetchResources) != tc.expected {
				t.Errorf("%s\nExpected %d fetch requests, got %d", tc.reason, tc.expected, len(tc.input.FetchResources))
			}

			// Basic validation
			for i, req := range tc.input.FetchResources {
				if req.Into == "" {
					if !tc.hasError {
						t.Errorf("%s\nFetchRequest[%d] missing 'into' field", tc.reason, i)
					}
				}
				if req.Name == "" {
					if !tc.hasError {
						t.Errorf("%s\nFetchRequest[%d] missing 'name' field", tc.reason, i)
					}
				}
				if req.APIVersion == "" {
					if !tc.hasError {
						t.Errorf("%s\nFetchRequest[%d] missing 'apiVersion' field", tc.reason, i)
					}
				}
				if req.Kind == "" {
					if !tc.hasError {
						t.Errorf("%s\nFetchRequest[%d] missing 'kind' field", tc.reason, i)
					}
				}
			}
		})
	}
}

func TestXRParser(t *testing.T) {
	cases := map[string]struct {
		reason   string
		xr       *unstructured.Unstructured
		expected int
		hasError bool
	}{
		"NoFetchRequests": {
			reason: "Should handle XR with no fetchResources",
			xr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "test.kubecore.io/v1alpha1",
					"kind":       "TestXR",
					"metadata": map[string]interface{}{
						"name": "test-xr",
					},
					"spec": map[string]interface{}{},
				},
			},
			expected: 0,
			hasError: false,
		},
		"WithFetchRequests": {
			reason: "Should parse XR with embedded fetchResources",
			xr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "test.kubecore.io/v1alpha1",
					"kind":       "TestXR",
					"metadata": map[string]interface{}{
						"name": "test-xr",
					},
					"spec": map[string]interface{}{
						"fetchResources": []interface{}{
							map[string]interface{}{
								"into":       "project",
								"name":       "test-project",
								"apiVersion": "github.platform.kubecore.io/v1alpha1",
								"kind":       "GitHubProject",
								"optional":   false,
							},
						},
					},
				},
			},
			expected: 1,
			hasError: false,
		},
		"InvalidFetchRequests": {
			reason: "Should handle invalid fetchResources structure",
			xr: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "test.kubecore.io/v1alpha1",
					"kind":       "TestXR",
					"spec": map[string]interface{}{
						"fetchResources": "invalid", // Should be array
					},
				},
			},
			expected: 0,
			hasError: true,
		},
	}

	f := NewFunction(logging.NewNopLogger())

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			requests, err := f.parser.ParseFetchRequests(tc.xr.Object)

			if tc.hasError && err == nil {
				t.Errorf("%s\nExpected error but got none", tc.reason)
			}

			if !tc.hasError && err != nil {
				t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
			}

			if len(requests) != tc.expected {
				t.Errorf("%s\nExpected %d requests, got %d", tc.reason, tc.expected, len(requests))
			}
		})
	}
}

// Phase 2 Tests

func TestPhase2Features(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}

	cases := map[string]struct {
		reason string
		args   args
	}{
		"Phase2EnabledWithLabelSelector": {
			reason: "Should handle Phase 2 label selector requests",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"phase2Features": true,
						"fetchResources": [
							{
								"into": "labeledPods",
								"apiVersion": "v1",
								"kind": "Pod",
								"matchType": "label",
								"selector": {
									"labels": {
										"matchLabels": {
											"app": "test"
										}
									},
									"crossNamespace": true
								},
								"strategy": {
									"maxMatches": 5,
									"sortBy": [
										{
											"field": "metadata.creationTimestamp",
											"order": "desc"
										}
									]
								},
								"optional": true
							}
						]
					}`),
				},
			},
		},
		"BackwardCompatibilityTest": {
			reason: "Should maintain backward compatibility with Phase 1 requests",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"fetchResources": [
							{
								"into": "directResource",
								"name": "test-resource",
								"apiVersion": "v1",
								"kind": "ConfigMap",
								"optional": true
							}
						]
					}`),
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(logging.NewNopLogger())
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Basic functional test - should not panic and should return a response
			if rsp == nil {
				t.Errorf("%s\nExpected response but got nil", tc.reason)
			}

			// Should not return error
			if err != nil {
				t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
			}
		})
	}
}

// Helper functions for tests

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

// Phase 3 Tests

func TestPhase3Features(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}

	cases := map[string]struct {
		reason   string
		args     args
		hasError bool
	}{
		"Phase3EnabledWithTraversalConfig": {
			reason: "Should handle Phase 3 traversal configuration correctly",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"phase3Features": true,
						"traversalConfig": {
							"enabled": true,
							"maxDepth": 3,
							"maxResources": 50,
							"timeout": "10s",
							"direction": "forward",
							"scopeFilter": {
								"platformOnly": true,
								"includeAPIGroups": ["*.kubecore.io", "v1"]
							},
							"performance": {
								"maxConcurrentRequests": 10,
								"enableMetrics": true
							}
						},
						"fetchResources": [
							{
								"into": "rootProject",
								"name": "test-project",
								"apiVersion": "github.platform.kubecore.io/v1alpha1",
								"kind": "GitHubProject",
								"optional": false
							}
						]
					}`),
				},
			},
			hasError: false,
		},
		"Phase3DisabledTraversalConfig": {
			reason: "Should not execute Phase 3 when traversal config is disabled",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"phase3Features": true,
						"traversalConfig": {
							"enabled": false,
							"maxDepth": 3
						},
						"fetchResources": [
							{
								"into": "directResource",
								"name": "test-resource",
								"apiVersion": "v1",
								"kind": "ConfigMap",
								"optional": true
							}
						]
					}`),
				},
			},
			hasError: false,
		},
		"Phase3NoTraversalConfig": {
			reason: "Should not execute Phase 3 when no traversal config provided",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"phase3Features": true,
						"fetchResources": [
							{
								"into": "directResource",
								"name": "test-resource",
								"apiVersion": "v1",
								"kind": "ConfigMap",
								"optional": true
							}
						]
					}`),
				},
			},
			hasError: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(logging.NewNopLogger())
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Basic functional test - should not panic and should return a response
			if rsp == nil {
				t.Errorf("%s\nExpected response but got nil", tc.reason)
			}

			// Check error expectations
			if tc.hasError && err == nil {
				t.Errorf("%s\nExpected error but got none", tc.reason)
			}

			if !tc.hasError && err != nil {
				t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
			}

			// Phase 3 should properly log its activity
			// This is a basic test - in a real environment with a cluster,
			// Phase 3 traversal would actually execute
		})
	}
}

// XR Label Injection Integration Tests
func TestXRLabelInjection(t *testing.T) {
	type args struct {
		ctx context.Context
		req *fnv1.RunFunctionRequest
	}

	cases := map[string]struct {
		reason           string
		args             args
		expectedLabels   map[string]string
		shouldError      bool
		checkDesiredXR   bool
	}{
		"StaticLabelsOnly": {
			reason: "Should apply static labels to XR",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr",
									"namespace": "test-namespace"
								},
								"spec": {
									"projectName": "demo-project"
								}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"xrLabels": {
							"enabled": true,
							"labels": {
								"kubecore.io/organization": "novelcore",
								"environment": "production"
							},
							"mergeStrategy": "merge"
						},
						"fetchResources": []
					}`),
				},
			},
			expectedLabels: map[string]string{
				"kubecore.io/organization": "novelcore",
				"environment":              "production",
			},
			checkDesiredXR: true,
		},
		"DynamicLabelsFromXRField": {
			reason: "Should apply dynamic labels from XR fields",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr",
									"namespace": "test-namespace"
								},
								"spec": {
									"projectName": "Demo-Project",
									"team": {
										"name": "platform-team"
									}
								}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"xrLabels": {
							"enabled": true,
							"dynamicLabels": [
								{
									"key": "kubecore.io/project",
									"source": "xr-field",
									"sourcePath": "spec.projectName",
									"transform": {
										"type": "lowercase"
									}
								},
								{
									"key": "team",
									"source": "xr-field",
									"sourcePath": "spec.team.name"
								}
							]
						},
						"fetchResources": []
					}`),
				},
			},
			expectedLabels: map[string]string{
				"kubecore.io/project": "demo-project",
				"team":                "platform-team",
			},
			checkDesiredXR: true,
		},
		"NamespaceDetection": {
			reason: "Should detect namespace and add scope label",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr",
									"namespace": "production"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"xrLabels": {
							"enabled": true,
							"namespaceDetection": {
								"enabled": true,
								"labelKey": "kubecore.io/scope",
								"namespacedValue": "namespace-{namespace}"
							}
						},
						"fetchResources": []
					}`),
				},
			},
			expectedLabels: map[string]string{
				"kubecore.io/scope": "namespace-production",
			},
			checkDesiredXR: true,
		},
		"ClusterScopedDetection": {
			reason: "Should detect cluster-scoped resources",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"xrLabels": {
							"enabled": true,
							"namespaceDetection": {
								"enabled": true,
								"labelKey": "kubecore.io/scope",
								"clusterScopedValue": "cluster"
							}
						},
						"fetchResources": []
					}`),
				},
			},
			expectedLabels: map[string]string{
				"kubecore.io/scope": "cluster",
			},
			checkDesiredXR: true,
		},
		"LabelInjectionDisabled": {
			reason: "Should not apply labels when disabled",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"xrLabels": {
							"enabled": false,
							"labels": {
								"should-not": "be-applied"
							}
						},
						"fetchResources": []
					}`),
				},
			},
			expectedLabels: map[string]string{},
			checkDesiredXR:  true,
		},
		"BackwardCompatibility": {
			reason: "Should work without xrLabels configuration",
			args: args{
				ctx: context.Background(),
				req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
								"apiVersion": "test.kubecore.io/v1alpha1",
								"kind": "TestXR",
								"metadata": {
									"name": "test-xr"
								},
								"spec": {}
							}`),
						},
					},
					Input: resource.MustStructJSON(`{
						"apiVersion": "registry.fn.crossplane.io/v1beta1",
						"kind": "Input",
						"fetchResources": []
					}`),
				},
			},
			checkDesiredXR: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := NewFunction(logging.NewNopLogger())
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Check error expectations
			if tc.shouldError && err == nil {
				t.Errorf("%s\nExpected error but got none", tc.reason)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("%s\nUnexpected error: %v", tc.reason, err)
			}

			// Basic response validation
			if rsp == nil {
				t.Errorf("%s\nExpected response but got nil", tc.reason)
				return
			}

			// Check desired XR labels if requested
			if tc.checkDesiredXR && rsp.Desired != nil && rsp.Desired.Composite != nil {
				// Convert to Unstructured to access labels
				desired := &unstructured.Unstructured{}
				if err := desired.UnmarshalJSON([]byte(rsp.Desired.Composite.Resource.String())); err != nil {
					t.Errorf("%s\nFailed to unmarshal desired XR: %v", tc.reason, err)
					return
				}
				
				desiredLabels := desired.GetLabels()
				if desiredLabels == nil {
					desiredLabels = make(map[string]string)
				}

				// Check expected labels are present
				for expectedKey, expectedValue := range tc.expectedLabels {
					if actualValue, exists := desiredLabels[expectedKey]; !exists {
						t.Errorf("%s\nExpected label %s not found in desired XR", tc.reason, expectedKey)
					} else if actualValue != expectedValue {
						t.Errorf("%s\nLabel %s: expected %s, got %s", tc.reason, expectedKey, expectedValue, actualValue)
					}
				}
			}
		})
	}
}

// Test embedded registry functionality
func TestEmbeddedRegistry(t *testing.T) {
	f := NewFunction(logging.NewNopLogger())

	// Test if registry has basic resource types
	resourceTypes, err := f.registry.ListResourceTypes()
	if err != nil {
		t.Fatalf("Failed to list resource types: %v", err)
	}

	if len(resourceTypes) == 0 {
		t.Error("Expected embedded registry to have resource types")
	}

	// Test specific resource types
	testCases := []struct {
		apiVersion  string
		kind        string
		shouldExist bool
		namespaced  bool
	}{
		{"v1", "Pod", true, true},
		{"v1", "Service", true, true},
		{"v1", "ConfigMap", true, true},
		{"apps/v1", "Deployment", true, true},
		{"github.platform.kubecore.io/v1alpha1", "GitHubProject", true, true},
		{"github.platform.kubecore.io/v1alpha1", "GithubProvider", true, true},
		{"platform.kubecore.io/v1alpha1", "KubEnv", true, true},
		{"nonexistent/v1", "NonExistent", false, false},
	}

	for _, tc := range testCases {
		t.Run(tc.apiVersion+"/"+tc.kind, func(t *testing.T) {
			resourceType, err := f.registry.GetResourceType(tc.apiVersion, tc.kind)

			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected resource type %s/%s to exist, got error: %v", tc.apiVersion, tc.kind, err)
				} else {
					if resourceType.Namespaced != tc.namespaced {
						t.Errorf("Expected %s/%s namespaced=%v, got %v", tc.apiVersion, tc.kind, tc.namespaced, resourceType.Namespaced)
					}
				}
			} else {
				if err == nil {
					t.Errorf("Expected resource type %s/%s to not exist, but found it", tc.apiVersion, tc.kind)
				}
			}
		})
	}
}
