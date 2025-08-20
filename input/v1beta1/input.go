// Package v1beta1 contains the input type for KubeCore Schema Registry Function (Phase 1 & 2)
// +kubebuilder:object:generate=true
// +groupName=registry.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Input defines the input schema for the KubeCore Schema Registry Function
// Supports both Phase 1 (direct) and Phase 2 (label/expression-based) discovery
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// FetchResources defines a list of resource references to fetch
	// Supports Phase 1 direct references and Phase 2 selector-based discovery
	// +kubebuilder:validation:Required
	FetchResources []ResourceRequest `json:"fetchResources"`

	// FetchTimeout specifies the maximum time to wait for each resource fetch
	// +kubebuilder:default="5s"
	// +kubebuilder:validation:Pattern="^[0-9]+(s|m|h)$"
	FetchTimeout *string `json:"fetchTimeout,omitempty"`

	// MaxConcurrentFetches limits the number of concurrent fetch operations
	// +kubebuilder:default=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	MaxConcurrentFetches *int `json:"maxConcurrentFetches,omitempty"`

	// Phase2Features enables Phase 2 capabilities (label/expression-based discovery)
	// +kubebuilder:default=false
	Phase2Features *bool `json:"phase2Features,omitempty"`
}

// ResourceRequest defines a resource reference for fetching
// Supports both direct references (Phase 1) and selector-based discovery (Phase 2)
type ResourceRequest struct {
	// Into specifies the field name where the resource(s) will be stored in the response
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-zA-Z][a-zA-Z0-9_]*$"
	Into string `json:"into"`

	// MatchType determines how resources are matched
	// +kubebuilder:validation:Enum=direct;label;expression
	// +kubebuilder:default="direct"
	MatchType MatchType `json:"matchType,omitempty"`

	// --- Phase 1 (Direct) Fields ---
	// Name is the name of the resource to fetch (required for direct match)
	Name string `json:"name,omitempty"`

	// Namespace is the namespace of the resource (required for namespaced resources in direct match)
	Namespace *string `json:"namespace,omitempty"`

	// --- Common Fields ---
	// APIVersion of the resource (e.g., "v1", "apps/v1")
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource (e.g., "Pod", "Service", "GitHubProject")
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Optional indicates whether the fetch should fail if the resource is not found
	// +kubebuilder:default=false
	Optional bool `json:"optional,omitempty"`

	// --- Phase 2 (Discovery) Fields ---
	// Selector defines label-based or expression-based resource selection (Phase 2)
	Selector *Selector `json:"selector,omitempty"`

	// Strategy defines the matching strategy for selector-based discovery
	Strategy *MatchStrategy `json:"strategy,omitempty"`
}

// MatchType defines how resources are matched
type MatchType string

const (
	// MatchTypeDirect matches resources by exact name (Phase 1)
	MatchTypeDirect MatchType = "direct"
	// MatchTypeLabel matches resources using label selectors (Phase 2)
	MatchTypeLabel MatchType = "label"
	// MatchTypeExpression matches resources using expression-based queries (Phase 2)
	MatchTypeExpression MatchType = "expression"
)

// Selector defines resource selection criteria for Phase 2 discovery
type Selector struct {
	// Labels defines label-based selection (for MatchTypeLabel)
	Labels *LabelSelector `json:"labels,omitempty"`

	// Expressions defines expression-based selection (for MatchTypeExpression)
	Expressions []Expression `json:"expressions,omitempty"`

	// Namespaces specifies which namespaces to search in
	// Empty list means cluster-wide search for cluster-scoped resources
	// For namespaced resources, defaults to function's namespace if empty
	Namespaces []string `json:"namespaces,omitempty"`

	// CrossNamespace enables cross-namespace discovery
	// +kubebuilder:default=false
	CrossNamespace *bool `json:"crossNamespace,omitempty"`
}

// LabelSelector defines label-based resource selection
type LabelSelector struct {
	// MatchLabels are key-value pairs for exact label matches
	MatchLabels map[string]string `json:"matchLabels,omitempty"`

	// MatchExpressions define more complex label selection criteria
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement defines a label selector requirement
type LabelSelectorRequirement struct {
	// Key is the label key
	// +kubebuilder:validation:Required
	Key string `json:"key"`

	// Operator defines the relationship between key and values
	// +kubebuilder:validation:Enum=In;NotIn;Exists;DoesNotExist
	// +kubebuilder:validation:Required
	Operator LabelSelectorOperator `json:"operator"`

	// Values is an array of string values for In and NotIn operations
	Values []string `json:"values,omitempty"`
}

// LabelSelectorOperator represents a label selector operator
type LabelSelectorOperator string

const (
	LabelSelectorOpIn           LabelSelectorOperator = "In"
	LabelSelectorOpNotIn        LabelSelectorOperator = "NotIn"
	LabelSelectorOpExists       LabelSelectorOperator = "Exists"
	LabelSelectorOpDoesNotExist LabelSelectorOperator = "DoesNotExist"
)

// Expression defines expression-based resource selection
type Expression struct {
	// Field specifies the resource field to evaluate (e.g., "metadata.labels", "spec.enabled")
	// +kubebuilder:validation:Required
	Field string `json:"field"`

	// Operator defines the comparison operation
	// +kubebuilder:validation:Enum=Equals;NotEquals;In;NotIn;Contains;StartsWith;EndsWith;Regex;Exists;NotExists
	// +kubebuilder:validation:Required
	Operator ExpressionOperator `json:"operator"`

	// Value is the value to compare against (not used for Exists/NotExists)
	Value *string `json:"value,omitempty"`

	// Values is an array of values for In/NotIn operations
	Values []string `json:"values,omitempty"`
}

// ExpressionOperator represents an expression operator
type ExpressionOperator string

const (
	ExpressionOpEquals     ExpressionOperator = "Equals"
	ExpressionOpNotEquals  ExpressionOperator = "NotEquals"
	ExpressionOpIn         ExpressionOperator = "In"
	ExpressionOpNotIn      ExpressionOperator = "NotIn"
	ExpressionOpContains   ExpressionOperator = "Contains"
	ExpressionOpStartsWith ExpressionOperator = "StartsWith"
	ExpressionOpEndsWith   ExpressionOperator = "EndsWith"
	ExpressionOpRegex      ExpressionOperator = "Regex"
	ExpressionOpExists     ExpressionOperator = "Exists"
	ExpressionOpNotExists  ExpressionOperator = "NotExists"
)

// MatchStrategy defines the strategy for matching resources
type MatchStrategy struct {
	// MinMatches specifies the minimum number of resources that must match
	// +kubebuilder:validation:Minimum=0
	MinMatches *int `json:"minMatches,omitempty"`

	// MaxMatches specifies the maximum number of resources to return
	// +kubebuilder:validation:Minimum=1
	MaxMatches *int `json:"maxMatches,omitempty"`

	// StopOnFirst stops searching after finding the first match
	// +kubebuilder:default=false
	StopOnFirst *bool `json:"stopOnFirst,omitempty"`

	// SortBy defines how to sort matched resources
	SortBy []SortCriteria `json:"sortBy,omitempty"`

	// FailOnConstraintViolation fails the request if min/max constraints are violated
	// +kubebuilder:default=false
	FailOnConstraintViolation *bool `json:"failOnConstraintViolation,omitempty"`
}

// SortCriteria defines sorting criteria for matched resources
type SortCriteria struct {
	// Field specifies the field to sort by (e.g., "metadata.name", "metadata.creationTimestamp")
	// +kubebuilder:validation:Required
	Field string `json:"field"`

	// Order specifies the sort order
	// +kubebuilder:validation:Enum=asc;desc
	// +kubebuilder:default="asc"
	Order SortOrder `json:"order,omitempty"`
}

// SortOrder represents the sort order
type SortOrder string

const (
	SortOrderAsc  SortOrder = "asc"
	SortOrderDesc SortOrder = "desc"
)