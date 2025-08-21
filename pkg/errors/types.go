package errors

import (
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// ErrorCode represents the type of error that occurred
type ErrorCode string

const (
	// Resource fetch errors
	ErrorCodeResourceNotFound    ErrorCode = "RESOURCE_NOT_FOUND"
	ErrorCodeResourceForbidden   ErrorCode = "RESOURCE_FORBIDDEN"
	ErrorCodeResourceTimeout     ErrorCode = "RESOURCE_TIMEOUT"
	ErrorCodeResourceUnavailable ErrorCode = "RESOURCE_UNAVAILABLE"

	// Input validation errors
	ErrorCodeInvalidInput       ErrorCode = "INVALID_INPUT"
	ErrorCodeInvalidResourceRef ErrorCode = "INVALID_RESOURCE_REF"

	// System errors
	ErrorCodeKubernetesClient ErrorCode = "KUBERNETES_CLIENT_ERROR"
	ErrorCodeInternalError    ErrorCode = "INTERNAL_ERROR"
	ErrorCodeTimeout          ErrorCode = "TIMEOUT"

	// Phase 2 specific errors
	ErrorCodeInvalidSelector      ErrorCode = "INVALID_SELECTOR"
	ErrorCodeInvalidExpression    ErrorCode = "INVALID_EXPRESSION"
	ErrorCodeConstraintViolation  ErrorCode = "CONSTRAINT_VIOLATION"
	ErrorCodeUnsupportedMatchType ErrorCode = "UNSUPPORTED_MATCH_TYPE"
	ErrorCodeQueryOptimization    ErrorCode = "QUERY_OPTIMIZATION_ERROR"
	ErrorCodeSelectorCompilation  ErrorCode = "SELECTOR_COMPILATION_ERROR"
)

// FunctionError represents a comprehensive error with context
type FunctionError struct {
	Code        ErrorCode         `json:"code"`
	Message     string            `json:"message"`
	ResourceRef *ResourceRef      `json:"resourceRef,omitempty"`
	Context     map[string]string `json:"context,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Cause       error             `json:"-"`
}

// ResourceRef identifies a specific resource
type ResourceRef struct {
	Into       string `json:"into"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

// Error implements the error interface
func (e *FunctionError) Error() string {
	var parts []string

	if e.ResourceRef != nil {
		if e.ResourceRef.Namespace != "" {
			parts = append(parts, fmt.Sprintf("resource %s/%s/%s (%s)",
				e.ResourceRef.Kind, e.ResourceRef.Namespace, e.ResourceRef.Name, e.ResourceRef.Into))
		} else {
			parts = append(parts, fmt.Sprintf("resource %s/%s (%s)",
				e.ResourceRef.Kind, e.ResourceRef.Name, e.ResourceRef.Into))
		}
	}

	parts = append(parts, string(e.Code))
	parts = append(parts, e.Message)

	if e.Cause != nil {
		parts = append(parts, fmt.Sprintf("cause: %s", e.Cause.Error()))
	}

	return strings.Join(parts, ": ")
}

// Unwrap returns the underlying cause
func (e *FunctionError) Unwrap() error {
	return e.Cause
}

// New creates a new FunctionError
func New(code ErrorCode, message string) *FunctionError {
	return &FunctionError{
		Code:      code,
		Message:   message,
		Timestamp: time.Now(),
		Context:   make(map[string]string),
	}
}

// Wrap creates a FunctionError wrapping another error
func Wrap(err error, message string) error {
	if err == nil {
		return nil
	}

	// If it's already a FunctionError, wrap it
	if fe, ok := err.(*FunctionError); ok {
		return &FunctionError{
			Code:      fe.Code,
			Message:   message,
			Timestamp: time.Now(),
			Context:   make(map[string]string),
			Cause:     fe,
		}
	}

	// Use standard errors.Wrap for non-FunctionErrors
	return errors.Wrap(err, message)
}

// Wrapf creates a FunctionError wrapping another error with formatting
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return Wrap(err, fmt.Sprintf(format, args...))
}

// WithResource adds resource context to an error
func (e *FunctionError) WithResource(ref ResourceRef) *FunctionError {
	e.ResourceRef = &ref
	return e
}

// WithContext adds additional context
func (e *FunctionError) WithContext(key, value string) *FunctionError {
	if e.Context == nil {
		e.Context = make(map[string]string)
	}
	e.Context[key] = value
	return e
}

// IsErrorCode checks if an error has a specific error code
func IsErrorCode(err error, code ErrorCode) bool {
	if fe, ok := err.(*FunctionError); ok {
		return fe.Code == code
	}
	return false
}

// GetErrorCode extracts the error code from an error
func GetErrorCode(err error) ErrorCode {
	if fe, ok := err.(*FunctionError); ok {
		return fe.Code
	}
	return ErrorCodeInternalError
}

// ValidationError creates a validation error
func ValidationError(message string) *FunctionError {
	return New(ErrorCodeInvalidInput, message)
}

// ResourceNotFoundError creates a resource not found error
func ResourceNotFoundError(ref ResourceRef) *FunctionError {
	return New(ErrorCodeResourceNotFound, "resource not found").WithResource(ref)
}

// ResourceForbiddenError creates a resource forbidden error
func ResourceForbiddenError(ref ResourceRef) *FunctionError {
	return New(ErrorCodeResourceForbidden, "access forbidden").WithResource(ref)
}

// ResourceTimeoutError creates a resource timeout error
func ResourceTimeoutError(ref ResourceRef, timeout time.Duration) *FunctionError {
	return New(ErrorCodeResourceTimeout, fmt.Sprintf("timeout after %s", timeout)).
		WithResource(ref).
		WithContext("timeout", timeout.String())
}

// KubernetesClientError creates a Kubernetes client error
func KubernetesClientError(message string) *FunctionError {
	return New(ErrorCodeKubernetesClient, message)
}

// Phase 2 Error Constructors

// InvalidSelectorError creates an invalid selector error
func InvalidSelectorError(message string) *FunctionError {
	return New(ErrorCodeInvalidSelector, message)
}

// InvalidExpressionError creates an invalid expression error
func InvalidExpressionError(message string) *FunctionError {
	return New(ErrorCodeInvalidExpression, message)
}

// ConstraintViolationError creates a constraint violation error
func ConstraintViolationError(message string) *FunctionError {
	return New(ErrorCodeConstraintViolation, message)
}

// UnsupportedMatchTypeError creates an unsupported match type error
func UnsupportedMatchTypeError(matchType string) *FunctionError {
	return New(ErrorCodeUnsupportedMatchType, fmt.Sprintf("unsupported match type: %s", matchType))
}

// QueryOptimizationError creates a query optimization error
func QueryOptimizationError(message string) *FunctionError {
	return New(ErrorCodeQueryOptimization, message)
}

// SelectorCompilationError creates a selector compilation error
func SelectorCompilationError(message string) *FunctionError {
	return New(ErrorCodeSelectorCompilation, message)
}
