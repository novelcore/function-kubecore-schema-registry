package resolver

import (
	"context"
	"fmt"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// LabelResolver handles Phase 2 label-based resource matching
type LabelResolver struct {
	dynamicClient dynamic.Interface
	typedClient   kubernetes.Interface
	registry      registry.Registry
	context       DiscoveryContext
}

// NewLabelResolver creates a new label resolver
func NewLabelResolver(dynamicClient dynamic.Interface, typedClient kubernetes.Interface, registry registry.Registry, ctx DiscoveryContext) *LabelResolver {
	return &LabelResolver{
		dynamicClient: dynamicClient,
		typedClient:   typedClient,
		registry:      registry,
		context:       ctx,
	}
}

// SupportsMatchType checks if this resolver supports the given match type
func (r *LabelResolver) SupportsMatchType(matchType v1beta1.MatchType) bool {
	return matchType == v1beta1.MatchTypeLabel
}

// Resolve resolves resources using label selector matching
func (r *LabelResolver) Resolve(ctx context.Context, request v1beta1.ResourceRequest) ([]*FetchedResource, error) {
	startTime := time.Now()

	// Validate label selector requirements
	if request.Selector == nil || request.Selector.Labels == nil {
		return nil, functionerrors.InvalidSelectorError("labels selector is required for label match type")
	}

	// Convert to Kubernetes label selector
	labelSelector, err := r.buildLabelSelector(request.Selector.Labels)
	if err != nil {
		return nil, functionerrors.SelectorCompilationError(
			fmt.Sprintf("failed to compile label selector: %v", err))
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
		resources, err := r.fetchFromNamespace(ctx, gvr, namespace, labelSelector, request, startTime)
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

// buildLabelSelector converts our label selector to Kubernetes label selector
func (r *LabelResolver) buildLabelSelector(labelSelector *v1beta1.LabelSelector) (labels.Selector, error) {
	selector := labels.NewSelector()

	// Add match labels
	if labelSelector.MatchLabels != nil {
		for key, value := range labelSelector.MatchLabels {
			requirement, err := labels.NewRequirement(key, selection.Equals, []string{value})
			if err != nil {
				return nil, fmt.Errorf("invalid label requirement %s=%s: %v", key, value, err)
			}
			selector = selector.Add(*requirement)
		}
	}

	// Add match expressions
	for _, expr := range labelSelector.MatchExpressions {
		var op selection.Operator
		switch expr.Operator {
		case v1beta1.LabelSelectorOpIn:
			op = selection.In
		case v1beta1.LabelSelectorOpNotIn:
			op = selection.NotIn
		case v1beta1.LabelSelectorOpExists:
			op = selection.Exists
		case v1beta1.LabelSelectorOpDoesNotExist:
			op = selection.DoesNotExist
		default:
			return nil, fmt.Errorf("unsupported label selector operator: %s", expr.Operator)
		}

		requirement, err := labels.NewRequirement(expr.Key, op, expr.Values)
		if err != nil {
			return nil, fmt.Errorf("invalid label expression %s %s %v: %v", expr.Key, expr.Operator, expr.Values, err)
		}
		selector = selector.Add(*requirement)
	}

	return selector, nil
}

// getTargetNamespaces determines which namespaces to search
func (r *LabelResolver) getTargetNamespaces(request v1beta1.ResourceRequest) []string {
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

// fetchFromNamespace fetches resources from a specific namespace
func (r *LabelResolver) fetchFromNamespace(ctx context.Context, gvr schema.GroupVersionResource, 
	namespace string, labelSelector labels.Selector, request v1beta1.ResourceRequest, 
	startTime time.Time) ([]*FetchedResource, error) {

	var resource dynamic.ResourceInterface
	if namespace != "" {
		resource = r.dynamicClient.Resource(gvr).Namespace(namespace)
	} else {
		resource = r.dynamicClient.Resource(gvr)
	}

	// List resources with label selector
	listOptions := metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	}

	// Apply strategy early termination if needed
	if request.Strategy != nil && request.Strategy.StopOnFirst != nil && *request.Strategy.StopOnFirst {
		listOptions.Limit = 1
	} else if request.Strategy != nil && request.Strategy.MaxMatches != nil {
		listOptions.Limit = int64(*request.Strategy.MaxMatches)
	}

	list, err := resource.List(ctx, listOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to list resources in namespace %s: %v", namespace, err)
	}

	var resources []*FetchedResource
	for i, item := range list.Items {
		fetchedResource := &FetchedResource{
			Request:   request,
			Resource:  &item,
			FetchedAt: startTime,
			Metadata: ResourceMetadata{
				FetchStatus:    FetchStatusSuccess,
				ResourceExists: true,
				FetchDuration:  time.Since(startTime),
				Phase2Metadata: &Phase2Metadata{
					MatchedBy: "label",
					MatchDetails: &MatchDetails{
						MatchedLabels: r.getMatchedLabels(&item, labelSelector),
					},
					SearchNamespaces: []string{namespace},
					SortPosition:     &i,
				},
			},
		}
		resources = append(resources, fetchedResource)
	}

	return resources, nil
}

// getMatchedLabels extracts the labels that actually matched
func (r *LabelResolver) getMatchedLabels(resource interface{}, selector labels.Selector) map[string]string {
	// TODO: Implement label extraction logic
	// For now, return empty map
	return make(map[string]string)
}

// applyMatchStrategy applies strategy constraints and sorting
func (r *LabelResolver) applyMatchStrategy(resources []*FetchedResource, 
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
func (r *LabelResolver) sortResources(resources []*FetchedResource, sortBy []v1beta1.SortCriteria) {
	sort.Slice(resources, func(i, j int) bool {
		for _, criteria := range sortBy {
			valI := r.getFieldValue(resources[i].Resource, criteria.Field)
			valJ := r.getFieldValue(resources[j].Resource, criteria.Field)

			if valI == valJ {
				continue // Try next criteria
			}

			if criteria.Order == v1beta1.SortOrderDesc {
				return valI > valJ
			}
			return valI < valJ
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

// getFieldValue extracts field value for sorting
func (r *LabelResolver) getFieldValue(resource interface{}, field string) string {
	// TODO: Implement sophisticated field extraction
	// For now, return empty string
	return ""
}

// getGVR converts apiVersion and kind to GroupVersionResource
func (r *LabelResolver) getGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
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

// pluralize provides basic English pluralization rules
func pluralize(kind string) string {
	lower := kind
	if len(lower) == 0 {
		return lower
	}

	// Convert to lowercase for resource names
	plural := ""
	for i, r := range lower {
		if i == 0 {
			plural += string(r + 32) // Convert first char to lowercase
		} else {
			plural += string(r)
		}
	}

	// Basic pluralization rules
	switch {
	case len(plural) > 2 && plural[len(plural)-2:] == "ey":
		return plural + "s"
	case len(plural) > 0 && plural[len(plural)-1] == 'y':
		return plural[:len(plural)-1] + "ies"
	case len(plural) > 0 && (plural[len(plural)-1] == 's' ||
		plural[len(plural)-1] == 'x' ||
		plural[len(plural)-1] == 'z'):
		return plural + "es"
	case len(plural) > 2 && plural[len(plural)-2:] == "ch":
		return plural + "es"
	case len(plural) > 2 && plural[len(plural)-2:] == "sh":
		return plural + "es"
	default:
		return plural + "s"
	}
}