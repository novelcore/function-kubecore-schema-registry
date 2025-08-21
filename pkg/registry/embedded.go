package registry

import (
	"fmt"
	"sync"

	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// EmbeddedRegistry implements the Registry interface with embedded resource type definitions
type EmbeddedRegistry struct {
	resourceTypes map[string]*ResourceType // key: "apiVersion/kind"
	mu            sync.RWMutex
}

// NewEmbeddedRegistry creates a new embedded registry with predefined resource types
func NewEmbeddedRegistry() *EmbeddedRegistry {
	r := &EmbeddedRegistry{
		resourceTypes: make(map[string]*ResourceType),
	}

	r.loadBuiltinTypes()
	return r
}

// GetResourceType returns metadata for a given resource type
func (r *EmbeddedRegistry) GetResourceType(apiVersion, kind string) (*ResourceType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("%s/%s", apiVersion, kind)
	rt, exists := r.resourceTypes[key]
	if !exists {
		return nil, errors.New(errors.ErrorCodeResourceNotFound,
			fmt.Sprintf("resource type %s not found in registry", key))
	}

	return rt, nil
}

// ListResourceTypes returns all registered resource types
func (r *EmbeddedRegistry) ListResourceTypes() ([]*ResourceType, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]*ResourceType, 0, len(r.resourceTypes))
	for _, rt := range r.resourceTypes {
		types = append(types, rt)
	}

	return types, nil
}

// IsNamespaced returns whether a resource type is namespaced
func (r *EmbeddedRegistry) IsNamespaced(apiVersion, kind string) (bool, error) {
	rt, err := r.GetResourceType(apiVersion, kind)
	if err != nil {
		return false, err
	}
	return rt.Namespaced, nil
}

// GetReferences returns all reference relationships for a resource type
func (r *EmbeddedRegistry) GetReferences(apiVersion, kind string) ([]ResourceReference, error) {
	rt, err := r.GetResourceType(apiVersion, kind)
	if err != nil {
		return nil, err
	}

	var references []ResourceReference
	for _, field := range rt.Fields {
		references = append(references, field.References...)
	}

	return references, nil
}

// RegisterType adds a new resource type to the registry
func (r *EmbeddedRegistry) RegisterType(rt *ResourceType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := fmt.Sprintf("%s/%s", rt.APIVersion, rt.Kind)
	r.resourceTypes[key] = rt
}

// loadBuiltinTypes loads the predefined Kubernetes and KubeCore resource types
func (r *EmbeddedRegistry) loadBuiltinTypes() {
	// Core Kubernetes types
	r.loadCoreKubernetesTypes()

	// KubeCore platform types
	r.loadKubeCoreTypes()

	// GitHub platform types
	r.loadGitHubPlatformTypes()
}

// loadCoreKubernetesTypes loads standard Kubernetes resource types
func (r *EmbeddedRegistry) loadCoreKubernetesTypes() {
	// Pod
	r.RegisterType(&ResourceType{
		APIVersion: "v1",
		Kind:       "Pod",
		Namespaced: true,
		Group:      "",
		Version:    "v1",
		Plural:     "pods",
		Singular:   "pod",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"serviceAccountName": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.serviceAccountName",
								TargetKind:  "ServiceAccount",
								TargetGroup: "",
								RefType:     RefTypeCustom,
							},
						},
					},
					"volumes": {
						Type: "array",
						Items: &FieldSchema{
							Type: "object",
							Properties: map[string]FieldSchema{
								"configMap": {
									Type: "object",
									Properties: map[string]FieldSchema{
										"name": {
											Type: "string",
											References: []ResourceReference{
												{
													FieldPath:   "$.spec.volumes[*].configMap.name",
													TargetKind:  "ConfigMap",
													TargetGroup: "",
													RefType:     RefTypeConfigMap,
												},
											},
										},
									},
								},
								"secret": {
									Type: "object",
									Properties: map[string]FieldSchema{
										"secretName": {
											Type: "string",
											References: []ResourceReference{
												{
													FieldPath:   "$.spec.volumes[*].secret.secretName",
													TargetKind:  "Secret",
													TargetGroup: "",
													RefType:     RefTypeSecret,
												},
											},
										},
									},
								},
								"persistentVolumeClaim": {
									Type: "object",
									Properties: map[string]FieldSchema{
										"claimName": {
											Type: "string",
											References: []ResourceReference{
												{
													FieldPath:   "$.spec.volumes[*].persistentVolumeClaim.claimName",
													TargetKind:  "PersistentVolumeClaim",
													TargetGroup: "",
													RefType:     RefTypePVC,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})

	// Service
	r.RegisterType(&ResourceType{
		APIVersion: "v1",
		Kind:       "Service",
		Namespaced: true,
		Group:      "",
		Version:    "v1",
		Plural:     "services",
		Singular:   "service",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"selector": {
						Type:        "object",
						Description: "Label selector for pods",
					},
				},
			},
		},
	})

	// ConfigMap
	r.RegisterType(&ResourceType{
		APIVersion: "v1",
		Kind:       "ConfigMap",
		Namespaced: true,
		Group:      "",
		Version:    "v1",
		Plural:     "configmaps",
		Singular:   "configmap",
		Fields: map[string]FieldSchema{
			"data": {
				Type:        "object",
				Description: "Configuration data",
			},
		},
	})

	// Secret
	r.RegisterType(&ResourceType{
		APIVersion: "v1",
		Kind:       "Secret",
		Namespaced: true,
		Group:      "",
		Version:    "v1",
		Plural:     "secrets",
		Singular:   "secret",
		Fields: map[string]FieldSchema{
			"data": {
				Type:        "object",
				Description: "Secret data",
			},
		},
	})

	// PersistentVolumeClaim
	r.RegisterType(&ResourceType{
		APIVersion: "v1",
		Kind:       "PersistentVolumeClaim",
		Namespaced: true,
		Group:      "",
		Version:    "v1",
		Plural:     "persistentvolumeclaims",
		Singular:   "persistentvolumeclaim",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"storageClassName": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.storageClassName",
								TargetKind:  "StorageClass",
								TargetGroup: "storage.k8s.io",
								RefType:     RefTypeCustom,
							},
						},
					},
				},
			},
		},
	})

	// Deployment
	r.RegisterType(&ResourceType{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Namespaced: true,
		Group:      "apps",
		Version:    "v1",
		Plural:     "deployments",
		Singular:   "deployment",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"template": {
						Type:        "object",
						Description: "Pod template",
					},
				},
			},
		},
	})
}

// loadKubeCoreTypes loads KubeCore platform resource types
func (r *EmbeddedRegistry) loadKubeCoreTypes() {
	// KubEnv
	r.RegisterType(&ResourceType{
		APIVersion: "platform.kubecore.io/v1alpha1",
		Kind:       "KubEnv",
		Namespaced: true,
		Group:      "platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "kubenvs",
		Singular:   "kubenv",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"kubeCluster": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.kubeCluster",
								TargetKind:  "KubeCluster",
								TargetGroup: "platform.kubecore.io",
								RefType:     RefTypeCustom,
							},
						},
					},
				},
			},
		},
	})

	// KubeCluster
	r.RegisterType(&ResourceType{
		APIVersion: "platform.kubecore.io/v1alpha1",
		Kind:       "KubeCluster",
		Namespaced: false,
		Group:      "platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "kubeclusters",
		Singular:   "kubecluster",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"region": {
						Type: "string",
					},
				},
			},
		},
	})

	// KubeApp
	r.RegisterType(&ResourceType{
		APIVersion: "platform.kubecore.io/v1alpha1",
		Kind:       "KubeApp",
		Namespaced: true,
		Group:      "platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "kubeapps",
		Singular:   "kubeapp",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"kubEnv": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.kubEnv",
								TargetKind:  "KubEnv",
								TargetGroup: "platform.kubecore.io",
								RefType:     RefTypeCustom,
							},
						},
					},
				},
			},
		},
	})
}

// loadGitHubPlatformTypes loads GitHub platform resource types
func (r *EmbeddedRegistry) loadGitHubPlatformTypes() {
	// GitHubProject
	r.RegisterType(&ResourceType{
		APIVersion: "github.platform.kubecore.io/v1alpha1",
		Kind:       "GitHubProject",
		Namespaced: true,
		Group:      "github.platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "githubprojects",
		Singular:   "githubproject",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"name": {
						Type: "string",
					},
					"description": {
						Type: "string",
					},
					"visibility": {
						Type: "string",
					},
					"githubProviderRef": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"name": {
								Type: "string",
								References: []ResourceReference{
									{
										FieldPath:   "$.spec.githubProviderRef.name",
										TargetKind:  "GithubProvider",
										TargetGroup: "github.platform.kubecore.io",
										RefType:     RefTypeCustom,
									},
								},
							},
						},
					},
					"repositorySource": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"type": {
								Type: "string",
							},
							"template": {
								Type: "object",
								Properties: map[string]FieldSchema{
									"owner": {
										Type: "string",
									},
									"repository": {
										Type: "string",
									},
								},
							},
						},
					},
				},
			},
			"status": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"conditions": {
						Type: "array",
						Items: &FieldSchema{
							Type: "object",
							Properties: map[string]FieldSchema{
								"type": {
									Type: "string",
								},
								"status": {
									Type: "string",
								},
								"reason": {
									Type: "string",
								},
								"message": {
									Type: "string",
								},
							},
						},
					},
					"repositoryUrl": {
						Type: "string",
					},
				},
			},
		},
	})

	// GitHubInfra
	r.RegisterType(&ResourceType{
		APIVersion: "github.platform.kubecore.io/v1alpha1",
		Kind:       "GitHubInfra",
		Namespaced: false,
		Group:      "github.platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "githubinfras",
		Singular:   "githubinfra",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"project": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.project",
								TargetKind:  "GitHubProject",
								TargetGroup: "github.platform.kubecore.io",
								RefType:     RefTypeCustom,
							},
						},
					},
				},
			},
		},
	})

	// GitHubSystem
	r.RegisterType(&ResourceType{
		APIVersion: "github.platform.kubecore.io/v1alpha1",
		Kind:       "GitHubSystem",
		Namespaced: false,
		Group:      "github.platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "githubsystems",
		Singular:   "githubsystem",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"infra": {
						Type: "string",
						References: []ResourceReference{
							{
								FieldPath:   "$.spec.infra",
								TargetKind:  "GitHubInfra",
								TargetGroup: "github.platform.kubecore.io",
								RefType:     RefTypeCustom,
							},
						},
					},
				},
			},
		},
	})

	// GithubProvider
	r.RegisterType(&ResourceType{
		APIVersion: "github.platform.kubecore.io/v1alpha1",
		Kind:       "GithubProvider",
		Namespaced: true,
		Group:      "github.platform.kubecore.io",
		Version:    "v1alpha1",
		Plural:     "githubproviders",
		Singular:   "githubprovider",
		Fields: map[string]FieldSchema{
			"spec": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"secretStoreRef": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"kind": {
								Type: "string",
							},
							"name": {
								Type: "string",
							},
						},
					},
					"aws": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"secretId": {
								Type: "string",
							},
						},
					},
					"secret": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"name": {
								Type: "string",
							},
							"namespace": {
								Type: "string",
							},
						},
					},
					"github": {
						Type: "object",
						Properties: map[string]FieldSchema{
							"organization": {
								Type: "string",
							},
							"isEnterprise": {
								Type: "boolean",
							},
							"baseURL": {
								Type: "string",
							},
						},
					},
					"refreshInterval": {
						Type: "string",
					},
					"kubernetesProviderConfigRef": {
						Type: "string",
					},
				},
			},
			"status": {
				Type: "object",
				Properties: map[string]FieldSchema{
					"conditions": {
						Type: "array",
						Items: &FieldSchema{
							Type: "object",
							Properties: map[string]FieldSchema{
								"type": {
									Type: "string",
								},
								"status": {
									Type: "string",
								},
								"reason": {
									Type: "string",
								},
								"message": {
									Type: "string",
								},
							},
						},
					},
				},
			},
		},
	})
}
