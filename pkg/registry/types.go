package registry

// ResourceType represents metadata about a Kubernetes resource type
type ResourceType struct {
	APIVersion string                 `json:"apiVersion"`
	Kind       string                 `json:"kind"`
	Namespaced bool                   `json:"namespaced"`
	Group      string                 `json:"group,omitempty"`
	Version    string                 `json:"version"`
	Plural     string                 `json:"plural,omitempty"`
	Singular   string                 `json:"singular,omitempty"`
	Categories []string               `json:"categories,omitempty"`
	Fields     map[string]FieldSchema `json:"fields,omitempty"`
}

// FieldSchema describes a field in a resource schema
type FieldSchema struct {
	Type        string                 `json:"type"`
	Description string                 `json:"description,omitempty"`
	Required    bool                   `json:"required,omitempty"`
	References  []ResourceReference    `json:"references,omitempty"`
	Properties  map[string]FieldSchema `json:"properties,omitempty"`
	Items       *FieldSchema           `json:"items,omitempty"` // For array types
}

// ResourceReference describes a reference to another resource
type ResourceReference struct {
	FieldPath    string `json:"fieldPath"`    // JSONPath to the field
	TargetKind   string `json:"targetKind"`   // Kind of referenced resource
	TargetGroup  string `json:"targetGroup"`  // Group of referenced resource
	RefType      RefType `json:"refType"`     // Type of reference
}

// RefType represents the type of reference relationship
type RefType string

const (
	RefTypeOwnerRef   RefType = "ownerRef"   // metadata.ownerReferences
	RefTypeConfigMap  RefType = "configMap"  // Reference to ConfigMap
	RefTypeSecret     RefType = "secret"     // Reference to Secret
	RefTypeService    RefType = "service"    // Reference to Service
	RefTypePVC        RefType = "pvc"        // Reference to PersistentVolumeClaim
	RefTypeCustom     RefType = "custom"     // Custom reference (platform-specific)
)

// Registry defines the interface for resource type registry
type Registry interface {
	// GetResourceType returns metadata for a given resource type
	GetResourceType(apiVersion, kind string) (*ResourceType, error)
	
	// ListResourceTypes returns all registered resource types
	ListResourceTypes() ([]*ResourceType, error)
	
	// IsNamespaced returns whether a resource type is namespaced
	IsNamespaced(apiVersion, kind string) (bool, error)
	
	// GetReferences returns all reference relationships for a resource type
	GetReferences(apiVersion, kind string) ([]ResourceReference, error)
}