## Usage
`/create-composition <LAYER> <SERVICE_NAME> <DESCRIPTION>`

## Context
- Target layer: $ARGUMENT_1 (project/app/platform/core)
- Service name: $ARGUMENT_2 (e.g., "redis", "monitoring", "api-gateway")
- Description: $ARGUMENT_3 (brief description of what this composition provides)

## Your Role
You are the **Crossplane Architect** responsible for creating new compositions following KubeCore Provider patterns and best practices.

## Creation Workflow

### 1. Structure Analysis
First, analyze the target layer and service requirements:
- Examine existing compositions in the `apis/$ARGUMENT_1/` directory
- Identify similar patterns and dependencies
- Determine required providers and functions
- Plan cross-composition dependencies

### 2. XRD Development
Create the Composite Resource Definition following KubeCore patterns:

**File**: `apis/$ARGUMENT_1/$ARGUMENT_2/definition.yaml`

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: CompositeResourceDefinition
metadata:
  name: x$ARGUMENT_2s.$ARGUMENT_1.kubecore.io
spec:
  group: $ARGUMENT_1.kubecore.io
  names:
    kind: X$ARGUMENT_2  # PascalCase
    plural: x$ARGUMENT_2s  # lowercase plural
  claimNames:
    kind: $ARGUMENT_2  # PascalCase
    plural: $ARGUMENT_2s  # lowercase plural
  connectionSecretKeys:
    - kubeconfig  # Standard for all compositions
    # Add service-specific keys
  versions:
  - name: v1alpha1
    served: true
    referenceable: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              id:
                type: string
                description: "Unique identifier for this $ARGUMENT_2"
              parameters:
                type: object
                description: "$ARGUMENT_2 configuration parameters"
                properties:
                  # Define service-specific parameters
              # Add required references based on layer
            required:
            - id
            - parameters
          status:
            type: object
            properties:
              # Define status fields that can be referenced by other compositions
```

### 3. Composition Implementation
Create the composition with proper function pipeline:

**File**: `apis/$ARGUMENT_1/$ARGUMENT_2/composition.yaml`

```yaml
apiVersion: apiextensions.crossplane.io/v1
kind: Composition
metadata:
  name: $ARGUMENT_2
  labels:
    crossplane.io/xrd: x$ARGUMENT_2s.$ARGUMENT_1.kubecore.io
    provider: multi-cloud  # or specific provider
    service: $ARGUMENT_2
    layer: $ARGUMENT_1
spec:
  writeConnectionSecretsToNamespace: crossplane-system
  compositeTypeRef:
    apiVersion: $ARGUMENT_1.kubecore.io/v1alpha1
    kind: X$ARGUMENT_2
  mode: Pipeline
  pipeline:
  
  # Load dependencies if needed
  - step: load-extra-resources
    functionRef:
      name: function-extra-resources
    input:
      apiVersion: extra-resources.fn.crossplane.io/v1beta1
      kind: Input
      spec:
        extraResources:
        # Define required external resources
  
  # Load environment configuration
  - step: load-environment-configs
    functionRef:
      name: function-environment-configs
    input:
      apiVersion: environmentconfigs.fn.crossplane.io/v1beta1
      kind: Input
      spec:
        environmentConfigs:
        - ref:
            name: platform-defaults
          type: Reference
  
  # Generate resources
  - step: go-template-resources
    functionRef:
      name: function-go-templating
    input:
      apiVersion: gotemplating.fn.crossplane.io/v1beta1
      kind: GoTemplate
      source: Inline
      inline:
        template: |
          {{/* Extract basic variables with nil safety */}}
          {{ $claimName := .observed.composite.resource.spec.claimRef.name }}
          {{ $serviceId := .observed.composite.resource.spec.id }}
          
          {{/* Generate service-specific resources */}}
          # Implementation here
  
  # Aggregate readiness
  - step: auto-ready
    functionRef:
      name: function-auto-ready
```

### 4. Example Creation
Create a comprehensive example:

**File**: `examples/$ARGUMENT_1/$ARGUMENT_2-example.yaml`

```yaml
apiVersion: $ARGUMENT_1.kubecore.io/v1alpha1
kind: $ARGUMENT_2
metadata:
  name: example-$ARGUMENT_2
  namespace: default
spec:
  id: example-$ARGUMENT_2
  parameters:
    # Realistic example parameters
  # Add required references based on layer
  writeConnectionSecretToRef:
    name: $ARGUMENT_2-connection
    namespace: default
```

### 5. Test Suite Creation
Create comprehensive KUTTL tests:

**Directory**: `tests/compositions/$ARGUMENT_1/$ARGUMENT_2/`

Files to create:
- `00-given-install-xrd-composition.yaml` - XRD and Composition installation
- `01-when-applying-claim.yaml` - Test claim application
- `01-assert.yaml` - Validation assertions

### 6. Documentation
Create service-specific documentation:

**File**: `apis/$ARGUMENT_1/$ARGUMENT_2/README.md`

Include:
- Service overview and capabilities
- Parameter reference
- Usage examples
- Integration patterns
- Troubleshooting guide

## Layer-Specific Considerations

### Project Layer (`apis/project/`)
- Focus on project setup and team management
- Integrate with GitHub and organizational tools
- Minimal infrastructure dependencies

### App Layer (`apis/app/`)
- Application deployment and management
- Depends on platform and core layers
- Include ingress, scaling, and monitoring

### Platform Layer (`apis/platform/`)
- Infrastructure services and system components
- May depend on core layer
- Provide services for app layer

### Core Layer (`apis/core/`)
- Fundamental infrastructure (clusters, networking)
- No dependencies on other layers
- Foundation for all other layers

## Quality Checklist

Before completing the composition:

- [ ] **XRD Schema**: Comprehensive OpenAPI v3 schema with descriptions
- [ ] **Function Pipeline**: Proper function ordering and nil-safe templates
- [ ] **Cross-References**: Correct references to dependencies
- [ ] **Examples**: Realistic and tested examples
- [ ] **Tests**: Complete KUTTL test coverage
- [ ] **Documentation**: Comprehensive README with usage patterns
- [ ] **Integration**: Update main kustomization.yaml if needed

## File Updates Required

1. **Add to kustomization**: Update `apis/kustomization.yaml`
2. **Update main README**: Add service to architecture documentation
3. **Update examples README**: Document new example
4. **Test integration**: Verify with `kubectl kuttl test`

Focus on following established patterns while adapting to the specific service requirements. Ensure the composition is robust, well-tested, and properly documented.
