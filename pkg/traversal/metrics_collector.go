package traversal

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// MetricsCollector collects performance metrics during traversal
type MetricsCollector struct {
	// enabled indicates if metrics collection is enabled
	enabled bool

	// apiRequestLatencies tracks API request latencies
	apiRequestLatencies []time.Duration

	// referenceResolutionLatencies tracks reference resolution latencies
	referenceResolutionLatencies []time.Duration

	// startTime tracks when metrics collection started
	startTime time.Time

	// totalAPIRequests tracks the total number of API requests
	totalAPIRequests int64

	// totalReferencesResolved tracks the total number of references resolved
	totalReferencesResolved int64

	// totalResourcesProcessed tracks the total number of resources processed
	totalResourcesProcessed int64

	// graphBuildingTime tracks time spent building the resource graph
	graphBuildingTime time.Duration

	// cycleDetectionTime tracks time spent detecting cycles
	cycleDetectionTime time.Duration

	// filteringTime tracks time spent filtering resources and references
	filteringTime time.Duration

	// memoryUsageSnapshots tracks memory usage over time
	memoryUsageSnapshots []MemorySnapshot

	// mu protects access to metrics
	mu sync.RWMutex
}

// MemorySnapshot represents a point-in-time memory usage measurement
type MemorySnapshot struct {
	// Timestamp is when the snapshot was taken
	Timestamp time.Time

	// UsedMemory is the amount of memory in use
	UsedMemory int64

	// AllocatedMemory is the amount of memory allocated
	AllocatedMemory int64

	// GCCount is the number of garbage collections performed
	GCCount int64

	// Context provides additional context about when the snapshot was taken
	Context string
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector(enabled bool) *MetricsCollector {
	return &MetricsCollector{
		enabled:                      enabled,
		apiRequestLatencies:          make([]time.Duration, 0),
		referenceResolutionLatencies: make([]time.Duration, 0),
		memoryUsageSnapshots:         make([]MemorySnapshot, 0),
		startTime:                    time.Now(),
	}
}

// IsEnabled returns whether metrics collection is enabled
func (mc *MetricsCollector) IsEnabled() bool {
	return mc.enabled
}

// RecordAPIRequestLatency records the latency of an API request
func (mc *MetricsCollector) RecordAPIRequestLatency(latency time.Duration) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.apiRequestLatencies = append(mc.apiRequestLatencies, latency)
	mc.totalAPIRequests++
}

// RecordReferenceResolutionLatency records the latency of reference resolution
func (mc *MetricsCollector) RecordReferenceResolutionLatency(latency time.Duration) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.referenceResolutionLatencies = append(mc.referenceResolutionLatencies, latency)
	mc.totalReferencesResolved++
}

// RecordResourceProcessed increments the count of processed resources
func (mc *MetricsCollector) RecordResourceProcessed() {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.totalResourcesProcessed++
}

// RecordGraphBuildingTime records the time spent building the resource graph
func (mc *MetricsCollector) RecordGraphBuildingTime(duration time.Duration) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.graphBuildingTime += duration
}

// RecordCycleDetectionTime records the time spent detecting cycles
func (mc *MetricsCollector) RecordCycleDetectionTime(duration time.Duration) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.cycleDetectionTime += duration
}

// RecordFilteringTime records the time spent filtering
func (mc *MetricsCollector) RecordFilteringTime(duration time.Duration) {
	if !mc.enabled {
		return
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.filteringTime += duration
}

// TakeMemorySnapshot takes a snapshot of current memory usage
func (mc *MetricsCollector) TakeMemorySnapshot(context string) {
	if !mc.enabled {
		return
	}

	// This would typically use runtime.ReadMemStats() but for simplicity
	// we'll create a placeholder implementation
	snapshot := MemorySnapshot{
		Timestamp:       time.Now(),
		UsedMemory:      0, // Would be set from runtime.ReadMemStats()
		AllocatedMemory: 0, // Would be set from runtime.ReadMemStats()
		GCCount:         0, // Would be set from runtime.ReadMemStats()
		Context:         context,
	}

	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.memoryUsageSnapshots = append(mc.memoryUsageSnapshots, snapshot)
}

// GetMetrics returns the collected performance metrics
func (mc *MetricsCollector) GetMetrics() *PerformanceMetrics {
	if !mc.enabled {
		return &PerformanceMetrics{}
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()

	totalTime := time.Since(mc.startTime)

	metrics := &PerformanceMetrics{
		APIRequestLatency:          mc.calculateLatencyStats(mc.apiRequestLatencies),
		ReferenceResolutionLatency: mc.calculateLatencyStats(mc.referenceResolutionLatencies),
		GraphBuildingTime:          mc.graphBuildingTime,
		CycleDetectionTime:         mc.cycleDetectionTime,
		FilteringTime:              mc.filteringTime,
		ThroughputMetrics:          mc.calculateThroughputStats(totalTime),
	}

	return metrics
}

// GetTotalAPIRequests returns the total number of API requests made
func (mc *MetricsCollector) GetTotalAPIRequests() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.totalAPIRequests
}

// GetTotalReferencesResolved returns the total number of references resolved
func (mc *MetricsCollector) GetTotalReferencesResolved() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.totalReferencesResolved
}

// GetTotalResourcesProcessed returns the total number of resources processed
func (mc *MetricsCollector) GetTotalResourcesProcessed() int64 {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return mc.totalResourcesProcessed
}

// GetMemoryUsageSnapshots returns all memory usage snapshots
func (mc *MetricsCollector) GetMemoryUsageSnapshots() []MemorySnapshot {
	if !mc.enabled {
		return nil
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()

	// Return a copy to prevent external modification
	snapshots := make([]MemorySnapshot, len(mc.memoryUsageSnapshots))
	copy(snapshots, mc.memoryUsageSnapshots)

	return snapshots
}

// Reset resets all collected metrics
func (mc *MetricsCollector) Reset() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.apiRequestLatencies = make([]time.Duration, 0)
	mc.referenceResolutionLatencies = make([]time.Duration, 0)
	mc.memoryUsageSnapshots = make([]MemorySnapshot, 0)
	mc.startTime = time.Now()
	mc.totalAPIRequests = 0
	mc.totalReferencesResolved = 0
	mc.totalResourcesProcessed = 0
	mc.graphBuildingTime = 0
	mc.cycleDetectionTime = 0
	mc.filteringTime = 0
}

// GetCollectionDuration returns how long metrics have been collected
func (mc *MetricsCollector) GetCollectionDuration() time.Duration {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	return time.Since(mc.startTime)
}

// Helper methods

// calculateLatencyStats calculates latency statistics from a slice of durations
func (mc *MetricsCollector) calculateLatencyStats(latencies []time.Duration) *LatencyStats {
	if len(latencies) == 0 {
		return &LatencyStats{}
	}

	// Sort latencies for percentile calculations
	sortedLatencies := make([]time.Duration, len(latencies))
	copy(sortedLatencies, latencies)
	sort.Slice(sortedLatencies, func(i, j int) bool {
		return sortedLatencies[i] < sortedLatencies[j]
	})

	// Calculate statistics
	stats := &LatencyStats{
		Min: sortedLatencies[0],
		Max: sortedLatencies[len(sortedLatencies)-1],
	}

	// Calculate average
	var total time.Duration
	for _, latency := range latencies {
		total += latency
	}
	stats.Average = total / time.Duration(len(latencies))

	// Calculate median
	if len(sortedLatencies)%2 == 0 {
		mid := len(sortedLatencies) / 2
		stats.Median = (sortedLatencies[mid-1] + sortedLatencies[mid]) / 2
	} else {
		stats.Median = sortedLatencies[len(sortedLatencies)/2]
	}

	// Calculate percentiles
	stats.P95 = mc.calculatePercentile(sortedLatencies, 0.95)
	stats.P99 = mc.calculatePercentile(sortedLatencies, 0.99)

	return stats
}

// calculatePercentile calculates the specified percentile from sorted latencies
func (mc *MetricsCollector) calculatePercentile(sortedLatencies []time.Duration, percentile float64) time.Duration {
	if len(sortedLatencies) == 0 {
		return 0
	}

	index := int(float64(len(sortedLatencies)) * percentile)
	if index >= len(sortedLatencies) {
		index = len(sortedLatencies) - 1
	}

	return sortedLatencies[index]
}

// calculateThroughputStats calculates throughput statistics
func (mc *MetricsCollector) calculateThroughputStats(totalTime time.Duration) *ThroughputStats {
	stats := &ThroughputStats{}

	if totalTime > 0 {
		seconds := totalTime.Seconds()

		stats.ResourcesPerSecond = float64(mc.totalResourcesProcessed) / seconds
		stats.ReferencesPerSecond = float64(mc.totalReferencesResolved) / seconds
		stats.APICallsPerSecond = float64(mc.totalAPIRequests) / seconds
	}

	return stats
}

// GetSummary returns a summary of collected metrics
func (mc *MetricsCollector) GetSummary() *MetricsSummary {
	if !mc.enabled {
		return &MetricsSummary{
			Enabled: false,
		}
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()

	totalTime := time.Since(mc.startTime)

	summary := &MetricsSummary{
		Enabled:                 true,
		CollectionDuration:      totalTime,
		TotalAPIRequests:        mc.totalAPIRequests,
		TotalReferencesResolved: mc.totalReferencesResolved,
		TotalResourcesProcessed: mc.totalResourcesProcessed,
		GraphBuildingTime:       mc.graphBuildingTime,
		CycleDetectionTime:      mc.cycleDetectionTime,
		FilteringTime:           mc.filteringTime,
	}

	// Calculate average latencies
	if len(mc.apiRequestLatencies) > 0 {
		var total time.Duration
		for _, latency := range mc.apiRequestLatencies {
			total += latency
		}
		summary.AverageAPILatency = total / time.Duration(len(mc.apiRequestLatencies))
	}

	if len(mc.referenceResolutionLatencies) > 0 {
		var total time.Duration
		for _, latency := range mc.referenceResolutionLatencies {
			total += latency
		}
		summary.AverageReferenceResolutionLatency = total / time.Duration(len(mc.referenceResolutionLatencies))
	}

	// Calculate throughput
	if totalTime > 0 {
		seconds := totalTime.Seconds()
		summary.ResourcesPerSecond = float64(mc.totalResourcesProcessed) / seconds
		summary.ReferencesPerSecond = float64(mc.totalReferencesResolved) / seconds
		summary.APICallsPerSecond = float64(mc.totalAPIRequests) / seconds
	}

	return summary
}

// MetricsSummary provides a high-level summary of metrics
type MetricsSummary struct {
	// Enabled indicates if metrics collection was enabled
	Enabled bool

	// CollectionDuration is how long metrics were collected
	CollectionDuration time.Duration

	// TotalAPIRequests is the total number of API requests made
	TotalAPIRequests int64

	// TotalReferencesResolved is the total number of references resolved
	TotalReferencesResolved int64

	// TotalResourcesProcessed is the total number of resources processed
	TotalResourcesProcessed int64

	// AverageAPILatency is the average API request latency
	AverageAPILatency time.Duration

	// AverageReferenceResolutionLatency is the average reference resolution latency
	AverageReferenceResolutionLatency time.Duration

	// GraphBuildingTime is the total time spent building graphs
	GraphBuildingTime time.Duration

	// CycleDetectionTime is the total time spent detecting cycles
	CycleDetectionTime time.Duration

	// FilteringTime is the total time spent filtering
	FilteringTime time.Duration

	// ResourcesPerSecond is the resource processing throughput
	ResourcesPerSecond float64

	// ReferencesPerSecond is the reference resolution throughput
	ReferencesPerSecond float64

	// APICallsPerSecond is the API call throughput
	APICallsPerSecond float64
}

// String returns a string representation of the metrics summary
func (ms *MetricsSummary) String() string {
	if !ms.Enabled {
		return "Metrics collection disabled"
	}

	return fmt.Sprintf(
		"Metrics Summary: Duration=%v, Resources=%d (%.2f/s), References=%d (%.2f/s), API=%d (%.2f/s), AvgAPILatency=%v, AvgRefLatency=%v",
		ms.CollectionDuration,
		ms.TotalResourcesProcessed, ms.ResourcesPerSecond,
		ms.TotalReferencesResolved, ms.ReferencesPerSecond,
		ms.TotalAPIRequests, ms.APICallsPerSecond,
		ms.AverageAPILatency,
		ms.AverageReferenceResolutionLatency,
	)
}
