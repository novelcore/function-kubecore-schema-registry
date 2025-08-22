# Architectural Plan: Kubernetes Label Key Validation Fix

## Executive Summary

The Crossplane function deployment is failing due to an incorrect Kubernetes label key validation pattern in the CRD. The current pattern `^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$` does not allow forward slashes, which are required for properly namespaced Kubernetes labels like `kubecore.io/namespace`.

## Problem Analysis

### Root Cause

1. **Current Validation Pattern**: `^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$`
   - Only allows alphanumeric characters, dots, underscores, and hyphens
   - **Does NOT allow forward slashes (`/`)**
   - Fails for standard Kubernetes label keys with prefixes

2. **Default Value Issue**: `kubecore.io/namespace`
   - Contains a forward slash separator between prefix and name
   - Valid according to Kubernetes standards
   - Invalid according to current CRD validation pattern

3. **Location of Issues**:
   - `/input/v1beta1/xr_labels.go`: Lines 33, 142 (validation patterns)
   - `/input/v1beta1/xr_labels.go`: Line 141 (default value)
   - Generated CRD: `/package/input/registry.fn.crossplane.io_inputs.yaml`

### Kubernetes Label Key Standards

According to official Kubernetes documentation:

1. **Label Key Structure**: `[prefix/]name`
   - **Prefix** (optional): DNS subdomain, max 253 chars, followed by `/`
   - **Name** (required): 63 chars max, alphanumeric with `-`, `_`, `.` allowed

2. **Valid Examples**:
   - `kubecore.io/namespace` ✓
   - `app.kubernetes.io/name` ✓
   - `environment` ✓ (no prefix)
   - `team-name` ✓ (no prefix)

3. **Correct Validation Pattern**:
   ```regex
   ^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/)?([a-zA-Z0-9][-a-zA-Z0-9_.]*)?[a-zA-Z0-9]$
   ```
   
   Or simplified for practical use:
   ```regex
   ^([a-z0-9.-]+\/)?[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$
   ```

## Architectural Solution

### Phase 1: Immediate Fix (Critical Path)

#### 1.1 Update Validation Pattern

**File**: `/input/v1beta1/xr_labels.go`

**Changes Required**:
```go
// Line 33 - DynamicLabel.Key validation
// OLD:
// +kubebuilder:validation:Pattern="^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$"

// NEW:
// +kubebuilder:validation:Pattern="^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\\/)?([a-zA-Z0-9][-a-zA-Z0-9_.]*)?[a-zA-Z0-9]$"

// Line 142 - NamespaceDetection.LabelKey validation
// Same pattern update as above
```

#### 1.2 Keep Default Value

The default value `kubecore.io/namespace` is actually **correct** and follows Kubernetes best practices. It should remain unchanged.

### Phase 2: Backward Compatibility Strategy

#### 2.1 Graceful Handling of Existing Deployments

1. **Detection Logic**: Check if `xrLabels` field is present in input
2. **Default Behavior**: If `xrLabels` is not specified, function operates in legacy mode
3. **Migration Helper**: Provide validation warnings for invalid label keys

#### 2.2 Input Schema Structure

```yaml
input:
  # Legacy fields remain untouched
  fetchResources: [...]
  traversal: [...]
  
  # New XR Labels feature - optional
  xrLabels:
    enabled: false  # Default to false for backward compatibility
    labels: {}
    dynamicLabels: []
    namespaceDetection:
      enabled: false
      labelKey: "kubecore.io/namespace"  # Valid default
```

### Phase 3: Implementation Architecture

#### 3.1 Component Design

```
┌─────────────────────────────────────────────────┐
│                 RunFunction                      │
├─────────────────────────────────────────────────┤
│                                                   │
│  1. Input Validation                             │
│     ├─ Check xrLabels.enabled                    │
│     └─ Validate label key patterns               │
│                                                   │
│  2. Label Processing Pipeline                    │
│     ├─ Static Label Application                  │
│     ├─ Dynamic Label Computation                 │
│     └─ Namespace Detection                       │
│                                                   │
│  3. XR Update                                    │
│     ├─ Merge Strategy Application                │
│     └─ Conflict Resolution                       │
│                                                   │
└─────────────────────────────────────────────────┘
```

#### 3.2 Label Key Validator Component

```go
// labelKeyValidator validates Kubernetes label keys
type labelKeyValidator struct {
    // Regex pattern for valid label keys
    pattern *regexp.Regexp
}

func NewLabelKeyValidator() *labelKeyValidator {
    // Compile the correct Kubernetes label key pattern
    pattern := regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/)?([a-zA-Z0-9][-a-zA-Z0-9_.]*)?[a-zA-Z0-9]$`)
    return &labelKeyValidator{pattern: pattern}
}

func (v *labelKeyValidator) Validate(key string) error {
    if !v.pattern.MatchString(key) {
        return fmt.Errorf("invalid label key %q: must match Kubernetes label key format", key)
    }
    
    // Additional validation for prefix length
    if idx := strings.Index(key, "/"); idx != -1 {
        prefix := key[:idx]
        if len(prefix) > 253 {
            return fmt.Errorf("label key prefix %q exceeds 253 characters", prefix)
        }
        name := key[idx+1:]
        if len(name) > 63 {
            return fmt.Errorf("label key name %q exceeds 63 characters", name)
        }
    } else if len(key) > 63 {
        return fmt.Errorf("label key %q exceeds 63 characters", key)
    }
    
    return nil
}
```

### Phase 4: Migration Strategy

#### 4.1 Version Compatibility Matrix

| Function Version | xrLabels Support | Default Behavior | Migration Required |
|-----------------|------------------|------------------|-------------------|
| v1.0.x          | No              | Legacy mode      | No               |
| v1.1.x          | Yes (optional)  | Legacy mode      | No               |
| v1.2.x          | Yes (optional)  | Legacy mode      | Optional         |

#### 4.2 User Migration Path

1. **Step 1**: Update function to v1.1.x (with fix)
   - No changes required to existing compositions
   - xrLabels feature available but disabled by default

2. **Step 2**: Test xrLabels in development
   ```yaml
   xrLabels:
     enabled: true
     labels:
       environment: dev
   ```

3. **Step 3**: Gradual rollout
   - Enable xrLabels per composition as needed
   - Monitor for any issues

#### 4.3 Migration Examples

**Before (v1.0.x)**:
```yaml
input:
  kind: Input
  apiVersion: registry.fn.crossplane.io/v1beta1
  fetchResources:
    - name: my-resource
      # ... existing configuration
```

**After (v1.1.x) - Backward Compatible**:
```yaml
input:
  kind: Input
  apiVersion: registry.fn.crossplane.io/v1beta1
  fetchResources:
    - name: my-resource
      # ... existing configuration
  # xrLabels not specified - works exactly as before
```

**After (v1.1.x) - With New Feature**:
```yaml
input:
  kind: Input
  apiVersion: registry.fn.crossplane.io/v1beta1
  fetchResources:
    - name: my-resource
      # ... existing configuration
  xrLabels:
    enabled: true
    labels:
      kubecore.io/managed-by: crossplane
    namespaceDetection:
      enabled: true
      labelKey: "kubecore.io/namespace"  # Now valid!
```

## Implementation Checklist

### Immediate Actions (P0 - Critical)

- [ ] Update validation pattern in `/input/v1beta1/xr_labels.go` (lines 33, 142)
- [ ] Run `go generate ./...` to regenerate CRD with correct pattern
- [ ] Test CRD installation with `kubectl apply`
- [ ] Verify function deployment succeeds

### Short-term Actions (P1 - High)

- [ ] Add comprehensive unit tests for label key validation
- [ ] Implement labelKeyValidator component
- [ ] Add integration tests for xrLabels feature
- [ ] Update example files with valid label keys

### Medium-term Actions (P2 - Medium)

- [ ] Create migration guide documentation
- [ ] Add validation warnings for deprecated patterns
- [ ] Implement metrics for xrLabels usage
- [ ] Create CI/CD validation for label keys

## Testing Strategy

### Unit Tests

```go
func TestLabelKeyValidation(t *testing.T) {
    tests := []struct {
        name    string
        key     string
        wantErr bool
    }{
        {"valid with prefix", "kubecore.io/namespace", false},
        {"valid kubernetes prefix", "app.kubernetes.io/name", false},
        {"valid no prefix", "environment", false},
        {"valid with hyphen", "team-name", false},
        {"invalid double slash", "kubecore.io//namespace", true},
        {"invalid special char", "kubecore.io/name@space", true},
        {"prefix too long", strings.Repeat("a", 254) + "/name", true},
        {"name too long", "prefix/" + strings.Repeat("a", 64), true},
    }
    
    validator := NewLabelKeyValidator()
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := validator.Validate(tt.key)
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
            }
        })
    }
}
```

### Integration Tests

1. **CRD Validation Test**: Apply CRD with various label key patterns
2. **Function Deployment Test**: Deploy function with xrLabels configuration
3. **Label Application Test**: Verify labels are correctly applied to XRs
4. **Backward Compatibility Test**: Ensure legacy configurations still work

## Risk Assessment

### Risks

1. **Breaking Change Risk**: LOW - Default behavior unchanged
2. **Performance Impact**: MINIMAL - Regex validation is fast
3. **Migration Complexity**: LOW - Opt-in feature
4. **Security Risk**: NONE - Validation is more permissive but still secure

### Mitigation Strategies

1. **Phased Rollout**: Deploy to dev/staging first
2. **Feature Flag**: Keep xrLabels disabled by default initially
3. **Monitoring**: Track function execution times and errors
4. **Rollback Plan**: Previous version remains compatible

## Success Criteria

1. **Immediate Success**: Function deploys without CRD validation errors
2. **Feature Success**: XR labels are correctly applied with namespaced keys
3. **Compatibility Success**: Existing deployments continue working unchanged
4. **Adoption Success**: Users can gradually adopt xrLabels feature

## Conclusion

This architectural plan provides a comprehensive solution to the Kubernetes label key validation issue while ensuring backward compatibility and providing a smooth migration path. The key insight is that the current validation pattern is too restrictive and doesn't follow Kubernetes standards for label keys with prefixes.

The proposed solution:
1. Fixes the immediate deployment blocker
2. Maintains backward compatibility
3. Follows Kubernetes best practices
4. Provides a clear migration path
5. Includes comprehensive testing strategy

By implementing this plan, the function will support proper Kubernetes label keys while maintaining stability for existing users.