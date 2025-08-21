package graph

import (
	"time"
)

// CycleDetector provides functionality to detect cycles in resource dependency graphs
type CycleDetector interface {
	// DetectCycles detects all cycles in the graph
	DetectCycles(graph *ResourceGraph) *CycleDetectionResult
	
	// HasCycle checks if the graph contains any cycles
	HasCycle(graph *ResourceGraph) bool
	
	// DetectCyclesFromNode detects cycles starting from a specific node
	DetectCyclesFromNode(graph *ResourceGraph, startNode NodeID) *CycleDetectionResult
	
	// FindStronglyConnectedComponents finds strongly connected components
	FindStronglyConnectedComponents(graph *ResourceGraph) *SCCResult
}

// CycleDetectionResult contains the result of cycle detection
type CycleDetectionResult struct {
	// CyclesFound indicates if any cycles were detected
	CyclesFound bool
	
	// Cycles contains all detected cycles
	Cycles []DetectedCycle
	
	// TotalCycles is the number of cycles found
	TotalCycles int
	
	// SimpleCycles contains cycles with no repeated nodes (except start/end)
	SimpleCycles []DetectedCycle
	
	// ComplexCycles contains cycles with repeated nodes
	ComplexCycles []DetectedCycle
	
	// DetectionMetadata contains metadata about the detection process
	DetectionMetadata *CycleDetectionMetadata
}

// DetectedCycle represents a detected cycle with additional metadata
type DetectedCycle struct {
	// Cycle contains the basic cycle information
	Cycle
	
	// CycleLength is the number of edges in the cycle
	CycleLength int
	
	// IsSimple indicates if this is a simple cycle (no repeated nodes)
	IsSimple bool
	
	// Weight is the total weight of edges in the cycle
	Weight float64
	
	// DetectionMethod indicates how the cycle was detected
	DetectionMethod string
	
	// ReferenceTypes contains the types of references involved in the cycle
	ReferenceTypes []RelationType
}

// SCCResult contains the result of strongly connected components analysis
type SCCResult struct {
	// Components contains all strongly connected components
	Components []StronglyConnectedComponent
	
	// TotalComponents is the number of components found
	TotalComponents int
	
	// LargestComponent is the component with the most nodes
	LargestComponent *StronglyConnectedComponent
	
	// CyclicComponents contains only components with cycles
	CyclicComponents []StronglyConnectedComponent
	
	// DetectionTime is the time taken to find components
	DetectionTime time.Duration
}

// StronglyConnectedComponent represents a strongly connected component
type StronglyConnectedComponent struct {
	// Nodes contains all nodes in the component
	Nodes []NodeID
	
	// NodeCount is the number of nodes in the component
	NodeCount int
	
	// HasCycles indicates if this component contains cycles
	HasCycles bool
	
	// InternalEdges contains edges within the component
	InternalEdges []EdgeID
	
	// ComponentID is a unique identifier for this component
	ComponentID int
}

// CycleDetectionMetadata contains metadata about cycle detection
type CycleDetectionMetadata struct {
	// DetectionAlgorithm indicates which algorithm was used
	DetectionAlgorithm string
	
	// DetectionTime is the total time spent detecting cycles
	DetectionTime time.Duration
	
	// NodesAnalyzed is the number of nodes analyzed
	NodesAnalyzed int
	
	// EdgesAnalyzed is the number of edges analyzed
	EdgesAnalyzed int
	
	// MaxDepthReached is the maximum depth reached during detection
	MaxDepthReached int
	
	// BacktrackCount is the number of times backtracking occurred
	BacktrackCount int
}

// DFSCycleDetector implements cycle detection using depth-first search
type DFSCycleDetector struct {
	// maxDepth limits the depth of cycle detection to prevent infinite loops
	maxDepth int
	
	// enableSCC enables strongly connected component analysis
	enableSCC bool
}

// NewDFSCycleDetector creates a new DFS-based cycle detector
func NewDFSCycleDetector(maxDepth int, enableSCC bool) *DFSCycleDetector {
	return &DFSCycleDetector{
		maxDepth:  maxDepth,
		enableSCC: enableSCC,
	}
}

// DetectCycles detects all cycles in the graph using DFS
func (cd *DFSCycleDetector) DetectCycles(graph *ResourceGraph) *CycleDetectionResult {
	startTime := time.Now()
	
	result := &CycleDetectionResult{
		CyclesFound:    false,
		Cycles:         make([]DetectedCycle, 0),
		SimpleCycles:   make([]DetectedCycle, 0),
		ComplexCycles:  make([]DetectedCycle, 0),
		DetectionMetadata: &CycleDetectionMetadata{
			DetectionAlgorithm: "DFS",
			NodesAnalyzed:      len(graph.Nodes),
			EdgesAnalyzed:      len(graph.Edges),
		},
	}
	
	// DFS state tracking
	visited := make(map[NodeID]bool)
	recursionStack := make(map[NodeID]bool)
	pathStack := make([]NodeID, 0)
	edgeStack := make([]EdgeID, 0)
	
	// Visit all nodes to catch cycles not reachable from root nodes
	for nodeID := range graph.Nodes {
		if !visited[nodeID] {
			cd.dfsDetectCycles(graph, nodeID, visited, recursionStack, pathStack, edgeStack, 0, result)
		}
	}
	
	result.TotalCycles = len(result.Cycles)
	result.CyclesFound = result.TotalCycles > 0
	result.DetectionMetadata.DetectionTime = time.Since(startTime)
	
	// Classify cycles as simple or complex
	for _, cycle := range result.Cycles {
		if cycle.IsSimple {
			result.SimpleCycles = append(result.SimpleCycles, cycle)
		} else {
			result.ComplexCycles = append(result.ComplexCycles, cycle)
		}
	}
	
	return result
}

// HasCycle checks if the graph contains any cycles
func (cd *DFSCycleDetector) HasCycle(graph *ResourceGraph) bool {
	visited := make(map[NodeID]bool)
	recursionStack := make(map[NodeID]bool)
	
	// Quick check - visit all nodes
	for nodeID := range graph.Nodes {
		if !visited[nodeID] {
			if cd.dfsHasCycle(graph, nodeID, visited, recursionStack) {
				return true
			}
		}
	}
	
	return false
}

// DetectCyclesFromNode detects cycles starting from a specific node
func (cd *DFSCycleDetector) DetectCyclesFromNode(graph *ResourceGraph, startNode NodeID) *CycleDetectionResult {
	startTime := time.Now()
	
	result := &CycleDetectionResult{
		CyclesFound:    false,
		Cycles:         make([]DetectedCycle, 0),
		SimpleCycles:   make([]DetectedCycle, 0),
		ComplexCycles:  make([]DetectedCycle, 0),
		DetectionMetadata: &CycleDetectionMetadata{
			DetectionAlgorithm: "DFS_Single_Node",
		},
	}
	
	// Verify start node exists
	if _, exists := graph.Nodes[startNode]; !exists {
		result.DetectionMetadata.DetectionTime = time.Since(startTime)
		return result
	}
	
	visited := make(map[NodeID]bool)
	recursionStack := make(map[NodeID]bool)
	pathStack := make([]NodeID, 0)
	edgeStack := make([]EdgeID, 0)
	
	cd.dfsDetectCycles(graph, startNode, visited, recursionStack, pathStack, edgeStack, 0, result)
	
	result.TotalCycles = len(result.Cycles)
	result.CyclesFound = result.TotalCycles > 0
	result.DetectionMetadata.DetectionTime = time.Since(startTime)
	
	return result
}

// FindStronglyConnectedComponents finds strongly connected components using Tarjan's algorithm
func (cd *DFSCycleDetector) FindStronglyConnectedComponents(graph *ResourceGraph) *SCCResult {
	startTime := time.Now()
	
	result := &SCCResult{
		Components:       make([]StronglyConnectedComponent, 0),
		CyclicComponents: make([]StronglyConnectedComponent, 0),
	}
	
	if !cd.enableSCC {
		result.DetectionTime = time.Since(startTime)
		return result
	}
	
	// Tarjan's algorithm state
	index := 0
	indices := make(map[NodeID]int)
	lowLinks := make(map[NodeID]int)
	onStack := make(map[NodeID]bool)
	stack := make([]NodeID, 0)
	
	// Run Tarjan's algorithm for all unvisited nodes
	for nodeID := range graph.Nodes {
		if _, visited := indices[nodeID]; !visited {
			cd.tarjanSCC(graph, nodeID, &index, indices, lowLinks, onStack, &stack, result)
		}
	}
	
	result.TotalComponents = len(result.Components)
	result.DetectionTime = time.Since(startTime)
	
	// Find largest component
	maxSize := 0
	for i, comp := range result.Components {
		if comp.NodeCount > maxSize {
			maxSize = comp.NodeCount
			result.LargestComponent = &result.Components[i]
		}
		
		// Identify cyclic components (components with more than 1 node or self-loops)
		if comp.NodeCount > 1 || cd.hasSelfLoop(graph, comp.Nodes) {
			comp.HasCycles = true
			result.CyclicComponents = append(result.CyclicComponents, comp)
		}
	}
	
	return result
}

// Helper methods

// dfsDetectCycles performs DFS cycle detection
func (cd *DFSCycleDetector) dfsDetectCycles(graph *ResourceGraph, nodeID NodeID, visited, recursionStack map[NodeID]bool, pathStack []NodeID, edgeStack []EdgeID, depth int, result *CycleDetectionResult) {
	if depth > cd.maxDepth {
		return
	}
	
	if depth > result.DetectionMetadata.MaxDepthReached {
		result.DetectionMetadata.MaxDepthReached = depth
	}
	
	visited[nodeID] = true
	recursionStack[nodeID] = true
	pathStack = append(pathStack, nodeID)
	
	// Explore adjacent nodes
	if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists {
				continue
			}
			
			edgeStack = append(edgeStack, edgeID)
			
			if recursionStack[edge.Target] {
				// Found a cycle - create DetectedCycle
				cycle := cd.createCycleFromPath(graph, pathStack, edgeStack, edge.Target)
				result.Cycles = append(result.Cycles, cycle)
				result.DetectionMetadata.BacktrackCount++
			} else if !visited[edge.Target] {
				cd.dfsDetectCycles(graph, edge.Target, visited, recursionStack, pathStack, edgeStack, depth+1, result)
			}
			
			// Remove edge from stack (backtrack)
			if len(edgeStack) > 0 {
				edgeStack = edgeStack[:len(edgeStack)-1]
			}
		}
	}
	
	// Remove node from recursion stack and path (backtrack)
	recursionStack[nodeID] = false
	if len(pathStack) > 0 {
		pathStack = pathStack[:len(pathStack)-1]
	}
}

// dfsHasCycle performs a quick DFS check for cycles
func (cd *DFSCycleDetector) dfsHasCycle(graph *ResourceGraph, nodeID NodeID, visited, recursionStack map[NodeID]bool) bool {
	visited[nodeID] = true
	recursionStack[nodeID] = true
	
	if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists {
				continue
			}
			
			if recursionStack[edge.Target] {
				return true // Found cycle
			}
			
			if !visited[edge.Target] && cd.dfsHasCycle(graph, edge.Target, visited, recursionStack) {
				return true
			}
		}
	}
	
	recursionStack[nodeID] = false
	return false
}

// createCycleFromPath creates a DetectedCycle from the current path and stack
func (cd *DFSCycleDetector) createCycleFromPath(graph *ResourceGraph, pathStack []NodeID, edgeStack []EdgeID, cycleStart NodeID) DetectedCycle {
	// Find the start of the cycle in the path
	cycleStartIndex := -1
	for i, nodeID := range pathStack {
		if nodeID == cycleStart {
			cycleStartIndex = i
			break
		}
	}
	
	if cycleStartIndex == -1 {
		// This shouldn't happen, but handle gracefully
		return DetectedCycle{
			Cycle: Cycle{
				Nodes:      []NodeID{cycleStart},
				Edges:      []EdgeID{},
				DetectedAt: time.Now(),
				CycleType:  "unknown",
			},
			CycleLength:     0,
			IsSimple:        false,
			DetectionMethod: "DFS",
		}
	}
	
	// Extract cycle nodes and edges
	cycleNodes := make([]NodeID, 0)
	cycleEdges := make([]EdgeID, 0)
	referenceTypes := make([]RelationType, 0)
	
	// Add nodes from cycle start to end of path
	for i := cycleStartIndex; i < len(pathStack); i++ {
		cycleNodes = append(cycleNodes, pathStack[i])
	}
	// Add cycle start node again to complete the cycle
	cycleNodes = append(cycleNodes, cycleStart)
	
	// Add corresponding edges
	if len(edgeStack) > cycleStartIndex {
		for i := cycleStartIndex; i < len(edgeStack); i++ {
			cycleEdges = append(cycleEdges, edgeStack[i])
			if edge, exists := graph.Edges[edgeStack[i]]; exists {
				referenceTypes = append(referenceTypes, edge.RelationType)
			}
		}
	}
	
	// Determine if cycle is simple (no repeated nodes except start/end)
	nodeSet := make(map[NodeID]bool)
	isSimple := true
	for _, nodeID := range cycleNodes[:len(cycleNodes)-1] { // Exclude the repeated start node
		if nodeSet[nodeID] {
			isSimple = false
			break
		}
		nodeSet[nodeID] = true
	}
	
	// Calculate cycle weight (sum of edge confidences)
	weight := 0.0
	for _, edgeID := range cycleEdges {
		if edge, exists := graph.Edges[edgeID]; exists {
			weight += edge.Confidence
		}
	}
	
	cycleType := "simple"
	if !isSimple {
		cycleType = "complex"
	}
	
	return DetectedCycle{
		Cycle: Cycle{
			Nodes:      cycleNodes,
			Edges:      cycleEdges,
			DetectedAt: time.Now(),
			CycleType:  cycleType,
		},
		CycleLength:     len(cycleEdges),
		IsSimple:        isSimple,
		Weight:          weight,
		DetectionMethod: "DFS",
		ReferenceTypes:  referenceTypes,
	}
}

// tarjanSCC implements Tarjan's strongly connected components algorithm
func (cd *DFSCycleDetector) tarjanSCC(graph *ResourceGraph, nodeID NodeID, index *int, indices, lowLinks map[NodeID]int, onStack map[NodeID]bool, stack *[]NodeID, result *SCCResult) {
	// Set the depth index for this node
	indices[nodeID] = *index
	lowLinks[nodeID] = *index
	*index++
	*stack = append(*stack, nodeID)
	onStack[nodeID] = true
	
	// Explore adjacent nodes
	if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
		for _, edgeID := range adjacentEdges {
			edge, edgeExists := graph.Edges[edgeID]
			if !edgeExists {
				continue
			}
			
			if _, visited := indices[edge.Target]; !visited {
				// Successor has not yet been visited; recurse on it
				cd.tarjanSCC(graph, edge.Target, index, indices, lowLinks, onStack, stack, result)
				lowLinks[nodeID] = min(lowLinks[nodeID], lowLinks[edge.Target])
			} else if onStack[edge.Target] {
				// Successor is in stack and hence in the current SCC
				lowLinks[nodeID] = min(lowLinks[nodeID], indices[edge.Target])
			}
		}
	}
	
	// If nodeID is a root node, pop the stack and create an SCC
	if lowLinks[nodeID] == indices[nodeID] {
		component := StronglyConnectedComponent{
			Nodes:         make([]NodeID, 0),
			InternalEdges: make([]EdgeID, 0),
			ComponentID:   len(result.Components),
		}
		
		for {
			w := (*stack)[len(*stack)-1]
			*stack = (*stack)[:len(*stack)-1]
			onStack[w] = false
			component.Nodes = append(component.Nodes, w)
			
			if w == nodeID {
				break
			}
		}
		
		component.NodeCount = len(component.Nodes)
		
		// Find internal edges for this component
		nodeSet := make(map[NodeID]bool)
		for _, n := range component.Nodes {
			nodeSet[n] = true
		}
		
		for _, n := range component.Nodes {
			if adjacentEdges, exists := graph.AdjacencyList[n]; exists {
				for _, edgeID := range adjacentEdges {
					if edge, edgeExists := graph.Edges[edgeID]; edgeExists && nodeSet[edge.Target] {
						component.InternalEdges = append(component.InternalEdges, edgeID)
					}
				}
			}
		}
		
		result.Components = append(result.Components, component)
	}
}

// hasSelfLoop checks if any node in the component has a self-loop
func (cd *DFSCycleDetector) hasSelfLoop(graph *ResourceGraph, nodes []NodeID) bool {
	for _, nodeID := range nodes {
		if adjacentEdges, exists := graph.AdjacencyList[nodeID]; exists {
			for _, edgeID := range adjacentEdges {
				if edge, edgeExists := graph.Edges[edgeID]; edgeExists && edge.Target == nodeID {
					return true
				}
			}
		}
	}
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}