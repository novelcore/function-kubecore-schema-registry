package cache

import (
	"sync"
	"time"

	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
)

// MemoryCache implements the CacheProvider interface using in-memory storage
type MemoryCache struct {
	cache map[string]*domain.CachedSchema
	ttl   time.Duration
	mu    sync.RWMutex
}

// NewMemoryCache creates a new memory-based cache
func NewMemoryCache(ttl time.Duration) interfaces.CacheProvider {
	return &MemoryCache{
		cache: make(map[string]*domain.CachedSchema),
		ttl:   ttl,
	}
}

// Get retrieves a cached schema if not expired
func (m *MemoryCache) Get(key string) (*domain.SchemaInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	cached, exists := m.cache[key]
	if !exists {
		return nil, false
	}
	
	// Check if cache entry is still valid
	if time.Since(cached.Timestamp) > m.ttl {
		delete(m.cache, key)
		return nil, false
	}
	
	return cached.Schema, true
}

// Set stores a schema in cache with timestamp
func (m *MemoryCache) Set(key string, schema *domain.SchemaInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.cache[key] = &domain.CachedSchema{
		Schema:    schema,
		Timestamp: time.Now(),
	}
}

// Size returns the current number of cached items
func (m *MemoryCache) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.cache)
}

// Clear removes all cached items
func (m *MemoryCache) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]*domain.CachedSchema)
}

// cleanup removes expired entries from the cache
func (m *MemoryCache) cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	now := time.Now()
	for key, cached := range m.cache {
		if now.Sub(cached.Timestamp) > m.ttl {
			delete(m.cache, key)
		}
	}
}

// StartCleanupRoutine starts a background routine to clean up expired entries
func (m *MemoryCache) StartCleanupRoutine(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for range ticker.C {
			m.cleanup()
		}
	}()
}