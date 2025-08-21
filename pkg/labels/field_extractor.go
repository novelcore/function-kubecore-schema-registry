package labels

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/crossplane/function-sdk-go/logging"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// FieldExtractor handles extraction of values from XR fields
type FieldExtractor struct {
	log logging.Logger
}

// NewFieldExtractor creates a new field extractor
func NewFieldExtractor(log logging.Logger) *FieldExtractor {
	return &FieldExtractor{
		log: log,
	}
}

// ExtractFromXR extracts a value from an XR using a JSONPath-like syntax
func (e *FieldExtractor) ExtractFromXR(obj map[string]interface{}, path string) (string, error) {
	if path == "" {
		return "", errors.ValidationError("field path cannot be empty")
	}

	// Split path into components
	pathComponents := strings.Split(path, ".")
	
	// Navigate through the object
	current := obj
	for i, component := range pathComponents {
		// Handle array indexing
		if strings.Contains(component, "[") && strings.Contains(component, "]") {
			arrayField, index, err := e.parseArrayAccess(component)
			if err != nil {
				return "", errors.Wrapf(err, "invalid array access in path component '%s'", component)
			}
			
			// Get array field
			if field, exists := current[arrayField]; exists {
				arrayValue, ok := field.([]interface{})
				if !ok {
					return "", errors.ValidationError(fmt.Sprintf("field '%s' is not an array", arrayField))
				}
				
				if index < 0 || index >= len(arrayValue) {
					return "", errors.ValidationError(fmt.Sprintf("array index %d out of bounds for field '%s'", index, arrayField))
				}
				
				// Continue with array element
				if mapValue, ok := arrayValue[index].(map[string]interface{}); ok {
					current = mapValue
				} else {
					// If this is the last component, return the array element as string
					if i == len(pathComponents)-1 {
						return e.convertToString(arrayValue[index])
					}
					return "", errors.ValidationError(fmt.Sprintf("array element at index %d is not an object", index))
				}
			} else {
				return "", errors.ValidationError(fmt.Sprintf("field '%s' not found", arrayField))
			}
		} else {
			// Regular field access
			if field, exists := current[component]; exists {
				// If this is the last component, convert to string
				if i == len(pathComponents)-1 {
					return e.convertToString(field)
				}
				
				// Continue navigating
				if mapValue, ok := field.(map[string]interface{}); ok {
					current = mapValue
				} else {
					return "", errors.ValidationError(fmt.Sprintf("field '%s' is not an object", component))
				}
			} else {
				return "", errors.ValidationError(fmt.Sprintf("field '%s' not found", component))
			}
		}
	}
	
	return "", errors.ValidationError("unexpected end of path navigation")
}

// parseArrayAccess parses array access syntax like "field[0]"
func (e *FieldExtractor) parseArrayAccess(component string) (string, int, error) {
	openBracket := strings.Index(component, "[")
	closeBracket := strings.Index(component, "]")
	
	if openBracket == -1 || closeBracket == -1 || closeBracket <= openBracket {
		return "", 0, errors.ValidationError("invalid array access syntax")
	}
	
	fieldName := component[:openBracket]
	indexStr := component[openBracket+1 : closeBracket]
	
	if fieldName == "" {
		return "", 0, errors.ValidationError("array field name cannot be empty")
	}
	
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid array index '%s'", indexStr)
	}
	
	if index < 0 {
		return "", 0, errors.ValidationError("negative array indices not supported")
	}
	
	return fieldName, index, nil
}

// convertToString converts various types to their string representation
func (e *FieldExtractor) convertToString(value interface{}) (string, error) {
	if value == nil {
		return "", errors.ValidationError("cannot convert nil to string")
	}
	
	switch v := value.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int32:
		return strconv.FormatInt(int64(v), 10), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(v), nil
	default:
		// Handle complex types using reflection
		rv := reflect.ValueOf(value)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array:
			return "", errors.ValidationError("cannot convert array/slice to string (use array indexing)")
		case reflect.Map:
			return "", errors.ValidationError("cannot convert object to string (specify field path)")
		default:
			// Fallback to string representation
			return fmt.Sprintf("%v", value), nil
		}
	}
}

// ValidatePath validates a field path for syntax correctness
func (e *FieldExtractor) ValidatePath(path string) error {
	if path == "" {
		return errors.ValidationError("field path cannot be empty")
	}
	
	pathComponents := strings.Split(path, ".")
	for _, component := range pathComponents {
		if component == "" {
			return errors.ValidationError("path cannot contain empty components")
		}
		
		// Validate array access syntax if present
		if strings.Contains(component, "[") || strings.Contains(component, "]") {
			_, _, err := e.parseArrayAccess(component)
			if err != nil {
				return errors.Wrapf(err, "invalid path component '%s'", component)
			}
		}
	}
	
	return nil
}

// ExtractMultiple extracts multiple values using different paths
func (e *FieldExtractor) ExtractMultiple(obj map[string]interface{}, paths map[string]string) (map[string]string, error) {
	results := make(map[string]string)
	var lastError error
	
	for key, path := range paths {
		value, err := e.ExtractFromXR(obj, path)
		if err != nil {
			e.log.Debug("Failed to extract field",
				"key", key,
				"path", path,
				"error", err.Error())
			lastError = err
			continue
		}
		results[key] = value
	}
	
	// If no values were extracted and we have errors, return the last error
	if len(results) == 0 && lastError != nil {
		return nil, errors.Wrapf(lastError, "failed to extract any values")
	}
	
	return results, nil
}