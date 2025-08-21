package graph

import (
	"container/heap"
)

// GraphTraverser provides functionality to traverse resource dependency graphs
type GraphTraverser interface {
	// BreadthFirstTraversal performs breadth-first traversal starting from root nodes
	BreadthFirstTraversal(graph *ResourceGraph, maxDepth int) *TraversalResult

	// DepthFirstTraversal performs depth-first traversal starting from root nodes
	DepthFirstTraversal(graph *ResourceGraph, maxDepth int) *TraversalResult

	// ForwardTraversal follows outbound edges from the given nodes
	ForwardTraversal(graph *ResourceGraph, startNodes []NodeID, maxDepth int) *TraversalResult

	// ReverseTraversal follows inbound edges to the given nodes
	ReverseTraversal(graph *ResourceGraph, targetNodes []NodeID, maxDepth int) *TraversalResult

	// ShortestPath finds the shortest path between two nodes
	ShortestPath(graph *ResourceGraph, source, target NodeID) *PathResult

	// FindAllPaths finds all paths between two nodes up to maxDepth
	FindAllPaths(graph *ResourceGraph, source, target NodeID, maxDepth int) *PathsResult

	// TopologicalSort performs topological sorting of the graph
	TopologicalSort(graph *ResourceGraph) *TopologicalResult
}

// TraversalResult contains the result of a graph traversal
type TraversalResult struct {
	// VisitedNodes contains all nodes visited during traversal in order
	VisitedNodes []NodeID

	// VisitedEdges contains all edges traversed during traversal
	VisitedEdges []EdgeID

	// TraversalOrder indicates the order in which nodes were visited
	TraversalOrder VisitOrder

	// MaxDepthReached is the maximum depth reached during traversal
	MaxDepthReached int

	// NodesByDepth groups visited nodes by their depth level
	NodesByDepth map[int][]NodeID

	// TraversalMetadata contains additional traversal information
	TraversalMetadata *TraversalMetadata
}

// PathResult contains the result of a shortest path search
type PathResult struct {
	// Path contains the sequence of nodes from source to target
	Path []NodeID

	// Edges contains the sequence of edges from source to target
	Edges []EdgeID

	// PathLength is the number of edges in the path
	PathLength int

	// TotalDistance is the sum of edge weights (if applicable)
	TotalDistance float64

	// Found indicates whether a path was found
	Found bool
}

// PathsResult contains the result of an all-paths search
type PathsResult struct {
	// Paths contains all found paths from source to target
	Paths []*PathResult

	// ShortestPath is the shortest path among all found paths
	ShortestPath *PathResult

	// TotalPathsFound is the number of paths found
	TotalPathsFound int

	// SearchDepthReached is the maximum depth reached during search
	SearchDepthReached int
}

// TopologicalResult contains the result of topological sorting
type TopologicalResult struct {
	// SortedNodes contains nodes in topologically sorted order
	SortedNodes []NodeID

	// Levels groups nodes by their topological level
	Levels map[int][]NodeID

	// MaxLevel is the maximum topological level
	MaxLevel int

	// CyclesFound indicates if cycles were detected (prevents valid sorting)
	CyclesFound bool

	// DetectedCycles contains any cycles found during sorting
	DetectedCycles []Cycle
}

// TraversalMetadata contains metadata about a traversal operation
type TraversalMetadata struct {
	// StartNodes contains the nodes where traversal began
	StartNodes []NodeID

	// Direction indicates the direction of traversal
	Direction TraversalDirection

	// SkippedNodes contains nodes that were skipped during traversal
	SkippedNodes []NodeID

	// SkippedEdges contains edges that were skipped during traversal
	SkippedEdges []EdgeID

	// Statistics contains performance statistics
	Statistics *TraversalStatistics
}

// TraversalStatistics contains statistics about traversal performance
type TraversalStatistics struct {
	// NodesVisited is the total number of nodes visited
	NodesVisited int

	// EdgesTraversed is the total number of edges traversed
	EdgesTraversed int

	// NodesSkipped is the number of nodes skipped
	NodesSkipped int

	// EdgesSkipped is the number of edges skipped
	EdgesSkipped int

	// MaxQueueSize is the maximum size of the traversal queue
	MaxQueueSize int

	// MemoryUsage is the estimated memory usage during traversal
	MemoryUsage int64
}

// DefaultGraphTraverser implements GraphTraverser interface
type DefaultGraphTraverser struct {
	// visitationStrategy defines how nodes are selected for visitation
	visitationStrategy VisitationStrategy
}

// VisitationStrategy defines how nodes are prioritized during traversal
type VisitationStrategy interface {
	// ShouldVisit determines if a node should be visited
	ShouldVisit(node *ResourceNode, currentDepth int, maxDepth int) bool

	// ShouldTraverseEdge determines if an edge should be traversed
	ShouldTraverseEdge(edge *ResourceEdge, currentDepth int, maxDepth int) bool

	// GetPriority returns the priority for visiting a node (lower number = higher priority)
	GetPriority(node *ResourceNode, depth int) int
}

// NewDefaultGraphTraverser creates a new default graph traverser
func NewDefaultGraphTraverser(strategy VisitationStrategy) *DefaultGraphTraverser {
	return &DefaultGraphTraverser{
		visitationStrategy: strategy,
	}
}

// BreadthFirstTraversal performs breadth-first traversal starting from root nodes
func (gt *DefaultGraphTraverser) BreadthFirstTraversal(graph *ResourceGraph, maxDepth int) *TraversalResult {
	result := &TraversalResult{
		VisitedNodes:   make([]NodeID, 0),
		VisitedEdges:   make([]EdgeID, 0),
		TraversalOrder: VisitOrderBreadthFirst,
		NodesByDepth:   make(map[int][]NodeID),
		TraversalMetadata: &TraversalMetadata{
			StartNodes: graph.Metadata.RootNodes,
			Direction:  TraversalDirectionForward,
			Statistics: &TraversalStatistics{},
		},
	}

	if len(graph.Metadata.RootNodes) == 0 {
		return result
	}

	visited := make(map[NodeID]bool)
	queue := make([]TraversalQueueItem, 0)

	// Initialize queue with root nodes
	for _, rootID := range graph.Metadata.RootNodes {
		if node, exists := graph.Nodes[rootID]; exists && gt.visitationStrategy.ShouldVisit(node, 0, maxDepth) {
			queue = append(queue, TraversalQueueItem{
				NodeID: rootID,
				Depth:  0,
				Parent: "",
				Path:   []NodeID{rootID},
			})
		}
	}

	// Track maximum queue size for statistics
	maxQueueSize := len(queue)

	for len(queue) > 0 {
		// Dequeue first item
		current := queue[0]
		queue = queue[1:]

		// Skip if already visited
		if visited[current.NodeID] {
			continue
		}

		// Mark as visited
		visited[current.NodeID] = true
		result.VisitedNodes = append(result.VisitedNodes, current.NodeID)
		result.TraversalMetadata.Statistics.NodesVisited++

		// Group by depth
		if result.NodesByDepth[current.Depth] == nil {
			result.NodesByDepth[current.Depth] = make([]NodeID, 0)
		}
		result.NodesByDepth[current.Depth] = append(result.NodesByDepth[current.Depth], current.NodeID)

		// Update max depth reached
		if current.Depth > result.MaxDepthReached {
			result.MaxDepthReached = current.Depth
		}

		// Don't explore further if max depth reached
		if current.Depth >= maxDepth {
			continue
		}

		// Add neighbors to queue
		if adjacentEdges, exists := graph.AdjacencyList[current.NodeID]; exists {
			for _, edgeID := range adjacentEdges {
				edge, edgeExists := graph.Edges[edgeID]
				if !edgeExists {
					continue
				}

				// Check if edge should be traversed
				if !gt.visitationStrategy.ShouldTraverseEdge(edge, current.Depth, maxDepth) {
					result.TraversalMetadata.SkippedEdges = append(result.TraversalMetadata.SkippedEdges, edgeID)
					result.TraversalMetadata.Statistics.EdgesSkipped++
					continue
				}

				// Check if target node should be visited
				targetNode, targetExists := graph.Nodes[edge.Target]
				if !targetExists || visited[edge.Target] {
					continue
				}

				if gt.visitationStrategy.ShouldVisit(targetNode, current.Depth+1, maxDepth) {
					newPath := make([]NodeID, len(current.Path))
					copy(newPath, current.Path)
					newPath = append(newPath, edge.Target)

					queue = append(queue, TraversalQueueItem{
						NodeID: edge.Target,
						Depth:  current.Depth + 1,
						Parent: current.NodeID,
						Path:   newPath,
					})

					result.VisitedEdges = append(result.VisitedEdges, edgeID)
					result.TraversalMetadata.Statistics.EdgesTraversed++
				} else {
					result.TraversalMetadata.SkippedNodes = append(result.TraversalMetadata.SkippedNodes, edge.Target)
					result.TraversalMetadata.Statistics.NodesSkipped++
				}
			}
		}

		// Update max queue size
		if len(queue) > maxQueueSize {
			maxQueueSize = len(queue)
		}
	}

	result.TraversalMetadata.Statistics.MaxQueueSize = maxQueueSize
	return result
}

// DepthFirstTraversal performs depth-first traversal starting from root nodes
func (gt *DefaultGraphTraverser) DepthFirstTraversal(graph *ResourceGraph, maxDepth int) *TraversalResult {
	result := &TraversalResult{
		VisitedNodes:   make([]NodeID, 0),
		VisitedEdges:   make([]EdgeID, 0),
		TraversalOrder: VisitOrderDepthFirst,
		NodesByDepth:   make(map[int][]NodeID),
		TraversalMetadata: &TraversalMetadata{
			StartNodes: graph.Metadata.RootNodes,
			Direction:  TraversalDirectionForward,
			Statistics: &TraversalStatistics{},
		},
	}

	if len(graph.Metadata.RootNodes) == 0 {
		return result
	}

	visited := make(map[NodeID]bool)

	// Perform DFS for each root node
	for _, rootID := range graph.Metadata.RootNodes {
		if node, exists := graph.Nodes[rootID]; exists && gt.visitationStrategy.ShouldVisit(node, 0, maxDepth) {
			gt.dfsVisit(graph, rootID, 0, maxDepth, visited, []NodeID{rootID}, result)
		}
	}

	return result
}

// ForwardTraversal follows outbound edges from the given nodes
func (gt *DefaultGraphTraverser) ForwardTraversal(graph *ResourceGraph, startNodes []NodeID, maxDepth int) *TraversalResult {
	// Temporarily set root nodes to start nodes and perform BFS
	originalRootNodes := graph.Metadata.RootNodes
	graph.Metadata.RootNodes = startNodes

	bfsResult := gt.BreadthFirstTraversal(graph, maxDepth)

	// Restore original root nodes
	graph.Metadata.RootNodes = originalRootNodes

	return bfsResult
}

// ReverseTraversal follows inbound edges to the given nodes
func (gt *DefaultGraphTraverser) ReverseTraversal(graph *ResourceGraph, targetNodes []NodeID, maxDepth int) *TraversalResult {
	result := &TraversalResult{
		VisitedNodes:   make([]NodeID, 0),
		VisitedEdges:   make([]EdgeID, 0),
		TraversalOrder: VisitOrderBreadthFirst,
		NodesByDepth:   make(map[int][]NodeID),
		TraversalMetadata: &TraversalMetadata{
			StartNodes: targetNodes,
			Direction:  TraversalDirectionReverse,
			Statistics: &TraversalStatistics{},
		},
	}

	visited := make(map[NodeID]bool)
	queue := make([]TraversalQueueItem, 0)

	// Initialize queue with target nodes
	for _, targetID := range targetNodes {
		if node, exists := graph.Nodes[targetID]; exists && gt.visitationStrategy.ShouldVisit(node, 0, maxDepth) {
			queue = append(queue, TraversalQueueItem{
				NodeID: targetID,
				Depth:  0,
				Parent: "",
				Path:   []NodeID{targetID},
			})
		}
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.NodeID] {
			continue
		}

		visited[current.NodeID] = true
		result.VisitedNodes = append(result.VisitedNodes, current.NodeID)
		result.TraversalMetadata.Statistics.NodesVisited++

		// Group by depth
		if result.NodesByDepth[current.Depth] == nil {
			result.NodesByDepth[current.Depth] = make([]NodeID, 0)
		}
		result.NodesByDepth[current.Depth] = append(result.NodesByDepth[current.Depth], current.NodeID)

		if current.Depth > result.MaxDepthReached {
			result.MaxDepthReached = current.Depth
		}

		if current.Depth >= maxDepth {
			continue
		}

		// Follow inbound edges (reverse adjacency list)
		if reverseEdges, exists := graph.ReverseAdjacencyList[current.NodeID]; exists {
			for _, edgeID := range reverseEdges {
				edge, edgeExists := graph.Edges[edgeID]
				if !edgeExists {
					continue
				}

				if !gt.visitationStrategy.ShouldTraverseEdge(edge, current.Depth, maxDepth) {
					result.TraversalMetadata.SkippedEdges = append(result.TraversalMetadata.SkippedEdges, edgeID)
					continue
				}

				sourceNode, sourceExists := graph.Nodes[edge.Source]
				if !sourceExists || visited[edge.Source] {
					continue
				}

				if gt.visitationStrategy.ShouldVisit(sourceNode, current.Depth+1, maxDepth) {
					newPath := make([]NodeID, len(current.Path))
					copy(newPath, current.Path)
					newPath = append(newPath, edge.Source)

					queue = append(queue, TraversalQueueItem{
						NodeID: edge.Source,
						Depth:  current.Depth + 1,
						Parent: current.NodeID,
						Path:   newPath,
					})

					result.VisitedEdges = append(result.VisitedEdges, edgeID)
					result.TraversalMetadata.Statistics.EdgesTraversed++
				}
			}
		}
	}

	return result
}

// ShortestPath finds the shortest path between two nodes using Dijkstra's algorithm
func (gt *DefaultGraphTraverser) ShortestPath(graph *ResourceGraph, source, target NodeID) *PathResult {
	result := &PathResult{
		Found: false,
	}

	// Verify source and target exist
	if _, exists := graph.Nodes[source]; !exists {
		return result
	}
	if _, exists := graph.Nodes[target]; !exists {
		return result
	}

	// Use Dijkstra's algorithm with uniform edge weights
	distances := make(map[NodeID]float64)
	previous := make(map[NodeID]NodeID)
	unvisited := &NodePriorityQueue{}
	heap.Init(unvisited)

	// Initialize distances
	for nodeID := range graph.Nodes {
		distances[nodeID] = float64(^uint(0) >> 1) // Max float64
		heap.Push(unvisited, &PriorityQueueItem{
			NodeID:   nodeID,
			Distance: distances[nodeID],
		})
	}
	distances[source] = 0
	heap.Fix(unvisited, 0) // Update source distance in heap

	for unvisited.Len() > 0 {
		current := heap.Pop(unvisited).(*PriorityQueueItem)

		if current.NodeID == target {
			// Found shortest path to target
			result.Found = true
			result.TotalDistance = distances[target]

			// Reconstruct path
			path := make([]NodeID, 0)
			edges := make([]EdgeID, 0)

			currentNode := target
			for currentNode != source {
				path = append([]NodeID{currentNode}, path...)
				prevNode := previous[currentNode]

				// Find edge between previous and current node
				if adjacentEdges, exists := graph.AdjacencyList[prevNode]; exists {
					for _, edgeID := range adjacentEdges {
						if edge, edgeExists := graph.Edges[edgeID]; edgeExists && edge.Target == currentNode {
							edges = append([]EdgeID{edgeID}, edges...)
							break
						}
					}
				}

				currentNode = prevNode
			}
			path = append([]NodeID{source}, path...)

			result.Path = path
			result.Edges = edges
			result.PathLength = len(edges)
			break
		}

		// Update distances to neighbors
		if adjacentEdges, exists := graph.AdjacencyList[current.NodeID]; exists {
			for _, edgeID := range adjacentEdges {
				edge, edgeExists := graph.Edges[edgeID]
				if !edgeExists {
					continue
				}

				// Use uniform weight of 1 for all edges
				alt := distances[current.NodeID] + 1
				if alt < distances[edge.Target] {
					distances[edge.Target] = alt
					previous[edge.Target] = current.NodeID

					// Update heap (simplified - would need proper heap update in production)
					for i, item := range *unvisited {
						if item.NodeID == edge.Target {
							(*unvisited)[i].Distance = alt
							heap.Fix(unvisited, i)
							break
						}
					}
				}
			}
		}
	}

	return result
}

// FindAllPaths finds all paths between two nodes up to maxDepth
func (gt *DefaultGraphTraverser) FindAllPaths(graph *ResourceGraph, source, target NodeID, maxDepth int) *PathsResult {
	result := &PathsResult{
		Paths: make([]*PathResult, 0),
	}

	// Verify source and target exist
	if _, exists := graph.Nodes[source]; !exists {
		return result
	}
	if _, exists := graph.Nodes[target]; !exists {
		return result
	}

	// Use DFS to find all paths
	visited := make(map[NodeID]bool)
	currentPath := []NodeID{source}
	currentEdges := []EdgeID{}

	gt.findAllPathsDFS(graph, source, target, maxDepth, 0, visited, currentPath, currentEdges, result)

	result.TotalPathsFound = len(result.Paths)

	// Find shortest path
	if len(result.Paths) > 0 {
		shortest := result.Paths[0]
		for _, path := range result.Paths[1:] {
			if path.PathLength < shortest.PathLength {
				shortest = path
			}
		}
		result.ShortestPath = shortest
	}

	return result
}

// TopologicalSort performs topological sorting of the graph
func (gt *DefaultGraphTraverser) TopologicalSort(graph *ResourceGraph) *TopologicalResult {
	result := &TopologicalResult{
		SortedNodes:    make([]NodeID, 0),
		Levels:         make(map[int][]NodeID),
		DetectedCycles: make([]Cycle, 0),
	}

	// Calculate in-degrees
	inDegree := make(map[NodeID]int)
	for nodeID := range graph.Nodes {
		inDegree[nodeID] = 0
	}
	for _, edge := range graph.Edges {
		inDegree[edge.Target]++
	}

	// Find nodes with no incoming edges
	queue := make([]NodeID, 0)
	for nodeID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeID)
		}
	}

	level := 0
	for len(queue) > 0 {
		// Process current level
		currentLevel := make([]NodeID, len(queue))
		copy(currentLevel, queue)
		result.Levels[level] = currentLevel

		nextQueue := make([]NodeID, 0)

		for _, nodeID := range queue {
			result.SortedNodes = append(result.SortedNodes, nodeID)

			// Reduce in-degree of adjacent nodes
			if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
				for _, edgeID := range adjacentEdges {
					if edge, edgeExists := graph.Edges[edgeID]; edgeExists {
						inDegree[edge.Target]--
						if inDegree[edge.Target] == 0 {
							nextQueue = append(nextQueue, edge.Target)
						}
					}
				}
			}
		}

		queue = nextQueue
		level++
	}

	result.MaxLevel = level - 1

	// Check for cycles
	if len(result.SortedNodes) != len(graph.Nodes) {
		result.CyclesFound = true
		// Could implement cycle detection algorithm here
		// For now, just mark that cycles exist
	}

	return result
}

// Helper methods

// TraversalQueueItem represents an item in the traversal queue
type TraversalQueueItem struct {
	NodeID NodeID
	Depth  int
	Parent NodeID
	Path   []NodeID
}

// dfsVisit performs depth-first search recursively
func (gt *DefaultGraphTraverser) dfsVisit(graph *ResourceGraph, nodeID NodeID, depth int, maxDepth int, visited map[NodeID]bool, path []NodeID, result *TraversalResult) {
	visited[nodeID] = true
	result.VisitedNodes = append(result.VisitedNodes, nodeID)
	result.TraversalMetadata.Statistics.NodesVisited++

	// Group by depth
	if result.NodesByDepth[depth] == nil {
		result.NodesByDepth[depth] = make([]NodeID, 0)
	}
	result.NodesByDepth[depth] = append(result.NodesByDepth[depth], nodeID)

	if depth > result.MaxDepthReached {
		result.MaxDepthReached = depth
	}

	if depth >= maxDepth {
		return
	}

	// Visit adjacent nodes
	if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists {
				continue
			}

			if !gt.visitationStrategy.ShouldTraverseEdge(edge, depth, maxDepth) {
				result.TraversalMetadata.SkippedEdges = append(result.TraversalMetadata.SkippedEdges, edgeID)
				continue
			}

			if !visited[edge.Target] {
				targetNode, targetExists := graph.Nodes[edge.Target]
				if targetExists && gt.visitationStrategy.ShouldVisit(targetNode, depth+1, maxDepth) {
					newPath := make([]NodeID, len(path))
					copy(newPath, path)
					newPath = append(newPath, edge.Target)

					result.VisitedEdges = append(result.VisitedEdges, edgeID)
					result.TraversalMetadata.Statistics.EdgesTraversed++

					gt.dfsVisit(graph, edge.Target, depth+1, maxDepth, visited, newPath, result)
				}
			}
		}
	}
}

// findAllPathsDFS recursively finds all paths using DFS
func (gt *DefaultGraphTraverser) findAllPathsDFS(graph *ResourceGraph, current, target NodeID, maxDepth, currentDepth int, visited map[NodeID]bool, currentPath []NodeID, currentEdges []EdgeID, result *PathsResult) {
	if currentDepth > result.SearchDepthReached {
		result.SearchDepthReached = currentDepth
	}

	if current == target {
		// Found a path
		pathResult := &PathResult{
			Path:          make([]NodeID, len(currentPath)),
			Edges:         make([]EdgeID, len(currentEdges)),
			PathLength:    len(currentEdges),
			TotalDistance: float64(len(currentEdges)),
			Found:         true,
		}
		copy(pathResult.Path, currentPath)
		copy(pathResult.Edges, currentEdges)

		result.Paths = append(result.Paths, pathResult)
		return
	}

	if currentDepth >= maxDepth {
		return
	}

	visited[current] = true

	// Explore adjacent nodes
	if adjacentEdges, exists := graph.AdjacencyList[current]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists || visited[edge.Target] {
				continue
			}

			// Add to current path
			newPath := make([]NodeID, len(currentPath))
			copy(newPath, currentPath)
			newPath = append(newPath, edge.Target)

			newEdges := make([]EdgeID, len(currentEdges))
			copy(newEdges, currentEdges)
			newEdges = append(newEdges, edgeID)

			gt.findAllPathsDFS(graph, edge.Target, target, maxDepth, currentDepth+1, visited, newPath, newEdges, result)
		}
	}

	visited[current] = false // Backtrack
}

// PriorityQueueItem represents an item in the priority queue for Dijkstra's algorithm
type PriorityQueueItem struct {
	NodeID   NodeID
	Distance float64
	Index    int
}

// NodePriorityQueue implements heap.Interface for Dijkstra's algorithm
type NodePriorityQueue []*PriorityQueueItem

func (pq NodePriorityQueue) Len() int { return len(pq) }

func (pq NodePriorityQueue) Less(i, j int) bool {
	return pq[i].Distance < pq[j].Distance
}

func (pq NodePriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].Index = i
	pq[j].Index = j
}

func (pq *NodePriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*PriorityQueueItem)
	item.Index = n
	*pq = append(*pq, item)
}

func (pq *NodePriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.Index = -1
	*pq = old[0 : n-1]
	return item
}
