# Crossplane Function: KubeCore Schema Registry

## Repository Overview
This repository contains a Crossplane Composition Function written in Go that manages schema registry resources for KubeCore platform. The function follows Crossplane's composition function architecture to dynamically template and manage cloud resources.

## What is a Crossplane Function?
Composition functions are custom programs that template Crossplane resources. When you create a Composite Resource (XR), Crossplane calls this function to determine what managed resources should be created. This allows for advanced templating logic using a full programming language rather than static YAML patches.

## Repository Structure
```
function-kubecore-schema-registry/
├── fn.go                 # Main function logic - RunFunction implementation
├── fn_test.go           # Unit tests for the function
├── main.go              # Function entrypoint (rarely needs editing)
├── Dockerfile           # Builds the function runtime
├── input/               # Function input type definitions
│   └── v1beta1/        # Input API version
├── package/            # Package metadata for distribution
│   ├── crossplane.yaml # Package configuration
│   └── input/         # Generated OpenAPI schemas
└── example/           # Example XR, Composition, and Functions
```

## Key Concepts

### RunFunction Method
The core of this function is the `RunFunction` method in `fn.go`. It:
1. Receives a `RunFunctionRequest` containing the observed XR and any existing desired resources
2. Processes the XR's spec to determine what resources to create
3. Returns a `RunFunctionResponse` with the desired composed resources

### Input Types
Functions can optionally accept structured input via the `input/` directory. Input is defined as Go structs and automatically generates OpenAPI schemas.

### Protocol Buffers
This function uses Protocol Buffers (protobuf) for communication with Crossplane. Key types:
- `fnv1.RunFunctionRequest`: What Crossplane sends to the function
- `fnv1.RunFunctionResponse`: What the function returns to Crossplane

## Development Workflow

### Local Development
1. **Edit Logic**: Modify `fn.go` to implement your resource templating logic
2. **Update Input**: If using input types, edit `input/v1beta1/input.go` and run `go generate ./...`
3. **Test Locally**: Run `go test -v -cover .` for unit tests
4. **Live Testing**: Use `go run . --insecure --debug` with `crossplane render` for end-to-end testing

### Building & Packaging
1. **Build Runtime**: `docker build . --platform=linux/amd64 --tag runtime-amd64`
2. **Build Package**: `crossplane xpkg build --package-root=package --embed-runtime-image=runtime-amd64 --package-file=function.xpkg`
3. **Push Package**: `crossplane xpkg push --package-files=function.xpkg <registry>/<repo>:<tag>`

## Testing Strategy

### Unit Tests
- Located in `fn_test.go`
- Test individual functions and logic paths
- Mock Crossplane requests/responses
- Run with: `go test -v -cover .`

### End-to-End Testing
- Use `crossplane render` with example files
- No Kubernetes cluster required
- Tests actual function behavior with real XR inputs

### Example Testing Command
```bash
# Terminal 1: Run function locally
go run . --insecure --debug

# Terminal 2: Test with example files
crossplane render example/xr.yaml example/composition.yaml example/functions.yaml
```

## Important Files to Edit

### When Starting Development
1. **package/crossplane.yaml**: Update package name and metadata
2. **go.mod**: Change module name to match your repository
3. **input/v1beta1/input.go**: Define your input schema (or delete if not needed)

### Core Development
1. **fn.go**: Implement your function logic
2. **fn_test.go**: Add comprehensive unit tests
3. **example/**: Update example files to match your use case

## SDK and Dependencies

### Crossplane Function SDK
The function uses Crossplane's Go SDK for easier development:
- `github.com/crossplane/function-sdk-go/request`: Request handling utilities
- `github.com/crossplane/function-sdk-go/response`: Response building utilities
- `github.com/crossplane/function-sdk-go/resource`: Resource manipulation helpers

### Provider Types
When composing provider resources (e.g., AWS S3 buckets), import the provider's Go module:
```go
import "github.com/crossplane-contrib/provider-upjet-aws/apis/s3/v1beta1"
```

## Best Practices

### Error Handling
- Always use `response.Fatal()` for unrecoverable errors
- Include context in error messages with `errors.Wrapf()`
- Return the response even when erroring (Crossplane expects it)

### Resource Naming
- Use deterministic names for composed resources
- Include the XR name in composed resource names for traceability

### Logging
- Use structured logging with `f.log.Info()`
- Include relevant context (XR name, kind, etc.)
- Use debug level for verbose operational details

### Performance
- Minimize external API calls
- Cache computed values when possible
- Keep function execution time under 20 seconds (default timeout)

## Common Patterns

### Reading XR Fields
```go
region, err := xr.Resource.GetString("spec.region")
names, err := xr.Resource.GetStringArray("spec.names")
```

### Creating Composed Resources
```go
desired[resource.Name("resource-name")] = &resource.DesiredComposed{
    Resource: cd,
}
```

### Setting External Names
```go
annotations: map[string]string{
    "crossplane.io/external-name": externalName,
}
```

## Debugging Tips

1. **Enable Debug Logging**: Run with `--debug` flag
2. **Check Request/Response**: Log the full request/response during development
3. **Use crossplane render**: Test without deploying to a cluster
4. **Validate Generated Resources**: Ensure all required fields are set
5. **Test Edge Cases**: Empty arrays, missing fields, invalid inputs

## CI/CD Integration
The repository includes GitHub Actions workflows that:
- Run linting and formatting checks
- Execute unit tests
- Build multi-platform images
- Push packages to registry on tags

## Getting Help
- [Crossplane Docs](https://docs.crossplane.io/latest/concepts/composition-functions/)
- [Function SDK Go Docs](https://pkg.go.dev/github.com/crossplane/function-sdk-go)
- [Buf Schema Registry](https://buf.build/crossplane/crossplane/docs) for protobuf schemas