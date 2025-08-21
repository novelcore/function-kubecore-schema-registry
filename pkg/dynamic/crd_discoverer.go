package dynamic

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CRDDiscoverer interface for discovering and analyzing CRDs
type CRDDiscoverer interface {
	DiscoverCRDs(ctx context.Context, patterns []string) ([]*CRDInfo, error)
	DiscoverWithTimeout(ctx context.Context, patterns []string, timeout time.Duration) ([]*CRDInfo, error)
	GetDiscoveryStatistics() *DiscoveryStatistics
}

// DefaultCRDDiscoverer implements CRD discovery using Kubernetes API
type DefaultCRDDiscoverer struct {
	client    apiextensionsclientset.Interface
	logger    logging.Logger
	cache     *CRDCache
	metrics   *DiscoveryMetrics
	mu        sync.RWMutex
}

// CRDCache provides caching for discovered CRDs
type CRDCache struct {
	entries map[string]*CacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

// CacheEntry represents a cached CRD entry
type CacheEntry struct {
	CRDInfo   *CRDInfo
	CachedAt  time.Time
	ExpiresAt time.Time
}

// DiscoveryMetrics tracks performance metrics
type DiscoveryMetrics struct {
	TotalCRDs       int
	MatchedCRDs     int
	CacheHits       int
	CacheMisses     int
	DiscoveryTime   time.Duration
	ProcessingTime  time.Duration
	Errors          []error
	mu              sync.RWMutex
}

// NewCRDDiscoverer creates a new CRD discoverer
func NewCRDDiscoverer(client apiextensionsclientset.Interface, logger logging.Logger) *DefaultCRDDiscoverer {
	return &DefaultCRDDiscoverer{
		client:  client,
		logger:  logger,
		cache:   NewCRDCache(DefaultCacheTTL),
		metrics: &DiscoveryMetrics{},
	}
}

// NewCRDCache creates a new CRD cache
func NewCRDCache(ttl time.Duration) *CRDCache {
	return &CRDCache{
		entries: make(map[string]*CacheEntry),
		ttl:     ttl,
	}
}

// DiscoverCRDs discovers CRDs matching the given patterns
func (d *DefaultCRDDiscoverer) DiscoverCRDs(ctx context.Context, patterns []string) ([]*CRDInfo, error) {
	return d.DiscoverWithTimeout(ctx, patterns, DefaultDiscoveryTimeout)
}

// DiscoverWithTimeout discovers CRDs with a specified timeout
func (d *DefaultCRDDiscoverer) DiscoverWithTimeout(ctx context.Context, patterns []string, timeout time.Duration) ([]*CRDInfo, error) {
	startTime := time.Now()
	
	d.logger.Info("Starting CRD discovery", "patterns", patterns, "timeout", timeout)
	
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	// Reset metrics
	d.resetMetrics()
	
	// List all CRDs from cluster
	crdList, err := d.client.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		d.recordError(errors.Wrap(err, "failed to list CRDs from cluster"))
		return nil, errors.Wrap(err, "failed to list CRDs from cluster")
	}
	
	d.logger.Info("Found CRDs in cluster", "total", len(crdList.Items))
	d.metrics.TotalCRDs = len(crdList.Items)
	
	// Filter CRDs by patterns
	var matchedCRDs []apiextv1.CustomResourceDefinition
	for _, crd := range crdList.Items {
		if d.matchesAnyPattern(crd.Spec.Group, patterns) {
			matchedCRDs = append(matchedCRDs, crd)
		}
	}
	
	d.logger.Info("CRDs matching patterns", "matched", len(matchedCRDs), "total", len(crdList.Items))
	d.metrics.MatchedCRDs = len(matchedCRDs)
	
	// Process CRDs concurrently
	crdInfos, err := d.processCRDsConcurrently(ctx, matchedCRDs)
	if err != nil {
		return nil, err
	}
	
	// Record timing
	duration := time.Since(startTime)
	d.metrics.DiscoveryTime = duration
	
	d.logger.Info("CRD discovery completed",
		"discovered", len(crdInfos),
		"duration", duration,
		"cache_hits", d.metrics.CacheHits,
		"cache_misses", d.metrics.CacheMisses)
	
	return crdInfos, nil
}

// processCRDsConcurrently processes CRDs using a worker pool
func (d *DefaultCRDDiscoverer) processCRDsConcurrently(ctx context.Context, crds []apiextv1.CustomResourceDefinition) ([]*CRDInfo, error) {
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(DefaultMaxConcurrency) // Limit concurrent workers
	
	var mu sync.Mutex
	var crdInfos []*CRDInfo
	
	for _, crd := range crds {
		crd := crd // capture loop variable
		g.Go(func() error {
			info, err := d.processCRD(gCtx, &crd)
			if err != nil {
				d.logger.Info("Failed to process CRD", "crd", crd.Name, "error", err)
				d.recordError(err)
				return nil // Don't fail the whole operation for one CRD
			}
			
			if info != nil {
				mu.Lock()
				crdInfos = append(crdInfos, info)
				mu.Unlock()
				
				d.logger.Debug("Processed CRD",
					"name", info.Name,
					"group", info.Group,
					"kind", info.Kind,
					"namespaced", info.Namespaced,
					"references", len(info.References))
			}
			
			return nil
		})
	}
	
	if err := g.Wait(); err != nil {
		return nil, err
	}
	
	return crdInfos, nil
}

// processCRD processes a single CRD and extracts information
func (d *DefaultCRDDiscoverer) processCRD(ctx context.Context, crd *apiextv1.CustomResourceDefinition) (*CRDInfo, error) {
	// Check cache first
	cacheKey := d.getCacheKey(crd)
	if cached := d.cache.Get(cacheKey); cached != nil {
		d.recordCacheHit()
		return cached, nil
	}
	
	d.recordCacheMiss()
	
	// Extract basic CRD information
	info, err := d.extractCRDInfo(crd)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to extract info from CRD %s", crd.Name)
	}
	
	// Cache the result
	d.cache.Set(cacheKey, info)
	
	return info, nil
}

// extractCRDInfo extracts basic information from a CRD
func (d *DefaultCRDDiscoverer) extractCRDInfo(crd *apiextv1.CustomResourceDefinition) (*CRDInfo, error) {
	// Get the latest version
	var latestVersion *apiextv1.CustomResourceDefinitionVersion
	for i := range crd.Spec.Versions {
		version := &crd.Spec.Versions[i]
		if version.Storage || latestVersion == nil {
			latestVersion = version
		}
	}
	
	if latestVersion == nil {
		return nil, fmt.Errorf("no versions found for CRD %s", crd.Name)
	}
	
	// Extract schema
	var schema *ResourceSchema
	if latestVersion.Schema != nil && latestVersion.Schema.OpenAPIV3Schema != nil {
		parsed, err := d.parseOpenAPISchema(latestVersion.Schema.OpenAPIV3Schema)
		if err != nil {
			d.logger.Debug("Failed to parse schema", "crd", crd.Name, "error", err)
			// Continue without schema rather than failing
		} else {
			schema = parsed
		}
	}
	
	// Create CRD info
	info := &CRDInfo{
		Name:       crd.Name,
		Group:      crd.Spec.Group,
		Version:    latestVersion.Name,
		Kind:       crd.Spec.Names.Kind,
		Plural:     crd.Spec.Names.Plural,
		Singular:   crd.Spec.Names.Singular,
		Namespaced: crd.Spec.Scope == apiextv1.NamespaceScoped,
		Schema:     schema,
		Metadata: &CRDMetadata{
			OriginalCRD: crd,
			Labels:      crd.Labels,
			Annotations: crd.Annotations,
			Categories:  crd.Spec.Names.Categories,
			ShortNames:  crd.Spec.Names.ShortNames,
		},
		ParsedAt: time.Now(),
	}
	
	return info, nil
}

// parseOpenAPISchema parses an OpenAPI v3 schema into our ResourceSchema format
func (d *DefaultCRDDiscoverer) parseOpenAPISchema(schema *apiextv1.JSONSchemaProps) (*ResourceSchema, error) {
	if schema == nil {
		return nil, fmt.Errorf("schema is nil")
	}
	
	resourceSchema := &ResourceSchema{
		Fields:      make(map[string]*FieldDefinition),
		Description: schema.Description,
		Required:    schema.Required,
		Properties:  make(map[string]*ResourceSchema),
	}
	
	// Parse properties recursively
	if schema.Properties != nil {
		for propName, propSchema := range schema.Properties {
			field := d.parseFieldDefinition(propName, &propSchema)
			resourceSchema.Fields[propName] = field
		}
	}
	
	return resourceSchema, nil
}

// parseFieldDefinition parses a single field definition
func (d *DefaultCRDDiscoverer) parseFieldDefinition(name string, schema *apiextv1.JSONSchemaProps) *FieldDefinition {
	field := &FieldDefinition{
		Type:        schema.Type,
		Format:      schema.Format,
		Description: schema.Description,
		Required:    false, // Will be set by parent
		Pattern:     schema.Pattern,
		Default:     schema.Default,
	}
	
	// Handle enum values
	if schema.Enum != nil {
		field.Enum = make([]string, len(schema.Enum))
		for i, val := range schema.Enum {
			// Unmarshal JSON to get the actual value
			var enumValue interface{}
			if err := json.Unmarshal(val.Raw, &enumValue); err == nil {
				if str, ok := enumValue.(string); ok {
					field.Enum[i] = str
				} else {
					// Convert other types to string
					field.Enum[i] = fmt.Sprintf("%v", enumValue)
				}
			}
		}
	}
	
	// Handle nested properties
	if schema.Properties != nil {
		field.Properties = make(map[string]*FieldDefinition)
		for propName, propSchema := range schema.Properties {
			field.Properties[propName] = d.parseFieldDefinition(propName, &propSchema)
		}
	}
	
	// Handle array items
	if schema.Items != nil && schema.Items.Schema != nil {
		field.Items = d.parseFieldDefinition("", schema.Items.Schema)
	}
	
	return field
}

// matchesAnyPattern checks if a group matches any of the given patterns
func (d *DefaultCRDDiscoverer) matchesAnyPattern(group string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, err := filepath.Match(pattern, group); err == nil && matched {
			return true
		}
		// Also try exact match for non-wildcard patterns
		if strings.EqualFold(pattern, group) {
			return true
		}
	}
	return false
}

// getCacheKey generates a cache key for a CRD
func (d *DefaultCRDDiscoverer) getCacheKey(crd *apiextv1.CustomResourceDefinition) string {
	return fmt.Sprintf("%s-%s", crd.Name, crd.ResourceVersion)
}

// GetDiscoveryStatistics returns the current discovery statistics
func (d *DefaultCRDDiscoverer) GetDiscoveryStatistics() *DiscoveryStatistics {
	d.metrics.mu.RLock()
	defer d.metrics.mu.RUnlock()
	
	return &DiscoveryStatistics{
		TotalCRDs:     d.metrics.TotalCRDs,
		MatchedCRDs:   d.metrics.MatchedCRDs,
		DiscoveryTime: d.metrics.DiscoveryTime,
		Errors:        d.metrics.Errors,
	}
}

// Cache methods

// Get retrieves an entry from cache if it's not expired
func (c *CRDCache) Get(key string) *CRDInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		return nil
	}
	
	if time.Now().After(entry.ExpiresAt) {
		// Entry expired
		delete(c.entries, key)
		return nil
	}
	
	return entry.CRDInfo
}

// Set stores an entry in cache
func (c *CRDCache) Set(key string, info *CRDInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries[key] = &CacheEntry{
		CRDInfo:   info,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes all entries from cache
func (c *CRDCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.entries = make(map[string]*CacheEntry)
}

// Helper methods for metrics

func (d *DefaultCRDDiscoverer) resetMetrics() {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()
	
	d.metrics.TotalCRDs = 0
	d.metrics.MatchedCRDs = 0
	d.metrics.CacheHits = 0
	d.metrics.CacheMisses = 0
	d.metrics.DiscoveryTime = 0
	d.metrics.ProcessingTime = 0
	d.metrics.Errors = nil
}

func (d *DefaultCRDDiscoverer) recordError(err error) {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()
	
	d.metrics.Errors = append(d.metrics.Errors, err)
}

func (d *DefaultCRDDiscoverer) recordCacheHit() {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()
	
	d.metrics.CacheHits++
}

func (d *DefaultCRDDiscoverer) recordCacheMiss() {
	d.metrics.mu.Lock()
	defer d.metrics.mu.Unlock()
	
	d.metrics.CacheMisses++
}