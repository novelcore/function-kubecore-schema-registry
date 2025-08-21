# Product Requirements Document (PRD)
## Kubecore Schema Registry Function - Phase 3: DAG Construction & Transitive Discovery

**Document Version**: 3.0.0  
**Date**: January 2025  
**Author**: Platform Architecture Team  
**Target Audience**: Implementation Architect/Developer  
**Prerequisites**: 
- Phase 1 Complete (Direct Reference Discovery)
- Phase 2 Enhancement Complete (Dynamic CRD Discovery)
- Phase 2 Label Search (Optional - not required for Phase 3)

---

## 1. Executive Summary

Phase 3 transforms the Kubecore Schema Registry Function into an intelligent relationship explorer capable of discovering and traversing resource dependencies automatically. The function will construct directed acyclic graphs (DAGs) of platform resources, following references to specified depths while maintaining strict boundaries around platform-only resources (*.kubecore.io APIs).

**Phase 3 Scope**: Implement transitive relationship discovery through automatic reference following, with depth control, cycle detection, and platform-scoped traversal that excludes infrastructure resources like Secrets and ConfigMaps.

---

## 2. Problem Statement

### Current Limitations (After Phase 1 & 2)
While the function can fetch resources directly and discover them by labels, it cannot:
- Automatically discover related resources through reference chains
- Understand resource dependencies and relationships
- Provide complete resource topology for a given starting point
- Perform impact analysis (what depends on this resource)
- Prevent infinite loops in circular references
- Distinguish between platform and infrastructure resources

### Business Impact
- Compositions cannot automatically include dependent resources
- No visibility into resource relationship chains
- Manual tracking of dependencies across resources
- Cannot assess impact of changes to resources
- Risk of incomplete resource sets in templates
- Potential exposure of infrastructure details

---

## 3. Solution Overview

### Phase 3 Core Features
1. **Transitive Reference Discovery** - Automatically follow references to discover related resources
2. **Depth-Controlled Traversal** - Configurable levels of relationship following
3. **Platform-Scoped Boundaries** - Only traverse *.kubecore.io resources, exclude infrastructure
4. **Cycle Detection** - Prevent infinite loops in circular references
5. **Bidirectional Discovery** - Find both dependencies and dependents
6. **Path Tracking** - Complete audit trail of discovery paths
7. **Efficient Graph Construction** - Optimized API usage for deep traversals

### Critical Design Constraint
**Platform-Only Traversal**: The function MUST NOT follow references to non-KubeCore resources (Secrets, ConfigMaps, ServiceAccounts, etc.) even if reference fields exist.

---

## 4. Functional Requirements

### 4.1 Transitive Reference Discovery

**FR-1.1**: The function SHALL automatically discover resources referenced by fetched resources.

**FR-1.2**: The function SHALL follow references found in resource specs based on field patterns (*Ref, *Reference).

**FR-1.3**: The function SHALL ONLY follow references to resources in *.kubecore.io API groups.

**FR-1.4**: The function SHALL ignore references to core Kubernetes resources (Secret, ConfigMap, etc.).

**FR-1.5**: The function SHALL use the dynamic CRD registry to identify reference fields.

**FR-1.6**: The function SHALL handle missing referenced resources gracefully without failing traversal.

### 4.2 Depth Control

**FR-2.1**: The function SHALL support configurable traversal depth (0 to unlimited).

**FR-2.2**: The function SHALL treat depth=0 as no traversal (current behavior).

**FR-2.3**: The function SHALL support depth=-1 as unlimited traversal until terminal resources.

**FR-2.4**: The function SHALL track current depth for each resource in the traversal.

**FR-2.5**: The function SHALL stop traversal at specified depth even if more references exist.

### 4.3 Platform Scope Enforcement

**FR-3.1**: The function SHALL maintain a whitelist of allowed API groups for traversal (*.kubecore.io).

**FR-3.2**: The function SHALL NOT traverse references to resources outside allowed API groups.

**FR-3.3**: The function SHALL log when references are skipped due to scope restrictions.

**FR-3.4**: The function SHALL distinguish between platform and infrastructure references.

**FR-3.5**: The function SHALL provide configuration to customize allowed API groups.

### 4.4 Cycle Detection

**FR-4.1**: The function SHALL detect circular references during traversal.

**FR-4.2**: The function SHALL stop traversal when a cycle is detected.

**FR-4.3**: The function SHALL report detected cycles in response metadata.

**FR-4.4**: The function SHALL maintain a visited set per traversal branch.

**FR-4.5**: The function SHALL continue traversing other branches after cycle detection.

### 4.5 Bidirectional Discovery

**FR-5.1**: The function SHALL support forward traversal (following references FROM a resource).

**FR-5.2**: The function SHALL support reverse traversal (finding resources that reference TO a resource).

**FR-5.3**: The function SHALL support bidirectional traversal (both forward and reverse).

**FR-5.4**: The function SHALL efficiently query for reverse references using labels/annotations.

### 4.6 Path Tracking

**FR-6.1**: The function SHALL record the complete discovery path for each resource.

**FR-6.2**: The function SHALL include field names used in traversal.

**FR-6.3**: The function SHALL track relationship types between resources.

**FR-6.4**: The function SHALL provide traversal visualization in metadata.

---

## 5. Technical Requirements

### 5.1 Performance Requirements

**TR-1.1**: Traversal to depth 3 MUST complete within 2 seconds for typical resource graphs.

**TR-1.2**: The function MUST batch API calls for resources at the same depth level.

**TR-1.3**: Memory usage MUST remain under 256MB for graphs up to 100 resources.

**TR-1.4**: The function MUST implement traversal timeout (default 10 seconds).

**TR-1.5**: The function MUST cache resources within same execution to avoid duplicate fetches.

### 5.2 Graph Construction Requirements

**TR-2.1**: The function MUST build an internal DAG representation during traversal.

**TR-2.2**: The function MUST maintain both adjacency list and parent pointers.

**TR-2.3**: The function MUST support multiple edges between same resources.

**TR-2.4**: The function MUST track edge metadata (field name, relationship type).

### 5.3 Reference Resolution Requirements

**TR-3.1**: The function MUST resolve references across namespaces when specified.

**TR-3.2**: The function MUST handle both name-only and name+namespace references.

**TR-3.3**: The function MUST support optional references (continue on missing).

**TR-3.4**: The function MUST validate reference target types against registry.

### 5.4 Optimization Requirements

**TR-4.1**: The function MUST deduplicate resources discovered through multiple paths.

**TR-4.2**: The function MUST implement breadth-first traversal for better batching.

**TR-4.3**: The function MUST reuse parsed schemas from dynamic registry.

**TR-4.4**: The function MUST implement early termination for depth limits.

---

## 6. Input Schema Extensions

### 6.1 Transitive Discovery Input

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  # Platform scope configuration (global)
  traversalConfig:
    allowedAPIGroups:        # Which API groups to traverse
      - "platform.kubecore.io"
      - "github.platform.kubecore.io"
      - "app.kubecore.io"
    excludedKinds:          # Never traverse these
      - Secret
      - ConfigMap
      - ServiceAccount
    
  fetchResources:
    - apiVersion: platform.kubecore.io/v1alpha1
      kind: KubeSystem
      into: systemWithDeps
      matchType:
        type: direct
        selector:
          directReference:
            name: demo-kubesystem
            namespace: test
      
      # New in Phase 3: Response schema for traversal
      responseSchema:
        parentResources:     # Follow references FROM this resource
          include: true
          depth: 3           # How many levels to traverse
          direction: forward # forward, reverse, or bidirectional
          optional: true     # Continue if references missing
          
        traversalOptions:    # Fine-grained control
          includeOptionalRefs: true
          stopOnMissing: false
          deduplicateResults: true
          maxResources: 100  # Safety limit
```

### 6.2 Bidirectional Discovery Input

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  fetchResources:
    - apiVersion: github.platform.kubecore.io/v1alpha1
      kind: GitHubProject
      into: projectImpactAnalysis
      matchType:
        type: direct
        selector:
          directReference:
            name: demo-project
      
      responseSchema:
        parentResources:
          include: true
          depth: 2
          direction: bidirectional  # Find both dependencies and dependents
          
        reverseDiscovery:          # Options for reverse traversal
          searchNamespaces:        # Where to look for references
            - test
            - production
          useIndexes: true         # Use label indexes for performance
```

### 6.3 Unlimited Depth Discovery

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  fetchResources:
    - apiVersion: app.kubecore.io/v1alpha1
      kind: App
      into: completeAppTopology
      matchType:
        type: direct
        selector:
          directReference:
            name: art-api
      
      responseSchema:
        parentResources:
          include: true
          depth: -1               # Unlimited traversal
          timeout: 30s            # Safety timeout
          
        traversalOptions:
          cycleStrategy: stop    # stop, continue, or fail
          maxDepth: 10           # Hard limit even for unlimited
```

---

## 7. Output Schema Extensions

### 7.1 Enhanced Resource Metadata

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Response
spec:
  context:
    kubecore-schema-registry.fn.kubecore.platform.io/fetched-resources:
      systemWithDeps:
        # Root resource (directly requested)
        - metadata:
            name: demo-kubesystem
            namespace: test
          spec: {...}
          status: {...}
          _kubecore:
            matchedBy: "direct"
            depth: 0              # Root level
            fetchedAt: "2025-01-15T10:30:00Z"
            
        # Discovered through traversal
        - metadata:
            name: demo-cluster
            kind: KubeCluster
          spec: {...}
          _kubecore:
            matchedBy: "transitive"
            depth: 1
            discoveryPath:        # How this was discovered
              - source: "KubeSystem/demo-kubesystem"
                field: "spec.kubeClusterRef"
                relationship: "requires"
            parentResource: "KubeSystem/demo-kubesystem"
            
        - metadata:
            name: demo-project
            kind: GitHubProject
          _kubecore:
            matchedBy: "transitive"
            depth: 2
            discoveryPath:
              - source: "KubeSystem/demo-kubesystem"
                field: "spec.kubeClusterRef"
                relationship: "requires"
              - source: "KubeCluster/demo-cluster"
                field: "spec.githubProjectRef"
                relationship: "belongsTo"
            parentResource: "KubeCluster/demo-cluster"
```

### 7.2 Graph Metadata in Summary

```yaml
fetchSummary:
  totalResourcesFetched: 15
  traversalStatistics:           # New in Phase 3
    rootResources: 1
    discoveredResources: 14
    maxDepthReached: 3
    cyclesDetected: 0
    skippedReferences:           # Platform boundary enforcement
      - type: "Secret"
        count: 3
        reason: "Not in allowed API groups"
      - type: "ConfigMap"
        count: 2
        reason: "Not in allowed API groups"
    
  relationshipGraph:              # Graph visualization
    nodes: 15
    edges: 18
    components: 1                 # Connected components
    hasCycles: false
    
  fetchDetails:
    - into: systemWithDeps
      requested: 1
      fetched: 15
      matchType: direct
      traversalDepth: 3
      status: success
      graph:
        root: "KubeSystem/demo-kubesystem"
        leaves: ["GitHubProvider/gh-default", "KubeNet/demo-network"]
        longestPath: 3
```

### 7.3 Cycle Detection Response

```yaml
_kubecore:
  matchedBy: "transitive"
  depth: 3
  cycleDetected: true            # Cycle found
  cyclePath:                     # The circular reference
    - "KubeCluster/demo-cluster"
    - "GitHubProject/demo-project"
    - "GitHubApp/art-api"
    - "KubeCluster/demo-cluster"  # Back to start
  traversalStopped: true
  stopReason: "Circular reference detected"
```

---

## 8. Reference Detection & Filtering

### 8.1 Reference Pattern Configuration

```yaml
# Dynamic reference detection configuration
referenceDetection:
  # Patterns for identifying reference fields
  fieldPatterns:
    - "*Ref"              # kubeClusterRef, githubProjectRef
    - "*Reference"        # clusterReference
    - "*RefName"          # providerRefName
    
  # Nested reference patterns
  nestedPatterns:
    - "spec.*.ref"
    - "spec.*.*Ref"
    
  # Platform scope filter
  targetFilter:
    allowedGroups:
      - "platform.kubecore.io"
      - "github.platform.kubecore.io"
      - "app.kubecore.io"
    
    # Explicitly excluded even if in allowed groups
    excludedKinds:
      - Secret
      - ConfigMap
      - ServiceAccount
      - ClusterSecretStore
```

### 8.2 Reference Resolution Logic

```yaml
# Reference resolution process
resolution:
  steps:
    1. Identify reference field by pattern
    2. Extract target type from field or schema
    3. Check if target in allowed API groups
    4. If not allowed, skip with log entry
    5. If allowed, resolve namespace (inherit or explicit)
    6. Fetch resource or mark as missing
    7. Add to graph with relationship metadata
```

---

## 9. Implementation Architecture

### 9.1 New Components for Phase 3

```
pkg/
├── graph/
│   ├── builder.go              # DAG construction
│   ├── traverser.go            # Graph traversal algorithms
│   ├── cycle_detector.go       # Cycle detection
│   └── path_tracker.go         # Path recording
├── discovery/
│   ├── transitive/
│   │   ├── engine.go           # Main traversal engine
│   │   ├── reference_resolver.go # Reference resolution
│   │   ├── scope_filter.go     # Platform scope enforcement
│   │   └── batch_optimizer.go  # Batch fetch optimization
│   └── ...existing...
├── relationships/
│   ├── analyzer.go             # Relationship analysis
│   ├── reverse_index.go        # Reverse reference index
│   └── cache.go                # Relationship cache
```

### 9.2 Key Interfaces (Phase 3)

```go
// Graph builder interface
type GraphBuilder interface {
    AddNode(resource client.Object) NodeID
    AddEdge(from, to NodeID, metadata EdgeMetadata) error
    Build() (*ResourceGraph, error)
    DetectCycles() ([]Cycle, error)
}

// Traversal engine interface
type TraversalEngine interface {
    Traverse(ctx context.Context, root client.Object, opts TraversalOptions) (*TraversalResult, error)
    SetDepthLimit(depth int)
    SetScopeFilter(filter ScopeFilter)
    Stop() // For cancellation
}

// Reference resolver interface
type ReferenceResolver interface {
    ExtractReferences(resource client.Object) []Reference
    ResolveReference(ctx context.Context, ref Reference) (client.Object, error)
    IsInScope(ref Reference) bool
    FilterByAPIGroup(refs []Reference, groups []string) []Reference
}

// Scope filter interface
type ScopeFilter interface {
    IsAllowed(gvk schema.GroupVersionKind) bool
    IsExcluded(kind string) bool
    LogSkipped(ref Reference, reason string)
    GetStatistics() ScopeStatistics
}

// Path tracker interface
type PathTracker interface {
    StartPath(root client.Object)
    AddStep(from, to client.Object, field string, relationship string)
    GetPath(resource client.Object) []PathStep
    HasVisited(resource client.Object) bool
}
```

---

## 10. Testing Requirements

### 10.1 Unit Test Coverage

**Required Coverage**: Minimum 85% for Phase 3 code

**Test Categories**:
- Reference extraction from various resource types
- Scope filtering (allowed vs excluded resources)
- Cycle detection in various graph configurations
- Depth limit enforcement
- Path tracking accuracy
- Batch optimization logic
- Missing resource handling
- Cross-namespace reference resolution

### 10.2 Integration Test Scenarios

| Scenario | Description | Validation |
|----------|-------------|------------|
| Simple Chain | A→B→C linear references | All 3 resources fetched |
| Complex Graph | Multiple paths to same resource | Deduplication works |
| Cycle Detection | A→B→C→A circular reference | Cycle detected, traversal stops |
| Depth Limits | 5-level chain with depth=3 | Only 4 resources fetched |
| Platform Boundary | Resource with Secret reference | Secret not traversed |
| Missing Reference | Optional ref doesn't exist | Traversal continues |
| Bidirectional | Find all resources referencing X | Reverse index works |
| Large Graph | 100+ resources | <5 second completion |
| Cross-namespace | References across namespaces | Correct resolution |

### 10.3 Platform Scope Tests

| Test Case | Setup | Expected Result |
|-----------|-------|-----------------|
| KubeCore Only | Resource with mixed refs | Only *.kubecore.io followed |
| Secret Reference | GitHubProvider→Secret | Secret ignored |
| ConfigMap Reference | App→ConfigMap | ConfigMap ignored |
| Nested Platform Ref | A→B→C all KubeCore | All traversed |
| Mixed References | Platform + Infrastructure | Only platform in result |

### 10.4 Performance Benchmarks

```yaml
benchmarks:
  - name: "Depth 1 traversal - 5 references"
    target: < 500ms
    
  - name: "Depth 3 traversal - 20 resources"
    target: < 2s
    
  - name: "Depth 5 traversal - 50 resources"
    target: < 5s
    
  - name: "Cycle detection - 10 node cycle"
    target: < 100ms
    
  - name: "Reverse discovery - 100 resources"
    target: < 1s
    
  - name: "Memory usage - 100 resources"
    target: < 256MB
```

---

## 11. Error Handling

### 11.1 New Error Types for Phase 3

| Error Code | Description | User Action |
|------------|-------------|-------------|
| TRAVERSAL_DEPTH_EXCEEDED | Max depth reached | Increase depth or accept partial |
| CYCLE_DETECTED | Circular reference found | Review resource relationships |
| REFERENCE_NOT_ALLOWED | Ref outside platform scope | Expected behavior, no action |
| TRAVERSAL_TIMEOUT | Operation exceeded timeout | Reduce depth or increase timeout |
| MAX_RESOURCES_EXCEEDED | Too many resources | Increase limit or reduce scope |
| INVALID_REFERENCE_FORMAT | Malformed reference | Fix resource specification |
| NAMESPACE_NOT_FOUND | Referenced namespace missing | Create namespace or fix reference |

### 11.2 Error Response Examples

```yaml
error:
  code: "CYCLE_DETECTED"
  message: "Circular reference detected during traversal"
  details:
    startResource: "KubeCluster/demo-cluster"
    cyclePath:
      - "KubeCluster/demo-cluster"
      - "GitHubProject/demo-project"
      - "GitHubApp/art-api"
      - "KubeCluster/demo-cluster"
    traversalDepth: 3
    resourcesBeforeCycle: 8
    
error:
  code: "REFERENCE_NOT_ALLOWED"
  message: "Reference to Secret ignored (outside platform scope)"
  details:
    sourceResource: "GitHubProvider/gh-default"
    field: "spec.secretRef"
    targetKind: "Secret"
    targetName: "github-credentials"
    reason: "Resource type not in allowed API groups"
    allowedGroups: ["*.kubecore.io"]
```

---

## 12. Success Criteria

### 12.1 Functional Success

- [ ] Transitive discovery follows references correctly
- [ ] Depth limits enforced accurately
- [ ] Platform scope boundary maintained (no Secret/ConfigMap traversal)
- [ ] Cycles detected and handled gracefully
- [ ] Bidirectional discovery works
- [ ] Path tracking provides complete audit trail
- [ ] Missing optional references don't break traversal
- [ ] Cross-namespace references resolved correctly

### 12.2 Performance Success

- [ ] Depth 3 traversal completes in <2 seconds
- [ ] Memory usage <256MB for 100 resources
- [ ] Batch optimization reduces API calls by >50%
- [ ] No memory leaks in stress testing
- [ ] Timeout prevents runaway traversals

### 12.3 Quality Metrics

- [ ] 85% unit test coverage for Phase 3 code
- [ ] All integration tests passing
- [ ] Performance benchmarks met
- [ ] Zero regression in Phase 1 & 2 functionality
- [ ] Platform boundaries never violated

### 12.4 Deliverables

- [ ] Updated function with Phase 3 traversal
- [ ] Graph construction and traversal engine
- [ ] Cycle detection implementation
- [ ] Platform scope filter
- [ ] Comprehensive test suite
- [ ] Performance benchmark results
- [ ] Architecture documentation
- [ ] Usage examples with traversal

---

## 13. Usage Examples

### 13.1 Complete Application Topology

```yaml
# Discover everything related to an application
fetchResources:
  - apiVersion: app.kubecore.io/v1alpha1
    kind: App
    into: appTopology
    matchType:
      type: direct
      selector:
        directReference:
          name: art-api
    responseSchema:
      parentResources:
        include: true
        depth: -1  # Unlimited
        direction: bidirectional

# Returns:
# - App/art-api
# - GitHubApp/art-api (via githubAppRef)
# - GitHubProject/demo-project (via githubProjectRef)
# - KubEnv/demo-dev (via kubenvRef)
# - KubeCluster/demo-cluster (via kubeClusterRef)
# - KubeNet/demo-network (via kubeNetRef)
# - QualityGate/smoke-test (via qualityGateRef)
# (Secrets, ConfigMaps excluded even if referenced)
```

### 13.2 Impact Analysis

```yaml
# What depends on this GitHubProject?
fetchResources:
  - apiVersion: github.platform.kubecore.io/v1alpha1
    kind: GitHubProject
    into: impactAnalysis
    matchType:
      type: direct
      selector:
        directReference:
          name: demo-project
    responseSchema:
      parentResources:
        include: true
        depth: 3
        direction: reverse  # Find dependents

# Returns resources that reference demo-project
```

### 13.3 Platform Resource Chain

```yaml
# Discover platform infrastructure chain
fetchResources:
  - apiVersion: platform.kubecore.io/v1alpha1
    kind: KubeSystem
    into: platformChain
    matchType:
      type: direct
      selector:
        directReference:
          name: demo-kubesystem
    responseSchema:
      parentResources:
        include: true
        depth: 5
        
# Follows only platform.kubecore.io references
# Stops at Secret/ConfigMap boundaries
```

---

## 14. Risk Mitigation

### 14.1 Technical Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Infinite loops | High | Cycle detection with visited sets |
| Memory exhaustion | High | Resource limits, streaming |
| API rate limiting | Medium | Batch optimization, caching |
| Graph explosion | High | Depth limits, resource caps |
| Scope violations | High | Strict filter enforcement |

### 14.2 Security Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Secret exposure | Critical | Platform scope enforcement |
| Data leakage | High | No infrastructure traversal |
| RBAC bypass | Medium | Respect namespace boundaries |
| Resource enumeration | Low | Result filtering |

---

## 15. Architecture Notes for Implementer

### Critical Implementation Considerations

1. **Scope Enforcement MUST be fail-safe**: Default to NOT traversing if uncertain
2. **Use allowlist, not denylist**: Only traverse known platform API groups
3. **Reference detection from CRD schemas**: Leverage Phase 2's dynamic discovery
4. **Batch optimization is critical**: Group by namespace and type
5. **Memory management**: Stream large graphs, don't load all at once
6. **Clear boundaries**: Document why each reference was/wasn't followed

### Performance Optimization Strategies

1. **Breadth-first traversal**: Better for batching than depth-first
2. **Resource deduplication**: Use resource UID as unique key
3. **Parallel fetching**: At same depth level
4. **Early termination**: Stop branch when depth reached
5. **Caching**: Within execution context only

---

**END OF PRD - Phase 3**

This PRD provides comprehensive requirements for implementing platform-scoped transitive discovery. The architect should focus on maintaining strict platform boundaries while enabling powerful relationship exploration capabilities.