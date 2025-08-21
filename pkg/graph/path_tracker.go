package graph

import (
	"fmt"
	"strings"
	"time"
)

// PathTracker provides functionality to track and audit discovery paths in resource graphs
type PathTracker interface {
	// TrackPath records a discovery path from source to target
	TrackPath(graph *ResourceGraph, source, target NodeID, path []NodeID, edges []EdgeID, metadata *PathMetadata)
	
	// GetDiscoveryPaths returns all discovery paths for a node
	GetDiscoveryPaths(graph *ResourceGraph, nodeID NodeID) []DiscoveryPath
	
	// GetShortestDiscoveryPath returns the shortest discovery path to a node
	GetShortestDiscoveryPath(graph *ResourceGraph, nodeID NodeID) *DiscoveryPath
	
	// GetDiscoveryTree builds a tree representation of discovery paths
	GetDiscoveryTree(graph *ResourceGraph) *DiscoveryTree
	
	// ValidateDiscoveryPaths validates all discovery paths in the graph
	ValidateDiscoveryPaths(graph *ResourceGraph) *PathValidationResult
	
	// GetPathStatistics calculates statistics about discovery paths
	GetPathStatistics(graph *ResourceGraph) *PathStatistics
}

// DiscoveryPath represents a path from root to a discovered resource
type DiscoveryPath struct {
	// ID is a unique identifier for this path
	ID string
	
	// Source is the starting node (typically a root node)
	Source NodeID
	
	// Target is the ending node (the discovered resource)
	Target NodeID
	
	// Nodes contains the sequence of nodes in the path
	Nodes []NodeID
	
	// Edges contains the sequence of edges in the path
	Edges []EdgeID
	
	// Length is the number of edges in the path
	Length int
	
	// Depth is the depth of the target node (same as Length)
	Depth int
	
	// DiscoveredAt indicates when this path was created
	DiscoveredAt time.Time
	
	// PathType indicates the type of discovery path
	PathType PathType
	
	// Metadata contains additional path information
	Metadata *PathMetadata
}

// PathType represents the type of discovery path
type PathType string

const (
	// PathTypeDirect represents a direct path from root
	PathTypeDirect PathType = "direct"
	// PathTypeTransitive represents a path through intermediate resources
	PathTypeTransitive PathType = "transitive"
	// PathTypeOwnerChain represents a path following owner references
	PathTypeOwnerChain PathType = "ownerChain"
	// PathTypeCustomRef represents a path following custom references
	PathTypeCustomRef PathType = "customRef"
	// PathTypeMixed represents a path using multiple reference types
	PathTypeMixed PathType = "mixed"
)

// PathMetadata contains metadata about a discovery path
type PathMetadata struct {
	// ReferenceTypes contains the types of references used in the path
	ReferenceTypes []RelationType
	
	// CrossNamespaceHops indicates how many times the path crosses namespaces
	CrossNamespaceHops int
	
	// PlatformBoundaryHops indicates how many times the path crosses platform boundaries
	PlatformBoundaryHops int
	
	// TotalConfidence is the product of all edge confidences in the path
	TotalConfidence float64
	
	// AverageConfidence is the average confidence of edges in the path
	AverageConfidence float64
	
	// MinConfidence is the minimum confidence of any edge in the path
	MinConfidence float64
	
	// IsOptimal indicates if this is the optimal path to the target
	IsOptimal bool
	
	// AlternativePaths is the number of alternative paths to the same target
	AlternativePaths int
	
	// DetectionMethods contains the methods used to detect references in the path
	DetectionMethods []string
}

// DiscoveryTree represents a tree structure of discovery paths
type DiscoveryTree struct {
	// Root is the root node of the discovery tree
	Root NodeID
	
	// Children contains child trees for each root node
	Children map[NodeID]*DiscoveryTreeNode
	
	// AllPaths contains all discovery paths in the tree
	AllPaths []DiscoveryPath
	
	// MaxDepth is the maximum depth of any path in the tree
	MaxDepth int
	
	// TotalNodes is the total number of unique nodes in the tree
	TotalNodes int
	
	// TreeMetadata contains metadata about the tree
	TreeMetadata *DiscoveryTreeMetadata
}

// DiscoveryTreeNode represents a node in the discovery tree
type DiscoveryTreeNode struct {
	// NodeID is the identifier of this node
	NodeID NodeID
	
	// Parent is the parent node in the tree (nil for root)
	Parent *DiscoveryTreeNode
	
	// Children contains child nodes
	Children map[NodeID]*DiscoveryTreeNode
	
	// Depth is the depth of this node in the tree
	Depth int
	
	// PathFromRoot is the path from root to this node
	PathFromRoot []NodeID
	
	// EdgesFromRoot is the edges from root to this node
	EdgesFromRoot []EdgeID
	
	// IsLeaf indicates if this node has no children
	IsLeaf bool
	
	// Resource is the actual resource at this node
	Resource *ResourceNode
}

// DiscoveryTreeMetadata contains metadata about the discovery tree
type DiscoveryTreeMetadata struct {
	// BuildTime is the time taken to build the tree
	BuildTime time.Duration
	
	// TotalBranches is the number of branches in the tree
	TotalBranches int
	
	// LeafNodes is the number of leaf nodes
	LeafNodes int
	
	// AverageDepth is the average depth of all nodes
	AverageDepth float64
	
	// BalanceFactor indicates how balanced the tree is
	BalanceFactor float64
}

// PathValidationResult contains the result of path validation
type PathValidationResult struct {
	// Valid indicates if all paths are valid
	Valid bool
	
	// TotalPaths is the total number of paths validated
	TotalPaths int
	
	// ValidPaths is the number of valid paths
	ValidPaths int
	
	// InvalidPaths is the number of invalid paths
	InvalidPaths int
	
	// ValidationErrors contains any validation errors
	ValidationErrors []PathValidationError
	
	// ValidationWarnings contains any validation warnings
	ValidationWarnings []PathValidationWarning
	
	// ValidationTime is the time taken for validation
	ValidationTime time.Duration
}

// PathValidationError represents a path validation error
type PathValidationError struct {
	// PathID is the ID of the problematic path
	PathID string
	
	// ErrorType is the type of validation error
	ErrorType string
	
	// Message is the error message
	Message string
	
	// NodeID is the node associated with the error (if applicable)
	NodeID *NodeID
	
	// EdgeID is the edge associated with the error (if applicable)
	EdgeID *EdgeID
}

// PathValidationWarning represents a path validation warning
type PathValidationWarning struct {
	// PathID is the ID of the path with warning
	PathID string
	
	// WarningType is the type of validation warning
	WarningType string
	
	// Message is the warning message
	Message string
	
	// Severity indicates the severity of the warning
	Severity string
}

// PathStatistics contains statistics about discovery paths
type PathStatistics struct {
	// TotalPaths is the total number of discovery paths
	TotalPaths int
	
	// UniqueTargets is the number of unique target nodes
	UniqueTargets int
	
	// AveragePathLength is the average length of all paths
	AveragePathLength float64
	
	// MinPathLength is the minimum path length
	MinPathLength int
	
	// MaxPathLength is the maximum path length
	MaxPathLength int
	
	// PathsByDepth groups paths by their depth
	PathsByDepth map[int]int
	
	// PathsByType groups paths by their type
	PathsByType map[PathType]int
	
	// AverageConfidence is the average confidence across all paths
	AverageConfidence float64
	
	// CrossNamespacePaths is the number of paths that cross namespaces
	CrossNamespacePaths int
	
	// PlatformBoundaryPaths is the number of paths that cross platform boundaries
	PlatformBoundaryPaths int
	
	// RedundantPaths is the number of redundant paths (multiple paths to same target)
	RedundantPaths int
	
	// OptimalPaths is the number of optimal paths
	OptimalPaths int
}

// DefaultPathTracker implements PathTracker interface
type DefaultPathTracker struct {
	// pathIndex maintains an index of all paths for efficient retrieval
	pathIndex map[NodeID][]DiscoveryPath
	
	// pathCache caches computed paths and trees
	pathCache map[string]interface{}
	
	// enableCaching controls whether to cache computed results
	enableCaching bool
}

// NewDefaultPathTracker creates a new default path tracker
func NewDefaultPathTracker(enableCaching bool) *DefaultPathTracker {
	return &DefaultPathTracker{
		pathIndex:     make(map[NodeID][]DiscoveryPath),
		pathCache:     make(map[string]interface{}),
		enableCaching: enableCaching,
	}
}

// TrackPath records a discovery path from source to target
func (pt *DefaultPathTracker) TrackPath(graph *ResourceGraph, source, target NodeID, path []NodeID, edges []EdgeID, metadata *PathMetadata) {
	if len(path) < 2 || len(edges) != len(path)-1 {
		return // Invalid path
	}
	
	// Generate unique path ID
	pathID := pt.generatePathID(source, target, path)
	
	// Determine path type
	pathType := pt.determinePathType(graph, edges)
	
	// Calculate metadata if not provided
	if metadata == nil {
		metadata = pt.calculatePathMetadata(graph, edges)
	}
	
	// Create discovery path
	discoveryPath := DiscoveryPath{
		ID:           pathID,
		Source:       source,
		Target:       target,
		Nodes:        make([]NodeID, len(path)),
		Edges:        make([]EdgeID, len(edges)),
		Length:       len(edges),
		Depth:        len(edges),
		DiscoveredAt: time.Now(),
		PathType:     pathType,
		Metadata:     metadata,
	}
	
	copy(discoveryPath.Nodes, path)
	copy(discoveryPath.Edges, edges)
	
	// Add to path index
	if pt.pathIndex[target] == nil {
		pt.pathIndex[target] = make([]DiscoveryPath, 0)
	}
	pt.pathIndex[target] = append(pt.pathIndex[target], discoveryPath)
	
	// Update graph node with discovery path
	if targetNode, exists := graph.Nodes[target]; exists {
		if len(path) < len(targetNode.DiscoveryPath) || len(targetNode.DiscoveryPath) == 0 {
			targetNode.DiscoveryPath = path
			targetNode.DiscoveryDepth = len(edges)
		}
	}
	
	// Clear cache if caching is enabled
	if pt.enableCaching {
		pt.clearCache()
	}
}

// GetDiscoveryPaths returns all discovery paths for a node
func (pt *DefaultPathTracker) GetDiscoveryPaths(graph *ResourceGraph, nodeID NodeID) []DiscoveryPath {
	if paths, exists := pt.pathIndex[nodeID]; exists {
		// Return copy to prevent modification
		result := make([]DiscoveryPath, len(paths))
		copy(result, paths)
		return result
	}
	
	// If no paths in index, try to reconstruct from graph
	if node, exists := graph.Nodes[nodeID]; exists && len(node.DiscoveryPath) > 0 {
		// Reconstruct path from graph data
		path := pt.reconstructPath(graph, node.DiscoveryPath)
		return []DiscoveryPath{path}
	}
	
	return []DiscoveryPath{}
}

// GetShortestDiscoveryPath returns the shortest discovery path to a node
func (pt *DefaultPathTracker) GetShortestDiscoveryPath(graph *ResourceGraph, nodeID NodeID) *DiscoveryPath {
	paths := pt.GetDiscoveryPaths(graph, nodeID)
	if len(paths) == 0 {
		return nil
	}
	
	// Find shortest path
	shortest := &paths[0]
	for i := 1; i < len(paths); i++ {
		if paths[i].Length < shortest.Length {
			shortest = &paths[i]
		}
	}
	
	return shortest
}

// GetDiscoveryTree builds a tree representation of discovery paths
func (pt *DefaultPathTracker) GetDiscoveryTree(graph *ResourceGraph) *DiscoveryTree {
	cacheKey := "discovery_tree"
	
	// Check cache
	if pt.enableCaching {
		if cached, exists := pt.pathCache[cacheKey]; exists {
			return cached.(*DiscoveryTree)
		}
	}
	
	startTime := time.Now()
	
	tree := &DiscoveryTree{
		Children:     make(map[NodeID]*DiscoveryTreeNode),
		AllPaths:     make([]DiscoveryPath, 0),
		TreeMetadata: &DiscoveryTreeMetadata{},
	}
	
	// Build tree for each root node
	for _, rootID := range graph.Metadata.RootNodes {
		if rootNode, exists := graph.Nodes[rootID]; exists {
			treeNode := &DiscoveryTreeNode{
				NodeID:        rootID,
				Parent:        nil,
				Children:      make(map[NodeID]*DiscoveryTreeNode),
				Depth:         0,
				PathFromRoot:  []NodeID{rootID},
				EdgesFromRoot: []EdgeID{},
				IsLeaf:        true,
				Resource:      rootNode,
			}
			
			tree.Children[rootID] = treeNode
			pt.buildTreeNode(graph, treeNode, tree)
		}
	}
	
	// Calculate tree metadata
	tree.TreeMetadata.BuildTime = time.Since(startTime)
	tree.TotalNodes = len(graph.Nodes)
	tree.MaxDepth = graph.Metadata.MaxDepth
	
	// Calculate additional metrics
	pt.calculateTreeMetrics(tree)
	
	// Cache result
	if pt.enableCaching {
		pt.pathCache[cacheKey] = tree
	}
	
	return tree
}

// ValidateDiscoveryPaths validates all discovery paths in the graph
func (pt *DefaultPathTracker) ValidateDiscoveryPaths(graph *ResourceGraph) *PathValidationResult {
	startTime := time.Now()
	
	result := &PathValidationResult{
		Valid:               true,
		ValidationErrors:    make([]PathValidationError, 0),
		ValidationWarnings:  make([]PathValidationWarning, 0),
	}
	
	// Collect all paths
	allPaths := make([]DiscoveryPath, 0)
	for _, paths := range pt.pathIndex {
		allPaths = append(allPaths, paths...)
	}
	
	result.TotalPaths = len(allPaths)
	
	// Validate each path
	for _, path := range allPaths {
		pt.validatePath(graph, path, result)
	}
	
	result.ValidPaths = result.TotalPaths - result.InvalidPaths
	result.ValidationTime = time.Since(startTime)
	
	if len(result.ValidationErrors) > 0 {
		result.Valid = false
	}
	
	return result
}

// GetPathStatistics calculates statistics about discovery paths
func (pt *DefaultPathTracker) GetPathStatistics(graph *ResourceGraph) *PathStatistics {
	cacheKey := "path_statistics"
	
	// Check cache
	if pt.enableCaching {
		if cached, exists := pt.pathCache[cacheKey]; exists {
			return cached.(*PathStatistics)
		}
	}
	
	stats := &PathStatistics{
		PathsByDepth: make(map[int]int),
		PathsByType:  make(map[PathType]int),
		MinPathLength: int(^uint(0) >> 1), // Max int
	}
	
	// Collect all paths
	allPaths := make([]DiscoveryPath, 0)
	uniqueTargets := make(map[NodeID]bool)
	
	for targetID, paths := range pt.pathIndex {
		allPaths = append(allPaths, paths...)
		uniqueTargets[targetID] = true
	}
	
	stats.TotalPaths = len(allPaths)
	stats.UniqueTargets = len(uniqueTargets)
	
	if stats.TotalPaths == 0 {
		return stats
	}
	
	// Calculate statistics
	totalLength := 0
	totalConfidence := 0.0
	crossNamespaceCount := 0
	platformBoundaryCount := 0
	redundantCount := 0
	optimalCount := 0
	
	for _, path := range allPaths {
		// Length statistics
		totalLength += path.Length
		stats.PathsByDepth[path.Length]++
		
		if path.Length < stats.MinPathLength {
			stats.MinPathLength = path.Length
		}
		if path.Length > stats.MaxPathLength {
			stats.MaxPathLength = path.Length
		}
		
		// Type statistics
		stats.PathsByType[path.PathType]++
		
		// Metadata statistics
		if path.Metadata != nil {
			totalConfidence += path.Metadata.AverageConfidence
			
			if path.Metadata.CrossNamespaceHops > 0 {
				crossNamespaceCount++
			}
			
			if path.Metadata.PlatformBoundaryHops > 0 {
				platformBoundaryCount++
			}
			
			if path.Metadata.AlternativePaths > 0 {
				redundantCount++
			}
			
			if path.Metadata.IsOptimal {
				optimalCount++
			}
		}
	}
	
	stats.AveragePathLength = float64(totalLength) / float64(stats.TotalPaths)
	stats.AverageConfidence = totalConfidence / float64(stats.TotalPaths)
	stats.CrossNamespacePaths = crossNamespaceCount
	stats.PlatformBoundaryPaths = platformBoundaryCount
	stats.RedundantPaths = redundantCount
	stats.OptimalPaths = optimalCount
	
	// Cache result
	if pt.enableCaching {
		pt.pathCache[cacheKey] = stats
	}
	
	return stats
}

// Helper methods

// generatePathID generates a unique ID for a discovery path
func (pt *DefaultPathTracker) generatePathID(source, target NodeID, path []NodeID) string {
	pathStr := strings.Join(pt.nodeIDsToStrings(path), "->")
	return fmt.Sprintf("%s::%s::%s", source, target, pathStr)
}

// determinePathType determines the type of a discovery path based on edge types
func (pt *DefaultPathTracker) determinePathType(graph *ResourceGraph, edges []EdgeID) PathType {
	if len(edges) == 0 {
		return PathTypeDirect
	}
	
	relTypes := make(map[RelationType]bool)
	for _, edgeID := range edges {
		if edge, exists := graph.Edges[edgeID]; exists {
			relTypes[edge.RelationType] = true
		}
	}
	
	// Determine path type based on reference types used
	if len(relTypes) == 1 {
		// Single reference type
		for relType := range relTypes {
			switch relType {
			case RelationTypeOwnerRef:
				return PathTypeOwnerChain
			case RelationTypeCustomRef:
				return PathTypeCustomRef
			default:
				return PathTypeTransitive
			}
		}
	}
	
	// Multiple reference types
	return PathTypeMixed
}

// calculatePathMetadata calculates metadata for a discovery path
func (pt *DefaultPathTracker) calculatePathMetadata(graph *ResourceGraph, edges []EdgeID) *PathMetadata {
	metadata := &PathMetadata{
		ReferenceTypes:   make([]RelationType, 0),
		DetectionMethods: make([]string, 0),
		TotalConfidence:  1.0,
		MinConfidence:    1.0,
	}
	
	if len(edges) == 0 {
		return metadata
	}
	
	confidenceSum := 0.0
	var prevNamespace string
	var prevPlatform bool
	
	for i, edgeID := range edges {
		edge, exists := graph.Edges[edgeID]
		if !exists {
			continue
		}
		
		// Collect reference types
		metadata.ReferenceTypes = append(metadata.ReferenceTypes, edge.RelationType)
		
		// Collect detection methods
		if edge.DetectionMethod != "" {
			metadata.DetectionMethods = append(metadata.DetectionMethods, edge.DetectionMethod)
		}
		
		// Calculate confidence metrics
		confidenceSum += edge.Confidence
		metadata.TotalConfidence *= edge.Confidence
		if edge.Confidence < metadata.MinConfidence {
			metadata.MinConfidence = edge.Confidence
		}
		
		// Check for namespace and platform boundary crossings
		sourceNode, sourceExists := graph.Nodes[edge.Source]
		targetNode, targetExists := graph.Nodes[edge.Target]
		
		if sourceExists && targetExists {
			// Check namespace boundaries
			if i > 0 && prevNamespace != "" && sourceNode.Metadata.Namespace != prevNamespace {
				metadata.CrossNamespaceHops++
			}
			if sourceNode.Metadata.Namespace != targetNode.Metadata.Namespace {
				metadata.CrossNamespaceHops++
			}
			prevNamespace = targetNode.Metadata.Namespace
			
			// Check platform boundaries
			if i > 0 && prevPlatform != sourceNode.Platform {
				metadata.PlatformBoundaryHops++
			}
			if sourceNode.Platform != targetNode.Platform {
				metadata.PlatformBoundaryHops++
			}
			prevPlatform = targetNode.Platform
		}
	}
	
	if len(edges) > 0 {
		metadata.AverageConfidence = confidenceSum / float64(len(edges))
	}
	
	return metadata
}

// reconstructPath reconstructs a discovery path from graph data
func (pt *DefaultPathTracker) reconstructPath(graph *ResourceGraph, nodePath []NodeID) DiscoveryPath {
	if len(nodePath) < 2 {
		return DiscoveryPath{}
	}
	
	edges := make([]EdgeID, 0, len(nodePath)-1)
	
	// Find edges between consecutive nodes
	for i := 0; i < len(nodePath)-1; i++ {
		source := nodePath[i]
		target := nodePath[i+1]
		
		// Find edge from source to target
		if adjacentEdges, exists := graph.AdjacencyList[source]; exists {
			for _, edgeID := range adjacentEdges {
				if edge, edgeExists := graph.Edges[edgeID]; edgeExists && edge.Target == target {
					edges = append(edges, edgeID)
					break
				}
			}
		}
	}
	
	metadata := pt.calculatePathMetadata(graph, edges)
	pathType := pt.determinePathType(graph, edges)
	
	return DiscoveryPath{
		ID:           pt.generatePathID(nodePath[0], nodePath[len(nodePath)-1], nodePath),
		Source:       nodePath[0],
		Target:       nodePath[len(nodePath)-1],
		Nodes:        nodePath,
		Edges:        edges,
		Length:       len(edges),
		Depth:        len(edges),
		DiscoveredAt: time.Now(),
		PathType:     pathType,
		Metadata:     metadata,
	}
}

// buildTreeNode recursively builds a discovery tree node
func (pt *DefaultPathTracker) buildTreeNode(graph *ResourceGraph, node *DiscoveryTreeNode, tree *DiscoveryTree) {
	// Find child nodes
	if adjacentEdges, exists := graph.AdjacencyList[node.NodeID]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists {
				continue
			}
			
			targetNode, targetExists := graph.Nodes[edge.Target]
			if !targetExists {
				continue
			}
			
			// Create child tree node
			childPath := make([]NodeID, len(node.PathFromRoot))
			copy(childPath, node.PathFromRoot)
			childPath = append(childPath, edge.Target)
			
			childEdges := make([]EdgeID, len(node.EdgesFromRoot))
			copy(childEdges, node.EdgesFromRoot)
			childEdges = append(childEdges, edgeID)
			
			childTreeNode := &DiscoveryTreeNode{
				NodeID:        edge.Target,
				Parent:        node,
				Children:      make(map[NodeID]*DiscoveryTreeNode),
				Depth:         node.Depth + 1,
				PathFromRoot:  childPath,
				EdgesFromRoot: childEdges,
				IsLeaf:        true,
				Resource:      targetNode,
			}
			
			node.Children[edge.Target] = childTreeNode
			node.IsLeaf = false
			
			// Create discovery path for this child
			metadata := pt.calculatePathMetadata(graph, childEdges)
			pathType := pt.determinePathType(graph, childEdges)
			
			discoveryPath := DiscoveryPath{
				ID:           pt.generatePathID(childPath[0], edge.Target, childPath),
				Source:       childPath[0],
				Target:       edge.Target,
				Nodes:        childPath,
				Edges:        childEdges,
				Length:       len(childEdges),
				Depth:        len(childEdges),
				DiscoveredAt: time.Now(),
				PathType:     pathType,
				Metadata:     metadata,
			}
			
			tree.AllPaths = append(tree.AllPaths, discoveryPath)
			
			// Recursively build child nodes
			pt.buildTreeNode(graph, childTreeNode, tree)
		}
	}
}

// calculateTreeMetrics calculates additional metrics for the discovery tree
func (pt *DefaultPathTracker) calculateTreeMetrics(tree *DiscoveryTree) {
	branchCount := 0
	leafCount := 0
	totalDepth := 0
	nodeCount := 0
	
	// Traverse tree to calculate metrics
	var traverse func(*DiscoveryTreeNode)
	traverse = func(node *DiscoveryTreeNode) {
		nodeCount++
		totalDepth += node.Depth
		
		if node.IsLeaf {
			leafCount++
		} else {
			branchCount++
		}
		
		for _, child := range node.Children {
			traverse(child)
		}
	}
	
	for _, rootChild := range tree.Children {
		traverse(rootChild)
	}
	
	tree.TreeMetadata.TotalBranches = branchCount
	tree.TreeMetadata.LeafNodes = leafCount
	
	if nodeCount > 0 {
		tree.TreeMetadata.AverageDepth = float64(totalDepth) / float64(nodeCount)
	}
	
	// Calculate balance factor (simplified)
	if tree.MaxDepth > 0 {
		tree.TreeMetadata.BalanceFactor = tree.TreeMetadata.AverageDepth / float64(tree.MaxDepth)
	}
}

// validatePath validates a single discovery path
func (pt *DefaultPathTracker) validatePath(graph *ResourceGraph, path DiscoveryPath, result *PathValidationResult) {
	// Validate path structure
	if len(path.Nodes) != len(path.Edges)+1 {
		result.InvalidPaths++
		result.ValidationErrors = append(result.ValidationErrors, PathValidationError{
			PathID:    path.ID,
			ErrorType: "invalid_structure",
			Message:   fmt.Sprintf("Path has %d nodes but %d edges", len(path.Nodes), len(path.Edges)),
		})
		return
	}
	
	// Validate nodes exist
	for i, nodeID := range path.Nodes {
		if _, exists := graph.Nodes[nodeID]; !exists {
			result.InvalidPaths++
			result.ValidationErrors = append(result.ValidationErrors, PathValidationError{
				PathID:    path.ID,
				ErrorType: "missing_node",
				Message:   fmt.Sprintf("Path references non-existent node: %s", nodeID),
				NodeID:    &nodeID,
			})
			return
		}
	}
	
	// Validate edges exist and connect correctly
	for i, edgeID := range path.Edges {
		edge, exists := graph.Edges[edgeID]
		if !exists {
			result.InvalidPaths++
			result.ValidationErrors = append(result.ValidationErrors, PathValidationError{
				PathID:    path.ID,
				ErrorType: "missing_edge",
				Message:   fmt.Sprintf("Path references non-existent edge: %s", edgeID),
				EdgeID:    &edgeID,
			})
			return
		}
		
		// Validate edge connects the right nodes
		expectedSource := path.Nodes[i]
		expectedTarget := path.Nodes[i+1]
		
		if edge.Source != expectedSource || edge.Target != expectedTarget {
			result.InvalidPaths++
			result.ValidationErrors = append(result.ValidationErrors, PathValidationError{
				PathID:    path.ID,
				ErrorType: "edge_mismatch",
				Message:   fmt.Sprintf("Edge %s connects %s->%s but path expects %s->%s", edgeID, edge.Source, edge.Target, expectedSource, expectedTarget),
				EdgeID:    &edgeID,
			})
			return
		}
	}
	
	// Validate path metadata consistency
	if path.Metadata != nil {
		if len(path.Metadata.ReferenceTypes) != len(path.Edges) {
			result.ValidationWarnings = append(result.ValidationWarnings, PathValidationWarning{
				PathID:      path.ID,
				WarningType: "metadata_inconsistency",
				Message:     "Number of reference types doesn't match number of edges",
				Severity:    "low",
			})
		}
		
		if path.Metadata.TotalConfidence < 0 || path.Metadata.TotalConfidence > 1 {
			result.ValidationWarnings = append(result.ValidationWarnings, PathValidationWarning{
				PathID:      path.ID,
				WarningType: "invalid_confidence",
				Message:     fmt.Sprintf("Invalid total confidence: %f", path.Metadata.TotalConfidence),
				Severity:    "medium",
			})
		}
	}
}

// nodeIDsToStrings converts a slice of NodeIDs to strings
func (pt *DefaultPathTracker) nodeIDsToStrings(nodeIDs []NodeID) []string {
	result := make([]string, len(nodeIDs))
	for i, nodeID := range nodeIDs {
		result[i] = string(nodeID)
	}
	return result
}

// clearCache clears the path cache
func (pt *DefaultPathTracker) clearCache() {
	pt.pathCache = make(map[string]interface{})
}