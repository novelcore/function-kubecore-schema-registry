# Product Requirements Document (PRD)
## Kubecore Schema Registry Function - Phase 1: Foundation

**Document Version**: 1.0.0  
**Date**: January 2025  
**Author**: Platform Architecture Team  
**Target Audience**: Implementation Architect/Developer

---

## 1. Executive Summary

The Kubecore Schema Registry Function is a Crossplane composition function that enables dynamic discovery and retrieval of Kubernetes resources and their relationships. Phase 1 establishes the foundation by implementing direct reference discovery with explicit field mapping.

**Phase 1 Scope**: Build core function infrastructure with direct reference resolution capabilities, enabling compositions to fetch explicitly referenced resources through a well-defined input/output schema.

---

## 2. Problem Statement

### Current Challenge
Crossplane compositions cannot dynamically discover and load related Kubernetes resources at runtime. This limitation forces developers to:
- Hardcode resource references in compositions
- Duplicate configuration across related resources
- Manually maintain relationship mappings
- Cannot access transitive relationships (A→B→C)

### Impact
- Compositions cannot adapt to dynamic resource relationships
- Increased maintenance overhead for complex platforms
- Limited reusability of composition logic
- No runtime discovery of resource dependencies

---

## 3. Solution Overview

### Phase 1 Deliverable
A Crossplane composition function that:
1. Accepts structured requests for resource discovery
2. Fetches resources using direct references (name/namespace)
3. Returns resources in a structured, go-template-friendly format
4. Provides comprehensive error handling and reporting

### Core Capabilities (Phase 1)
- Direct resource fetching by name/namespace
- Structured response format for go-template consumption
- Relationship registry for known resource types
- Error handling with detailed fetch summaries

---

## 4. Functional Requirements

### 4.1 Resource Fetching

**FR-1.1**: The function SHALL accept requests to fetch resources by direct reference (name and optional namespace).

**FR-1.2**: The function SHALL support fetching multiple resources in a single request.

**FR-1.3**: The function SHALL validate that requested resources exist and are accessible.

**FR-1.4**: The function SHALL return the complete resource (metadata, spec, status) for successfully fetched resources.

### 4.2 Relationship Registry

**FR-2.1**: The function SHALL maintain an internal registry of known resource relationships.

**FR-2.2**: The registry SHALL define which fields contain references for each resource type.

**FR-2.3**: The registry SHALL specify the relationship type (requires, uses, belongsTo, etc.).

**FR-2.4**: The registry SHALL be extensible without code changes (configuration-based).

### 4.3 Response Structure

**FR-3.1**: The function SHALL return resources grouped by their `into` field names.

**FR-3.2**: Each resource SHALL include standard Kubernetes fields (metadata, spec, status).

**FR-3.3**: Each resource SHALL include discovery metadata (_kubecore field).

**FR-3.4**: The response SHALL include a fetch summary with success/failure counts.

### 4.4 Error Handling

**FR-4.1**: The function SHALL continue processing remaining resources if one fetch fails.

**FR-4.2**: The function SHALL report detailed errors for each failed fetch.

**FR-4.3**: The function SHALL distinguish between "not found" and "access denied" errors.

**FR-4.4**: The function SHALL validate input parameters and provide clear error messages.

---

## 5. Technical Requirements

### 5.1 Implementation Constraints

**TR-1.1**: The function MUST be implemented using the Crossplane Function SDK for Go.

**TR-1.2**: The function MUST use controller-runtime client for Kubernetes API access.

**TR-1.3**: The function MUST handle both namespaced and cluster-scoped resources.

**TR-1.4**: The function MUST respect Kubernetes RBAC permissions.

### 5.2 Performance Requirements

**TR-2.1**: Direct reference fetches MUST complete within 500ms per resource.

**TR-2.2**: The function MUST handle up to 20 resources per request.

**TR-2.3**: The function MUST timeout individual fetch operations after 5 seconds.

### 5.3 Compatibility Requirements

**TR-3.1**: The function MUST support Kubernetes API versions 1.26+.

**TR-3.2**: The function MUST work with Crossplane 1.14+.

**TR-3.3**: The function MUST handle CRDs with any apiVersion.

---

## 6. Input/Output Schemas

### 6.1 Input Schema

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  # Phase 1: Only direct fetch type is supported
  fetchResources:
    - apiVersion: string        # Required: API version of resource
      kind: string              # Required: Kind of resource
      into: string              # Required: Key for response map
      matchType:                # Required: How to find resource
        type: "direct"          # Phase 1: Only "direct" supported
        selector:
          minMatches: integer   # Optional: Default 1
          maxMatches: integer   # Optional: Default 1
          directReference:
            name: string        # Required: Resource name
            namespace: string   # Optional: For namespaced resources
```

### 6.2 Output Schema

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Response
spec:
  context:
    kubecore-schema-registry.fn.kubecore.platform.io/fetched-resources:
      # Map key is the 'into' field from input
      <into-key>:
        - metadata:
            name: string
            namespace: string
            uid: string
            resourceVersion: string
            labels: map[string]string
            annotations: map[string]string
          spec: object          # Complete spec from resource
          status: object        # Complete status from resource
          _kubecore:
            matchedBy: "direct"
            fetchedAt: string   # ISO 8601 timestamp
            error: string       # Only present if fetch failed
  
  fetchSummary:
    totalResourcesFetched: integer
    fetchDetails:
      - into: string
        requested: integer
        fetched: integer
        matchType: string
        status: "success" | "partial" | "failed"
        errors: []string        # List of error messages
```

---

## 7. Relationship Registry Specification

### 7.1 Registry Structure

```yaml
# Internal registry (embedded in function)
relationshipRegistry:
  version: "v1"
  resources:
    platform.kubecore.io/v1alpha1:
      KubeSystem:
        references:
          - field: "spec.kubeClusterRef"
            targetGVK:
              group: "platform.kubecore.io"
              version: "v1alpha1"
              kind: "KubeCluster"
            relationship: "requires"
            required: true
            
      KubeCluster:
        references:
          - field: "spec.kubeNetRef"
            targetGVK:
              group: "platform.kubecore.io"
              version: "v1alpha1"
              kind: "KubeNet"
            relationship: "uses"
            required: false
          - field: "spec.githubProjectRef"
            targetGVK:
              group: "github.platform.kubecore.io"
              version: "v1alpha1"
              kind: "GitHubProject"
            relationship: "belongsTo"
            required: true
            
      App:
        references:
          - field: "spec.githubAppRef"
            targetGVK:
              group: "github.platform.kubecore.io"
              version: "v1alpha1"
              kind: "GitHubApp"
            relationship: "implements"
            required: true
```

---

## 8. Error Handling Specifications

### 8.1 Error Categories

| Error Type | HTTP Equivalent | Description | User Action |
|------------|----------------|-------------|-------------|
| NotFound | 404 | Resource doesn't exist | Check name/namespace |
| Forbidden | 403 | RBAC denies access | Check permissions |
| Invalid | 400 | Invalid input parameters | Fix input |
| Timeout | 408 | Fetch operation timeout | Retry |
| ServerError | 500 | Kubernetes API error | Check cluster health |

### 8.2 Error Response Format

```yaml
error:
  code: "RESOURCE_NOT_FOUND"
  message: "Resource KubeCluster/demo-cluster not found in namespace test"
  details:
    apiVersion: "platform.kubecore.io/v1alpha1"
    kind: "KubeCluster"
    name: "demo-cluster"
    namespace: "test"
  timestamp: "2025-01-15T10:30:00Z"
```

---

## 9. Implementation Guidelines

### 9.1 Code Structure

```
function-kubecore-schema-registry/
├── main.go                 # Function entrypoint
├── pkg/
│   ├── function/
│   │   ├── function.go     # Main function logic
│   │   └── function_test.go
│   ├── fetcher/
│   │   ├── direct.go       # Direct reference fetcher
│   │   └── direct_test.go
│   ├── registry/
│   │   ├── registry.go     # Relationship registry
│   │   ├── loader.go       # Registry loader
│   │   └── types.go        # Registry types
│   ├── types/
│   │   ├── input.go        # Input schema types
│   │   └── output.go       # Output schema types
│   └── utils/
│       ├── errors.go       # Error handling utilities
│       └── k8s.go          # Kubernetes client utilities
├── test/
│   ├── fixtures/           # Test resource fixtures
│   └── integration/        # Integration tests
└── registry.yaml           # Default relationship registry
```

### 9.2 Key Interfaces

```go
// Core fetcher interface
type ResourceFetcher interface {
    Fetch(ctx context.Context, ref ResourceReference) (*Resource, error)
}

// Registry interface
type RelationshipRegistry interface {
    GetReferences(gvk schema.GroupVersionKind) []ReferenceDefinition
    IsTraversable(from, to schema.GroupVersionKind) bool
}

// Function interface
type SchemaRegistryFunction interface {
    Run(ctx context.Context, req *fnv1beta1.RunFunctionRequest) (*fnv1beta1.RunFunctionResponse, error)
}
```

---

## 10. Testing Requirements

### 10.1 Unit Tests

- **Coverage**: Minimum 80% code coverage
- **Test Cases**:
  - Successful direct fetch
  - Resource not found
  - Invalid input parameters
  - Namespace vs cluster-scoped resources
  - Multiple resource fetches
  - Registry loading and lookup

### 10.2 Integration Tests

- Test against real Kubernetes cluster (kind/k3d)
- Test with actual CRDs from provided examples
- Test RBAC permission scenarios
- Test timeout handling

### 10.3 Test Data

Use the provided example resources (00-githubprovider.yaml through 06-kubeapp.yaml) as test fixtures.

---

## 11. Success Criteria

### 11.1 Functional Success

- [ ] Function successfully fetches resources by direct reference
- [ ] Function returns resources in specified output format
- [ ] Function handles errors gracefully without crashing
- [ ] Function respects minMatches/maxMatches parameters
- [ ] Function works with both namespaced and cluster-scoped resources

### 11.2 Quality Metrics

- [ ] 80% unit test coverage achieved
- [ ] All integration tests passing
- [ ] Response time <500ms for single resource fetch
- [ ] Zero memory leaks in 1-hour stress test
- [ ] Clear error messages for all failure scenarios

### 11.3 Deliverables

- [ ] Working function container image
- [ ] Complete unit and integration tests
- [ ] API documentation (OpenAPI spec)
- [ ] Usage examples with provided resources
- [ ] README with setup and usage instructions

---

## 12. Out of Scope (Phase 1)

The following features are explicitly OUT OF SCOPE for Phase 1:

- Label-based resource discovery
- Expression-based matching
- Transitive relationship discovery (depth > 0)
- Parent resource fetching
- Caching mechanisms
- Watch-based updates
- Batch API optimizations
- Cross-namespace discovery
- Relationship inference
- Custom relationship types

---

## 13. Dependencies

### 13.1 External Dependencies

- Crossplane Runtime v1.14+
- Crossplane Function SDK Go v0.2.0+
- controller-runtime v0.16+
- Kubernetes API machinery v0.28+

### 13.2 Runtime Requirements

- Kubernetes cluster 1.26+
- Crossplane 1.14+ installed
- Function RBAC configured
- Access to target namespaces

---

## 14. Timeline

**Total Duration**: 2-3 weeks

| Milestone | Duration | Deliverable |
|-----------|----------|-------------|
| Setup & Scaffolding | 2 days | Basic function structure |
| Registry Implementation | 2 days | Relationship registry |
| Direct Fetcher | 3 days | Direct reference fetching |
| Response Builder | 2 days | Output formatting |
| Error Handling | 2 days | Comprehensive error handling |
| Testing | 3 days | Unit and integration tests |
| Documentation | 1 day | API docs and examples |
| Buffer | 2 days | Bug fixes and polish |

---

## 15. Appendix A: Example Usage

### Input Example

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  fetchResources:
    - apiVersion: platform.kubecore.io/v1alpha1
      kind: KubeCluster
      into: cluster
      matchType:
        type: direct
        selector:
          directReference:
            name: demo-cluster
            namespace: test
            
    - apiVersion: github.platform.kubecore.io/v1alpha1
      kind: GitHubProject
      into: project
      matchType:
        type: direct
        selector:
          directReference:
            name: demo-project
            namespace: test
```

### Expected Output

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Response
spec:
  context:
    kubecore-schema-registry.fn.kubecore.platform.io/fetched-resources:
      cluster:
        - metadata:
            name: demo-cluster
            namespace: test
            uid: "abc-123-def"
          spec:
            kubeNetRef:
              name: demo-network
            githubProjectRef:
              name: demo-project
            cloudProvider: aws
          status:
            conditions:
              - type: Ready
                status: "True"
          _kubecore:
            matchedBy: direct
            fetchedAt: "2025-01-15T10:30:00Z"
            
      project:
        - metadata:
            name: demo-project
            namespace: test
          spec:
            githubProviderRef:
              name: gh-default
          _kubecore:
            matchedBy: direct
            fetchedAt: "2025-01-15T10:30:00Z"
            
  fetchSummary:
    totalResourcesFetched: 2
    fetchDetails:
      - into: cluster
        requested: 1
        fetched: 1
        matchType: direct
        status: success
      - into: project
        requested: 1
        fetched: 1
        matchType: direct
        status: success
```

---

## 16. Appendix B: Registry Configuration Format

The relationship registry will be embedded in the function but should be designed to be externalizable in future phases:

```yaml
# registry.yaml - Embedded in function for Phase 1
version: v1
resources:
  # Platform resources
  - gvk:
      group: platform.kubecore.io
      version: v1alpha1
      kind: KubeSystem
    references:
      - field: spec.kubeClusterRef
        target:
          group: platform.kubecore.io
          version: v1alpha1
          kind: KubeCluster
        type: requires
        
  - gvk:
      group: platform.kubecore.io
      version: v1alpha1
      kind: KubeCluster
    references:
      - field: spec.kubeNetRef
        target:
          group: platform.kubecore.io
          version: v1alpha1
          kind: KubeNet
        type: uses
      - field: spec.githubProjectRef
        target:
          group: github.platform.kubecore.io
          version: v1alpha1
          kind: GitHubProject
        type: belongsTo
```

---

**END OF PRD - Phase 1**

This PRD provides comprehensive requirements for Phase 1 implementation. The architect should focus on building a solid foundation that can be extended in future phases without major refactoring.