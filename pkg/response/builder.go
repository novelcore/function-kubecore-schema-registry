package response

import (
	"encoding/json"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/discovery"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// Builder provides methods to build structured responses for Go templates
type Builder interface {
	// BuildContext creates the context data structure for templates
	BuildContext(fetchResult *discovery.FetchResult) (map[string]interface{}, error)
	
	// SetContext sets the context in the Crossplane response
	SetContext(rsp *fnv1.RunFunctionResponse, fetchResult *discovery.FetchResult) error
}

// DefaultBuilder implements the Builder interface
type DefaultBuilder struct{}

// NewDefaultBuilder creates a new default response builder
func NewDefaultBuilder() *DefaultBuilder {
	return &DefaultBuilder{}
}

// BuildContext creates the context data structure for templates
func (b *DefaultBuilder) BuildContext(fetchResult *discovery.FetchResult) (map[string]interface{}, error) {
	if fetchResult == nil {
		return nil, errors.ValidationError("fetchResult cannot be nil")
	}

	context := make(map[string]interface{})

	// Add each resource to the context by its 'into' field name
	for into, fetchedResource := range fetchResult.Resources {
		resourceData := make(map[string]interface{})

		if fetchedResource.Resource != nil {
			// Extract standard Kubernetes fields
			resourceData["apiVersion"] = fetchedResource.Resource.GetAPIVersion()
			resourceData["kind"] = fetchedResource.Resource.GetKind()
			resourceData["metadata"] = fetchedResource.Resource.Object["metadata"]
			
			// Add spec and status if they exist
			if spec, found := fetchedResource.Resource.Object["spec"]; found {
				resourceData["spec"] = spec
			}
			if status, found := fetchedResource.Resource.Object["status"]; found {
				resourceData["status"] = status
			}
		}

		// Add KubeCore metadata
		resourceData["_kubecore"] = map[string]interface{}{
			"fetchStatus":    string(fetchedResource.Metadata.FetchStatus),
			"fetchDuration":  fetchedResource.Metadata.FetchDuration.Milliseconds(),
			"resourceExists": fetchedResource.Metadata.ResourceExists,
			"fetchedAt":      fetchedResource.FetchedAt.Format(time.RFC3339),
		}

		// Add error information if present
		if fetchedResource.Metadata.Error != nil {
			resourceData["_kubecore"].(map[string]interface{})["error"] = map[string]interface{}{
				"code":    string(fetchedResource.Metadata.Error.Code),
				"message": fetchedResource.Metadata.Error.Message,
			}
		}

		// Add permissions if available
		if fetchedResource.Metadata.Permissions != nil {
			resourceData["_kubecore"].(map[string]interface{})["permissions"] = map[string]interface{}{
				"canGet":    fetchedResource.Metadata.Permissions.CanGet,
				"canList":   fetchedResource.Metadata.Permissions.CanList,
				"canWatch":  fetchedResource.Metadata.Permissions.CanWatch,
				"canCreate": fetchedResource.Metadata.Permissions.CanCreate,
				"canUpdate": fetchedResource.Metadata.Permissions.CanUpdate,
				"canDelete": fetchedResource.Metadata.Permissions.CanDelete,
			}
		}

		context[into] = resourceData
	}

	// Add fetch summary
	context["fetchSummary"] = map[string]interface{}{
		"totalRequested":   fetchResult.Summary.TotalRequested,
		"successful":       fetchResult.Summary.Successful,
		"failed":           fetchResult.Summary.Failed,
		"skipped":          fetchResult.Summary.Skipped,
		"notFound":         fetchResult.Summary.NotFound,
		"forbidden":        fetchResult.Summary.Forbidden,
		"timeout":          fetchResult.Summary.Timeout,
		"totalDuration":    fetchResult.Summary.TotalDuration.Milliseconds(),
		"averageDuration":  fetchResult.Summary.AverageDuration.Milliseconds(),
		"errors":           b.buildErrorSummary(fetchResult.Summary.Errors),
	}

	return context, nil
}

// SetContext sets the context in the Crossplane response
func (b *DefaultBuilder) SetContext(rsp *fnv1.RunFunctionResponse, fetchResult *discovery.FetchResult) error {
	context, err := b.BuildContext(fetchResult)
	if err != nil {
		return errors.Wrap(err, "failed to build context")
	}

	// Convert to JSON and back for clean marshaling
	contextJSON, err := json.Marshal(context)
	if err != nil {
		return errors.Wrap(err, "failed to marshal context to JSON")
	}

	var contextMap map[string]interface{}
	if err := json.Unmarshal(contextJSON, &contextMap); err != nil {
		return errors.Wrap(err, "failed to unmarshal context from JSON")
	}

	// Set the main context
	if contextStruct, err := structpb.NewStruct(contextMap); err == nil {
		response.SetContextKey(rsp, "schemaRegistryResults", structpb.NewStructValue(contextStruct))
	} else {
		return errors.Wrap(err, "failed to create structured context")
	}

	// Also set individual resource contexts for direct access
	for into, fetchedResource := range fetchResult.Resources {
		resourceContext := b.buildResourceContext(fetchedResource)
		if resourceJSON, err := json.Marshal(resourceContext); err == nil {
			var resourceMap map[string]interface{}
			if err := json.Unmarshal(resourceJSON, &resourceMap); err == nil {
				if resourceStruct, err := structpb.NewStruct(resourceMap); err == nil {
					response.SetContextKey(rsp, "resource_"+into, structpb.NewStructValue(resourceStruct))
				}
			}
		}
	}

	return nil
}

// buildResourceContext creates a context structure for a single resource
func (b *DefaultBuilder) buildResourceContext(fetchedResource *discovery.FetchedResource) map[string]interface{} {
	context := make(map[string]interface{})

	if fetchedResource.Resource != nil {
		context["apiVersion"] = fetchedResource.Resource.GetAPIVersion()
		context["kind"] = fetchedResource.Resource.GetKind()
		context["metadata"] = fetchedResource.Resource.Object["metadata"]
		
		if spec, found := fetchedResource.Resource.Object["spec"]; found {
			context["spec"] = spec
		}
		if status, found := fetchedResource.Resource.Object["status"]; found {
			context["status"] = status
		}
	}

	context["_kubecore"] = map[string]interface{}{
		"fetchStatus":    string(fetchedResource.Metadata.FetchStatus),
		"fetchDuration":  fetchedResource.Metadata.FetchDuration.Milliseconds(),
		"resourceExists": fetchedResource.Metadata.ResourceExists,
		"fetchedAt":      fetchedResource.FetchedAt.Format(time.RFC3339),
	}

	if fetchedResource.Metadata.Error != nil {
		context["_kubecore"].(map[string]interface{})["error"] = map[string]interface{}{
			"code":    string(fetchedResource.Metadata.Error.Code),
			"message": fetchedResource.Metadata.Error.Message,
		}
	}

	return context
}

// buildErrorSummary creates a summary of errors for the context
func (b *DefaultBuilder) buildErrorSummary(fetchErrors []*discovery.FetchError) []map[string]interface{} {
	var errors []map[string]interface{}
	
	for _, fetchError := range fetchErrors {
		errorSummary := map[string]interface{}{
			"into":      fetchError.ResourceRequest.Into,
			"name":      fetchError.ResourceRequest.Name,
			"kind":      fetchError.ResourceRequest.Kind,
			"code":      string(fetchError.Error.Code),
			"message":   fetchError.Error.Message,
			"timestamp": fetchError.Timestamp.Format(time.RFC3339),
		}
		
		if fetchError.ResourceRequest.Namespace != nil {
			errorSummary["namespace"] = *fetchError.ResourceRequest.Namespace
		}
		
		errors = append(errors, errorSummary)
	}
	
	return errors
}

// TemplateHelpers provides helper functions for Go templates
type TemplateHelpers struct{}

// NewTemplateHelpers creates template helper functions
func NewTemplateHelpers() *TemplateHelpers {
	return &TemplateHelpers{}
}

// HasResource checks if a resource exists and was successfully fetched
func (h *TemplateHelpers) HasResource(context map[string]interface{}, into string) bool {
	resource, exists := context[into]
	if !exists {
		return false
	}

	resourceMap, ok := resource.(map[string]interface{})
	if !ok {
		return false
	}

	kubecore, exists := resourceMap["_kubecore"]
	if !exists {
		return false
	}

	kubecoreMap, ok := kubecore.(map[string]interface{})
	if !ok {
		return false
	}

	fetchStatus, exists := kubecoreMap["fetchStatus"]
	if !exists {
		return false
	}

	status, ok := fetchStatus.(string)
	if !ok {
		return false
	}

	return status == string(discovery.FetchStatusSuccess)
}

// GetResourceField safely gets a field from a resource
func (h *TemplateHelpers) GetResourceField(context map[string]interface{}, into string, fieldPath ...string) interface{} {
	resource, exists := context[into]
	if !exists {
		return nil
	}

	current := resource
	for _, field := range fieldPath {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}

		current, exists = currentMap[field]
		if !exists {
			return nil
		}
	}

	return current
}

// IsSuccessfulFetch checks if a resource was successfully fetched
func (h *TemplateHelpers) IsSuccessfulFetch(context map[string]interface{}) bool {
	fetchSummary, exists := context["fetchSummary"]
	if !exists {
		return false
	}

	summaryMap, ok := fetchSummary.(map[string]interface{})
	if !ok {
		return false
	}

	failed, exists := summaryMap["failed"]
	if !exists {
		return false
	}

	failedCount, ok := failed.(int)
	if !ok {
		return false
	}

	return failedCount == 0
}