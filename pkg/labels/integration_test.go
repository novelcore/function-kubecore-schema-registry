package labels

import (
	"context"
	"testing"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
)

func TestXRLabelIntegration(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		xr          string
		expectError bool
		checkLabels func(t *testing.T, xr map[string]interface{})
	}{
		{
			name: "static labels with merge strategy",
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": true,
					"labels": {
						"environment": "production",
						"team": "platform"
					},
					"mergeStrategy": "merge"
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "test-xr",
					"labels": {
						"existing": "value"
					}
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				labelsInterface, exists := metadata["labels"]
				require.True(t, exists, "labels should exist in metadata")
				require.NotNil(t, labelsInterface, "labels should not be nil")
				
				labels, ok := labelsInterface.(map[string]interface{})
				require.True(t, ok, "labels should be a map[string]interface{}")
				
				// Should have merged labels
				assert.Equal(t, "value", labels["existing"])
				assert.Equal(t, "production", labels["environment"])
				assert.Equal(t, "platform", labels["team"])
			},
		},
		{
			name: "dynamic labels from XR fields",
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": true,
					"dynamicLabels": [
						{
							"key": "xr-name",
							"source": "xr-field",
							"sourcePath": "metadata.name"
						},
						{
							"key": "region",
							"source": "xr-field", 
							"sourcePath": "spec.parameters.region"
						}
					],
					"mergeStrategy": "merge"
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "production-cluster"
				},
				"spec": {
					"parameters": {
						"region": "us-west-2"
					}
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				labelsInterface, exists := metadata["labels"]
				require.True(t, exists, "labels should exist in metadata")
				require.NotNil(t, labelsInterface, "labels should not be nil")
				
				labels, ok := labelsInterface.(map[string]interface{})
				require.True(t, ok, "labels should be a map[string]interface{}")
				
				assert.Equal(t, "production-cluster", labels["xr-name"])
				assert.Equal(t, "us-west-2", labels["region"])
			},
		},
		{
			name: "dynamic labels with transformations",
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": true,
					"dynamicLabels": [
						{
							"key": "xr-name-lower",
							"source": "xr-field",
							"sourcePath": "metadata.name",
							"transform": {
								"type": "lowercase"
							}
						},
						{
							"key": "prefixed-name",
							"source": "xr-field",
							"sourcePath": "metadata.name",
							"transform": {
								"type": "prefix",
								"options": {
									"prefix": "managed-"
								}
							}
						}
					],
					"mergeStrategy": "merge"
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "Production-Cluster"
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				labelsInterface, exists := metadata["labels"]
				require.True(t, exists, "labels should exist in metadata")
				require.NotNil(t, labelsInterface, "labels should not be nil")
				
				labels, ok := labelsInterface.(map[string]interface{})
				require.True(t, ok, "labels should be a map[string]interface{}")
				
				assert.Equal(t, "production-cluster", labels["xr-name-lower"])
				assert.Equal(t, "managed-Production-Cluster", labels["prefixed-name"])
			},
		},
		{
			name: "constant source labels", 
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": true,
					"dynamicLabels": [
						{
							"key": "managed-by",
							"source": "constant",
							"value": "crossplane"
						},
						{
							"key": "environment",
							"source": "constant",
							"value": "production"
						}
					],
					"mergeStrategy": "merge"
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "test-xr"
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				labelsInterface, exists := metadata["labels"]
				require.True(t, exists, "labels should exist in metadata")
				require.NotNil(t, labelsInterface, "labels should not be nil")
				
				labels, ok := labelsInterface.(map[string]interface{})
				require.True(t, ok, "labels should be a map[string]interface{}")
				
				assert.Equal(t, "crossplane", labels["managed-by"])
				assert.Equal(t, "production", labels["environment"])
			},
		},
		{
			name: "namespace detection",
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": true,
					"namespaceDetection": {
						"enabled": true,
						"labelKey": "kubecore.io/namespace",
						"strategy": "xr-namespace"
					},
					"mergeStrategy": "merge"
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "test-xr",
					"namespace": "production"
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				labelsInterface, exists := metadata["labels"]
				require.True(t, exists, "labels should exist in metadata")
				require.NotNil(t, labelsInterface, "labels should not be nil")
				
				labels, ok := labelsInterface.(map[string]interface{})
				require.True(t, ok, "labels should be a map[string]interface{}")
				
				assert.Equal(t, "production", labels["kubecore.io/namespace"])
			},
		},
		{
			name: "disabled XR labels should not affect XR",
			input: `{
				"apiVersion": "registry.fn.crossplane.io/v1beta1",
				"kind": "Input",
				"xrLabels": {
					"enabled": false,
					"labels": {
						"should": "not-appear"
					}
				},
				"fetchResources": []
			}`,
			xr: `{
				"apiVersion": "test.kubecore.io/v1alpha1",
				"kind": "TestXR",
				"metadata": {
					"name": "test-xr"
				}
			}`,
			expectError: false,
			checkLabels: func(t *testing.T, xr map[string]interface{}) {
				metadata, ok := xr["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a map[string]interface{}")
				
				// Should not have labels added when XR labels are disabled
				if labelsInterface, exists := metadata["labels"]; exists && labelsInterface != nil {
					labels, ok := labelsInterface.(map[string]interface{})
					require.True(t, ok, "labels should be a map[string]interface{}")
					assert.NotContains(t, labels, "should", "should not contain labels when XR labels are disabled")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a request with the test input
			req := &fnv1.RunFunctionRequest{
				Meta: &fnv1.RequestMeta{Tag: "test"},
				Observed: &fnv1.State{
					Composite: &fnv1.Resource{
						Resource: resource.MustStructJSON(tt.xr),
					},
				},
				Input: resource.MustStructJSON(tt.input),
			}

			// Create function instance and run it
			f := createTestFunction()
			
			rsp, err := f.RunFunction(context.Background(), req)

			// Check error expectations
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, rsp)

			// Check that XR has been modified with labels
			if tt.checkLabels != nil {
				// For enabled cases, check the desired state
				if rsp.Desired != nil && rsp.Desired.Composite != nil && rsp.Desired.Composite.Resource != nil {
					xrObj := rsp.Desired.Composite.Resource.AsMap()
					tt.checkLabels(t, xrObj)
				} else {
					// For disabled case, check the original observed XR
					xrObj := req.Observed.Composite.Resource.AsMap()
					tt.checkLabels(t, xrObj)
				}
			}
		})
	}
}

// createTestFunction creates a test function instance with minimal dependencies
func createTestFunction() testableFunction {
	return testableFunction{
		processor: NewProcessor(logging.NewNopLogger(), "crossplane-system"),
	}
}

// testableFunction is a minimal version for testing label functionality
type testableFunction struct {
	processor *Processor
}

func (f testableFunction) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	// Extract function input
	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		return nil, err
	}

	// Get observed XR
	observedXR, err := request.GetObservedCompositeResource(req)
	if err != nil {
		return nil, err
	}

	// Return minimal successful response
	rsp := response.To(req, response.DefaultTTL)

	// Process XR label injection if enabled
	if in.XRLabels != nil && in.XRLabels.Enabled {
		if err := f.processor.ProcessLabels(ctx, observedXR, in.XRLabels); err != nil {
			return nil, err
		}
		// Set the modified XR in the desired state
		response.SetDesiredCompositeResource(rsp, observedXR)
	}

	return rsp, nil
}