package resolver

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// DirectResolver handles Phase 1 direct resource matching
type DirectResolver struct {
	dynamicClient dynamic.Interface
	typedClient   kubernetes.Interface
	registry      registry.Registry
}

// NewDirectResolver creates a new direct resolver
func NewDirectResolver(dynamicClient dynamic.Interface, typedClient kubernetes.Interface, registry registry.Registry) *DirectResolver {
	return &DirectResolver{
		dynamicClient: dynamicClient,
		typedClient:   typedClient,
		registry:      registry,
	}
}

// SupportsMatchType checks if this resolver supports the given match type
func (r *DirectResolver) SupportsMatchType(matchType v1beta1.MatchType) bool {
	return matchType == v1beta1.MatchTypeDirect
}

// Resolve resolves resources using direct name/namespace matching (Phase 1)
func (r *DirectResolver) Resolve(ctx context.Context, request v1beta1.ResourceRequest) ([]*FetchedResource, error) {
	startTime := time.Now()

	// Validate direct match requirements
	if request.Name == "" {
		return nil, functionerrors.InvalidSelectorError("name is required for direct match type")
	}

	fetchedResource := &FetchedResource{
		Request:   request,
		FetchedAt: startTime,
		Metadata: ResourceMetadata{
			FetchStatus: FetchStatusError, // Default to error, update on success
			Phase2Metadata: &Phase2Metadata{
				MatchedBy: "direct",
			},
		},
	}

	// Convert APIVersion and Kind to GVR
	gvr, err := r.getGVR(request.APIVersion, request.Kind)
	if err != nil {
		fetchedResource.Metadata.Error = functionerrors.ValidationError(
			fmt.Sprintf("failed to resolve GVR for %s/%s: %v", request.APIVersion, request.Kind, err)).
			WithResource(functionerrors.ResourceRef{
				Into:       request.Into,
				Name:       request.Name,
				Namespace:  stringPtrValue(request.Namespace),
				APIVersion: request.APIVersion,
				Kind:       request.Kind,
			})
		fetchedResource.Metadata.FetchDuration = time.Since(startTime)
		return []*FetchedResource{fetchedResource}, nil
	}

	// Determine if the resource is namespaced
	var resource dynamic.ResourceInterface
	if request.Namespace != nil && *request.Namespace != "" {
		resource = r.dynamicClient.Resource(gvr).Namespace(*request.Namespace)
	} else {
		resource = r.dynamicClient.Resource(gvr)
	}

	// Fetch the resource
	obj, err := resource.Get(ctx, request.Name, metav1.GetOptions{})
	fetchedResource.Metadata.FetchDuration = time.Since(startTime)

	if err != nil {
		resourceRef := functionerrors.ResourceRef{
			Into:       request.Into,
			Name:       request.Name,
			Namespace:  stringPtrValue(request.Namespace),
			APIVersion: request.APIVersion,
			Kind:       request.Kind,
		}

		if errors.IsNotFound(err) {
			fetchedResource.Metadata.FetchStatus = FetchStatusNotFound
			fetchedResource.Metadata.ResourceExists = false
			fetchedResource.Metadata.Error = functionerrors.ResourceNotFoundError(resourceRef)
		} else if errors.IsForbidden(err) {
			fetchedResource.Metadata.FetchStatus = FetchStatusForbidden
			fetchedResource.Metadata.Error = functionerrors.ResourceForbiddenError(resourceRef)
		} else if errors.IsTimeout(err) || ctx.Err() == context.DeadlineExceeded {
			fetchedResource.Metadata.FetchStatus = FetchStatusTimeout
			fetchedResource.Metadata.Error = functionerrors.ResourceTimeoutError(resourceRef, time.Since(startTime))
		} else {
			fetchedResource.Metadata.FetchStatus = FetchStatusError
			fetchedResource.Metadata.Error = functionerrors.New(
				functionerrors.ErrorCodeKubernetesClient,
				fmt.Sprintf("failed to fetch resource: %v", err)).
				WithResource(resourceRef)
		}

		return []*FetchedResource{fetchedResource}, nil
	}

	// Success case
	fetchedResource.Resource = obj
	fetchedResource.Metadata.FetchStatus = FetchStatusSuccess
	fetchedResource.Metadata.ResourceExists = true

	return []*FetchedResource{fetchedResource}, nil
}

// getGVR converts apiVersion and kind to GroupVersionResource
func (r *DirectResolver) getGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
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
	plural := r.pluralize(kind)

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
func (r *DirectResolver) pluralize(kind string) string {
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

// stringPtrValue safely gets the value of a string pointer
func stringPtrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}