# KubeCore Schema Registry Function - Architecture

## Overview

This document describes the refactored architecture of the KubeCore Schema Registry function, implementing clean architecture principles with proper separation of concerns and design patterns.

## Architecture Layers

### 1. Presentation Layer
- **Location**: `fn.go`, `fn_refactored.go`, `main.go`
- **Responsibility**: HTTP/gRPC endpoints and request/response handling
- **Components**:
  - `Function`: Wrapper around RefactoredFunction for compatibility
  - `RefactoredFunction`: Main function implementation with dependency injection

### 2. Service Layer
- **Location**: `internal/service/`
- **Responsibility**: Business logic and orchestration
- **Components**:
  - `DiscoveryService`: Orchestrates schema discovery workflow
  - Implements transitive discovery algorithms
  - Coordinates between repository, cache, and factory components

### 3. Repository Layer
- **Location**: `internal/repository/`
- **Responsibility**: Data access and external API interactions
- **Components**:
  - `KubernetesRepository`: Interacts with Kubernetes API for CRD schemas
  - Handles CRD parsing and validation
  - Manages API version parsing and CRD name construction

### 4. Domain Layer
- **Location**: `internal/domain/`
- **Responsibility**: Core business models and domain logic
- **Components**:
  - Domain models: `SchemaInfo`, `ResourceReference`, `ExecutionContext`
  - Discovery options and statistics
  - Error types and custom errors

### 5. Infrastructure Layer
- **Location**: `internal/cache/`, `internal/config/`
- **Responsibility**: Infrastructure concerns (caching, configuration)
- **Components**:
  - `MemoryCache`: In-memory caching with TTL
  - `Config`: Environment-based configuration management

### 6. Utilities Layer
- **Location**: `pkg/utils/`
- **Responsibility**: Shared utilities and cross-cutting concerns
- **Components**:
  - `ReferenceExtractor`: Extracts reference fields from specs
  - `ContextExtractor`: Extracts execution context from XR
  - `Logger`: Structured logging implementation

## Design Patterns Implemented

### 1. Repository Pattern
```go
// interfaces/interfaces.go
type SchemaRepository interface {
    GetCRDSchema(ctx context.Context, kind, apiVersion string) (*domain.SchemaInfo, error)
    ListCRDs(ctx context.Context, labelSelector string) ([]*domain.CRDInfo, error)
    ValidateSchema(schema *domain.SchemaInfo) error
}
```

**Benefits**:
- Decouples business logic from data access
- Easy to test with mock implementations
- Swappable implementations (Kubernetes API, file system, etc.)

### 2. Factory Pattern
```go
// pkg/factory/schema.go  
type SchemaFactory interface {
    CreateSchema(crd interface{}, includeFullSchema bool) (*domain.SchemaInfo, error)
    CreateFallbackSchema(ref domain.ResourceReference, includeFullSchema bool) *domain.SchemaInfo
}
```

**Benefits**:
- Centralized object creation logic
- Consistent schema structure
- Easy to extend with new schema types

### 3. Dependency Injection
```go
// fn_refactored.go
func NewRefactoredFunction(log logging.Logger) *RefactoredFunction {
    cfg := config.New()
    logger := utils.NewSlogLogger()
    cacheProvider := cache.NewMemoryCache(cfg.CacheTTL)
    // ... other dependencies
    
    return &RefactoredFunction{
        config:           cfg,
        logger:           logger,
        cache:            cacheProvider,
        // ... injected dependencies
    }
}
```

**Benefits**:
- Loose coupling between components
- Easy unit testing with mocks
- Flexible configuration and swapping of implementations

### 4. Strategy Pattern
```go
// Different caching strategies can be implemented
type CacheProvider interface {
    Get(key string) (*domain.SchemaInfo, bool)
    Set(key string, value *domain.SchemaInfo) 
    Size() int
    Clear()
}
```

**Benefits**:
- Pluggable caching strategies (memory, Redis, etc.)
- Runtime strategy selection
- Easy to add new caching behaviors

### 5. Builder Pattern (Configuration)
```go
// internal/config/config.go
func New() *Config {
    return &Config{
        CacheTTL:                 getEnvDuration("CACHE_TTL", 5*time.Minute),
        DefaultTraversalDepth:    getEnvInt("DEFAULT_TRAVERSAL_DEPTH", 3),
        // ... other configuration
    }
}
```

## Package Structure

```
function-kubecore-schema-registry/
├── main.go                           # Application entry point
├── fn.go                            # Function wrapper (compatibility)
├── fn_refactored.go                 # Main function implementation
├── fn_test.go                       # Integration tests
├── 
├── internal/                        # Private application code
│   ├── domain/                      # Domain models and business logic
│   │   └── models.go               # Core domain models
│   ├── service/                     # Business logic layer
│   │   └── discovery.go            # Schema discovery orchestration
│   ├── repository/                  # Data access layer
│   │   └── kubernetes.go           # Kubernetes API interactions
│   ├── cache/                       # Caching infrastructure
│   │   └── memory.go               # In-memory cache implementation
│   └── config/                      # Configuration management
│       └── config.go               # Environment-based configuration
├── 
├── pkg/                            # Public library code
│   ├── interfaces/                 # Contract definitions
│   │   └── interfaces.go          # All interface definitions
│   ├── factory/                    # Object creation patterns
│   │   └── schema.go              # Schema factory implementation
│   └── utils/                      # Shared utilities
│       ├── reference.go           # Reference extraction logic
│       ├── context.go             # Context extraction logic
│       └── logger.go              # Logging utilities
└── 
└── input/v1beta1/                  # Function input definitions
    └── input.go                   # Input type definitions
```

## Component Interactions

### Discovery Flow
1. **Request** → `Function.RunFunction()`
2. **Context Extraction** → `ContextExtractor.ExtractExecutionContext()`
3. **Reference Processing** → `ReferenceExtractor.ExtractReferences()`
4. **Schema Discovery** → `DiscoveryService.DiscoverSchemas()`
   - Cache check → `CacheProvider.Get()`
   - Repository call → `SchemaRepository.GetCRDSchema()`
   - Fallback creation → `SchemaFactory.CreateFallbackSchema()`
   - Cache storage → `CacheProvider.Set()`
5. **Response Building** → Return structured response

### Dependency Graph
```
RefactoredFunction
├── Config
├── Logger
├── CacheProvider (MemoryCache)
├── SchemaRepository (KubernetesRepository)
├── SchemaFactory
├── ReferenceExtractor
├── ContextExtractor
└── DiscoveryService
    ├── SchemaRepository
    ├── CacheProvider
    ├── SchemaFactory
    ├── ReferenceExtractor
    └── Logger
```

## Benefits of Refactored Architecture

### 1. **Maintainability**
- Clear separation of concerns
- Single Responsibility Principle applied
- Easy to locate and modify functionality

### 2. **Testability**
- Interface-based design allows easy mocking
- Each component can be tested in isolation
- Dependency injection enables test configurations

### 3. **Extensibility**
- New cache providers can be added without changing business logic
- New schema sources can be implemented via Repository interface
- New reference patterns can be added to ReferenceExtractor

### 4. **Performance**
- Structured caching with TTL support
- Lazy loading of dependencies
- Efficient resource discovery algorithms

### 5. **Reliability**
- Graceful error handling with typed errors
- Fallback mechanisms when external dependencies fail
- Circuit breaker patterns can be easily added

## Configuration

### Environment Variables
- `CACHE_TTL`: Cache time-to-live (default: 5m)
- `DEFAULT_TRAVERSAL_DEPTH`: Default discovery depth (default: 3)
- `DEFAULT_ENABLE_TRANSITIVE`: Enable transitive discovery (default: true)
- `LOG_LEVEL`: Logging level (default: info)
- `DEBUG_ENABLED`: Enable debug logging (default: false)

### Reference Patterns
The system supports configurable reference patterns in `config/config.go`:
- `githubProjectRef` → `GitHubProject`
- `githubProviderRef` → `GithubProvider`
- `providerConfigRef` → `ProviderConfig`
- `secretRef` → `Secret`
- And more...

## Testing Strategy

### Unit Tests
Each component has focused unit tests:
- Repository tests with mock Kubernetes client
- Service tests with mock dependencies
- Factory tests with various input scenarios

### Integration Tests
End-to-end tests in `fn_test.go`:
- Full request/response cycle testing
- Real and mock CRD scenarios
- Performance validation

### Test Doubles
- `fake.NewSimpleClientset()` for Kubernetes API mocking
- Interface implementations for dependency injection
- Structured test data for consistent scenarios

## Migration Guide

### From Old Architecture
The refactored code maintains full backward compatibility:
- Same function interface (`RunFunction`)
- Same input/output contracts
- Same behavior and functionality
- Existing tests continue to pass

### For New Development
Use the new architecture patterns:
```go
// Create function with dependency injection
f := NewRefactoredFunction(logger)

// Set test dependencies
f.SetKubernetesClient(mockClient)

// Use service layer for business logic
result, err := discoveryService.DiscoverSchemas(ctx, execCtx, opts)
```

## Future Enhancements

### Potential Improvements
1. **Circuit Breaker**: Add circuit breaker for external API calls
2. **Metrics**: Add Prometheus metrics collection
3. **Distributed Cache**: Support Redis or other distributed caches
4. **Schema Validation**: Enhanced CRD schema validation
5. **Rate Limiting**: Add rate limiting for API calls
6. **Tracing**: Add distributed tracing support

### Plugin Architecture
The current design supports plugin-like extensions:
- Custom cache providers
- Additional schema repositories
- Custom reference extractors
- New factory implementations

This architecture provides a solid foundation for future growth while maintaining the existing functionality and performance characteristics.