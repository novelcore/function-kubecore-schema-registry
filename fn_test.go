package main

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
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

	cases := map[string]struct {
		reason string
		args   args
		want   want
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
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := &Function{log: logging.NewNopLogger()}
			rsp, err := f.RunFunction(tc.args.ctx, tc.args.req)

			// Basic error validation
			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Errorf("%s\nf.RunFunction(...): -want err, +got err:\n%s", tc.reason, diff)
			}

			// Manual validation for specific test cases
			if name == "SchemaRegistryBasicTest" {
				if rsp == nil {
					t.Errorf("%s\nExpected response, got nil", tc.reason)
					return
				}

				// Verify we have results
				if len(rsp.Results) == 0 {
					t.Errorf("%s\nExpected results in response", tc.reason)
				}

				// Verify we have conditions
				if len(rsp.Conditions) == 0 {
					t.Errorf("%s\nExpected conditions in response", tc.reason)
				}

				// Verify no fatal errors (function should complete successfully)
				for _, result := range rsp.Results {
					if result.Severity == fnv1.Severity_SEVERITY_FATAL {
						t.Errorf("%s\nUnexpected fatal error: %s", tc.reason, result.Message)
					}
				}

				t.Logf("Schema registry test completed successfully with %d results", len(rsp.Results))
			}
		})
	}
}
