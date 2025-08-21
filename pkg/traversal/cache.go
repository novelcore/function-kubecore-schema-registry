package traversal

import (
	"container/list"
	"sync"
	"time"
)

// Cache provides execution-scoped caching for traversal operations
type Cache interface {
	// Get retrieves a value from the cache
	Get(key string) (interface{}, bool)
	
	// Set stores a value in the cache with TTL
	Set(key string, value interface{}, ttl time.Duration)
	
	// Delete removes a value from the cache
	Delete(key string)
	
	// Clear removes all values from the cache
	Clear()
	
	// Size returns the current cache size
	Size() int
	
	// Stats returns cache statistics
	Stats() *CacheStats
	
	// Cleanup removes expired entries
	Cleanup()
}

// CacheStats contains cache statistics
type CacheStats struct {
	// Size is the current number of entries
	Size int
	
	// Capacity is the maximum number of entries
	Capacity int
	
	// Hits is the number of cache hits
	Hits int64
	
	// Misses is the number of cache misses
	Misses int64
	
	// Evictions is the number of evicted entries
	Evictions int64
	
	// ExpiredEntries is the number of expired entries cleaned up
	ExpiredEntries int64
	
	// HitRate is the cache hit rate
	HitRate float64
}

// CacheEntry represents a cached entry
type CacheEntry struct {
	// Key is the cache key
	Key string
	
	// Value is the cached value
	Value interface{}
	
	// ExpiresAt is when the entry expires
	ExpiresAt time.Time
	
	// AccessedAt is when the entry was last accessed
	AccessedAt time.Time
	
	// CreatedAt is when the entry was created
	CreatedAt time.Time
	
	// AccessCount is how many times the entry has been accessed
	AccessCount int64
}

// LRUCache implements an LRU (Least Recently Used) cache
type LRUCache struct {
	// capacity is the maximum number of entries
	capacity int
	
	// defaultTTL is the default time-to-live for entries
	defaultTTL time.Duration
	
	// entries maps keys to list elements
	entries map[string]*list.Element
	
	// lruList maintains the LRU order
	lruList *list.List
	
	// stats tracks cache statistics
	stats *CacheStats
	
	// mu protects access to the cache
	mu sync.RWMutex
	
	// cleanupTicker triggers periodic cleanup
	cleanupTicker *time.Ticker
	
	// stopCleanup stops the cleanup goroutine
	stopCleanup chan struct{}
}

// NewLRUCache creates a new LRU cache with the specified capacity and default TTL
func NewLRUCache(capacity int, defaultTTL time.Duration) *LRUCache {
	cache := &LRUCache{
		capacity:   capacity,
		defaultTTL: defaultTTL,
		entries:    make(map[string]*list.Element),
		lruList:    list.New(),
		stats: &CacheStats{
			Capacity: capacity,
		},
		stopCleanup: make(chan struct{}),
	}
	
	// Start cleanup goroutine
	cache.cleanupTicker = time.NewTicker(defaultTTL / 4) // Cleanup every quarter of TTL
	go cache.cleanupLoop()
	
	return cache
}

// Get retrieves a value from the cache
func (c *LRUCache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	element, exists := c.entries[key]
	if !exists {
		c.stats.Misses++
		c.updateHitRate()
		return nil, false
	}
	
	entry := element.Value.(*CacheEntry)
	
	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.removeElement(element)
		c.stats.Misses++
		c.stats.ExpiredEntries++
		c.updateHitRate()
		return nil, false
	}
	
	// Update access information
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	
	// Move to front (most recently used)
	c.lruList.MoveToFront(element)
	
	c.stats.Hits++
	c.updateHitRate()
	
	return entry.Value, true
}

// Set stores a value in the cache with TTL
func (c *LRUCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	if ttl <= 0 {
		ttl = c.defaultTTL
	}
	
	// Check if entry already exists
	if element, exists := c.entries[key]; exists {
		// Update existing entry
		entry := element.Value.(*CacheEntry)
		entry.Value = value
		entry.ExpiresAt = now.Add(ttl)
		entry.AccessedAt = now
		entry.AccessCount++
		
		// Move to front
		c.lruList.MoveToFront(element)
		return
	}
	
	// Create new entry
	entry := &CacheEntry{
		Key:         key,
		Value:       value,
		ExpiresAt:   now.Add(ttl),
		AccessedAt:  now,
		CreatedAt:   now,
		AccessCount: 1,
	}
	
	// Add to front of list
	element := c.lruList.PushFront(entry)
	c.entries[key] = element
	
	c.stats.Size++
	
	// Evict least recently used entries if over capacity
	for c.lruList.Len() > c.capacity {
		c.evictLRU()
	}
}

// Delete removes a value from the cache
func (c *LRUCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if element, exists := c.entries[key]; exists {
		c.removeElement(element)
	}
}

// Clear removes all values from the cache
func (c *LRUCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries = make(map[string]*list.Element)
	c.lruList.Init()
	c.stats.Size = 0
}

// Size returns the current cache size
func (c *LRUCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.stats.Size
}

// Stats returns cache statistics
func (c *LRUCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Return a copy to prevent concurrent access
	return &CacheStats{
		Size:           c.stats.Size,
		Capacity:       c.stats.Capacity,
		Hits:           c.stats.Hits,
		Misses:         c.stats.Misses,
		Evictions:      c.stats.Evictions,
		ExpiredEntries: c.stats.ExpiredEntries,
		HitRate:        c.stats.HitRate,
	}
}

// Cleanup removes expired entries
func (c *LRUCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	var expiredKeys []string
	
	// Find expired entries
	for key, element := range c.entries {
		entry := element.Value.(*CacheEntry)
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	
	// Remove expired entries
	for _, key := range expiredKeys {
		if element, exists := c.entries[key]; exists {
			c.removeElement(element)
			c.stats.ExpiredEntries++
		}
	}
}

// Close stops the cache cleanup goroutine
func (c *LRUCache) Close() {
	close(c.stopCleanup)
	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}
}

// Helper methods

// cleanupLoop runs periodic cleanup of expired entries
func (c *LRUCache) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.Cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// evictLRU evicts the least recently used entry
func (c *LRUCache) evictLRU() {
	element := c.lruList.Back()
	if element != nil {
		c.removeElement(element)
		c.stats.Evictions++
	}
}

// removeElement removes an element from the cache
func (c *LRUCache) removeElement(element *list.Element) {
	entry := element.Value.(*CacheEntry)
	delete(c.entries, entry.Key)
	c.lruList.Remove(element)
	c.stats.Size--
}

// updateHitRate calculates and updates the hit rate
func (c *LRUCache) updateHitRate() {
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRate = float64(c.stats.Hits) / float64(total)
	} else {
		c.stats.HitRate = 0
	}
}

// TTLCache implements a TTL-based cache
type TTLCache struct {
	// entries maps keys to cache entries
	entries map[string]*CacheEntry
	
	// stats tracks cache statistics
	stats *CacheStats
	
	// mu protects access to the cache
	mu sync.RWMutex
	
	// cleanupTicker triggers periodic cleanup
	cleanupTicker *time.Ticker
	
	// stopCleanup stops the cleanup goroutine
	stopCleanup chan struct{}
}

// NewTTLCache creates a new TTL-based cache
func NewTTLCache(cleanupInterval time.Duration) *TTLCache {
	cache := &TTLCache{
		entries:     make(map[string]*CacheEntry),
		stats:       &CacheStats{},
		stopCleanup: make(chan struct{}),
	}
	
	// Start cleanup goroutine
	cache.cleanupTicker = time.NewTicker(cleanupInterval)
	go cache.cleanupLoop()
	
	return cache
}

// Get retrieves a value from the TTL cache
func (c *TTLCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		c.stats.Misses++
		return nil, false
	}
	
	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, key)
		c.stats.Size--
		c.stats.ExpiredEntries++
		c.stats.Misses++
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}
	
	// Update access information
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	
	c.stats.Hits++
	
	return entry.Value, true
}

// Set stores a value in the TTL cache
func (c *TTLCache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	
	entry := &CacheEntry{
		Key:         key,
		Value:       value,
		ExpiresAt:   now.Add(ttl),
		AccessedAt:  now,
		CreatedAt:   now,
		AccessCount: 1,
	}
	
	// Check if entry already exists
	if _, exists := c.entries[key]; !exists {
		c.stats.Size++
	}
	
	c.entries[key] = entry
}

// Delete removes a value from the TTL cache
func (c *TTLCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	if _, exists := c.entries[key]; exists {
		delete(c.entries, key)
		c.stats.Size--
	}
}

// Clear removes all values from the TTL cache
func (c *TTLCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries = make(map[string]*CacheEntry)
	c.stats.Size = 0
}

// Size returns the current cache size
func (c *TTLCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	return c.stats.Size
}

// Stats returns cache statistics
func (c *TTLCache) Stats() *CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	// Calculate hit rate
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRate = float64(c.stats.Hits) / float64(total)
	}
	
	// Return a copy
	return &CacheStats{
		Size:           c.stats.Size,
		Capacity:       c.stats.Capacity,
		Hits:           c.stats.Hits,
		Misses:         c.stats.Misses,
		Evictions:      c.stats.Evictions,
		ExpiredEntries: c.stats.ExpiredEntries,
		HitRate:        c.stats.HitRate,
	}
}

// Cleanup removes expired entries from the TTL cache
func (c *TTLCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	now := time.Now()
	var expiredKeys []string
	
	// Find expired entries
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	
	// Remove expired entries
	for _, key := range expiredKeys {
		delete(c.entries, key)
		c.stats.Size--
		c.stats.ExpiredEntries++
	}
}

// Close stops the TTL cache cleanup goroutine
func (c *TTLCache) Close() {
	close(c.stopCleanup)
	if c.cleanupTicker != nil {
		c.cleanupTicker.Stop()
	}
}

// cleanupLoop runs periodic cleanup of expired entries for TTL cache
func (c *TTLCache) cleanupLoop() {
	for {
		select {
		case <-c.cleanupTicker.C:
			c.Cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// NoOpCache implements a no-operation cache (for disabling caching)
type NoOpCache struct{}

// NewNoOpCache creates a new no-op cache
func NewNoOpCache() *NoOpCache {
	return &NoOpCache{}
}

// Get always returns cache miss
func (c *NoOpCache) Get(key string) (interface{}, bool) {
	return nil, false
}

// Set does nothing
func (c *NoOpCache) Set(key string, value interface{}, ttl time.Duration) {
	// No-op
}

// Delete does nothing
func (c *NoOpCache) Delete(key string) {
	// No-op
}

// Clear does nothing
func (c *NoOpCache) Clear() {
	// No-op
}

// Size always returns 0
func (c *NoOpCache) Size() int {
	return 0
}

// Stats returns empty stats
func (c *NoOpCache) Stats() *CacheStats {
	return &CacheStats{}
}

// Cleanup does nothing
func (c *NoOpCache) Cleanup() {
	// No-op
}