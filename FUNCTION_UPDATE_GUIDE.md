# Function Update Guide - XR Label Injection Feature

## Current Issue
The XR label injection feature has been implemented in the code but the deployed function package doesn't include it yet. This causes the error:
```
cannot unmarshal JSON from *structpb.Struct into *v1beta1.Input: json: cannot unmarshal Go value of type v1beta1.Input: unknown name "xrLabels"
```

## Solution: Build and Deploy Updated Function Package

### Step 1: Build the Function Runtime Image

```bash
# Build the Docker image with the new XR label injection code
docker build . --platform=linux/amd64,linux/arm64 \
  --tag ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels

# Or if building for a specific platform only
docker build . --platform=linux/amd64 \
  --tag ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels
```

### Step 2: Push the Runtime Image

```bash
# Login to your registry (GitHub Container Registry in this example)
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# Push the image
docker push ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels
```

### Step 3: Build the Crossplane Package

```bash
# Build the Crossplane package with the embedded runtime
crossplane xpkg build \
  --package-root=package \
  --embed-runtime-image=ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels \
  --package-file=function-xr-labels.xpkg
```

### Step 4: Push the Crossplane Package

```bash
# Push the package to your registry
crossplane xpkg push \
  --package-files=function-xr-labels.xpkg \
  ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels
```

### Step 5: Update the Function in Kubernetes

```bash
# Update the function to use the new package
kubectl patch function function-kubecore-schema-registry --type='merge' -p \
  '{"spec":{"package":"ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels"}}'

# Or create a new function YAML
cat <<EOF | kubectl apply -f -
apiVersion: pkg.crossplane.io/v1beta1
kind: Function
metadata:
  name: function-kubecore-schema-registry
spec:
  package: ghcr.io/novelcore/function-kubecore-schema-registry:v1.0.0-xr-labels
EOF
```

### Step 6: Verify the Update

```bash
# Check the function status
kubectl get function function-kubecore-schema-registry

# Watch the function package installation
kubectl get functionrevision -w

# Check if the new revision is healthy
kubectl describe functionrevision function-kubecore-schema-registry-<revision>
```

## Testing the Updated Function

Once the function is updated, you can test the XR label injection:

### 1. Apply the test composition with XR labels
```bash
kubectl apply -f example/cases/test-xr-labels.yaml
```

### 2. Create a test claim
```bash
kubectl apply -f example/cases/claim-xr-labels.yaml
```

### 3. Verify labels are applied
```bash
# Check the XR labels
kubectl get xtestlabelinjection -o yaml | grep -A 30 "labels:"

# Check the events for any errors
kubectl describe xtestlabelinjection xr-label-injection-demo-<hash>
```

## Workaround (Current)

Until the function is updated, use the workaround compositions that apply labels via go-templating:

```bash
# Apply the workaround composition
kubectl apply -f example/cases/test-xr-labels-current.yaml

# Create the workaround claim
kubectl apply -f example/cases/claim-xr-labels-current.yaml

# This will apply labels using go-templating instead of native XR label injection
```

## Local Testing Before Deployment

You can test the function locally before deploying:

```bash
# Run the function locally
go run . --insecure --debug

# In another terminal, test with crossplane render
crossplane render \
  example/cases/claim-xr-labels.yaml \
  example/cases/test-xr-labels.yaml \
  example/functions.yaml
```

## Rollback Plan

If issues occur after updating:

```bash
# Rollback to previous version
kubectl patch function function-kubecore-schema-registry --type='merge' -p \
  '{"spec":{"package":"ghcr.io/novelcore/function-kubecore-schema-registry:v0.0.0-20250821061334-63685c3c92f3"}}'

# Use the workaround compositions for label application
kubectl apply -f example/cases/test-xr-labels-current.yaml
```

## Version Tags

Recommended version tags for the updated function:
- `v1.0.0-xr-labels` - First release with XR label injection
- `v1.0.0` - If making this the main release
- `latest` - Update if this should be the default

## Notes

1. **Backward Compatibility**: The XR label injection feature is backward compatible. Existing compositions without `xrLabels` will continue to work.

2. **Feature Flag**: The feature is controlled by `xrLabels.enabled` flag, so it's safe to deploy even if not all compositions are ready to use it.

3. **Testing**: Always test in a development environment before updating production functions.

4. **Registry**: Replace `ghcr.io/novelcore` with your actual container registry.

5. **Authentication**: Ensure you have proper credentials for pushing to your container registry.