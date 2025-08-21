## Usage
`/optimize-template <COMPOSITION_FILE> [FOCUS_AREA]`

## Context
- Composition file to optimize: $ARGUMENT_1
- Focus area (optional): $ARGUMENT_2 (performance/safety/readability/functionality)

## Your Role
You are the **Template Optimization Specialist** focused on improving Go template performance, safety, and maintainability in Crossplane compositions.

## Optimization Workflow

### 1. Template Analysis
First, examine the current template implementation:

```bash
# Read the composition file
cat $ARGUMENT_1

# Check for common optimization opportunities
grep -n "{{" $ARGUMENT_1 | head -20
```

Analyze for:
- **Nil safety patterns** - Missing nil checks
- **Performance bottlenecks** - Redundant operations
- **Readability issues** - Complex nested logic
- **Error handling** - Missing error recovery
- **YAML safety** - Unquoted special characters

### 2. Nil Safety Optimization
Transform unsafe template access patterns:

**❌ Unsafe Pattern:**
```yaml
template: |
  {{ $kubenet := .context.kubenet }}
  {{ $domain := $kubenet.spec.dns.domain }}
```

**✅ Optimized Pattern:**
```yaml
template: |
  {{/* Nil-safe external resource access */}}
  {{- $extraResources := index .context "apiextensions.crossplane.io/extra-resources" | default dict }}
  {{- $kubenets := index $extraResources "kubenet" | default list }}
  
  {{- if gt (len $kubenets) 0 }}
  {{- $kubenet := index $kubenets 0 }}
  {{- $domain := index $kubenet.spec.dns "domain" | default "" }}
  
  {{- if $domain }}
  # Generate resources only when domain is available
  {{- end }}
  {{- end }}
```

### 3. Performance Optimization
Identify and optimize performance bottlenecks:

**❌ Inefficient Pattern:**
```yaml
template: |
  {{- range .items }}
  {{- $resource := . }}
  {{- range .otherItems }}
  # Nested loops - O(n²) complexity
  {{- end }}
  {{- end }}
```

**✅ Optimized Pattern:**
```yaml
template: |
  {{/* Pre-process data to avoid nested loops */}}
  {{- $resourceMap := dict }}
  {{- range .items }}
  {{- $resourceMap = set $resourceMap .id . }}
  {{- end }}
  
  {{- range .otherItems }}
  {{- $resource := index $resourceMap .resourceId }}
  {{- if $resource }}
  # O(n) complexity with map lookup
  {{- end }}
  {{- end }}
```

### 4. Readability Enhancement
Break complex templates into manageable sections:

**❌ Complex Monolithic Template:**
```yaml
template: |
  {{/* 200+ lines of complex logic in single template */}}
```

**✅ Modular Template Structure:**
```yaml
# Step 1: Data preparation
- step: prepare-data
  functionRef:
    name: function-go-templating
  input:
    source: Inline
    inline:
      template: |
        {{/* Focus on data extraction and validation */}}

# Step 2: Resource generation  
- step: generate-resources
  functionRef:
    name: function-go-templating
  input:
    source: Inline
    inline:
      template: |
        {{/* Focus on resource creation */}}
```

### 5. Error Handling Improvement
Add comprehensive error handling and debugging:

**Enhanced Error Handling Pattern:**
```yaml
template: |
  {{/* Debug mode support */}}
  {{- $debug := .observed.composite.resource.spec.debug | default false }}
  
  {{/* Error tracking */}}
  {{- $errors := list }}
  
  {{/* Validation with error collection */}}
  {{- if not .observed.composite.resource.spec.id }}
  {{- $errors = append $errors "Missing required field: spec.id" }}
  {{- end }}
  
  {{/* Generate error resource if validation fails */}}
  {{- if gt (len $errors) 0 }}
  ---
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: {{ .observed.composite.resource.spec.claimRef.name }}-errors
    namespace: {{ .observed.composite.resource.spec.claimRef.namespace }}
  data:
    errors: |
      {{- range $errors }}
      - {{ . }}
      {{- end }}
  {{- else }}
  # Generate normal resources
  {{- end }}
```

## Optimization Categories

### Performance Optimizations

#### 1. Reduce Template Complexity
- **Split large templates** into multiple function steps
- **Cache computed values** to avoid recalculation
- **Use efficient data structures** (maps vs. lists for lookups)
- **Minimize nested loops** and complex iterations

#### 2. Optimize External Resource Access
```yaml
# Efficient external resource pattern
{{- $extraResources := index .context "apiextensions.crossplane.io/extra-resources" | default dict }}
{{- $allKubenets := index $extraResources "kubenet" | default list }}
{{- $allZones := index $extraResources "hostedzone" | default list }}

{{/* Create lookup maps for O(1) access */}}
{{- $kubenetMap := dict }}
{{- range $allKubenets }}
{{- $kubenetMap = set $kubenetMap .metadata.name . }}
{{- end }}
```

### Safety Optimizations

#### 1. Comprehensive Nil Checking
```yaml
# Safe nested field access pattern
{{- $value := "" }}
{{- if .observed.composite.resource.spec }}
  {{- if .observed.composite.resource.spec.parameters }}
    {{- if .observed.composite.resource.spec.parameters.networking }}
      {{- $value = .observed.composite.resource.spec.parameters.networking.domain | default "" }}
    {{- end }}
  {{- end }}
{{- end }}
```

#### 2. YAML Safety
```yaml
# Safe YAML generation
{{- $wildcardDomain := printf "*.%s" .domain }}
name: "{{ $wildcardDomain }}"  # Always quote wildcards
value: {{ .numericValue | quote }}  # Quote to prevent type issues
```

### Readability Optimizations

#### 1. Clear Variable Naming
```yaml
{{/* Use descriptive variable names */}}
{{- $clusterName := .observed.composite.resource.spec.claimRef.name }}
{{- $targetNamespace := .observed.composite.resource.spec.parameters.namespace | default "default" }}
{{- $isProduction := eq .observed.composite.resource.spec.environment "production" }}
```

#### 2. Logical Grouping
```yaml
template: |
  {{/* === DATA EXTRACTION === */}}
  {{- $claimName := .observed.composite.resource.spec.claimRef.name }}
  {{- $parameters := .observed.composite.resource.spec.parameters }}
  
  {{/* === EXTERNAL RESOURCE LOADING === */}}
  {{- $extraResources := index .context "apiextensions.crossplane.io/extra-resources" | default dict }}
  
  {{/* === RESOURCE GENERATION === */}}
  {{- if $parameters.enabled }}
  ---
  # Generate enabled resources
  {{- end }}
```

## Optimization Checklist

### Template Safety ✅
- [ ] All external resource access has nil checks
- [ ] YAML strings with special characters are quoted
- [ ] Type conversions are safe and validated
- [ ] Error conditions are handled gracefully

### Performance ✅
- [ ] No unnecessary nested loops or O(n²) operations
- [ ] Computed values are cached when reused
- [ ] External resource access is optimized
- [ ] Template complexity is manageable

### Readability ✅
- [ ] Variables have descriptive names
- [ ] Logic is grouped and commented
- [ ] Complex operations are broken into steps
- [ ] Debug output is available when needed

### Maintainability ✅
- [ ] Template follows consistent patterns
- [ ] Error handling is comprehensive
- [ ] Dependencies are clearly documented
- [ ] Template is testable in isolation

## Validation and Testing

### Template Testing
After optimization, validate the template:

```bash
# Test template rendering with sample data
crossplane render examples/test-claim.yaml apis/composition.yaml functions.yaml

# Validate YAML output
crossplane render examples/test-claim.yaml apis/composition.yaml functions.yaml | kubectl apply --dry-run=client -f -

# Performance testing
time crossplane render examples/test-claim.yaml apis/composition.yaml functions.yaml
```

### Regression Testing
Ensure optimizations don't break functionality:

```bash
# Run existing tests
kubectl kuttl test --test tests/compositions/path/to/composition

# Compare outputs before and after optimization
diff original-output.yaml optimized-output.yaml
```

## Deliverables

Provide optimized composition with:

1. **Optimization Summary** - List of changes made and rationale
2. **Performance Metrics** - Before/after performance comparison
3. **Safety Improvements** - Enhanced error handling and nil safety
4. **Readability Enhancements** - Code structure and documentation improvements
5. **Testing Validation** - Confirmation that functionality is preserved
6. **Maintenance Notes** - Guidelines for maintaining optimized patterns

Focus on practical improvements that enhance both performance and maintainability while preserving existing functionality.
