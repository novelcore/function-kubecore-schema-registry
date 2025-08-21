package graph

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// NodeID represents a unique identifier for a node in the resource graph
type NodeID string

// EdgeID represents a unique identifier for an edge in the resource graph
type EdgeID string

// RelationType defines the type of relationship between resources
type RelationType string

const (
	// RelationTypeOwnerRef represents an owner reference relationship
	RelationTypeOwnerRef RelationType = "ownerRef"
	// RelationTypeCustomRef represents a custom reference field relationship
	RelationTypeCustomRef RelationType = "customRef"
	// RelationTypeConfigMapRef represents a ConfigMap reference relationship
	RelationTypeConfigMapRef RelationType = "configMapRef"
	// RelationTypeSecretRef represents a Secret reference relationship
	RelationTypeSecretRef RelationType = "secretRef"
	// RelationTypeServiceRef represents a Service reference relationship
	RelationTypeServiceRef RelationType = "serviceRef"
	// RelationTypePVCRef represents a PersistentVolumeClaim reference relationship
	RelationTypePVCRef RelationType = "pvcRef"
)

// ResourceGraph represents a directed acyclic graph of Kubernetes resources
type ResourceGraph struct {
	// Nodes contains all resource nodes in the graph indexed by NodeID
	Nodes map[NodeID]*ResourceNode

	// Edges contains all relationships between resources indexed by EdgeID
	Edges map[EdgeID]*ResourceEdge

	// AdjacencyList provides efficient access to outbound relationships
	// Key is source NodeID, value is slice of EdgeIDs
	AdjacencyList map[NodeID][]EdgeID

	// ReverseAdjacencyList provides efficient access to inbound relationships
	// Key is target NodeID, value is slice of EdgeIDs
	ReverseAdjacencyList map[NodeID][]EdgeID

	// Metadata contains graph-level information
	Metadata *GraphMetadata
}

// ResourceNode represents a single resource in the graph
type ResourceNode struct {
	// ID is the unique identifier for this node
	ID NodeID

	// Resource is the actual Kubernetes resource
	Resource *unstructured.Unstructured

	// UID is the Kubernetes resource UID for deduplication
	UID types.UID

	// DiscoveredAt indicates when this resource was discovered
	DiscoveredAt time.Time

	// DiscoveryDepth indicates at which depth this resource was discovered
	DiscoveryDepth int

	// DiscoveryPath contains the path from root to this node
	DiscoveryPath []NodeID

	// Platform indicates if this resource belongs to the platform scope
	Platform bool

	// Metadata contains node-specific metadata
	Metadata *NodeMetadata
}

// ResourceEdge represents a relationship between two resources
type ResourceEdge struct {
	// ID is the unique identifier for this edge
	ID EdgeID

	// Source is the NodeID of the source resource
	Source NodeID

	// Target is the NodeID of the target resource
	Target NodeID

	// RelationType indicates the type of relationship
	RelationType RelationType

	// FieldPath is the path to the reference field in the source resource
	FieldPath string

	// FieldName is the name of the reference field
	FieldName string

	// Confidence indicates the confidence level of this relationship detection
	Confidence float64

	// DetectionMethod indicates how this relationship was detected
	DetectionMethod string

	// DiscoveredAt indicates when this relationship was discovered
	DiscoveredAt time.Time

	// Metadata contains edge-specific metadata
	Metadata *EdgeMetadata
}

// GraphMetadata contains metadata about the entire graph
type GraphMetadata struct {
	// RootNodes contains the initial resources that started traversal
	RootNodes []NodeID

	// TotalNodes is the total number of nodes in the graph
	TotalNodes int

	// TotalEdges is the total number of edges in the graph
	TotalEdges int

	// MaxDepth is the maximum depth reached during traversal
	MaxDepth int

	// PlatformNodes is the number of nodes belonging to platform scope
	PlatformNodes int

	// ExternalNodes is the number of nodes outside platform scope
	ExternalNodes int

	// CyclesDetected contains any cycles that were detected
	CyclesDetected []Cycle

	// TraversalStatistics contains traversal performance metrics
	TraversalStatistics *TraversalStats

	// CreatedAt indicates when the graph was built
	CreatedAt time.Time
}

// NodeMetadata contains metadata about a specific node
type NodeMetadata struct {
	// OutboundReferenceCount is the number of resources this node references
	OutboundReferenceCount int

	// InboundReferenceCount is the number of resources that reference this node
	InboundReferenceCount int

	// SkippedReferences contains references that were not followed
	SkippedReferences []SkippedReference

	// APIGroup is the API group of this resource
	APIGroup string

	// Kind is the Kubernetes kind of this resource
	Kind string

	// Namespace is the namespace of this resource (if namespaced)
	Namespace string

	// Name is the name of this resource
	Name string
}

// EdgeMetadata contains metadata about a specific edge
type EdgeMetadata struct {
	// ReferenceValue is the actual value of the reference field
	ReferenceValue interface{}

	// IsOptional indicates if this reference is optional
	IsOptional bool

	// IsCrossNamespace indicates if this reference crosses namespace boundaries
	IsCrossNamespace bool

	// TargetExists indicates if the target resource actually exists
	TargetExists bool

	// ResolutionError contains any error that occurred during reference resolution
	ResolutionError error
}

// Cycle represents a detected cycle in the graph
type Cycle struct {
	// Nodes contains the nodes that form the cycle
	Nodes []NodeID

	// Edges contains the edges that form the cycle
	Edges []EdgeID

	// DetectedAt indicates when the cycle was detected
	DetectedAt time.Time

	// CycleType indicates the type of cycle (simple, complex, etc.)
	CycleType string
}

// SkippedReference represents a reference that was not followed
type SkippedReference struct {
	// FieldPath is the path to the skipped reference field
	FieldPath string

	// FieldName is the name of the skipped reference field
	FieldName string

	// Reason indicates why the reference was skipped
	Reason string

	// TargetKind is the kind of the target resource (if determinable)
	TargetKind string

	// TargetGroup is the API group of the target resource (if determinable)
	TargetGroup string
}

// TraversalStats contains statistics about graph traversal
type TraversalStats struct {
	// TotalTraversalTime is the total time spent building the graph
	TotalTraversalTime time.Duration

	// APICallTime is the time spent making Kubernetes API calls
	APICallTime time.Duration

	// ReferenceResolutionTime is the time spent resolving references
	ReferenceResolutionTime time.Duration

	// CycleDetectionTime is the time spent detecting cycles
	CycleDetectionTime time.Duration

	// ResourcesProcessed is the total number of resources processed
	ResourcesProcessed int

	// ReferencesEvaluated is the total number of references evaluated
	ReferencesEvaluated int

	// ReferencesFollowed is the number of references that were followed
	ReferencesFollowed int

	// ReferencesSkipped is the number of references that were skipped
	ReferencesSkipped int

	// APICallCount is the total number of Kubernetes API calls made
	APICallCount int

	// CacheHitCount is the number of cache hits during traversal
	CacheHitCount int

	// CacheMissCount is the number of cache misses during traversal
	CacheMissCount int
}

// TraversalDirection defines the direction of graph traversal
type TraversalDirection string

const (
	// TraversalDirectionForward follows references from source to target
	TraversalDirectionForward TraversalDirection = "forward"
	// TraversalDirectionReverse follows back-references from target to source
	TraversalDirectionReverse TraversalDirection = "reverse"
	// TraversalDirectionBidirectional follows both forward and reverse references
	TraversalDirectionBidirectional TraversalDirection = "bidirectional"
)

// VisitOrder defines the order in which nodes are visited during traversal
type VisitOrder string

const (
	// VisitOrderBreadthFirst visits nodes level by level
	VisitOrderBreadthFirst VisitOrder = "breadthFirst"
	// VisitOrderDepthFirst visits nodes depth by depth
	VisitOrderDepthFirst VisitOrder = "depthFirst"
)

// GraphValidationResult contains the result of graph validation
type GraphValidationResult struct {
	// Valid indicates if the graph is valid
	Valid bool

	// Errors contains any validation errors
	Errors []GraphValidationError

	// Warnings contains any validation warnings
	Warnings []GraphValidationWarning

	// Statistics contains validation statistics
	Statistics *ValidationStatistics
}

// GraphValidationError represents a validation error
type GraphValidationError struct {
	// Type is the type of validation error
	Type string

	// Message is the error message
	Message string

	// NodeID is the node associated with the error (if applicable)
	NodeID *NodeID

	// EdgeID is the edge associated with the error (if applicable)
	EdgeID *EdgeID
}

// GraphValidationWarning represents a validation warning
type GraphValidationWarning struct {
	// Type is the type of validation warning
	Type string

	// Message is the warning message
	Message string

	// NodeID is the node associated with the warning (if applicable)
	NodeID *NodeID

	// EdgeID is the edge associated with the warning (if applicable)
	EdgeID *EdgeID
}

// ValidationStatistics contains statistics about graph validation
type ValidationStatistics struct {
	// NodesValidated is the number of nodes validated
	NodesValidated int

	// EdgesValidated is the number of edges validated
	EdgesValidated int

	// ErrorCount is the total number of errors found
	ErrorCount int

	// WarningCount is the total number of warnings found
	WarningCount int

	// ValidationTime is the time spent validating the graph
	ValidationTime time.Duration
}
