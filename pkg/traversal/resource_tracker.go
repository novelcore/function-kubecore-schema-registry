package traversal

import (
	"sync"
	"time"
	
	"k8s.io/apimachinery/pkg/types"
)

// ResourceTracker tracks processed resources to prevent infinite loops and duplicates
type ResourceTracker struct {
	// processedResources maps resource IDs to processing information
	processedResources map[string]*ProcessedResourceInfo
	
	// processingOrder tracks the order in which resources were processed
	processingOrder []string
	
	// uidIndex maps UIDs to resource IDs for deduplication
	uidIndex map[types.UID]string
	
	// depthIndex groups resources by their discovery depth
	depthIndex map[int][]string
	
	// mu protects access to the tracker
	mu sync.RWMutex
	
	// startTime tracks when tracking started
	startTime time.Time
}

// ProcessedResourceInfo contains information about a processed resource
type ProcessedResourceInfo struct {
	// ResourceID is the unique identifier for the resource
	ResourceID string
	
	// UID is the Kubernetes resource UID
	UID types.UID
	
	// ProcessedAt indicates when the resource was processed
	ProcessedAt time.Time
	
	// Depth is the depth at which the resource was discovered
	Depth int
	
	// ProcessingCount is the number of times this resource has been processed
	ProcessingCount int
	
	// LastProcessedAt is when the resource was last processed
	LastProcessedAt time.Time
	
	// DiscoveryPath contains the path from root to this resource
	DiscoveryPath []string
	
	// ReferenceCount is the number of references found in this resource
	ReferenceCount int
	
	// ProcessingTime is the time taken to process this resource
	ProcessingTime time.Duration
	
	// Metadata contains additional processing metadata
	Metadata map[string]interface{}
}

// ResourceTrackerStats contains statistics about resource tracking
type ResourceTrackerStats struct {
	// TotalResources is the total number of unique resources tracked
	TotalResources int
	
	// ResourcesByDepth groups resources by their discovery depth
	ResourcesByDepth map[int]int
	
	// DuplicateAttempts is the number of duplicate processing attempts prevented
	DuplicateAttempts int
	
	// AverageProcessingTime is the average time taken to process resources
	AverageProcessingTime time.Duration
	
	// TotalProcessingTime is the total time spent processing all resources
	TotalProcessingTime time.Duration
	
	// TrackingDuration is the total duration of tracking
	TrackingDuration time.Duration
	
	// MaxDepth is the maximum depth reached during traversal
	MaxDepth int
	
	// UniqueUIDs is the number of unique resource UIDs tracked
	UniqueUIDs int
}

// NewResourceTracker creates a new resource tracker
func NewResourceTracker() *ResourceTracker {
	return &ResourceTracker{
		processedResources: make(map[string]*ProcessedResourceInfo),
		processingOrder:    make([]string, 0),
		uidIndex:          make(map[types.UID]string),
		depthIndex:        make(map[int][]string),
		startTime:         time.Now(),
	}
}

// IsProcessed checks if a resource has been processed
func (rt *ResourceTracker) IsProcessed(resourceID string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	_, exists := rt.processedResources[resourceID]
	return exists
}

// IsProcessedByUID checks if a resource with the given UID has been processed
func (rt *ResourceTracker) IsProcessedByUID(uid types.UID) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	_, exists := rt.uidIndex[uid]
	return exists
}

// MarkProcessed marks a resource as processed
func (rt *ResourceTracker) MarkProcessed(resourceID string, depth int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	now := time.Now()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		// Update existing entry
		info.ProcessingCount++
		info.LastProcessedAt = now
		return
	}
	
	// Create new entry
	info := &ProcessedResourceInfo{
		ResourceID:      resourceID,
		ProcessedAt:     now,
		LastProcessedAt: now,
		Depth:           depth,
		ProcessingCount: 1,
		DiscoveryPath:   make([]string, 0),
		Metadata:        make(map[string]interface{}),
	}
	
	rt.processedResources[resourceID] = info
	rt.processingOrder = append(rt.processingOrder, resourceID)
	
	// Add to depth index
	rt.depthIndex[depth] = append(rt.depthIndex[depth], resourceID)
}

// MarkProcessedWithUID marks a resource as processed with its UID
func (rt *ResourceTracker) MarkProcessedWithUID(resourceID string, uid types.UID, depth int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	now := time.Now()
	
	// Check if UID already exists
	if existingID, exists := rt.uidIndex[uid]; exists {
		if existingID != resourceID {
			// Same UID but different resource ID - update the existing one
			if info, exists := rt.processedResources[existingID]; exists {
				info.ProcessingCount++
				info.LastProcessedAt = now
				return
			}
		}
	}
	
	if info, exists := rt.processedResources[resourceID]; exists {
		// Update existing entry
		info.UID = uid
		info.ProcessingCount++
		info.LastProcessedAt = now
		rt.uidIndex[uid] = resourceID
		return
	}
	
	// Create new entry
	info := &ProcessedResourceInfo{
		ResourceID:      resourceID,
		UID:             uid,
		ProcessedAt:     now,
		LastProcessedAt: now,
		Depth:           depth,
		ProcessingCount: 1,
		DiscoveryPath:   make([]string, 0),
		Metadata:        make(map[string]interface{}),
	}
	
	rt.processedResources[resourceID] = info
	rt.processingOrder = append(rt.processingOrder, resourceID)
	rt.uidIndex[uid] = resourceID
	
	// Add to depth index
	rt.depthIndex[depth] = append(rt.depthIndex[depth], resourceID)
}

// SetDiscoveryPath sets the discovery path for a resource
func (rt *ResourceTracker) SetDiscoveryPath(resourceID string, path []string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		info.DiscoveryPath = make([]string, len(path))
		copy(info.DiscoveryPath, path)
	}
}

// SetReferenceCount sets the reference count for a resource
func (rt *ResourceTracker) SetReferenceCount(resourceID string, count int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		info.ReferenceCount = count
	}
}

// SetProcessingTime sets the processing time for a resource
func (rt *ResourceTracker) SetProcessingTime(resourceID string, duration time.Duration) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		info.ProcessingTime = duration
	}
}

// SetMetadata sets metadata for a resource
func (rt *ResourceTracker) SetMetadata(resourceID string, key string, value interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		info.Metadata[key] = value
	}
}

// GetProcessedResource returns information about a processed resource
func (rt *ResourceTracker) GetProcessedResource(resourceID string) *ProcessedResourceInfo {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	if info, exists := rt.processedResources[resourceID]; exists {
		// Return a copy to prevent external modification
		infoCopy := *info
		infoCopy.DiscoveryPath = make([]string, len(info.DiscoveryPath))
		copy(infoCopy.DiscoveryPath, info.DiscoveryPath)
		infoCopy.Metadata = make(map[string]interface{})
		for k, v := range info.Metadata {
			infoCopy.Metadata[k] = v
		}
		return &infoCopy
	}
	
	return nil
}

// GetResourcesByDepth returns all resources at a specific depth
func (rt *ResourceTracker) GetResourcesByDepth(depth int) []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	if resources, exists := rt.depthIndex[depth]; exists {
		result := make([]string, len(resources))
		copy(result, resources)
		return result
	}
	
	return nil
}

// GetProcessingOrder returns the order in which resources were processed
func (rt *ResourceTracker) GetProcessingOrder() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	result := make([]string, len(rt.processingOrder))
	copy(result, rt.processingOrder)
	return result
}

// GetStats returns statistics about resource tracking
func (rt *ResourceTracker) GetStats() *ResourceTrackerStats {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	stats := &ResourceTrackerStats{
		TotalResources:   len(rt.processedResources),
		ResourcesByDepth: make(map[int]int),
		UniqueUIDs:       len(rt.uidIndex),
		TrackingDuration: time.Since(rt.startTime),
	}
	
	// Calculate depth distribution
	for depth, resources := range rt.depthIndex {
		stats.ResourcesByDepth[depth] = len(resources)
		if depth > stats.MaxDepth {
			stats.MaxDepth = depth
		}
	}
	
	// Calculate processing time statistics
	var totalProcessingTime time.Duration
	duplicateAttempts := 0
	
	for _, info := range rt.processedResources {
		totalProcessingTime += info.ProcessingTime
		if info.ProcessingCount > 1 {
			duplicateAttempts += info.ProcessingCount - 1
		}
	}
	
	stats.TotalProcessingTime = totalProcessingTime
	stats.DuplicateAttempts = duplicateAttempts
	
	if len(rt.processedResources) > 0 {
		stats.AverageProcessingTime = totalProcessingTime / time.Duration(len(rt.processedResources))
	}
	
	return stats
}

// Reset resets the resource tracker to initial state
func (rt *ResourceTracker) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	
	rt.processedResources = make(map[string]*ProcessedResourceInfo)
	rt.processingOrder = make([]string, 0)
	rt.uidIndex = make(map[types.UID]string)
	rt.depthIndex = make(map[int][]string)
	rt.startTime = time.Now()
}

// Size returns the number of tracked resources
func (rt *ResourceTracker) Size() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	return len(rt.processedResources)
}

// HasDuplicates checks if any resources have been processed multiple times
func (rt *ResourceTracker) HasDuplicates() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	for _, info := range rt.processedResources {
		if info.ProcessingCount > 1 {
			return true
		}
	}
	
	return false
}

// GetDuplicates returns resources that have been processed multiple times
func (rt *ResourceTracker) GetDuplicates() []string {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	var duplicates []string
	
	for resourceID, info := range rt.processedResources {
		if info.ProcessingCount > 1 {
			duplicates = append(duplicates, resourceID)
		}
	}
	
	return duplicates
}

// GetMaxDepth returns the maximum depth of processed resources
func (rt *ResourceTracker) GetMaxDepth() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	maxDepth := 0
	for depth := range rt.depthIndex {
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	
	return maxDepth
}

// IsAtDepth checks if there are any resources at the specified depth
func (rt *ResourceTracker) IsAtDepth(depth int) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	resources, exists := rt.depthIndex[depth]
	return exists && len(resources) > 0
}

// CountAtDepth returns the number of resources at a specific depth
func (rt *ResourceTracker) CountAtDepth(depth int) int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	if resources, exists := rt.depthIndex[depth]; exists {
		return len(resources)
	}
	
	return 0
}

// GetResourceIDByUID returns the resource ID for a given UID
func (rt *ResourceTracker) GetResourceIDByUID(uid types.UID) (string, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	
	resourceID, exists := rt.uidIndex[uid]
	return resourceID, exists
}