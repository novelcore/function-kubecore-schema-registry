# XR Label Injection Testing Guide

This guide demonstrates how to test the comprehensive XR Label Injection feature using the provided example compositions and claims.

## Overview

The XR Label Injection feature enables dynamic label application on Composite Resources (XRs) during the composition pipeline execution. This test suite validates all aspects of the feature including:

- Static label application
- Dynamic label extraction from XR fields
- Value transformations (lowercase, uppercase, prefix, suffix, replace, truncate, hash)
- Timestamp and UUID generation
- Namespace scope detection
- Merge strategies and label enforcement

## Test Files

### 1. Composition Definition: `test-xr-labels.yaml`
Defines the `XTestLabelInjection` XRD and composition that demonstrates all label injection capabilities.

**Key Features Demonstrated:**
- 5 static organizational labels
- 7 dynamic labels extracted from XR fields
- 6 different transformation types
- Timestamp and UUID generation
- Namespace scope detection
- Merge strategy configuration
- Label enforcement for critical labels

### 2. Claims: `claim-xr-labels.yaml`
Contains three different test claims:

1. **`xr-label-injection-demo`** - Primary test with all fields populated
2. **`xr-label-edge-cases`** - Tests merge conflicts and edge cases
3. **`xr-label-minimal`** - Tests with minimal configuration using defaults

## Testing Instructions

### Prerequisites

1. Ensure the function is built and running:
```bash
# Build the function
go build .

# Run the function locally
go run . --insecure --debug
```

2. Ensure you have the test resources in place:
```bash
# Apply the GitHubProject resource (optional, for combined testing)
kubectl apply -f example/resources/01-project.yaml
```

### Step 1: Apply the Composition

```bash
# Apply the XRD and Composition
kubectl apply -f example/cases/test-xr-labels.yaml
```

### Step 2: Test with crossplane render

Test the composition without deploying to Kubernetes:

```bash
# Test the primary claim
crossplane render \
  example/cases/claim-xr-labels.yaml \
  example/cases/test-xr-labels.yaml \
  example/functions.yaml

# Test edge cases
crossplane render \
  example/cases/claim-xr-labels.yaml \
  example/cases/test-xr-labels.yaml \
  example/functions.yaml \
  --include-claim-name xr-label-edge-cases

# Test minimal configuration
crossplane render \
  example/cases/claim-xr-labels.yaml \
  example/cases/test-xr-labels.yaml \
  example/functions.yaml \
  --include-claim-name xr-label-minimal
```

### Step 3: Deploy and Validate

```bash
# Deploy the claims
kubectl apply -f example/cases/claim-xr-labels.yaml

# Check the XR labels
kubectl get xtestlabelinjection xr-label-injection-demo -o yaml | grep -A 20 "labels:"

# Check the generated ConfigMap with results
kubectl get configmap xr-label-injection-demo-label-results -o yaml
```

## Expected Results

### For `xr-label-injection-demo` claim:

**Expected XR Labels:**
```yaml
metadata:
  labels:
    # Preserved from original
    existing-label: "should-be-preserved"
    test-type: "xr-label-injection"
    kubecore.io/test-case: "label-injection-1"
    
    # Static labels
    kubecore.io/organization: "novelcore"
    kubecore.io/managed-by: "crossplane"
    kubecore.io/function: "schema-registry"
    compliance/data-classification: "internal"
    compliance/sox-compliant: "true"
    
    # Dynamic labels with transformations
    kubecore.io/project: "demo-kubecore-project"  # lowercase
    kubecore.io/environment: "production"         # lowercase
    team/name: "team-Platform-Engineering"        # prefix
    billing/cost-center: "CC-PLATFORM-2024"       # direct
    kubecore.io/region: "region-west-2"           # replace us- with region-
    version/deployment: "v2.1.0-stable"           # suffix
    deployment/id: "deploy-202"                   # truncate to 10 chars
    
    # Generated labels
    kubecore.io/created-at: "2024-12-21T10:30:00Z"  # timestamp (actual will vary)
    kubecore.io/instance-id: "a1b2c3d4"             # UUID first 8 chars (actual will vary)
    
    # Constant label
    kubecore.io/label-test: "xr-injection-demo"
    
    # Namespace detection
    kubecore.io/scope: "namespace-default"
```

### For `xr-label-edge-cases` claim:

**Key Differences:**
- `kubecore.io/organization` should remain "novelcore" (enforced label)
- `custom-label: "preserved"` should be preserved (merge strategy)
- `kubecore.io/project: "uppercase-project-123"` (lowercase transformation)
- `kubecore.io/environment: "staging"`
- `kubecore.io/scope: "namespace-production"` (different namespace)

### For `xr-label-minimal` claim:

**Using Default Values:**
- All static labels applied
- Dynamic labels use default field values from XRD
- `kubecore.io/project: "demo-project"` (default value, lowercase)
- `kubecore.io/environment: "production"` (default value, lowercase)
- `kubecore.io/scope: "namespace-test"` (test namespace)

## Validation Points

### 1. Static Labels
✓ All 5 static labels should be present on every XR

### 2. Dynamic Label Extraction
✓ Values extracted correctly from nested fields (e.g., `spec.teamConfig.name`)
✓ Missing fields handled gracefully (minimal test case)

### 3. Transformations
✓ **lowercase**: `Demo-KubeCore-Project` → `demo-kubecore-project`
✓ **prefix**: `Platform-Engineering` → `team-Platform-Engineering`
✓ **replace**: `us-west-2` → `region-west-2`
✓ **suffix**: `v2.1.0` → `v2.1.0-stable`
✓ **truncate**: `deploy-2024-12-21-alpha` → `deploy-202`

### 4. Generated Values
✓ Timestamp in RFC3339 format
✓ UUID truncated to 8 characters
✓ Constant value applied

### 5. Namespace Detection
✓ Namespaced XRs get `namespace-{namespace}` label
✓ Cluster-scoped XRs get `cluster` label

### 6. Merge Strategy
✓ Existing labels preserved with merge strategy
✓ Conflicting labels handled according to strategy
✓ Enforced labels cannot be overridden

## Troubleshooting

### Labels Not Appearing
1. Check function logs: `kubectl logs -n crossplane-system deployment/function-kubecore-schema-registry`
2. Verify `xrLabels.enabled: true` in composition
3. Check for validation errors in function output

### Transformation Issues
1. Verify field paths are correct (use dot notation)
2. Check transformation parameters are valid
3. Review function logs for transformation errors

### Namespace Detection Issues
1. Ensure XR has metadata.namespace set (for namespaced resources)
2. Check namespaceDetection.enabled is true
3. Verify labelKey and template values

## Advanced Testing

### Test Different Merge Strategies

Modify the composition to test different strategies:
```yaml
mergeStrategy: "replace"  # Replace all existing labels
mergeStrategy: "fail-on-conflict"  # Fail if conflicts exist
```

### Test Hash Transformations

Add a hash transformation to the composition:
```yaml
- key: "kubecore.io/project-hash"
  source: "xr-field"
  sourcePath: "spec.projectName"
  transform:
    type: "hash"
    parameters:
      algorithm: "md5"  # or sha1, sha256
```

### Test Environment Variables

Add environment-sourced labels:
```yaml
- key: "kubecore.io/function-namespace"
  source: "environment"
  sourcePath: "NAMESPACE"
  value: "default"  # fallback if env var not found
```

## Success Criteria

The test is successful when:
1. ✅ All expected labels appear on the XR
2. ✅ Transformations produce correct values
3. ✅ Namespace detection works correctly
4. ✅ Merge strategy behaves as configured
5. ✅ ConfigMap documents successful execution
6. ✅ No errors in function logs

## Conclusion

This comprehensive test suite validates all aspects of the XR Label Injection feature. Use these examples as templates for your own label injection configurations, adapting the static labels, dynamic sources, and transformations to meet your organizational requirements.