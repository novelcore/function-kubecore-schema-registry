// Package v1beta1 contains the input type for Phase 1 of KubeCore Schema Registry Function
// +kubebuilder:object:generate=true
// +groupName=registry.fn.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Input defines the Phase 1 input schema for the KubeCore Schema Registry Function
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:categories=crossplane
type Input struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// FetchResources defines a list of direct resource references to fetch
	// Phase 1 supports only direct references (name/namespace)
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
}

// ResourceRequest defines a direct resource reference to fetch
type ResourceRequest struct {
	// Into specifies the field name where the resource will be stored in the response
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-zA-Z][a-zA-Z0-9_]*$"
	Into string `json:"into"`

	// Name is the name of the resource to fetch
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the resource (required for namespaced resources)
	Namespace *string `json:"namespace,omitempty"`

	// APIVersion of the resource (e.g., "v1", "apps/v1")
	// +kubebuilder:validation:Required
	APIVersion string `json:"apiVersion"`

	// Kind of the resource (e.g., "Pod", "Service", "GitHubProject")
	// +kubebuilder:validation:Required
	Kind string `json:"kind"`

	// Optional indicates whether the fetch should fail if the resource is not found
	// +kubebuilder:default=false
	Optional bool `json:"optional,omitempty"`
}