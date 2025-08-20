package resolver

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// ExpressionResolver handles Phase 2 expression-based resource matching
type ExpressionResolver struct {
	dynamicClient dynamic.Interface
	typedClient   kubernetes.Interface
	registry      registry.Registry
	context       DiscoveryContext
}

// NewExpressionResolver creates a new expression resolver
func NewExpressionResolver(dynamicClient dynamic.Interface, typedClient kubernetes.Interface, registry registry.Registry, ctx DiscoveryContext) *ExpressionResolver {
	return &ExpressionResolver{
		dynamicClient: dynamicClient,
		typedClient:   typedClient,
		registry:      registry,
		context:       ctx,
	}
}

// SupportsMatchType checks if this resolver supports the given match type
func (r *ExpressionResolver) SupportsMatchType(matchType v1beta1.MatchType) bool {
	return matchType == v1beta1.MatchTypeExpression
}

// Resolve resolves resources using expression-based matching
func (r *ExpressionResolver) Resolve(ctx context.Context, request v1beta1.ResourceRequest) ([]*FetchedResource, error) {
	startTime := time.Now()

	// Validate expression requirements
	if request.Selector == nil || len(request.Selector.Expressions) == 0 {
		return nil, functionerrors.InvalidSelectorError("expressions are required for expression match type")
	}

	// Compile expressions
	compiledExpressions, err := r.compileExpressions(request.Selector.Expressions)
	if err != nil {
		return nil, functionerrors.SelectorCompilationError(
			fmt.Sprintf("failed to compile expressions: %v", err))
	}

	// Convert APIVersion and Kind to GVR
	gvr, err := r.getGVR(request.APIVersion, request.Kind)
	if err != nil {
		return nil, functionerrors.ValidationError(
			fmt.Sprintf("failed to resolve GVR for %s/%s: %v", request.APIVersion, request.Kind, err))
	}

	// Determine target namespaces
	namespaces := r.getTargetNamespaces(request)

	// Collect resources from target namespaces
	var allResources []*FetchedResource
	var searchedNamespaces []string

	for _, namespace := range namespaces {
		resources, err := r.fetchFromNamespace(ctx, gvr, namespace, compiledExpressions, request, startTime)
		if err != nil {
			// Log error but continue with other namespaces
			continue
		}

		allResources = append(allResources, resources...)
		searchedNamespaces = append(searchedNamespaces, namespace)
	}

	// Apply strategy constraints and sorting
	finalResources, err := r.applyMatchStrategy(allResources, request, searchedNamespaces)
	if err != nil {
		return nil, err
	}

	return finalResources, nil
}

// CompiledExpression represents a compiled expression for efficient evaluation
type CompiledExpression struct {
	Original v1beta1.Expression
	Regex    *regexp.Regexp // For regex operations
}

// compileExpressions pre-compiles expressions for efficient evaluation
func (r *ExpressionResolver) compileExpressions(expressions []v1beta1.Expression) ([]CompiledExpression, error) {
	var compiled []CompiledExpression

	for _, expr := range expressions {
		compiledExpr := CompiledExpression{
			Original: expr,
		}

		// Validate expression
		if err := r.validateExpression(expr); err != nil {
			return nil, fmt.Errorf("invalid expression for field %s: %v", expr.Field, err)
		}

		// Compile regex if needed
		if expr.Operator == v1beta1.ExpressionOpRegex {
			if expr.Value == nil {
				return nil, fmt.Errorf("regex expression requires a value for field %s", expr.Field)
			}
			regex, err := regexp.Compile(*expr.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern for field %s: %v", expr.Field, err)
			}
			compiledExpr.Regex = regex
		}

		compiled = append(compiled, compiledExpr)
	}

	return compiled, nil
}

// validateExpression validates a single expression
func (r *ExpressionResolver) validateExpression(expr v1beta1.Expression) error {
	if expr.Field == "" {
		return fmt.Errorf("field is required")
	}

	switch expr.Operator {
	case v1beta1.ExpressionOpEquals, v1beta1.ExpressionOpNotEquals, 
		 v1beta1.ExpressionOpContains, v1beta1.ExpressionOpStartsWith, 
		 v1beta1.ExpressionOpEndsWith, v1beta1.ExpressionOpRegex:
		if expr.Value == nil {
			return fmt.Errorf("operator %s requires a value", expr.Operator)
		}
	case v1beta1.ExpressionOpIn, v1beta1.ExpressionOpNotIn:
		if len(expr.Values) == 0 {
			return fmt.Errorf("operator %s requires values", expr.Operator)
		}
	case v1beta1.ExpressionOpExists, v1beta1.ExpressionOpNotExists:
		// No validation needed
	default:
		return fmt.Errorf("unsupported operator: %s", expr.Operator)
	}

	return nil
}

// getTargetNamespaces determines which namespaces to search
func (r *ExpressionResolver) getTargetNamespaces(request v1beta1.ResourceRequest) []string {
	// Check if resource is namespaced
	resourceType, err := r.registry.GetResourceType(request.APIVersion, request.Kind)
	if err != nil || !resourceType.Namespaced {
		// For cluster-scoped resources, return empty namespace list
		return []string{""}
	}

	// For namespaced resources
	if request.Selector.Namespaces != nil && len(request.Selector.Namespaces) > 0 {
		return request.Selector.Namespaces
	}

	// Check if cross-namespace discovery is enabled
	if request.Selector.CrossNamespace != nil && *request.Selector.CrossNamespace {
		// TODO: Get all namespaces from cluster
		// For now, return function namespace and common namespaces
		return []string{r.context.FunctionNamespace, "default", "kube-system"}
	}

	// Default to function namespace
	return []string{r.context.FunctionNamespace}
}

// fetchFromNamespace fetches and filters resources from a specific namespace
func (r *ExpressionResolver) fetchFromNamespace(ctx context.Context, gvr schema.GroupVersionResource,
	namespace string, expressions []CompiledExpression, request v1beta1.ResourceRequest,
	startTime time.Time) ([]*FetchedResource, error) {

	var resource dynamic.ResourceInterface
	if namespace != "" {
		resource = r.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resource = r.dynamicClient.Resource(gvr)
	}

	// List all resources (we'll filter client-side)
	listOptions := metav1.ListOptions{}

	// Apply strategy early termination if needed
	if request.Strategy != nil && request.Strategy.MaxMatches != nil {
		listOptions.Limit = int64(*request.Strategy.MaxMatches * 2) // Get more than needed for filtering
	}

	list, err := resource.List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources in namespace %s: %v", namespace, err)
	}

	var matchedResources []*FetchedResource
	
	// Evaluate each resource against expressions
	for _, item := range list.Items {
		matched, matchDetails := r.evaluateExpressions(&item, expressions)
		
		if matched {
			fetchedResource := &FetchedResource{
				Request:   request,
				Resource:  &item,
				FetchedAt: startTime,
				Metadata: ResourceMetadata{
					FetchStatus:    FetchStatusSuccess,
					ResourceExists: true,
					FetchDuration:  time.Since(startTime),
					Phase2Metadata: &Phase2Metadata{
						MatchedBy:        "expression",
						MatchDetails:     matchDetails,
						SearchNamespaces: []string{namespace},
					},
				},
			}
			matchedResources = append(matchedResources, fetchedResource)

			// Early termination if requested
			if request.Strategy != nil && request.Strategy.StopOnFirst != nil && *request.Strategy.StopOnFirst {
				break
			}
		}
	}

	return matchedResources, nil
}

// evaluateExpressions evaluates all expressions against a resource
func (r *ExpressionResolver) evaluateExpressions(resource *unstructured.Unstructured, expressions []CompiledExpression) (bool, *MatchDetails) {
	matchDetails := &MatchDetails{
		MatchedExpressions: make([]ExpressionMatch, 0),
	}

	allMatched := true
	var totalScore float64

	for _, compiledExpr := range expressions {
		expr := compiledExpr.Original
		actualValue := r.getFieldValue(resource, expr.Field)
		
		matched := r.evaluateExpression(expr, actualValue, compiledExpr.Regex)

		expressionMatch := ExpressionMatch{
			Field:       expr.Field,
			Operator:    string(expr.Operator),
			ActualValue: actualValue,
			Matched:     matched,
		}

		if expr.Value != nil {
			expressionMatch.ExpectedValue = *expr.Value
		} else if len(expr.Values) > 0 {
			expressionMatch.ExpectedValue = expr.Values
		}

		matchDetails.MatchedExpressions = append(matchDetails.MatchedExpressions, expressionMatch)

		if !matched {
			allMatched = false
		} else {
			totalScore += 1.0 // Simple scoring, can be enhanced
		}
	}

	if allMatched && len(expressions) > 0 {
		score := totalScore / float64(len(expressions))
		matchDetails.MatchScore = &score
	}

	return allMatched, matchDetails
}

// evaluateExpression evaluates a single expression against a value
func (r *ExpressionResolver) evaluateExpression(expr v1beta1.Expression, actualValue interface{}, regex *regexp.Regexp) bool {
	actualStr := fmt.Sprintf("%v", actualValue)

	switch expr.Operator {
	case v1beta1.ExpressionOpEquals:
		return actualStr == *expr.Value
	case v1beta1.ExpressionOpNotEquals:
		return actualStr != *expr.Value
	case v1beta1.ExpressionOpIn:
		for _, value := range expr.Values {
			if actualStr == value {
				return true
			}
		}
		return false
	case v1beta1.ExpressionOpNotIn:
		for _, value := range expr.Values {
			if actualStr == value {
				return false
			}
		}
		return true
	case v1beta1.ExpressionOpContains:
		return strings.Contains(actualStr, *expr.Value)
	case v1beta1.ExpressionOpStartsWith:
		return strings.HasPrefix(actualStr, *expr.Value)
	case v1beta1.ExpressionOpEndsWith:
		return strings.HasSuffix(actualStr, *expr.Value)
	case v1beta1.ExpressionOpRegex:
		return regex.MatchString(actualStr)
	case v1beta1.ExpressionOpExists:
		return actualValue != nil && actualStr != ""
	case v1beta1.ExpressionOpNotExists:
		return actualValue == nil || actualStr == ""
	default:
		return false
	}
}

// getFieldValue extracts a field value from a resource using dot notation
func (r *ExpressionResolver) getFieldValue(resource *unstructured.Unstructured, fieldPath string) interface{} {
	parts := strings.Split(fieldPath, ".")
	var current interface{} = resource.Object

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil
			}
			current = val
		case []interface{}:
			// Handle array indices
			if index, err := strconv.Atoi(part); err == nil && index >= 0 && index < len(v) {
				current = v[index]
			} else {
				return nil
			}
		default:
			// For non-map/slice types, try basic field access
			return nil
		}
	}

	return current
}

// applyMatchStrategy applies strategy constraints and sorting
func (r *ExpressionResolver) applyMatchStrategy(resources []*FetchedResource,
	request v1beta1.ResourceRequest, searchedNamespaces []string) ([]*FetchedResource, error) {

	// Update search namespaces for all resources
	for _, resource := range resources {
		if resource.Metadata.Phase2Metadata != nil {
			resource.Metadata.Phase2Metadata.SearchNamespaces = searchedNamespaces
		}
	}

	// Apply sorting if specified
	if request.Strategy != nil && len(request.Strategy.SortBy) > 0 {
		r.sortResources(resources, request.Strategy.SortBy)
	}

	// Apply limit constraints
	if request.Strategy != nil && request.Strategy.MaxMatches != nil {
		maxMatches := *request.Strategy.MaxMatches
		if len(resources) > maxMatches {
			resources = resources[:maxMatches]
		}
	}

	// Check minimum constraints
	if request.Strategy != nil && request.Strategy.MinMatches != nil {
		minMatches := *request.Strategy.MinMatches
		if len(resources) < minMatches {
			if request.Strategy.FailOnConstraintViolation != nil && *request.Strategy.FailOnConstraintViolation {
				return nil, functionerrors.ConstraintViolationError(
					fmt.Sprintf("minimum matches constraint violated: expected %d, got %d", minMatches, len(resources)))
			}
		}
	}

	return resources, nil
}

// sortResources sorts resources based on sort criteria
func (r *ExpressionResolver) sortResources(resources []*FetchedResource, sortBy []v1beta1.SortCriteria) {
	sort.Slice(resources, func(i, j int) bool {
		for _, criteria := range sortBy {
			valI := r.getFieldValueForSort(resources[i].Resource, criteria.Field)
			valJ := r.getFieldValueForSort(resources[j].Resource, criteria.Field)

			comparison := strings.Compare(valI, valJ)
			if comparison == 0 {
				continue // Try next criteria
			}

			if criteria.Order == v1beta1.SortOrderDesc {
				return comparison > 0
			}
			return comparison < 0
		}
		return false
	})

	// Update sort positions
	for i, resource := range resources {
		if resource.Metadata.Phase2Metadata != nil {
			resource.Metadata.Phase2Metadata.SortPosition = &i
		}
	}
}

// getFieldValueForSort extracts field value for sorting as string
func (r *ExpressionResolver) getFieldValueForSort(resource *unstructured.Unstructured, field string) string {
	value := r.getFieldValue(resource, field)
	return fmt.Sprintf("%v", value)
}

// getGVR converts apiVersion and kind to GroupVersionResource
func (r *ExpressionResolver) getGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	// First check registry for plural form
	if rt, err := r.registry.GetResourceType(apiVersion, kind); err == nil && rt.Plural != "" {
		gv, err := schema.ParseGroupVersion(apiVersion)
		if err != nil {
			return schema.GroupVersionResource{}, err
		}

		return schema.GroupVersionResource{
			Group:    gv.Group,
			Version:  gv.Version,
			Resource: rt.Plural,
		}, nil
	}

	// Fallback to basic pluralization for common cases
	plural := pluralize(kind)

	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}

	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: plural,
	}, nil
}