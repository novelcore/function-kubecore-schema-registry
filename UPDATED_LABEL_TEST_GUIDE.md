# Updated XR Label Injection Test Guide

## Updated Schema Changes

The XR label injection schema has been updated with the following changes:

### Key Schema Updates
1. **Transform Options**: Now uses `options` instead of `parameters`
   ```yaml
   # OLD
   transform:
     type: "prefix"
     parameters:
       prefix: "team-"
   
   # NEW
   transform:
     type: "prefix"
     options:
       prefix: "team-"
   ```

2. **Transform Option Fields**: Updated field names
   - `parameters.prefix` → `options.prefix`
   - `parameters.suffix` → `options.suffix`
   - `parameters.from/to` → `options.old/new`
   - `parameters.length` → `options.length`

3. **Hash Transformation**: Enhanced with algorithm and length options
   ```yaml
   transform:
     type: "hash"
     options:
       hashAlgorithm: "sha256"  # md5, sha1, sha256
       hashLength: 8            # 4-64 characters
   ```

4. **Namespace Detection**: Enhanced strategies
   ```yaml
   namespaceDetection:
     enabled: true
     labelKey: "kubecore.io/scope"
     strategy: "auto"                    # auto, xr-namespace, function-namespace
     fallbackStrategy: "function-namespace"  # fallback when primary fails
     defaultNamespace: "default"        # final fallback
   ```

5. **Required Field**: Added to dynamic labels
   ```yaml
   dynamicLabels:
     - key: "important-label"
       source: "xr-field"
       sourcePath: "spec.criticalField"
       required: true  # Function will fail if this label can't be applied
   ```

## Quick Test (Simple Example)

The simple example works with the current deployed function once you build and deploy the updated package:

### 1. Test Locally First
```bash
# Run function with updated code
go run . --insecure --debug

# Test with crossplane render
crossplane render \
  example/cases/claim-simple-labels.yaml \
  example/cases/test-simple-labels.yaml \
  example/functions.yaml
```

### 2. Expected Results for Simple Test
The XR should get these labels:
```yaml
labels:
  # Preserved existing
  existing-label: "preserved"
  test-category: "xr-label-injection"
  
  # Static labels applied by function
  kubecore.io/organization: "novelcore"
  kubecore.io/managed-by: "crossplane"
  environment: "test"
  
  # Dynamic labels with transformations
  kubecore.io/project: "demo-project-123"      # lowercase transform
  kubecore.io/env-prefixed: "env-PRODUCTION"   # prefix transform  
  kubecore.io/created: "2024-12-21T10:30:00Z"  # timestamp
  kubecore.io/test-type: "simple-label-test"   # constant
  kubecore.io/scope: "default"                 # namespace detection
```

## Deploy Updated Function Package

To use these examples in your cluster, you need to build and deploy the updated function:

### 1. Build and Push
```bash
# Build image with updated code
docker build . --platform=linux/amd64 \
  --tag ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-updated

# Push to registry
docker push ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-updated

# Build Crossplane package
crossplane xpkg build \
  --package-root=package \
  --embed-runtime-image=ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-updated \
  --package-file=function-updated.xpkg

# Push package
crossplane xpkg push \
  --package-files=function-updated.xpkg \
  ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-updated
```

### 2. Update Function in Kubernetes
```bash
# Update the function
kubectl patch function function-kubecore-schema-registry --type='merge' -p \
  '{"spec":{"package":"ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-updated"}}'

# Wait for it to be ready
kubectl get functionrevision -w
```

### 3. Test in Cluster
```bash
# Create the namespace if testing namespace detection
kubectl create namespace production --dry-run=client -o yaml | kubectl apply -f -

# Apply the simple test
kubectl apply -f example/cases/test-simple-labels.yaml
kubectl apply -f example/cases/claim-simple-labels.yaml

# Check results
kubectl get xsimplelabel simple-label-test-<hash> -o yaml | grep -A 20 "labels:"
kubectl get configmap simple-label-test-<hash>-simple-labels -o yaml
```

## Full Featured Test

Once the simple test works, you can try the comprehensive test:

```bash
# Apply the full featured test
kubectl apply -f example/cases/test-xr-labels.yaml
kubectl apply -f example/cases/claim-xr-labels.yaml

# This demonstrates all transformation types and features
```

## Troubleshooting

### Schema Validation Errors
If you get validation errors, ensure:
1. `options` is used instead of `parameters`
2. Correct field names in options (e.g., `old`/`new` not `from`/`to`)
3. `required` field is boolean
4. `hashAlgorithm` is one of: md5, sha1, sha256

### Function Not Recognizing xrLabels
This means the deployed function doesn't have the updated code yet. Follow the build and deploy steps above.

### Labels Not Appearing
Check function logs:
```bash
kubectl logs -n crossplane-system deployment/function-kubecore-schema-registry
```

Look for messages about label processing.

## Schema Differences Summary

| Feature | Old Schema | New Schema |
|---------|------------|------------|
| Transform options | `parameters` | `options` |
| Prefix option | `parameters.prefix` | `options.prefix` |
| Replace options | `parameters.from/to` | `options.old/new` |
| Hash config | Not available | `options.hashAlgorithm/hashLength` |
| Required labels | Not available | `required: true/false` |
| Namespace strategy | Simple template | Strategy-based with fallbacks |

The updated schema provides more control and flexibility while maintaining backward compatibility for the core functionality.