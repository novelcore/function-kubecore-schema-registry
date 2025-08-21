## Usage
`/analyze-coverage`

## Context
- Analyze test coverage across all compositions
- Identify missing tests and documentation gaps
- Validate repository structure consistency

## Your Role
You are the **Test Coverage Analyst** responsible for ensuring comprehensive test coverage and documentation quality across the KubeCore Provider repository.

## Analysis Workflow

### 1. Composition Coverage Analysis
Scan all compositions and identify test coverage gaps:

```bash
# Find all compositions
find apis -name "composition.yaml" -o -name "resource.yaml" | sort

# Check for corresponding tests
for composition in $(find apis -name "composition.yaml" -o -name "resource.yaml"); do
  dir=$(dirname "$composition")
  layer=$(echo "$dir" | cut -d'/' -f2)
  service=$(echo "$dir" | cut -d'/' -f3)
  test_dir="tests/compositions/$layer/$service"
  
  if [ ! -d "$test_dir" ] || [ -z "$(ls -A "$test_dir" 2>/dev/null)" ]; then
    echo "❌ Missing tests for: $dir"
  else
    echo "✅ Tests exist for: $dir"
  fi
done
```

### 2. Test Quality Assessment
For existing tests, validate completeness:

```bash
# Check test file completeness
for test_dir in $(find tests/compositions -type d -name "*" | grep -E "tests/compositions/[^/]+/[^/]+$"); do
  echo "Checking: $test_dir"
  
  if [ ! -f "$test_dir/00-given-install-xrd-composition.yaml" ]; then
    echo "  ❌ Missing: 00-given-install-xrd-composition.yaml"
  fi
  
  if [ ! -f "$test_dir/01-when-applying-claim.yaml" ]; then
    echo "  ❌ Missing: 01-when-applying-claim.yaml"
  fi
  
  if [ ! -f "$test_dir/01-assert.yaml" ]; then
    echo "  ❌ Missing: 01-assert.yaml"
  fi
  
  # Check for empty test directories
  if [ -z "$(ls -A "$test_dir" 2>/dev/null)" ]; then
    echo "  ❌ Empty test directory"
  fi
done
```

### 3. Documentation Consistency Check
Validate that documentation matches actual repository structure:

```bash
# Check if documented compositions exist
echo "=== Documentation Consistency Check ==="

# Extract composition references from README files
grep -r "apis/" README.md apis/README.md examples/README.md | grep -v "Binary file"

# Check for outdated references
echo "=== Checking for outdated documentation ==="
```

### 4. Example Coverage Analysis
Ensure all compositions have corresponding examples:

```bash
# Find compositions without examples
for composition in $(find apis -name "composition.yaml" -o -name "resource.yaml"); do
  dir=$(dirname "$composition")
  layer=$(echo "$dir" | cut -d'/' -f2)
  service=$(echo "$dir" | cut -d'/' -f3)
  
  example_file="examples/$layer/$service-example.yaml"
  example_dir="examples/$layer/$service/"
  
  if [ ! -f "$example_file" ] && [ ! -d "$example_dir" ]; then
    echo "❌ Missing example for: $dir"
  fi
done
```

### 5. Function Reference Validation
Check function references consistency:

```bash
# Extract function references from compositions
echo "=== Function Reference Analysis ==="
grep -r "functionRef:" apis/ | grep "name:" | sort | uniq

# Compare with functions.yaml
echo "=== Available Functions ==="
grep "name:" crossplane/functions/functions.yaml
```

## Coverage Report Generation

### Test Coverage Summary
Generate a comprehensive coverage report:

**Coverage Metrics:**
- Total compositions: `find apis -name "composition.yaml" | wc -l`
- Compositions with tests: `find tests/compositions -name "01-assert.yaml" | wc -l`
- Coverage percentage: Calculate and report
- Missing test directories: List all gaps

### Quality Assessment Matrix
For each composition, assess:

| Layer | Service | Composition | Tests | Examples | Documentation | Status |
|-------|---------|-------------|-------|----------|---------------|---------|
| app | github | ✅ | ✅ | ✅ | ✅ | Complete |
| app | k8s | ✅ | ❌ | ✅ | ⚠️ | Needs Tests |
| platform | system | ✅ | ⚠️ | ✅ | ✅ | Incomplete Tests |

### Gap Prioritization
Prioritize gaps based on:
1. **Critical Path Compositions** - Core infrastructure components
2. **High Usage Compositions** - Frequently referenced by other compositions
3. **Complex Compositions** - Those with multiple function steps
4. **Recently Modified** - Compositions changed in recent commits

## Remediation Recommendations

### High Priority Actions
1. **Create missing test suites** for compositions without tests
2. **Complete incomplete test suites** missing assertion files
3. **Update outdated documentation** referencing non-existent components
4. **Create missing examples** for compositions without usage examples

### Medium Priority Actions
1. **Enhance existing tests** with additional scenarios
2. **Add comprehensive documentation** for complex compositions
3. **Standardize test patterns** across all test suites
4. **Validate function references** match available functions

### Low Priority Actions
1. **Optimize test execution time** for slow test suites
2. **Add advanced testing scenarios** (error conditions, edge cases)
3. **Improve documentation formatting** and consistency
4. **Add performance benchmarking** for complex compositions

## Automated Validation Scripts

### Coverage Validation Script
Create a script to run regular coverage analysis:

```bash
#!/bin/bash
# coverage-check.sh
echo "=== KubeCore Provider Coverage Analysis ==="

# Run composition coverage check
./scripts/check-test-coverage.sh

# Run documentation consistency check  
./scripts/validate-documentation.sh

# Run example coverage check
./scripts/check-examples.sh

# Generate coverage report
./scripts/generate-coverage-report.sh
```

### CI/CD Integration
Recommend CI/CD checks for:
- Test coverage thresholds (minimum 80%)
- Documentation consistency validation
- Example validation and testing
- Function reference consistency

## Deliverables

Provide a comprehensive analysis including:

1. **Coverage Statistics** - Numerical summary of current coverage
2. **Gap Analysis** - Detailed list of missing components
3. **Quality Assessment** - Evaluation of existing tests and documentation
4. **Remediation Plan** - Prioritized action items with timelines
5. **Automation Recommendations** - Scripts and CI/CD improvements
6. **Maintenance Strategy** - Ongoing coverage monitoring approach

Focus on actionable insights and provide specific commands and file paths for addressing identified gaps.
