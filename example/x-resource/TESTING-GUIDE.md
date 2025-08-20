# Phase 1 Schema Registry Function Testing Guide

## Overview
This directory contains comprehensive test cases for validating the Phase 1 Schema Registry Function implementation. The tests cover basic functionality, error handling, performance, and edge cases.

## Test Files Structure

```
example/x-resource/
├── c.yaml                 # Main success case composition
├── c-error-tests.yaml     # Error scenario compositions  
├── r.yaml                 # All test case XR definitions
└── TESTING-GUIDE.md       # This file
```

## Test Prerequisites

### Required Resources in Cluster
Before running tests, ensure these resources exist:

```bash
# Create test namespace
kubectl create namespace test

# Create sample GitHubProject (modify as needed for your environment)
kubectl apply -f - <<EOF
apiVersion: github.platform.kubecore.io/v1alpha1
kind: GitHubProject
metadata:
  name: demo-project
  namespace: test
spec:
  name: demo-project
  description: "Demo project for schema registry testing"
  visibility: private
  githubProviderRef:
    name: gh-default
  repositorySource:
    type: template
    template:
      owner: kubecore
      repository: template-repo
EOF

# Create sample GithubProvider (modify as needed for your environment) 
kubectl apply -f - <<EOF
apiVersion: github.platform.kubecore.io/v1alpha1
kind: GithubProvider
metadata:
  name: gh-default
  namespace: default
spec:
  github:
    organization: kubecore
    isEnterprise: false
  secretStoreRef:
    kind: ClusterSecretStore
    name: default
  refreshInterval: 1m
EOF
```

## Test Cases Overview

### Test Case 1: Basic Dual Resource Fetch ✅
- **Purpose**: Primary success scenario
- **Test**: Fetch both GitHubProject and GithubProvider
- **Expected**: 2 successful, 0 failed, 0 skipped
- **Composition**: `c.yaml`
- **XR**: `phase1-dual-resource-test`

### Test Case 2: GitHubProject Only ✅
- **Purpose**: Isolation testing 
- **Test**: Fetch only GitHubProject resource
- **Expected**: 1 successful, 0 failed, 0 skipped
- **Composition**: `c.yaml` (modify fetchResources)
- **XR**: `phase1-project-only-test`

### Test Case 3: GithubProvider Only ✅
- **Purpose**: Isolation testing
- **Test**: Fetch only GithubProvider resource  
- **Expected**: 1 successful, 0 failed, 0 skipped
- **Composition**: `c.yaml` (modify fetchResources)
- **XR**: `phase1-provider-only-test`

### Test Case 4: Partial Failure Test ⚠️
- **Purpose**: Error recovery validation
- **Test**: 1 valid + 1 invalid resource
- **Expected**: 1 successful, 1 failed, 0 skipped
- **Composition**: `c-error-tests.yaml` (partial-failure)
- **XR**: `phase1-partial-failure-test`

### Test Case 5: Namespace Validation ✅
- **Purpose**: Cross-namespace fetch testing
- **Test**: Resources from different namespaces
- **Expected**: Proper namespace isolation
- **Composition**: `c.yaml`
- **XR**: `phase1-namespace-test`

### Test Case 6: Performance Test ✅
- **Purpose**: Multiple resource performance
- **Test**: Multiple concurrent fetches
- **Expected**: All resources fetched efficiently
- **Composition**: `c.yaml` (modify for multiple resources)
- **XR**: `phase1-performance-test`

### Test Case 7: Context Structure Validation ✅
- **Purpose**: Template compatibility testing
- **Test**: Validate exact context structure
- **Expected**: Proper context keys and nesting
- **Composition**: `c.yaml`
- **XR**: `phase1-context-validation-test`

### Test Case 8: Resource Not Found Test ⚠️
- **Purpose**: Full failure scenario
- **Test**: All requested resources don't exist
- **Expected**: 0 successful, N failed, 0 skipped
- **Composition**: `c-error-tests.yaml` (full-failure)
- **XR**: `phase1-not-found-test`

### Test Case 9: Timeout and Concurrency Test ⚠️
- **Purpose**: Performance limits testing
- **Test**: Short timeout + limited concurrency
- **Expected**: Function respects limits
- **Composition**: `c-error-tests.yaml` (timeout)
- **XR**: `phase1-timeout-test`

### Test Case 10: Legacy Context Key Test ✅
- **Purpose**: Backward compatibility
- **Test**: Both new and legacy context keys
- **Expected**: Both context keys populated
- **Composition**: `c-error-tests.yaml` (legacy-context)
- **XR**: `phase1-legacy-context-test`

## Running Tests

### Prerequisites Check
```bash
# Verify function is deployed
kubectl get functions function-kubecore-schema-registry

# Verify test resources exist
kubectl get githubproject demo-project -n test
kubectl get githubprovider gh-default -n default

# Verify compositions are applied
kubectl get compositions | grep xschemaregistrytest
```

### Run Individual Test Cases

#### Success Case Tests
```bash
# Test Case 1: Basic dual resource fetch
kubectl apply -f c.yaml
kubectl apply -f - <<EOF
apiVersion: test.kubecore.platform.io/v1alpha1
kind: SchemaRegistryTest
metadata:
  name: phase1-dual-resource-test
  namespace: default
spec:
  testName: phase1-dual-github
EOF

# Monitor results
kubectl describe xschemaregistrytest phase1-dual-resource-test
kubectl get configmaps -l kubecore.io/test-name=phase1-dual-github
```

#### Error Case Tests
```bash
# Test Case 4: Partial failure
kubectl apply -f c-error-tests.yaml
kubectl apply -f - <<EOF
apiVersion: test.kubecore.platform.io/v1alpha1
kind: SchemaRegistryTest
metadata:
  name: phase1-partial-failure-test
  namespace: default
spec:
  testName: phase1-partial-failure
EOF

# Monitor error handling
kubectl describe xschemaregistrytest phase1-partial-failure-test
kubectl get configmap phase1-partial-failure-error-report -o yaml
```

### Run All Tests
```bash
# Apply all compositions
kubectl apply -f c.yaml
kubectl apply -f c-error-tests.yaml

# Apply all test cases
kubectl apply -f r.yaml

# Monitor all tests
kubectl get xschemaregistrytest
kubectl get configmaps -l kubecore.io/test-type
```

### Monitor Function Execution
```bash
# Watch function logs
kubectl logs -l 'pkg.crossplane.io/function=function-kubecore-schema-registry' --tail=100 -f

# Check function pod status
kubectl get pods -l 'pkg.crossplane.io/function=function-kubecore-schema-registry' -n crossplane-system

# Monitor XR status across all tests
kubectl get xschemaregistrytest -o custom-columns=NAME:.metadata.name,READY:.status.conditions[0].status,REASON:.status.conditions[0].reason
```

## Validation Criteria

### Success Criteria
- **Function Execution**: No crashes or timeouts
- **Resource Fetching**: Correct resources fetched with proper metadata
- **Context Population**: Resources available in both context keys
- **Template Compatibility**: go-template can access all resource fields
- **Error Handling**: Graceful handling of missing resources
- **Performance**: Function completes within 30 seconds

### Expected ConfigMaps per Test
Each successful test should create:
- Summary ConfigMap with fetch statistics
- Resource detail ConfigMaps for each fetched resource  
- Relationships ConfigMap showing resource connections
- Debug ConfigMap with context structure
- (Error tests create specific error report ConfigMaps)

### Common Issues and Debugging

#### Issue: "Resource fetch partially failed"
- **Cause**: Missing or inaccessible resources
- **Debug**: Check resource existence with `kubectl get <resource>`
- **Fix**: Ensure test resources are created in correct namespaces

#### Issue: "reflect: call of reflect.Value.Type on zero Value"
- **Cause**: Template accessing non-existent context fields
- **Debug**: Check debug ConfigMap for actual context structure
- **Fix**: Update template to match actual context keys

#### Issue: Function timeout
- **Cause**: Network issues or resource API problems
- **Debug**: Check function logs and cluster connectivity
- **Fix**: Increase timeout or check cluster health

#### Issue: Context keys missing
- **Cause**: Function not setting context properly
- **Debug**: Check function logs for errors during context building
- **Fix**: Verify function version and input schema

## Expected Test Results

### Perfect Success Run (Test Case 1)
```yaml
Summary ConfigMap Data:
  fetch-summary-total: "2"
  fetch-summary-successful: "2" 
  fetch-summary-failed: "0"
  fetch-summary-skipped: "0"
  github-project-count: "1"
  github-provider-count: "1"

XR Status:
  conditions:
  - type: Ready
    status: "True"
    reason: Available
```

### Partial Failure Run (Test Case 4)
```yaml
Summary ConfigMap Data:
  fetch-summary-total: "2"
  fetch-summary-successful: "1"
  fetch-summary-failed: "1" 
  fetch-summary-skipped: "0"
  
XR Status:
  conditions:
  - type: Ready
    status: "False"
    reason: SomeResourcesFailed
```

## Cleanup

```bash
# Remove all test XRs
kubectl delete -f r.yaml

# Remove test compositions  
kubectl delete -f c.yaml
kubectl delete -f c-error-tests.yaml

# Clean up test ConfigMaps
kubectl delete configmaps -l kubecore.io/test-type

# Clean up test resources (optional)
kubectl delete githubproject demo-project -n test
kubectl delete githubprovider gh-default -n default
kubectl delete namespace test
```