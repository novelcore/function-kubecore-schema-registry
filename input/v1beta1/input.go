// Package v1beta1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=template.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// TODO: Add your input type here! It doesn't need to be called 'Input', you can
// rename it to anything you like.

// Input can be used to provide input to this Function.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// EnableTransitiveDiscovery enables discovery of transitive schema dependencies
	// +kubebuilder:default=true
	EnableTransitiveDiscovery *bool `json:"enableTransitiveDiscovery,omitempty"`

	// TraversalDepth specifies the maximum depth for transitive schema discovery
	// +kubebuilder:default=3
	TraversalDepth *int `json:"traversalDepth,omitempty"`

	// IncludeFullSchema includes complete OpenAPI schema in response
	// +kubebuilder:default=true
	IncludeFullSchema *bool `json:"includeFullSchema,omitempty"`
}
