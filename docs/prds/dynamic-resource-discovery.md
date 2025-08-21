
## **Enhanced PRD Section: Dynamic Resource Type Discovery**

### **New Feature: Dynamic CRD Discovery**

Replace the static embedded registry with dynamic discovery of all KubeCore platform CRDs at function startup.

---

## **Functional Requirements Addition**

### **FR-6: Dynamic Resource Type Discovery**

**FR-6.1**: The function SHALL discover all Custom Resource Definitions (CRDs) with API groups matching `*.kubecore.io` at startup.

**FR-6.2**: The function SHALL build the resource registry dynamically from discovered CRD schemas.

**FR-6.3**: The function SHALL extract reference fields from CRD OpenAPI schemas using pattern matching.

**FR-6.4**: The function SHALL cache discovered resource types for the duration of the function execution.

**FR-6.5**: The function SHALL log discovery statistics at startup showing resources found and registered.

**FR-6.6**: The function SHALL fall back to embedded definitions if CRD discovery fails.

---

## **Technical Requirements Addition**

### **TR-4: Dynamic Discovery Requirements**

**TR-4.1**: The function MUST query the Kubernetes API for CRDs on initialization.

**TR-4.2**: The function MUST parse OpenAPI v3 schemas from CRD specifications.

**TR-4.3**: The function MUST identify reference fields using naming conventions (`*Ref`, `*Reference`).

**TR-4.4**: The function MUST complete CRD discovery within 5 seconds at startup.

**TR-4.5**: The function MUST handle missing or malformed CRD schemas gracefully.

---

## **Implementation Specification**

### **Dynamic Discovery Algorithm**

```yaml
# Discovery Process
startup:
  steps:
    1. Query all CRDs from cluster
    2. Filter by API group pattern (*.kubecore.io)
    3. Extract schema information from each CRD
    4. Identify reference fields using patterns
    5. Build registry entries
    6. Log discovery results
    7. Cache for function lifetime
```

### **Reference Field Detection Patterns**

```go
// Reference field patterns to detect
referencePatterns := []string{
    "*Ref",           // e.g., kubeClusterRef
    "*Reference",     // e.g., clusterReference
    "*RefName",       // e.g., providerRefName
    "providerConfigRef.*",  // nested provider configs
    "secretRef.*",    // secret references
    "configMapRef.*", // configmap references
}

// Field analysis rules
rules:
  - If field name matches pattern AND type is string/object
  - If field has description containing "reference to"
  - If field schema has x-kubernetes-object-ref-* extensions
  - If field type matches known reference object patterns
```

### **Discovery Logging Specification**

```go
// Startup logging format
INFO[0000] Starting dynamic resource discovery...
INFO[0001] Discovering CRDs with API groups matching *.kubecore.io
INFO[0001] Found 24 CRDs matching criteria
INFO[0002] Processing CRD: platform.kubecore.io/v1alpha1 KubeCluster
INFO[0002]   - Detected 3 reference fields
INFO[0002]   - Namespaced: false
INFO[0002]   - Plural: kubeclusters
INFO[0002] Processing CRD: github.platform.kubecore.io/v1alpha1 GitHubProject
INFO[0002]   - Detected 1 reference field
INFO[0002]   - Namespaced: true
INFO[0002]   - Plural: githubprojects
...
INFO[0003] Discovery complete:
INFO[0003]   Total CRDs discovered: 24
INFO[0003]   Total reference fields: 47
INFO[0003]   API Groups found: [platform.kubecore.io, github.platform.kubecore.io, app.kubecore.io]
INFO[0003]   Discovery time: 2.347s
INFO[0003] Registry initialized with 24 resource types
```

### **Error Handling for Discovery**

```yaml
# Discovery failure scenarios
scenarios:
  - type: NO_CRDS_ACCESS
    action: Fall back to embedded registry
    log: "WARN: Cannot access CRDs, using embedded registry (24 types)"
    
  - type: PARTIAL_DISCOVERY
    action: Merge discovered with embedded
    log: "WARN: Partial CRD discovery (found 15/24), merging with embedded"
    
  - type: SCHEMA_PARSE_ERROR
    action: Skip problematic CRD
    log: "WARN: Cannot parse schema for CRD X, skipping"
    
  - type: TIMEOUT
    action: Use what was discovered
    log: "WARN: Discovery timeout after 5s, using 18 discovered types"
```

---

## **New Package Structure**

```
pkg/
├── discovery/
│   ├── dynamic/
│   │   ├── crd_discoverer.go      # CRD discovery logic
│   │   ├── schema_parser.go       # OpenAPI schema parsing
│   │   ├── reference_detector.go  # Reference field detection
│   │   └── registry_builder.go    # Build registry from CRDs
│   └── ...existing...
├── registry/
│   ├── dynamic_registry.go        # New: Dynamic registry implementation
│   ├── embedded_registry.go       # Existing: Fallback registry
│   ├── hybrid_registry.go         # New: Combines dynamic + embedded
│   └── ...existing...
```

---

## **Configuration Options**

```yaml
# Function configuration (via ConfigMap or flags)
discovery:
  mode: dynamic              # dynamic, embedded, or hybrid
  apiGroups:                # Patterns to match
    - "*.kubecore.io"
    - "*.platform.io"      # Optional additional patterns
  timeout: 5s
  cacheEnabled: true
  fallbackToEmbedded: true
  referencePatterns:        # Customizable patterns
    - "*Ref"
    - "*Reference"
  logging:
    level: info            # debug for detailed discovery
    showProgress: true
    showStatistics: true
```

---

## **Implementation Example**

```go
// DynamicDiscoverer interface
type DynamicDiscoverer interface {
    DiscoverCRDs(ctx context.Context, patterns []string) ([]*CRDInfo, error)
    ExtractSchema(crd *apiextv1.CustomResourceDefinition) (*ResourceSchema, error)
    DetectReferences(schema *ResourceSchema) []ReferenceField
    BuildRegistry(crds []*CRDInfo) Registry
}

// CRDInfo structure
type CRDInfo struct {
    Group       string
    Version     string
    Kind        string
    Plural      string
    Namespaced  bool
    Schema      *apiextv1.JSONSchemaProps
    References  []ReferenceField
}

// Startup sequence
func (f *Function) Initialize(ctx context.Context) error {
    logger.Info("Starting dynamic resource discovery...")
    
    discoverer := NewDynamicDiscoverer(f.client)
    crds, err := discoverer.DiscoverCRDs(ctx, []string{"*.kubecore.io"})
    
    if err != nil {
        logger.Warnf("CRD discovery failed: %v, falling back to embedded", err)
        f.registry = registry.NewEmbeddedRegistry()
        return nil
    }
    
    logger.Infof("Found %d CRDs matching criteria", len(crds))
    
    for _, crd := range crds {
        logger.Debugf("Processing CRD: %s/%s %s", crd.Group, crd.Version, crd.Kind)
        logger.Debugf("  - Detected %d reference fields", len(crd.References))
        logger.Debugf("  - Namespaced: %v", crd.Namespaced)
    }
    
    f.registry = discoverer.BuildRegistry(crds)
    
    // Log summary statistics
    f.logDiscoveryStatistics(crds)
    
    return nil
}
```

---

## **Benefits of Dynamic Discovery**

1. **Zero Maintenance**: New CRDs automatically discovered
2. **Version Agnostic**: Works with any CRD version
3. **Custom Resources**: Supports organization-specific CRDs
4. **Real-time Schema**: Always uses current CRD schema
5. **Extensible**: Pattern-based discovery is flexible
6. **Observable**: Detailed logging for troubleshooting

---

## **Testing Requirements Addition**

### **New Test Cases for Dynamic Discovery**

| Test Case | Description | Validation |
|-----------|-------------|------------|
| Discovery Success | All KubeCore CRDs discovered | 24+ types registered |
| Partial Discovery | Some CRDs inaccessible | Hybrid registry works |
| Discovery Failure | No CRD access | Falls back to embedded |
| Pattern Matching | Various API group patterns | Correct filtering |
| Reference Detection | Complex nested references | All refs found |
| Performance | Discovery of 50+ CRDs | <5 second completion |
| Schema Parsing | Various OpenAPI schemas | Correct extraction |

---

## **Success Criteria Addition**

- [ ] Dynamic discovery finds all `*.kubecore.io` CRDs
- [ ] Reference fields correctly identified using patterns
- [ ] Startup logging shows discovered resources
- [ ] Fallback to embedded registry on failure
- [ ] Discovery completes in <5 seconds
- [ ] Registry contains all discovered types
- [ ] Function works with both dynamic and embedded modes

---

## **Migration Path**

```yaml
# Phase 1: Embedded only (current)
mode: embedded

# Phase 2A: Hybrid mode (safe rollout)
mode: hybrid  # Try dynamic, fallback to embedded

# Phase 2B: Full dynamic (after validation)
mode: dynamic  # Pure dynamic discovery
```

---

## **Example Startup Logs**

```
INFO[0000] function-kubecore-schema-registry starting version=2.1.0
INFO[0000] Initializing dynamic resource discovery mode=dynamic
INFO[0001] Querying Kubernetes API for CRDs... 
INFO[0001] Found 142 total CRDs in cluster
INFO[0001] Filtering for API groups matching *.kubecore.io
INFO[0002] Discovered 26 matching CRDs:
INFO[0002]   ✓ platform.kubecore.io (12 types)
INFO[0002]     - KubeCluster (cluster-scoped)
INFO[0002]     - KubeSystem (namespaced)
INFO[0002]     - KubeNet (namespaced)
INFO[0002]     - KubEnv (namespaced)
INFO[0002]     - QualityGate (namespaced)
INFO[0002]     ...
INFO[0002]   ✓ github.platform.kubecore.io (8 types)
INFO[0002]     - GitHubProject (namespaced)
INFO[0002]     - GitHubApp (namespaced)
INFO[0002]     - GithubProvider (namespaced)
INFO[0002]     ...
INFO[0002]   ✓ app.kubecore.io (6 types)
INFO[0002]     - App (namespaced)
INFO[0002]     - AppEnvironment (namespaced)
INFO[0002]     ...
INFO[0003] Analyzing CRD schemas for reference fields...
INFO[0003] Detected 52 total reference fields across all types
INFO[0003] Registry initialization complete:
INFO[0003]   • Resource Types: 26
INFO[0003]   • Reference Fields: 52
INFO[0003]   • API Groups: 3
INFO[0003]   • Discovery Time: 2.73s
INFO[0003]   • Cache Status: Enabled
INFO[0003] Function ready to process requests
```
