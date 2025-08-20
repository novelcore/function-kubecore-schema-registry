package discovery

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	functionerrors "github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
)

// KubernetesEngine implements the Engine interface using Kubernetes client
type KubernetesEngine struct {
	dynamicClient dynamic.Interface
	typedClient   kubernetes.Interface
	registry      registry.Registry
	timeout       time.Duration
	maxConcurrent int
}

// NewKubernetesEngine creates a new Kubernetes discovery engine
func NewKubernetesEngine(config *rest.Config, registry registry.Registry) (*KubernetesEngine, error) {
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.KubernetesClientError(
			fmt.Sprintf("failed to create dynamic client: %v", err))
	}

	typedClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, functionerrors.KubernetesClientError(
			fmt.Sprintf("failed to create typed client: %v", err))
	}

	return &KubernetesEngine{
		dynamicClient: dynamicClient,
		typedClient:   typedClient,
		registry:      registry,
		timeout:       5 * time.Second, // Default timeout
		maxConcurrent: 10,              // Default max concurrent fetches
	}, nil
}

// NewKubernetesEngineWithTimeout creates a new engine with custom timeout
func NewKubernetesEngineWithTimeout(config *rest.Config, registry registry.Registry, 
	timeout time.Duration, maxConcurrent int) (*KubernetesEngine, error) {
	engine, err := NewKubernetesEngine(config, registry)
	if err != nil {
		return nil, err
	}
	
	engine.timeout = timeout
	engine.maxConcurrent = maxConcurrent
	return engine, nil
}

// FetchResources fetches resources based on the provided requests
func (e *KubernetesEngine) FetchResources(requests []v1beta1.ResourceRequest) (*FetchResult, error) {
	startTime := time.Now()
	
	result := &FetchResult{
		Resources: make(map[string]*FetchedResource),
		Summary: FetchSummary{
			TotalRequested: len(requests),
		},
	}

	// Create a semaphore to limit concurrent requests
	sem := make(chan struct{}, e.maxConcurrent)
	
	// Use errgroup for concurrent processing
	var mu sync.Mutex
	g, ctx := errgroup.WithContext(context.Background())

	for _, req := range requests {
		req := req // Capture loop variable
		g.Go(func() error {
			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			fetchedResource, _ := e.fetchSingleResource(ctx, req)
			
			mu.Lock()
			defer mu.Unlock()
			
			if fetchedResource != nil {
				result.Resources[req.Into] = fetchedResource
				
				// Update statistics
				switch fetchedResource.Metadata.FetchStatus {
				case FetchStatusSuccess:
					result.Summary.Successful++
				case FetchStatusNotFound:
					result.Summary.NotFound++
					if req.Optional {
						result.Summary.Skipped++
					} else {
						result.Summary.Failed++
					}
				case FetchStatusForbidden:
					result.Summary.Forbidden++
					if req.Optional {
						result.Summary.Skipped++
					} else {
						result.Summary.Failed++
					}
				case FetchStatusTimeout:
					result.Summary.Timeout++
					if req.Optional {
						result.Summary.Skipped++
					} else {
						result.Summary.Failed++
					}
				case FetchStatusError:
					if req.Optional {
						result.Summary.Skipped++
					} else {
						result.Summary.Failed++
					}
				}

				// Add to errors if failed and not optional
				if fetchedResource.Metadata.FetchStatus != FetchStatusSuccess && 
				   fetchedResource.Metadata.Error != nil && 
				   !req.Optional {
					result.Summary.Errors = append(result.Summary.Errors, &FetchError{
						ResourceRequest: req,
						Error:          fetchedResource.Metadata.Error,
						Timestamp:      fetchedResource.FetchedAt,
					})
				}
			}

			return nil // Don't propagate individual fetch errors
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		return nil, functionerrors.Wrap(err, "error during concurrent resource fetching")
	}

	// Calculate summary statistics
	result.Summary.TotalDuration = time.Since(startTime)
	if result.Summary.TotalRequested > 0 {
		result.Summary.AverageDuration = result.Summary.TotalDuration / time.Duration(result.Summary.TotalRequested)
	}

	return result, nil
}

// fetchSingleResource fetches a single resource with timeout
func (e *KubernetesEngine) fetchSingleResource(ctx context.Context, 
	req v1beta1.ResourceRequest) (*FetchedResource, error) {
	
	startTime := time.Now()
	fetchedResource := &FetchedResource{
		Request:   req,
		FetchedAt: startTime,
		Metadata: ResourceMetadata{
			FetchStatus: FetchStatusError, // Default to error, update on success
		},
	}

	// Create timeout context
	fetchCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Convert APIVersion and Kind to GVR
	gvr, err := e.getGVR(req.APIVersion, req.Kind)
	if err != nil {
		fetchedResource.Metadata.Error = functionerrors.ValidationError(
			fmt.Sprintf("failed to resolve GVR for %s/%s: %v", req.APIVersion, req.Kind, err)).
			WithResource(functionerrors.ResourceRef{
				Into:       req.Into,
				Name:       req.Name,
				Namespace:  stringPtrValue(req.Namespace),
				APIVersion: req.APIVersion,
				Kind:       req.Kind,
			})
		fetchedResource.Metadata.FetchDuration = time.Since(startTime)
		return fetchedResource, nil
	}

	// Determine if the resource is namespaced
	var resource dynamic.ResourceInterface
	if req.Namespace != nil && *req.Namespace != "" {
		resource = e.dynamicClient.Resource(gvr).Namespace(*req.Namespace)
	} else {
		resource = e.dynamicClient.Resource(gvr)
	}

	// Fetch the resource
	obj, err := resource.Get(fetchCtx, req.Name, metav1.GetOptions{})
	fetchedResource.Metadata.FetchDuration = time.Since(startTime)

	if err != nil {
		resourceRef := functionerrors.ResourceRef{
			Into:       req.Into,
			Name:       req.Name,
			Namespace:  stringPtrValue(req.Namespace),
			APIVersion: req.APIVersion,
			Kind:       req.Kind,
		}

		if errors.IsNotFound(err) {
			fetchedResource.Metadata.FetchStatus = FetchStatusNotFound
			fetchedResource.Metadata.ResourceExists = false
			fetchedResource.Metadata.Error = functionerrors.ResourceNotFoundError(resourceRef)
		} else if errors.IsForbidden(err) {
			fetchedResource.Metadata.FetchStatus = FetchStatusForbidden
			fetchedResource.Metadata.Error = functionerrors.ResourceForbiddenError(resourceRef)
		} else if errors.IsTimeout(err) || fetchCtx.Err() == context.DeadlineExceeded {
			fetchedResource.Metadata.FetchStatus = FetchStatusTimeout
			fetchedResource.Metadata.Error = functionerrors.ResourceTimeoutError(resourceRef, e.timeout)
		} else {
			fetchedResource.Metadata.FetchStatus = FetchStatusError
			fetchedResource.Metadata.Error = functionerrors.New(
				functionerrors.ErrorCodeKubernetesClient, 
				fmt.Sprintf("failed to fetch resource: %v", err)).
				WithResource(resourceRef)
		}

		return fetchedResource, nil
	}

	// Success case
	fetchedResource.Resource = obj
	fetchedResource.Metadata.FetchStatus = FetchStatusSuccess
	fetchedResource.Metadata.ResourceExists = true

	return fetchedResource, nil
}

// getGVR converts apiVersion and kind to GroupVersionResource
func (e *KubernetesEngine) getGVR(apiVersion, kind string) (schema.GroupVersionResource, error) {
	// First check registry for plural form
	if rt, err := e.registry.GetResourceType(apiVersion, kind); err == nil && rt.Plural != "" {
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
	plural := e.pluralize(kind)
	
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
func (e *KubernetesEngine) pluralize(kind string) string {
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