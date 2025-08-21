package graph

import (
	"fmt"
	"strings"
	"time"
	
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	
	"github.com/crossplane/function-kubecore-schema-registry/pkg/dynamic"
)

// GraphBuilder provides functionality to build resource dependency graphs
type GraphBuilder interface {
	// NewGraph creates a new empty resource graph
	NewGraph() *ResourceGraph
	
	// AddNode adds a resource node to the graph
	AddNode(graph *ResourceGraph, resource *unstructured.Unstructured, depth int, discoveryPath []NodeID) *ResourceNode
	
	// AddEdge adds a relationship edge between two nodes
	AddEdge(graph *ResourceGraph, source, target NodeID, relationType RelationType, fieldPath, fieldName string, confidence float64) *ResourceEdge
	
	// BuildGraph builds a graph from a set of root resources and their references
	BuildGraph(rootResources []*unstructured.Unstructured, references map[string][]dynamic.ReferenceField) (*ResourceGraph, error)
	
	// MergeGraphs merges multiple graphs into a single graph
	MergeGraphs(graphs []*ResourceGraph) (*ResourceGraph, error)
	
	// ValidateGraph validates the integrity of the graph
	ValidateGraph(graph *ResourceGraph) *GraphValidationResult
}

// DefaultGraphBuilder implements GraphBuilder interface
type DefaultGraphBuilder struct {
	// platformChecker determines if a resource belongs to platform scope
	platformChecker PlatformChecker
}

// PlatformChecker determines if resources belong to platform scope
type PlatformChecker interface {
	IsPlatformResource(resource *unstructured.Unstructured) bool
	GetAPIGroupScope(apiVersion string) string
}

// NewDefaultGraphBuilder creates a new default graph builder
func NewDefaultGraphBuilder(platformChecker PlatformChecker) *DefaultGraphBuilder {
	return &DefaultGraphBuilder{
		platformChecker: platformChecker,
	}
}

// NewGraph creates a new empty resource graph
func (gb *DefaultGraphBuilder) NewGraph() *ResourceGraph {
	return &ResourceGraph{
		Nodes:                make(map[NodeID]*ResourceNode),
		Edges:                make(map[EdgeID]*ResourceEdge),
		AdjacencyList:        make(map[NodeID][]EdgeID),
		ReverseAdjacencyList: make(map[NodeID][]EdgeID),
		Metadata: &GraphMetadata{
			RootNodes:           make([]NodeID, 0),
			CyclesDetected:      make([]Cycle, 0),
			TraversalStatistics: &TraversalStats{},
			CreatedAt:           time.Now(),
		},
	}
}

// AddNode adds a resource node to the graph
func (gb *DefaultGraphBuilder) AddNode(graph *ResourceGraph, resource *unstructured.Unstructured, depth int, discoveryPath []NodeID) *ResourceNode {
	nodeID := gb.generateNodeID(resource)
	
	// Check if node already exists (deduplication by UID)
	if existingNode, exists := graph.Nodes[nodeID]; exists {
		// Update discovery path if this is a shorter path
		if len(discoveryPath) < len(existingNode.DiscoveryPath) {
			existingNode.DiscoveryPath = discoveryPath
			existingNode.DiscoveryDepth = depth
		}
		return existingNode
	}
	
	// Create new node
	node := &ResourceNode{
		ID:             nodeID,
		Resource:       resource,
		UID:            resource.GetUID(),
		DiscoveredAt:   time.Now(),
		DiscoveryDepth: depth,
		DiscoveryPath:  discoveryPath,
		Platform:       gb.platformChecker.IsPlatformResource(resource),
		Metadata: &NodeMetadata{
			APIGroup:         gb.extractAPIGroup(resource.GetAPIVersion()),
			Kind:             resource.GetKind(),
			Namespace:        resource.GetNamespace(),
			Name:             resource.GetName(),
			SkippedReferences: make([]SkippedReference, 0),
		},
	}
	
	// Add to graph
	graph.Nodes[nodeID] = node
	graph.AdjacencyList[nodeID] = make([]EdgeID, 0)
	graph.ReverseAdjacencyList[nodeID] = make([]EdgeID, 0)
	
	// Update graph metadata
	graph.Metadata.TotalNodes++
	if node.Platform {
		graph.Metadata.PlatformNodes++
	} else {
		graph.Metadata.ExternalNodes++
	}
	if depth > graph.Metadata.MaxDepth {
		graph.Metadata.MaxDepth = depth
	}
	
	return node
}

// AddEdge adds a relationship edge between two nodes
func (gb *DefaultGraphBuilder) AddEdge(graph *ResourceGraph, source, target NodeID, relationType RelationType, fieldPath, fieldName string, confidence float64) *ResourceEdge {
	edgeID := gb.generateEdgeID(source, target, fieldPath)
	
	// Check if edge already exists
	if existingEdge, exists := graph.Edges[edgeID]; exists {
		return existingEdge
	}
	
	// Verify both nodes exist
	sourceNode, sourceExists := graph.Nodes[source]
	targetNode, targetExists := graph.Nodes[target]
	if !sourceExists || !targetExists {
		return nil
	}
	
	// Create new edge
	edge := &ResourceEdge{
		ID:              edgeID,
		Source:          source,
		Target:          target,
		RelationType:    relationType,
		FieldPath:       fieldPath,
		FieldName:       fieldName,
		Confidence:      confidence,
		DetectionMethod: "reference_field_analysis",
		DiscoveredAt:    time.Now(),
		Metadata: &EdgeMetadata{
			IsCrossNamespace: sourceNode.Metadata.Namespace != targetNode.Metadata.Namespace,
			TargetExists:     true,
		},
	}
	
	// Add to graph
	graph.Edges[edgeID] = edge
	graph.AdjacencyList[source] = append(graph.AdjacencyList[source], edgeID)
	graph.ReverseAdjacencyList[target] = append(graph.ReverseAdjacencyList[target], edgeID)
	
	// Update node metadata
	sourceNode.Metadata.OutboundReferenceCount++
	targetNode.Metadata.InboundReferenceCount++
	
	// Update graph metadata
	graph.Metadata.TotalEdges++
	
	return edge
}

// BuildGraph builds a graph from a set of root resources and their references
func (gb *DefaultGraphBuilder) BuildGraph(rootResources []*unstructured.Unstructured, references map[string][]dynamic.ReferenceField) (*ResourceGraph, error) {
	graph := gb.NewGraph()
	
	// Add root nodes
	for _, resource := range rootResources {
		node := gb.AddNode(graph, resource, 0, []NodeID{})
		graph.Metadata.RootNodes = append(graph.Metadata.RootNodes, node.ID)
	}
	
	// Build edges based on reference information
	for resourceKey, refFields := range references {
		sourceNodeID := NodeID(resourceKey)
		sourceNode, exists := graph.Nodes[sourceNodeID]
		if !exists {
			continue
		}
		
		for _, refField := range refFields {
			// Determine relation type from reference field
			relationType := gb.mapReferenceTypeToRelationType(refField.RefType)
			
			// For building graphs from discovered references, we need target resources
			// This method assumes all referenced resources are provided in the references map
			// In practice, this would be called after traversal has resolved all references
			
			// Add edge (target node should exist if traversal was complete)
			targetKey := gb.buildTargetResourceKey(refField.TargetKind, refField.TargetGroup, sourceNode.Metadata.Namespace)
			targetNodeID := NodeID(targetKey)
			
			if _, targetExists := graph.Nodes[targetNodeID]; targetExists {
				gb.AddEdge(graph, sourceNodeID, targetNodeID, relationType, refField.FieldPath, refField.FieldName, refField.Confidence)
			} else {
				// Record skipped reference
				sourceNode.Metadata.SkippedReferences = append(sourceNode.Metadata.SkippedReferences, SkippedReference{
					FieldPath:   refField.FieldPath,
					FieldName:   refField.FieldName,
					Reason:      "target_not_discovered",
					TargetKind:  refField.TargetKind,
					TargetGroup: refField.TargetGroup,
				})
			}
		}
	}
	
	return graph, nil
}

// MergeGraphs merges multiple graphs into a single graph
func (gb *DefaultGraphBuilder) MergeGraphs(graphs []*ResourceGraph) (*ResourceGraph, error) {
	if len(graphs) == 0 {
		return gb.NewGraph(), nil
	}
	
	if len(graphs) == 1 {
		return graphs[0], nil
	}
	
	mergedGraph := gb.NewGraph()
	nodeMapping := make(map[NodeID]NodeID) // Original to merged mapping
	
	// Merge all nodes (deduplicating by UID)
	uidToNodeID := make(map[types.UID]NodeID)
	for _, graph := range graphs {
		for _, node := range graph.Nodes {
			// Check for duplicate by UID
			if existingNodeID, exists := uidToNodeID[node.UID]; exists {
				nodeMapping[node.ID] = existingNodeID
				// Update discovery path if shorter
				existingNode := mergedGraph.Nodes[existingNodeID]
				if len(node.DiscoveryPath) < len(existingNode.DiscoveryPath) {
					existingNode.DiscoveryPath = node.DiscoveryPath
					existingNode.DiscoveryDepth = node.DiscoveryDepth
				}
			} else {
				// Add new node
				newNode := gb.AddNode(mergedGraph, node.Resource, node.DiscoveryDepth, node.DiscoveryPath)
				nodeMapping[node.ID] = newNode.ID
				uidToNodeID[node.UID] = newNode.ID
			}
		}
	}
	
	// Merge all edges (using node mapping)
	edgeSet := make(map[string]bool) // Deduplication set
	for _, graph := range graphs {
		for _, edge := range graph.Edges {
			mappedSource, sourceExists := nodeMapping[edge.Source]
			mappedTarget, targetExists := nodeMapping[edge.Target]
			
			if !sourceExists || !targetExists {
				continue
			}
			
			// Create deduplication key
			edgeKey := fmt.Sprintf("%s->%s:%s", mappedSource, mappedTarget, edge.FieldPath)
			if edgeSet[edgeKey] {
				continue
			}
			
			gb.AddEdge(mergedGraph, mappedSource, mappedTarget, edge.RelationType, edge.FieldPath, edge.FieldName, edge.Confidence)
			edgeSet[edgeKey] = true
		}
	}
	
	// Merge root nodes
	rootNodeSet := make(map[NodeID]bool)
	for _, graph := range graphs {
		for _, rootNodeID := range graph.Metadata.RootNodes {
			if mappedRootID, exists := nodeMapping[rootNodeID]; exists {
				if !rootNodeSet[mappedRootID] {
					mergedGraph.Metadata.RootNodes = append(mergedGraph.Metadata.RootNodes, mappedRootID)
					rootNodeSet[mappedRootID] = true
				}
			}
		}
	}
	
	// Merge cycles
	for _, graph := range graphs {
		for _, cycle := range graph.Metadata.CyclesDetected {
			// Remap cycle nodes and edges
			mappedCycle := Cycle{
				DetectedAt: cycle.DetectedAt,
				CycleType:  cycle.CycleType,
				Nodes:      make([]NodeID, 0, len(cycle.Nodes)),
				Edges:      make([]EdgeID, 0, len(cycle.Edges)),
			}
			
			// Remap nodes
			for _, nodeID := range cycle.Nodes {
				if mappedNodeID, exists := nodeMapping[nodeID]; exists {
					mappedCycle.Nodes = append(mappedCycle.Nodes, mappedNodeID)
				}
			}
			
			// Note: Edge remapping is more complex and would require edge mapping
			// For now, we'll skip detailed edge remapping in cycles
			
			mergedGraph.Metadata.CyclesDetected = append(mergedGraph.Metadata.CyclesDetected, mappedCycle)
		}
	}
	
	return mergedGraph, nil
}

// ValidateGraph validates the integrity of the graph
func (gb *DefaultGraphBuilder) ValidateGraph(graph *ResourceGraph) *GraphValidationResult {
	result := &GraphValidationResult{
		Valid:    true,
		Errors:   make([]GraphValidationError, 0),
		Warnings: make([]GraphValidationWarning, 0),
		Statistics: &ValidationStatistics{
			NodesValidated: len(graph.Nodes),
			EdgesValidated: len(graph.Edges),
		},
	}
	
	startTime := time.Now()
	
	// Validate nodes
	for nodeID, node := range graph.Nodes {
		gb.validateNode(nodeID, node, result)
	}
	
	// Validate edges
	for edgeID, edge := range graph.Edges {
		gb.validateEdge(edgeID, edge, graph, result)
	}
	
	// Validate adjacency lists
	gb.validateAdjacencyLists(graph, result)
	
	// Validate graph metadata consistency
	gb.validateGraphMetadata(graph, result)
	
	result.Statistics.ValidationTime = time.Since(startTime)
	result.Statistics.ErrorCount = len(result.Errors)
	result.Statistics.WarningCount = len(result.Warnings)
	
	if len(result.Errors) > 0 {
		result.Valid = false
	}
	
	return result
}

// Helper methods

func (gb *DefaultGraphBuilder) generateNodeID(resource *unstructured.Unstructured) NodeID {
	// Generate a unique node ID based on resource identity
	return NodeID(fmt.Sprintf("%s/%s/%s/%s", 
		resource.GetAPIVersion(), 
		resource.GetKind(), 
		resource.GetNamespace(), 
		resource.GetName()))
}

func (gb *DefaultGraphBuilder) generateEdgeID(source, target NodeID, fieldPath string) EdgeID {
	return EdgeID(fmt.Sprintf("%s->%s:%s", source, target, fieldPath))
}

func (gb *DefaultGraphBuilder) extractAPIGroup(apiVersion string) string {
	// Extract API group from apiVersion (e.g., "apps/v1" -> "apps")
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 2 {
		return parts[0]
	}
	return "" // Core API group
}

func (gb *DefaultGraphBuilder) mapReferenceTypeToRelationType(refType dynamic.RefType) RelationType {
	switch refType {
	case dynamic.RefTypeOwnerRef:
		return RelationTypeOwnerRef
	case dynamic.RefTypeConfigMap:
		return RelationTypeConfigMapRef
	case dynamic.RefTypeSecret:
		return RelationTypeSecretRef
	case dynamic.RefTypeService:
		return RelationTypeServiceRef
	case dynamic.RefTypePVC:
		return RelationTypePVCRef
	case dynamic.RefTypeCustom:
		return RelationTypeCustomRef
	default:
		return RelationTypeCustomRef
	}
}

func (gb *DefaultGraphBuilder) buildTargetResourceKey(kind, group, namespace string) string {
	if group == "" {
		group = "v1" // Core API group
	}
	return fmt.Sprintf("%s/%s/%s", group, kind, namespace)
}

func (gb *DefaultGraphBuilder) validateNode(nodeID NodeID, node *ResourceNode, result *GraphValidationResult) {
	// Validate node ID consistency
	if node.ID != nodeID {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "node_id_mismatch",
			Message: fmt.Sprintf("Node ID mismatch: map key %s != node.ID %s", nodeID, node.ID),
			NodeID:  &nodeID,
		})
	}
	
	// Validate required fields
	if node.Resource == nil {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "missing_resource",
			Message: "Node has nil resource",
			NodeID:  &nodeID,
		})
	}
	
	if node.Metadata == nil {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "missing_metadata",
			Message: "Node has nil metadata",
			NodeID:  &nodeID,
		})
	}
	
	// Validate discovery depth
	if node.DiscoveryDepth < 0 {
		result.Warnings = append(result.Warnings, GraphValidationWarning{
			Type:    "negative_depth",
			Message: fmt.Sprintf("Node has negative discovery depth: %d", node.DiscoveryDepth),
			NodeID:  &nodeID,
		})
	}
}

func (gb *DefaultGraphBuilder) validateEdge(edgeID EdgeID, edge *ResourceEdge, graph *ResourceGraph, result *GraphValidationResult) {
	// Validate edge ID consistency
	if edge.ID != edgeID {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "edge_id_mismatch",
			Message: fmt.Sprintf("Edge ID mismatch: map key %s != edge.ID %s", edgeID, edge.ID),
			EdgeID:  &edgeID,
		})
	}
	
	// Validate source node exists
	if _, exists := graph.Nodes[edge.Source]; !exists {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "missing_source_node",
			Message: fmt.Sprintf("Edge references non-existent source node: %s", edge.Source),
			EdgeID:  &edgeID,
		})
	}
	
	// Validate target node exists
	if _, exists := graph.Nodes[edge.Target]; !exists {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "missing_target_node",
			Message: fmt.Sprintf("Edge references non-existent target node: %s", edge.Target),
			EdgeID:  &edgeID,
		})
	}
	
	// Validate confidence range
	if edge.Confidence < 0 || edge.Confidence > 1 {
		result.Warnings = append(result.Warnings, GraphValidationWarning{
			Type:    "invalid_confidence",
			Message: fmt.Sprintf("Edge has invalid confidence value: %f", edge.Confidence),
			EdgeID:  &edgeID,
		})
	}
}

func (gb *DefaultGraphBuilder) validateAdjacencyLists(graph *ResourceGraph, result *GraphValidationResult) {
	// Validate forward adjacency list
	for nodeID, edgeIDs := range graph.AdjacencyList {
		if _, exists := graph.Nodes[nodeID]; !exists {
			result.Errors = append(result.Errors, GraphValidationError{
				Type:    "orphaned_adjacency_entry",
				Message: fmt.Sprintf("Adjacency list contains entry for non-existent node: %s", nodeID),
			})
		}
		
		for _, edgeID := range edgeIDs {
			edge, exists := graph.Edges[edgeID]
			if !exists {
				result.Errors = append(result.Errors, GraphValidationError{
					Type:    "missing_edge_in_adjacency",
					Message: fmt.Sprintf("Adjacency list references non-existent edge: %s", edgeID),
				})
				continue
			}
			
			if edge.Source != nodeID {
				result.Errors = append(result.Errors, GraphValidationError{
					Type:    "adjacency_source_mismatch",
					Message: fmt.Sprintf("Edge %s in adjacency list for %s but edge.Source is %s", edgeID, nodeID, edge.Source),
				})
			}
		}
	}
	
	// Validate reverse adjacency list
	for nodeID, edgeIDs := range graph.ReverseAdjacencyList {
		if _, exists := graph.Nodes[nodeID]; !exists {
			result.Errors = append(result.Errors, GraphValidationError{
				Type:    "orphaned_reverse_adjacency_entry",
				Message: fmt.Sprintf("Reverse adjacency list contains entry for non-existent node: %s", nodeID),
			})
		}
		
		for _, edgeID := range edgeIDs {
			edge, exists := graph.Edges[edgeID]
			if !exists {
				result.Errors = append(result.Errors, GraphValidationError{
					Type:    "missing_edge_in_reverse_adjacency",
					Message: fmt.Sprintf("Reverse adjacency list references non-existent edge: %s", edgeID),
				})
				continue
			}
			
			if edge.Target != nodeID {
				result.Errors = append(result.Errors, GraphValidationError{
					Type:    "reverse_adjacency_target_mismatch",
					Message: fmt.Sprintf("Edge %s in reverse adjacency list for %s but edge.Target is %s", edgeID, nodeID, edge.Target),
				})
			}
		}
	}
}

func (gb *DefaultGraphBuilder) validateGraphMetadata(graph *ResourceGraph, result *GraphValidationResult) {
	// Validate node counts
	expectedTotalNodes := len(graph.Nodes)
	if graph.Metadata.TotalNodes != expectedTotalNodes {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "node_count_mismatch",
			Message: fmt.Sprintf("Metadata TotalNodes (%d) doesn't match actual nodes (%d)", graph.Metadata.TotalNodes, expectedTotalNodes),
		})
	}
	
	// Validate edge counts
	expectedTotalEdges := len(graph.Edges)
	if graph.Metadata.TotalEdges != expectedTotalEdges {
		result.Errors = append(result.Errors, GraphValidationError{
			Type:    "edge_count_mismatch",
			Message: fmt.Sprintf("Metadata TotalEdges (%d) doesn't match actual edges (%d)", graph.Metadata.TotalEdges, expectedTotalEdges),
		})
	}
	
	// Validate platform node count
	platformCount := 0
	for _, node := range graph.Nodes {
		if node.Platform {
			platformCount++
		}
	}
	if graph.Metadata.PlatformNodes != platformCount {
		result.Warnings = append(result.Warnings, GraphValidationWarning{
			Type:    "platform_count_mismatch",
			Message: fmt.Sprintf("Metadata PlatformNodes (%d) doesn't match actual platform nodes (%d)", graph.Metadata.PlatformNodes, platformCount),
		})
	}
	
	// Validate root nodes exist
	for _, rootNodeID := range graph.Metadata.RootNodes {
		if _, exists := graph.Nodes[rootNodeID]; !exists {
			result.Errors = append(result.Errors, GraphValidationError{
				Type:    "missing_root_node",
				Message: fmt.Sprintf("Root node %s does not exist in graph", rootNodeID),
			})
		}
	}
}