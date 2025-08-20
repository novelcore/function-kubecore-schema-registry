package parser

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// XRParser extracts fetch requests from XR specs
type XRParser interface {
	// ParseFetchRequests extracts resource requests from the XR spec
	ParseFetchRequests(xr map[string]interface{}) ([]v1beta1.ResourceRequest, error)
}

// DefaultXRParser implements XRParser with basic extraction logic
type DefaultXRParser struct{}

// NewDefaultXRParser creates a new default XR parser
func NewDefaultXRParser() *DefaultXRParser {
	return &DefaultXRParser{}
}

// ParseFetchRequests extracts resource requests from the XR spec
// For Phase 1, this looks for embedded fetch requests in the XR spec
func (p *DefaultXRParser) ParseFetchRequests(xrObj map[string]interface{}) ([]v1beta1.ResourceRequest, error) {
	if xrObj == nil {
		return nil, errors.ValidationError("XR object cannot be nil")
	}

	// Look for fetchResources in spec.fetchResources
	fetchResourcesPath := []string{"spec", "fetchResources"}
	fetchResourcesRaw, found, err := unstructured.NestedFieldCopy(xrObj, fetchResourcesPath...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to extract fetchResources from XR spec")
	}

	if !found {
		// Return empty slice if no fetch requests found
		return []v1beta1.ResourceRequest{}, nil
	}

	// Convert to slice
	fetchResourcesList, ok := fetchResourcesRaw.([]interface{})
	if !ok {
		return nil, errors.ValidationError("spec.fetchResources must be an array")
	}

	var requests []v1beta1.ResourceRequest
	for i, item := range fetchResourcesList {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, errors.ValidationError(
				fmt.Sprintf("fetchResources[%d] must be an object", i))
		}

		// Marshal back to JSON and unmarshal to struct for proper type handling
		itemJSON, err := json.Marshal(itemMap)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to marshal fetchResources[%d]", i)
		}

		var request v1beta1.ResourceRequest
		if err := json.Unmarshal(itemJSON, &request); err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshal fetchResources[%d]", i)
		}

		// Validate required fields
		if err := p.validateRequest(request, i); err != nil {
			return nil, err
		}

		requests = append(requests, request)
	}

	return requests, nil
}

// validateRequest validates a single resource request
func (p *DefaultXRParser) validateRequest(request v1beta1.ResourceRequest, index int) error {
	if request.Into == "" {
		return errors.ValidationError(
			fmt.Sprintf("fetchResources[%d].into is required", index))
	}

	if request.Name == "" {
		return errors.ValidationError(
			fmt.Sprintf("fetchResources[%d].name is required", index))
	}

	if request.APIVersion == "" {
		return errors.ValidationError(
			fmt.Sprintf("fetchResources[%d].apiVersion is required", index))
	}

	if request.Kind == "" {
		return errors.ValidationError(
			fmt.Sprintf("fetchResources[%d].kind is required", index))
	}

	// Validate 'into' field naming convention
	if !isValidFieldName(request.Into) {
		return errors.ValidationError(
			fmt.Sprintf("fetchResources[%d].into '%s' must be a valid field name (alphanumeric + underscore, start with letter)", 
				index, request.Into))
	}

	return nil
}

// isValidFieldName checks if a string is a valid Go template field name
func isValidFieldName(name string) bool {
	if len(name) == 0 {
		return false
	}

	// Must start with letter or underscore
	first := name[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z') || first == '_') {
		return false
	}

	// Remaining characters must be alphanumeric or underscore
	for i := 1; i < len(name); i++ {
		c := name[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || 
			 (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}

	return true
}