package labels

import (
	"testing"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
)

func TestFieldExtractor(t *testing.T) {
	log := logging.NewNopLogger()
	extractor := NewFieldExtractor(log)

	testObj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "test-xr",
			"namespace": "test-ns",
		},
		"spec": map[string]interface{}{
			"parameters": map[string]interface{}{
				"region": "us-east-1",
				"count":  42,
				"enabled": true,
			},
		},
	}

	tests := []struct {
		name          string
		path          string
		expectedValue string
		expectError   bool
	}{
		{
			name:          "extract simple field",
			path:          "metadata.name",
			expectedValue: "test-xr",
			expectError:   false,
		},
		{
			name:          "extract nested field",
			path:          "spec.parameters.region",
			expectedValue: "us-east-1",
			expectError:   false,
		},
		{
			name:          "extract integer field",
			path:          "spec.parameters.count",
			expectedValue: "42",
			expectError:   false,
		},
		{
			name:          "extract boolean field",
			path:          "spec.parameters.enabled",
			expectedValue: "true",
			expectError:   false,
		},
		{
			name:        "missing field",
			path:        "missing.field",
			expectError: true,
		},
		{
			name:        "empty path",
			path:        "",
			expectError: true,
		},
		{
			name:        "invalid path",
			path:        "metadata.missing",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := extractor.ExtractFromXR(testObj, tt.path)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}

func TestTransformer(t *testing.T) {
	log := logging.NewNopLogger()
	transformer := NewTransformer(log)

	tests := []struct {
		name          string
		value         string
		transform     *v1beta1.LabelTransform
		expectedValue string
		expectError   bool
		checkPattern  string // for dynamic values like hash
	}{
		{
			name:  "no transformation",
			value: "unchanged",
			transform: nil,
			expectedValue: "unchanged",
			expectError:   false,
		},
		{
			name:  "lowercase transformation",
			value: "TEST-VALUE",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeLowercase,
			},
			expectedValue: "test-value",
			expectError:   false,
		},
		{
			name:  "uppercase transformation",
			value: "test-value",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeUppercase,
			},
			expectedValue: "TEST-VALUE",
			expectError:   false,
		},
		{
			name:  "prefix transformation",
			value: "test",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypePrefix,
				Options: &v1beta1.TransformOptions{
					Prefix: "managed-",
				},
			},
			expectedValue: "managed-test",
			expectError:   false,
		},
		{
			name:  "suffix transformation",
			value: "test",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeSuffix,
				Options: &v1beta1.TransformOptions{
					Suffix: "-prod",
				},
			},
			expectedValue: "test-prod",
			expectError:   false,
		},
		{
			name:  "replace transformation",
			value: "test-dev-value",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeReplace,
				Options: &v1beta1.TransformOptions{
					Old: "-dev-",
					New: "-prod-",
				},
			},
			expectedValue: "test-prod-value",
			expectError:   false,
		},
		{
			name:  "truncate transformation",
			value: "verylongtestvalue",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeTruncate,
				Options: &v1beta1.TransformOptions{
					Length: 10,
				},
			},
			expectedValue: "verylongte",
			expectError:   false,
		},
		{
			name:  "hash transformation",
			value: "test-value",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeHash,
				Options: &v1beta1.TransformOptions{
					HashAlgorithm: "sha256",
					HashLength:    8,
				},
			},
			checkPattern: "^[a-f0-9]{8}$",
			expectError:  false,
		},
		{
			name:  "prefix with missing option",
			value: "test",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypePrefix,
			},
			expectError: true,
		},
		{
			name:  "truncate with invalid length",
			value: "test",
			transform: &v1beta1.LabelTransform{
				Type: v1beta1.TransformTypeTruncate,
				Options: &v1beta1.TransformOptions{
					Length: 0,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := transformer.Transform(tt.value, tt.transform)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.checkPattern != "" {
					assert.Regexp(t, tt.checkPattern, value)
				} else {
					assert.Equal(t, tt.expectedValue, value)
				}
			}
		})
	}
}

func TestLabelValidation(t *testing.T) {
	log := logging.NewNopLogger()
	processor := NewProcessor(log, "test-namespace")

	tests := []struct {
		name        string
		key         string
		value       string
		expectError bool
	}{
		{
			name:        "valid label",
			key:         "environment",
			value:       "production",
			expectError: false,
		},
		{
			name:        "valid with dots and dashes",
			key:         "kubecore.io/environment",
			value:       "prod-env",
			expectError: false,
		},
		{
			name:        "valid with underscores",
			key:         "team_name",
			value:       "platform_team",
			expectError: false,
		},
		{
			name:        "empty value is valid",
			key:         "empty",
			value:       "",
			expectError: false,
		},
		{
			name:        "too long value",
			key:         "toolong",
			value:       "this-is-a-very-long-label-value-that-exceeds-the-kubernetes-limit-of-sixty-three-characters-and-should-fail",
			expectError: true,
		},
		{
			name:        "value with newline",
			key:         "invalid",
			value:       "test\nvalue",
			expectError: true,
		},
		{
			name:        "value with carriage return",
			key:         "invalid",
			value:       "test\rvalue",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.validateLabelValue(tt.key, tt.value)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMergeStrategies(t *testing.T) {
	log := logging.NewNopLogger()
	processor := NewProcessor(log, "test-namespace")

	existing := map[string]string{
		"existing": "value",
		"team":     "old-team",
	}

	new := map[string]string{
		"team":        "new-team",
		"environment": "production",
	}

	tests := []struct {
		name           string
		strategy       v1beta1.MergeStrategy
		enforceLabels  []string
		expected       map[string]string
		expectError    bool
	}{
		{
			name:     "merge strategy",
			strategy: v1beta1.MergeStrategyMerge,
			expected: map[string]string{
				"existing":    "value",
				"team":        "new-team", // Should overwrite
				"environment": "production",
			},
			expectError: false,
		},
		{
			name:     "replace strategy",
			strategy: v1beta1.MergeStrategyReplace,
			expected: map[string]string{
				"team":        "new-team",
				"environment": "production",
			},
			expectError: false,
		},
		{
			name:     "fail on conflict strategy",
			strategy: v1beta1.MergeStrategyFailOnConflict,
			expectError: true, // Should fail due to "team" conflict
		},
		{
			name:     "merge with enforce labels violation",
			strategy: v1beta1.MergeStrategyMerge,
			enforceLabels: []string{"team"},
			expectError: true, // Should fail because enforced label "team" is being changed
		},
		{
			name:     "merge with enforce labels no violation",
			strategy: v1beta1.MergeStrategyMerge,
			enforceLabels: []string{"existing"},
			expected: map[string]string{
				"existing":    "value", // Enforced label unchanged
				"team":        "new-team",
				"environment": "production",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.applyMergeStrategy(existing, new, tt.strategy, tt.enforceLabels)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestNamespaceDetectionStrategies(t *testing.T) {
	log := logging.NewNopLogger()

	// Test strategies individually since we can't easily mock the resource.Composite interface
	tests := []struct {
		name             string
		functionNS       string
		expectedContains string
	}{
		{
			name:       "function namespace strategy should use processor namespace",
			functionNS: "test-namespace",
			expectedContains: "test-namespace", // Should return function namespace
		},
		{
			name:       "different namespace should be stored correctly",
			functionNS: "production-namespace",
			expectedContains: "production-namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create processor with specific function namespace
			testProcessor := NewProcessor(log, tt.functionNS)
			
			// Test that the processor stores the function namespace correctly
			assert.Equal(t, tt.functionNS, testProcessor.functionNamespace)
		})
	}
}