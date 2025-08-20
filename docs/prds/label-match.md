# Product Requirements Document (PRD)
## Kubecore Schema Registry Function - Phase 2: Label-Based Discovery

**Document Version**: 2.0.0  
**Date**: January 2025  
**Author**: Platform Architecture Team  
**Target Audience**: Implementation Architect/Developer  
**Prerequisite**: Phase 1 Complete (Direct Reference Discovery)

---

## 1. Executive Summary

Phase 2 extends the Kubecore Schema Registry Function with dynamic resource discovery capabilities through label selectors and expression-based queries. Building on Phase 1's direct reference foundation, this phase transforms the function into a powerful discovery engine capable of finding resources based on their metadata labels and complex matching expressions.

**Phase 2 Scope**: Implement label-based and expression-based resource discovery with batch fetching optimizations, while maintaining full backward compatibility with Phase 1 functionality.

---

## 2. Problem Statement

### Current Limitations (After Phase 1)
While Phase 1 enables fetching resources by direct reference, compositions still cannot:
- Discover resources dynamically based on labels or metadata
- Query for multiple resources matching specific criteria  
- Find resources across namespaces with common characteristics
- Filter resources based on complex conditions
- Efficiently fetch multiple related resources

### Business Impact
- Cannot implement dynamic environment-based resource selection
- Unable to discover all resources belonging to a project or organization
- No support for multi-tenancy patterns using labels
- Inefficient resource fetching when multiple resources are needed
- Limited flexibility in composition design

---

## 3. Solution Overview

### Phase 2 Enhancements
Extend the Schema Registry Function with:
1. **Label Selector Discovery** - Find resources by exact label matches
2. **Expression-Based Queries** - Complex queries using Kubernetes selector syntax
3. **Batch Fetching** - Optimize API calls for multiple resources
4. **Cross-Namespace Discovery** - Controlled discovery across namespaces
5. **Result Set Management** - Pagination, sorting, and filtering

### Backward Compatibility
- All Phase 1 functionality remains unchanged
- Existing direct reference queries continue to work
- Response format extended but not modified
- Error handling additive, not breaking

---

## 4. Functional Requirements

### 4.1 Label Selector Discovery

**FR-1.1**: The function SHALL support resource discovery using Kubernetes label selectors.

**FR-1.2**: The function SHALL match resources having ALL specified labels (AND logic).

**FR-1.3**: The function SHALL respect minMatches and maxMatches constraints for result sets.

**FR-1.4**: The function SHALL scope label searches to specified namespaces when provided.

**FR-1.5**: The function SHALL support discovering resources across all accessible namespaces when no namespace is specified.

### 4.2 Expression-Based Selectors

**FR-2.1**: The function SHALL support Kubernetes label selector expressions with operators: In, NotIn, Exists, DoesNotExist.

**FR-2.2**: The function SHALL evaluate multiple expressions using AND logic.

**FR-2.3**: The function SHALL support combining matchLabels and matchExpressions in a single query.

**FR-2.4**: The function SHALL validate expression syntax and provide clear error messages for invalid expressions.

**FR-2.5**: The function SHALL support set-based operators with multiple values.

### 4.3 Batch Processing

**FR-3.1**: The function SHALL optimize multiple resource fetches by batching API calls where possible.

**FR-3.2**: The function SHALL group resources by namespace and type for efficient fetching.

**FR-3.3**: The function SHALL handle partial failures in batch operations without failing the entire request.

**FR-3.4**: The function SHALL provide detailed reporting on batch operation results.

### 4.4 Result Set Management

**FR-4.1**: The function SHALL enforce minMatches validation and fail if insufficient resources are found.

**FR-4.2**: The function SHALL limit results to maxMatches when more resources are available.

**FR-4.3**: The function SHALL support result ordering by metadata fields.

**FR-4.4**: The function SHALL provide match statistics in the response metadata.

### 4.5 Cross-Namespace Discovery

**FR-5.1**: The function SHALL support explicit namespace lists for scoped discovery.

**FR-5.2**: The function SHALL support namespace label selectors for dynamic namespace selection.

**FR-5.3**: The function SHALL respect RBAC permissions when discovering across namespaces.

**FR-5.4**: The function SHALL report namespace-specific errors when access is denied.

---

## 5. Technical Requirements

### 5.1 Performance Requirements

**TR-1.1**: Label selector queries MUST complete within 1 second for up to 100 resources.

**TR-1.2**: Expression evaluation MUST process at minimum 100 resources per second.

**TR-1.3**: Batch fetching MUST reduce API calls by at least 40% compared to individual fetches.

**TR-1.4**: Memory usage MUST scale linearly with result set size, not candidate set size.

**TR-1.5**: The function MUST implement query timeouts (default 10 seconds) to prevent runaway queries.

### 5.2 Compatibility Requirements

**TR-2.1**: The function MUST maintain full backward compatibility with Phase 1 inputs and outputs.

**TR-2.2**: The function MUST support standard Kubernetes label selector syntax.

**TR-2.3**: The function MUST work with Kubernetes 1.26+ label selector features.

**TR-2.4**: The function MUST handle both namespaced and cluster-scoped resources in discovery.

### 5.3 Optimization Requirements

**TR-3.1**: The function MUST use Kubernetes field selectors where applicable to reduce data transfer.

**TR-3.2**: The function MUST implement client-side caching for repeated queries within the same execution.

**TR-3.3**: The function MUST parallelize independent discovery operations with a configurable concurrency limit.

**TR-3.4**: The function MUST detect and use indexed labels when available for performance.

---

## 6. Input Schema Extensions

### 6.1 Label-Based Discovery Input

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  fetchResources:
    - apiVersion: platform.kubecore.io/v1alpha1
      kind: KubeCluster
      into: clustersByLabel
      matchType:
        type: label              # New in Phase 2
        selector:
          minMatches: 1          # Minimum resources required
          maxMatches: 10         # Maximum resources to return
          matchLabels:           # Exact label matches
            kubecore.io/cloud-provider: aws
            kubecore.io/organization: novelcore
          namespaces:            # Optional: explicit namespace list
            - test
            - production
```

### 6.2 Expression-Based Discovery Input

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  fetchResources:
    - apiVersion: github.platform.kubecore.io/v1alpha1
      kind: GitHubProject
      into: projectsByExpression
      matchType:
        type: expression         # New in Phase 2
        selector:
          minMatches: 0          # Can be optional
          maxMatches: 50
          matchExpressions:      # Kubernetes selector expressions
            - key: kubecore.io/environment
              operator: In       # In, NotIn, Exists, DoesNotExist
              values: 
                - development
                - staging
            - key: kubecore.io/region
              operator: Exists
            - key: deprecated
              operator: DoesNotExist
          matchLabels:           # Can combine with expressions
            kubecore.io/active: "true"
```

### 6.3 Advanced Selector Options

```yaml
selector:
  # Namespace selection
  namespaceSelector:             # Alternative to explicit namespace list
    matchLabels:
      kubecore.io/tier: production
  
  # Result management
  sortBy:                        # New in Phase 2
    - field: metadata.creationTimestamp
      order: desc                # asc or desc
    - field: metadata.name
      order: asc
  
  # Query hints
  hints:                         # New in Phase 2
    useIndex: true               # Hint to use indexed labels
    expectedSize: small          # small, medium, large
    timeout: 5s                  # Query-specific timeout
```

---

## 7. Output Schema Extensions

### 7.1 Enhanced Metadata for Discovered Resources

```yaml
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Response
spec:
  context:
    kubecore-schema-registry.fn.kubecore.platform.io/fetched-resources:
      clustersByLabel:           # Array even for single match
        - metadata: {...}
          spec: {...}
          status: {...}
          _kubecore:
            matchedBy: "label"   # "label" or "expression"
            fetchedAt: "2025-01-15T10:30:00Z"
            matchDetails:        # New in Phase 2
              labelsMatched:
                - "kubecore.io/cloud-provider=aws"
                - "kubecore.io/organization=novelcore"
              totalCandidates: 45
              totalMatched: 10
              queryTime: "234ms"
              namespace: "test"
```

### 7.2 Enhanced Fetch Summary

```yaml
fetchSummary:
  totalResourcesFetched: 25
  totalApiCalls: 3              # New: batch efficiency metric
  queryExecutionTime: "1.2s"    # New: total query time
  fetchDetails:
    - into: clustersByLabel
      requested: "1-10"         # Shows min-max
      fetched: 5
      matchType: label
      status: success
      candidates: 12             # New: resources evaluated
      namespaces: ["test"]       # New: namespaces searched
      queryOptimization: "indexed" # New: optimization used
    - into: projectsByExpression
      requested: "0-50"
      fetched: 23
      matchType: expression
      status: success
      candidates: 45
      namespaces: ["test", "production"]
      expressionsEvaluated: 3    # New: complexity metric
```

---

## 8. Error Specifications

### 8.1 New Error Types for Phase 2

| Error Code | Description | Example | User Action |
|------------|-------------|---------|-------------|
| INSUFFICIENT_MATCHES | Found fewer than minMatches | Found 2, need 5 | Broaden search criteria |
| TOO_MANY_MATCHES | Found more than maxMatches | Found 150, limit 10 | Add more specific labels |
| INVALID_LABEL_SELECTOR | Malformed label selector | Invalid operator "Contains" | Fix selector syntax |
| INVALID_EXPRESSION | Malformed expression | Missing required field "key" | Fix expression structure |
| NAMESPACE_ACCESS_DENIED | No permission to namespace | Cannot list in "production" | Check RBAC permissions |
| QUERY_TIMEOUT | Query exceeded timeout | Timeout after 10s | Optimize query or increase timeout |
| PARTIAL_BATCH_FAILURE | Some batch operations failed | 3 of 5 namespaces failed | Check specific failures |

### 8.2 Enhanced Error Response

```yaml
error:
  code: "INSUFFICIENT_MATCHES"
  message: "Query returned 2 resources but minMatches requires 5"
  details:
    matchType: "label"
    selector:
      matchLabels:
        kubecore.io/environment: "production"
    searchScope:
      namespaces: ["test", "production"]
      resourceType: "KubeCluster"
    results:
      found: 2
      required: 5
      candidates: 8
    suggestions:
      - "Remove the 'kubecore.io/environment' label to broaden search"
      - "Add more namespaces to the search scope"
      - "Reduce minMatches requirement"
```

---

## 9. Implementation Architecture

### 9.1 New Components for Phase 2

```
pkg/
├── discovery/
│   ├── label_selector.go       # New: Label matching logic
│   ├── expression_evaluator.go # New: Expression evaluation
│   ├── batch_fetcher.go        # New: Batch optimization
│   ├── query_planner.go        # New: Query optimization
│   └── result_aggregator.go    # New: Result set management
├── selector/
│   ├── parser.go               # New: Selector parsing
│   ├── validator.go            # New: Selector validation
│   └── compiler.go             # New: Compile to K8s selectors
├── optimizer/
│   ├── indexer.go              # New: Index detection
│   ├── batcher.go              # New: Batch grouping
│   └── cache.go                # New: Query cache
```

### 9.2 Key Interfaces (Phase 2)

```go
// Selector interface for different match types
type Selector interface {
    Matches(obj client.Object) bool
    ToKubernetesSelector() labels.Selector
    Validate() error
}

// Query planner for optimization
type QueryPlanner interface {
    Plan(ctx context.Context, requests []FetchRequest) QueryPlan
    EstimateCost(plan QueryPlan) QueryCost
    Optimize(plan QueryPlan) QueryPlan
}

// Batch fetcher for efficient API usage
type BatchFetcher interface {
    FetchBatch(ctx context.Context, requests []FetchRequest) BatchResult
    CanBatch(req1, req2 FetchRequest) bool
}

// Result aggregator for managing result sets
type ResultAggregator interface {
    Aggregate(results []FetchResult) AggregatedResult
    Apply(result AggregatedResult, constraints Constraints) FinalResult
    Sort(resources []client.Object, sortBy []SortField) []client.Object
}
```

---

## 10. Testing Requirements

### 10.1 Unit Test Coverage

**Required Coverage**: Minimum 85% for new Phase 2 code

**Test Categories**:
- Label selector parsing and matching
- Expression evaluation with all operators
- Batch fetching logic
- Result set constraints (min/max)
- Sort operations
- Query planning decisions
- Error handling for discovery failures

### 10.2 Integration Test Scenarios

| Scenario | Description | Validation |
|----------|-------------|------------|
| Multi-namespace discovery | Search across 3 namespaces | Correct aggregation |
| Large result set | Query returning 100+ resources | Performance <1s |
| Complex expressions | 5+ expression conditions | Correct evaluation |
| Mixed match types | Label + expression combination | Proper AND logic |
| RBAC restrictions | Limited namespace access | Graceful degradation |
| Batch optimization | 10 different queries | >40% API call reduction |
| Sort stability | Multi-field sorting | Consistent ordering |

### 10.3 Performance Benchmarks

```yaml
benchmarks:
  - name: "Label selector - 10 resources"
    target: < 200ms
    
  - name: "Label selector - 100 resources"
    target: < 1s
    
  - name: "Expression evaluation - 1000 candidates"
    target: < 500ms
    
  - name: "Batch fetch - 5 namespaces"
    target: < 500ms
    
  - name: "Complex query - labels + 5 expressions"
    target: < 1.5s
```

### 10.4 Backward Compatibility Tests

- All Phase 1 tests must continue passing
- Direct reference queries unchanged
- Response format compatibility
- Error code compatibility

---

## 11. Migration & Compatibility

### 11.1 Upgrade Path

- Phase 2 function can directly replace Phase 1
- No changes required to existing compositions
- New features are opt-in through matchType

### 11.2 Feature Detection

```yaml
# Compositions can detect Phase 2 features
apiVersion: kubecore-schema-registry.fn.kubecore.platform.io/v1beta1
kind: Input
spec:
  version: "v1beta1"
  capabilities:              # Response includes capabilities
    - direct                # Phase 1
    - label                 # Phase 2
    - expression            # Phase 2
```

---

## 12. Success Criteria

### 12.1 Functional Success

- [ ] Label selector discovery works with single and multiple labels
- [ ] Expression evaluation supports all specified operators
- [ ] Batch fetching reduces API calls by >40%
- [ ] Cross-namespace discovery respects RBAC
- [ ] Result constraints (min/max) enforced correctly
- [ ] Sorting produces consistent, correct ordering
- [ ] All Phase 1 functionality remains intact

### 12.2 Performance Success

- [ ] Label queries complete in <1s for 100 resources
- [ ] Expression evaluation processes >100 resources/second
- [ ] Memory usage scales linearly with result size
- [ ] No memory leaks in 4-hour stress test
- [ ] Query timeout prevents runaway operations

### 12.3 Quality Metrics

- [ ] 85% unit test coverage for Phase 2 code
- [ ] All integration tests passing
- [ ] Performance benchmarks met
- [ ] Zero regression in Phase 1 functionality
- [ ] Documentation complete with examples

### 12.4 Deliverables

- [ ] Updated function with Phase 2 features
- [ ] Complete test suite (unit + integration)
- [ ] Performance benchmark results
- [ ] Migration guide from Phase 1
- [ ] API documentation for new features
- [ ] Example compositions using new features

---

## 13. Risk Analysis

### 13.1 Technical Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Query performance degradation | High | Implement query cost estimation and limits |
| Memory exhaustion with large sets | High | Stream processing, pagination |
| RBAC complexity | Medium | Clear error messages, partial results |
| Label indexing unavailable | Medium | Fallback to non-indexed queries |
| Batch operation complexity | Medium | Extensive testing, gradual rollout |

### 13.2 Operational Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Increased API server load | High | Rate limiting, query optimization |
| Debugging complexity | Medium | Enhanced logging, query explain |
| Unexpected resource matches | Medium | Dry-run mode, match preview |

---

## 14. Timeline

**Total Duration**: 2 weeks

| Task | Duration | Dependencies | Deliverable |
|------|----------|--------------|-------------|
| Selector parsing/validation | 2 days | None | Label/expression parsers |
| Label matching implementation | 2 days | Parsing | Label discovery |
| Expression evaluator | 2 days | Parsing | Expression discovery |
| Batch fetcher | 2 days | None | Batching logic |
| Query planner | 1 day | All selectors | Optimization |
| Result aggregator | 1 day | Fetchers | Result management |
| Integration & testing | 2 days | All components | Test suite |
| Performance optimization | 1 day | Testing | Benchmarks |
| Documentation | 1 day | Implementation | Docs & examples |

---

## 15. Example Usage Scenarios

### 15.1 Find All Production Clusters

```yaml
fetchResources:
  - apiVersion: platform.kubecore.io/v1alpha1
    kind: KubeCluster
    into: productionClusters
    matchType:
      type: label
      selector:
        matchLabels:
          kubecore.io/environment: production
          kubecore.io/active: "true"
        sortBy:
          - field: metadata.creationTimestamp
            order: desc
```

### 15.2 Discover Resources for Migration

```yaml
fetchResources:
  - apiVersion: platform.kubecore.io/v1alpha1
    kind: GitHubApp
    into: appsToMigrate
    matchType:
      type: expression
      selector:
        matchExpressions:
          - key: kubecore.io/migration-wave
            operator: In
            values: ["wave-1", "wave-2"]
          - key: kubecore.io/legacy
            operator: DoesNotExist
        maxMatches: 20
```

### 15.3 Multi-Environment Resource Discovery

```yaml
fetchResources:
  - apiVersion: app.kubecore.io/v1alpha1
    kind: App
    into: allEnvironmentApps
    matchType:
      type: expression
      selector:
        namespaceSelector:
          matchLabels:
            kubecore.io/tier: user-facing
        matchLabels:
          app.kubernetes.io/name: art-api
        matchExpressions:
          - key: kubecore.io/environment
            operator: In
            values: [dev, staging, prod]
```

---

## 16. Appendix A: Operator Specifications

### Supported Selector Operators

| Operator | Description | Example | SQL Equivalent |
|----------|-------------|---------|----------------|
| In | Value in set | environment In [dev, prod] | IN |
| NotIn | Value not in set | tier NotIn [legacy] | NOT IN |
| Exists | Key exists | region Exists | IS NOT NULL |
| DoesNotExist | Key doesn't exist | deprecated DoesNotExist | IS NULL |
| Equals | Exact match (labels) | env=prod | = |
| NotEquals | Not equal (labels) | env!=prod | != |

---

## 17. Appendix B: Query Optimization Rules

### Optimization Strategies

1. **Index Detection**: Detect commonly indexed labels (kubernetes.io/*, app.kubernetes.io/*)
2. **Namespace Grouping**: Group queries by namespace for batch operations
3. **Selector Compilation**: Pre-compile selectors for repeated use
4. **Result Streaming**: Stream large result sets instead of loading all
5. **Parallel Fetching**: Parallelize independent namespace queries
6. **Cache Reuse**: Reuse results within same function execution

---

**END OF PRD - Phase 2**

This PRD provides comprehensive requirements for implementing label-based discovery while maintaining the solid foundation from Phase 1. The implementation should focus on performance optimization and maintaining backward compatibility.