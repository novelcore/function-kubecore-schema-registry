# KubeCore Schema Registry Function - Phase 2 Implementation

## Overview

This document describes the Phase 2 implementation of the KubeCore Schema Registry Function, which extends the existing Phase 1 functionality with advanced label-based and expression-based resource discovery capabilities while maintaining full backward compatibility.

## Architecture

### Enhanced Input Schema

The input schema in `input/v1beta1/input.go` has been extended with new Phase 2 fields:

- `phase2Features` - Boolean flag to enable Phase 2 capabilities
- `matchType` - Enum specifying how resources are matched (direct, label, expression)
- `selector` - Complex selector configuration for Phase 2 discovery
- `strategy` - Match strategy with constraints and sorting options

### Resolver Pattern Implementation

Phase 2 uses a resolver pattern to handle different resource matching strategies:

```
pkg/discovery/resolver/
├── types.go           # Resolver interface and common types
├── direct.go          # Phase 1 direct name/namespace resolution
├── label.go           # Phase 2 label selector resolution
└── expression.go      # Phase 2 expression-based resolution
```

#### DirectResolver (Phase 1)
- Handles direct resource fetching by name and namespace
- Maintains full backward compatibility
- Used when `matchType: "direct"` or no matchType specified

#### LabelResolver (Phase 2)
- Implements Kubernetes label selector matching
- Supports `matchLabels` and `matchExpressions`
- Enables cross-namespace discovery with RBAC respect
- Used when `matchType: "label"`

#### ExpressionResolver (Phase 2)
- Provides flexible expression-based resource filtering
- Supports operators: Equals, NotEquals, In, NotIn, Contains, StartsWith, EndsWith, Regex, Exists, NotExists
- Evaluates expressions against any resource field using dot notation
- Used when `matchType: "expression"`

### Enhanced Discovery Engine

The `pkg/discovery/engine.go` implements an enhanced discovery engine that:

- Automatically selects the appropriate resolver based on match type
- Provides query optimization and batching capabilities
- Includes performance monitoring and metrics collection
- Supports constraint evaluation (min/max matches)
- Maintains compatibility with legacy Phase 1 engine

### Extended Result Types

Phase 2 introduces comprehensive result metadata:

- **FetchResult**: Extended with `MultiResources` for multiple resource results and `Phase2Results` for advanced metadata
- **Phase2Metadata**: Contains match details, search namespaces, and sort positions
- **MatchDetails**: Provides information about how resources were matched
- **PerformanceMetrics**: Includes query planning time, API time, and resource scan counts
- **ConstraintResults**: Shows constraint evaluation results

## Key Features

### 1. Label-Based Discovery

```yaml
apiVersion: registry.fn.crossplane.io/v1beta1
kind: Input
phase2Features: true
fetchResources:
  - into: "readyPods"
    apiVersion: "v1"
    kind: "Pod"
    matchType: "label"
    selector:
      labels:
        matchLabels:
          app: "demo"
          ready: "true"
        matchExpressions:
          - key: "tier"
            operator: "In"
            values: ["frontend", "backend"]
      crossNamespace: true
    strategy:
      maxMatches: 10
      sortBy:
        - field: "metadata.creationTimestamp"
          order: "desc"
```

### 2. Expression-Based Discovery

```yaml
fetchResources:
  - into: "loadBalancerServices"
    apiVersion: "v1"
    kind: "Service"
    matchType: "expression"
    selector:
      expressions:
        - field: "spec.type"
          operator: "Equals"
          value: "LoadBalancer"
        - field: "status.loadBalancer.ingress"
          operator: "Exists"
        - field: "metadata.name"
          operator: "StartsWith"
          value: "web-"
```

### 3. Advanced Matching Strategies

- **Constraints**: min/max matches with optional constraint violation handling
- **Sorting**: Multi-field sorting with ascending/descending order
- **Early termination**: Stop on first match for performance
- **Cross-namespace**: Search across multiple namespaces with RBAC respect

### 4. Enhanced Response Format

Phase 2 responses include:

```json
{
  "resources": {
    "resourceName": { /* single resource */ }
  },
  "multiResources": {
    "resourceName": [ /* array of resources */ ]
  },
  "fetchSummary": { /* existing summary */ },
  "phase2Results": {
    "queryPlan": {
      "totalQueries": 3,
      "optimizedQueries": 2,
      "executionSteps": ["..."]
    },
    "performance": {
      "queryPlanningTime": 5,
      "kubernetesAPITime": 150,
      "totalResourcesScanned": 45
    },
    "constraintResults": {
      "resourceName": {
        "satisfied": true,
        "message": "All constraints satisfied"
      }
    }
  }
}
```

## Backward Compatibility

Phase 2 maintains 100% backward compatibility with Phase 1:

1. **Default Behavior**: Without `phase2Features: true`, the function operates in Phase 1 mode
2. **Direct Matching**: Default `matchType` is "direct" for existing requests
3. **Response Format**: Phase 1 response format is preserved and enhanced
4. **Legacy Engine**: Phase 1 requests use the original Kubernetes discovery engine

## Performance Optimizations

### Query Planning
- Batches similar requests for efficiency
- Reorders queries for optimal cache usage
- Combines compatible label selectors

### Resource Scanning
- Client-side filtering for expression-based queries
- Early termination strategies
- Parallel namespace scanning

### Metrics and Monitoring
- Query execution time tracking
- Resource scan count monitoring
- Cache hit rate calculation
- Performance bottleneck identification

## Error Handling

Phase 2 introduces comprehensive error handling:

- **InvalidSelector**: Malformed selector configurations
- **InvalidExpression**: Invalid expression syntax or operators
- **ConstraintViolation**: Min/max match constraint violations
- **UnsupportedMatchType**: Unknown match type specifications
- **QueryOptimization**: Query planning and optimization errors
- **SelectorCompilation**: Label selector compilation failures

## Testing

The implementation includes comprehensive test coverage:

### Unit Tests
- Phase 2 feature flag handling
- Label selector compilation and validation
- Expression evaluation and field access
- Constraint evaluation and strategy application
- Backward compatibility verification

### Integration Tests
- End-to-end Phase 2 discovery workflows
- Mixed Phase 1/Phase 2 request handling
- Cross-namespace discovery with RBAC
- Performance metric collection

## Configuration Examples

See `example/phase2-example.yaml` for comprehensive configuration examples including:
- Mixed Phase 1 and Phase 2 usage
- Complex label selector configurations
- Advanced expression-based filtering
- Strategy and constraint configurations

## Migration Guide

### Enabling Phase 2
1. Add `phase2Features: true` to your Input configuration
2. Existing requests continue to work unchanged
3. New Phase 2 features become available

### Converting to Label-Based Discovery
```yaml
# Phase 1 (multiple direct requests)
fetchResources:
  - into: "pod1"
    name: "app-pod-1"
  - into: "pod2"
    name: "app-pod-2"

# Phase 2 (single label-based request)
fetchResources:
  - into: "appPods"
    matchType: "label"
    selector:
      labels:
        matchLabels:
          app: "myapp"
```

### Using Expression-Based Filtering
```yaml
# Advanced filtering with expressions
fetchResources:
  - into: "filteredResources"
    matchType: "expression"
    selector:
      expressions:
        - field: "metadata.labels.environment"
          operator: "In"
          values: ["prod", "staging"]
        - field: "spec.replicas"
          operator: "NotEquals"
          value: "0"
```

## Implementation Status

- ✅ Enhanced input schema with Phase 2 types
- ✅ Resolver pattern architecture
- ✅ DirectResolver (Phase 1 compatibility)
- ✅ LabelResolver (label-based discovery)
- ✅ ExpressionResolver (expression-based discovery)
- ✅ Enhanced discovery engine
- ✅ Extended result types and metadata
- ✅ Response builder enhancements
- ✅ Comprehensive test coverage
- ✅ Backward compatibility verification
- ✅ Performance optimization framework
- ✅ Error handling and validation

## Next Steps

1. **Query Optimization**: Implement advanced batching and caching
2. **Field Indexing**: Add indexed field access for better performance
3. **Custom Operators**: Extend expression operators based on usage patterns
4. **Metrics Dashboard**: Create monitoring dashboard for performance metrics
5. **Documentation**: Expand user guide with real-world examples

---

The Phase 2 implementation successfully extends the KubeCore Schema Registry Function with powerful discovery capabilities while maintaining complete backward compatibility and following Go best practices for clean, maintainable code.