package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"google.golang.org/protobuf/types/known/structpb"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// ResourceReference represents a reference to another resource
type ResourceReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"`
	APIVersion string `json:"apiVersion,omitempty"`
}

// SchemaInfo holds schema metadata and structure
type SchemaInfo struct {
	Kind               string                   `json:"kind"`
	APIVersion         string                   `json:"apiVersion"`
	OpenAPIV3Schema    *apiextensionsv1.JSONSchemaProps `json:"openAPIV3Schema,omitempty"`
	ReferenceFields    []string                 `json:"referenceFields"`
	RequiredFields     []string                 `json:"requiredFields"`
	TransitiveRefs     map[string]*SchemaInfo   `json:"transitiveReferences,omitempty"`
	Depth              int                      `json:"depth,omitempty"`
	ReferencePath      string                   `json:"referencePath,omitempty"`
	Source             string                   `json:"source,omitempty"` // "kubernetes-api" | "cache" | "fallback"
}

// ExecutionContext holds extracted context from the XResource
type ExecutionContext struct {
	SourceXResource        string                         `json:"sourceXResource"`
	ClaimName              string                         `json:"claimName"`
	ClaimNamespace         string                         `json:"claimNamespace"`
	DirectReferences       map[string]ResourceReference  `json:"directReferences"`
}

// DiscoveryStats holds metrics about the discovery process
type DiscoveryStats struct {
	TotalReferencesFound int   `json:"totalReferencesFound"`
	MaxDepthReached      int   `json:"maxDepthReached"`
	SchemasRetrieved     int   `json:"schemasRetrieved"`
	RealSchemasFound     int   `json:"realSchemasFound"`
	CacheHits           int   `json:"cacheHits"`
	APICallsMade        int   `json:"apiCalls"`
	ExecutionTimeMs     int64 `json:"executionTimeMs"`
}

// Function implements the schema registry function
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log          logging.Logger
	slogger      *slog.Logger
	k8sClient    clientset.Interface
	schemaCache  *SchemaCache
	mu           sync.RWMutex
}

// CachedSchema represents a cached schema with timestamp
type CachedSchema struct {
	Schema    *SchemaInfo
	Timestamp time.Time
}

// SchemaCache provides caching for CRD schemas with TTL
type SchemaCache struct {
	cache map[string]*CachedSchema
	mu    sync.RWMutex
	ttl   time.Duration
}

// DiscoveryResult holds the result of a schema discovery operation
type DiscoveryResult struct {
	Schema      *SchemaInfo
	Error       error
	Source      string
	Depth       int
	RefPath     string
}

// ReferencePattern represents different types of reference patterns
type ReferencePattern struct {
	FieldName   string
	KindHint    string
	APIVersion  string
	IsArray     bool
}

// NewSchemaCache creates a new schema cache
func NewSchemaCache(ttl time.Duration) *SchemaCache {
	return &SchemaCache{
		cache: make(map[string]*CachedSchema),
		ttl:   ttl,
	}
}

// Get retrieves schema from cache if not expired
func (sc *SchemaCache) Get(key string) (*SchemaInfo, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	cached, exists := sc.cache[key]
	if !exists {
		return nil, false
	}
	// Check if cache entry is still valid
	if time.Since(cached.Timestamp) > sc.ttl {
		delete(sc.cache, key)
		return nil, false
	}
	return cached.Schema, true
}

// Set stores schema in cache with timestamp
func (sc *SchemaCache) Set(key string, schema *SchemaInfo) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache[key] = &CachedSchema{
		Schema:    schema,
		Timestamp: time.Now(),
	}
}

// Size returns the current cache size
func (sc *SchemaCache) Size() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return len(sc.cache)
}

// RunFunction implements the main function logic
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	startTime := time.Now()
	correlationID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	
	f.initializeLogging()
	f.slogger.Info("RunFunction started", 
		"correlationId", correlationID,
		"tag", req.GetMeta().GetTag())

	// Initialize response with TTL
	rsp := response.To(req, response.DefaultTTL)

	// Extract and validate function input
	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		f.slogger.Error("Failed to get function input", 
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrapf(err, "cannot get function input"))
		return rsp, nil
	}

	// Set default values for input parameters
	enableTransitive := true
	if in.EnableTransitiveDiscovery != nil {
		enableTransitive = *in.EnableTransitiveDiscovery
	}
	
	traversalDepth := 3
	if in.TraversalDepth != nil {
		traversalDepth = *in.TraversalDepth
	}
	
	includeFullSchema := true
	if in.IncludeFullSchema != nil {
		includeFullSchema = *in.IncludeFullSchema
	}

	f.slogger.Debug("Function input processed",
		"correlationId", correlationID,
		"enableTransitive", enableTransitive,
		"traversalDepth", traversalDepth,
		"includeFullSchema", includeFullSchema)

	// Extract XR (composite resource)
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		f.slogger.Error("Failed to get observed composite resource",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	// Extract execution context from XR
	execCtx, err := f.extractExecutionContext(ctx, xr, correlationID)
	if err != nil {
		f.slogger.Error("Failed to extract execution context",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "cannot extract execution context"))
		return rsp, nil
	}

	f.slogger.Info("Execution context extracted",
		"correlationId", correlationID,
		"sourceXResource", execCtx.SourceXResource,
		"claimName", execCtx.ClaimName,
		"directReferences", len(execCtx.DirectReferences))

	// Initialize Kubernetes client if not already done (skip in testing environment)
	if f.k8sClient == nil {
		if err := f.initializeK8sClient(); err != nil {
			// In testing environment or local development, we can continue without K8s client
			f.slogger.Warn("Kubernetes client not available, will use fallback schemas for missing CRDs",
				"correlationId", correlationID,
				"error", err)
		}
	}

	// Initialize schema cache if not already done
	if f.schemaCache == nil {
		f.schemaCache = NewSchemaCache(5 * time.Minute)
	}

	// Perform schema discovery
	schemas, stats, err := f.performSchemaDiscovery(ctx, execCtx, enableTransitive, traversalDepth, includeFullSchema, correlationID)
	if err != nil {
		f.slogger.Error("Schema discovery failed",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "schema discovery failed"))
		return rsp, nil
	}

	// Calculate execution time
	stats.ExecutionTimeMs = time.Since(startTime).Milliseconds()

	f.slogger.Info("Schema discovery completed",
		"correlationId", correlationID,
		"schemasRetrieved", stats.SchemasRetrieved,
		"executionTimeMs", stats.ExecutionTimeMs)

	// Build response structure
	responseData := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"type":   "Ready",
					"status": "True",
				},
			},
			"executionContext": execCtx,
			"referencedResourceSchemas": schemas,
			"discoveryStats": stats,
		},
	}

	// IMPORTANT: Don't try to write composite resource status directly
	// This causes managedFields conflicts in pipeline mode with multiple functions
	// Instead, store the data in context and log it for access via other means
	
	// Store the status in context for potential access by other functions  
	statusJSON, _ := json.Marshal(map[string]interface{}{
		"executionContext": execCtx,
		"referencedResourceSchemas": schemas,
		"discoveryStats": stats,
	})
	
	if statusValue, err := structpb.NewValue(string(statusJSON)); err == nil {
		response.SetContextKey(rsp, "kubecore.schemaRegistry.detailedStatus", statusValue)
	}
	
	f.slogger.Info("Schema registry discovery results available", 
		"correlationId", correlationID,
		"schemasCount", len(schemas),
		"referencesCount", stats.TotalReferencesFound,
		"statusSize", len(statusJSON))

	// Convert response data to JSON for logging
	if responseJSON, err := json.Marshal(responseData); err == nil {
		f.slogger.Debug("Schema registry response prepared",
			"correlationId", correlationID,
			"responseSize", len(responseJSON))
		
		response.Normalf(rsp, "Schema registry discovery completed successfully. Found %d schemas in %dms", 
			stats.SchemasRetrieved, stats.ExecutionTimeMs)
	}

	// Set success condition
	response.ConditionTrue(rsp, "FunctionSuccess", "SchemaDiscoveryComplete").
		WithMessage(fmt.Sprintf("Successfully discovered %d schemas with %d references", 
			stats.SchemasRetrieved, stats.TotalReferencesFound)).
		TargetCompositeAndClaim()

	f.slogger.Info("RunFunction completed successfully",
		"correlationId", correlationID,
		"executionTimeMs", stats.ExecutionTimeMs)

	return rsp, nil
}

// initializeLogging sets up structured logging
func (f *Function) initializeLogging() {
	if f.slogger != nil {
		return
	}

	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG_ENABLED") == "true" {
		logLevel = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	f.slogger = slog.New(handler)
}

// extractExecutionContext extracts context from the XResource
func (f *Function) extractExecutionContext(ctx context.Context, xr *resource.Composite, correlationID string) (*ExecutionContext, error) {
	f.slogger.Debug("Extracting execution context", "correlationId", correlationID)

	if xr == nil || xr.Resource == nil {
		return nil, fmt.Errorf("composite resource is nil")
	}

	// Get the underlying object from the composite
	xrObj := xr.Resource.Object

	execCtx := &ExecutionContext{
		DirectReferences: make(map[string]ResourceReference),
	}

	// Extract metadata
	if metadata, ok := xrObj["metadata"].(map[string]interface{}); ok {
		if name, ok := metadata["name"].(string); ok {
			execCtx.SourceXResource = name
		}

		// Extract claim information from labels
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			if claimName, ok := labels["crossplane.io/claim-name"].(string); ok {
				execCtx.ClaimName = claimName
			}
			if claimNamespace, ok := labels["crossplane.io/claim-namespace"].(string); ok {
				execCtx.ClaimNamespace = claimNamespace
			}
		}
	}

	// Extract direct references from spec
	if spec, ok := xrObj["spec"].(map[string]interface{}); ok {
		f.extractReferencesFromSpec(spec, execCtx.DirectReferences, correlationID)
	}

	f.slogger.Debug("Execution context extracted successfully",
		"correlationId", correlationID,
		"sourceXResource", execCtx.SourceXResource,
		"directReferences", len(execCtx.DirectReferences))

	return execCtx, nil
}

// extractReferencesFromSpec recursively extracts reference fields from spec with enhanced patterns
func (f *Function) extractReferencesFromSpec(spec map[string]interface{}, refs map[string]ResourceReference, correlationID string) {
	for key, value := range spec {
		// Enhanced reference pattern detection
		if f.isReferenceField(key) {
			f.processReferenceField(key, value, refs, correlationID)
		} else if nested, ok := value.(map[string]interface{}); ok {
			f.extractReferencesFromSpec(nested, refs, correlationID)
		}
	}
}

// isReferenceField determines if a field name indicates a reference
func (f *Function) isReferenceField(fieldName string) bool {
	// Enhanced patterns beyond just *Ref
	patterns := []string{
		"Ref$",           // ends with Ref
		"Refs$",          // ends with Refs (arrays)
		"Reference$",     // ends with Reference
		"References$",    // ends with References
		"Config$",        // configuration references
		"Provider$",      // provider references
		"Secret$",        // secret references
	}

	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, fieldName); matched {
			return true
		}
	}

	// Additional logic for nested reference patterns
	if strings.Contains(fieldName, "providerConfig") ||
		strings.Contains(fieldName, "secretRef") ||
		strings.Contains(fieldName, "configMapRef") {
		return true
	}

	return false
}

// processReferenceField processes a reference field and extracts reference info
func (f *Function) processReferenceField(fieldName string, value interface{}, refs map[string]ResourceReference, correlationID string) {
	switch v := value.(type) {
	case map[string]interface{}:
		ref := ResourceReference{}
		if name, ok := v["name"].(string); ok {
			ref.Name = name
		}
		if namespace, ok := v["namespace"].(string); ok {
			ref.Namespace = namespace
		}
		if kind, ok := v["kind"].(string); ok {
			ref.Kind = kind
		}
		if apiVersion, ok := v["apiVersion"].(string); ok {
			ref.APIVersion = apiVersion
		}

		// If kind/apiVersion not provided in the reference, infer from field name
		if ref.Kind == "" || ref.APIVersion == "" {
			inferred := f.inferReferenceTarget(fieldName, nil, correlationID)
			if inferred != nil {
				if ref.Kind == "" {
					ref.Kind = inferred.Kind
				}
				if ref.APIVersion == "" {
					ref.APIVersion = inferred.APIVersion
				}
			}
		}

		if ref.Name != "" {
			refs[fieldName] = ref
			f.slogger.Debug("Reference field processed",
				"correlationId", correlationID,
				"fieldName", fieldName,
				"refName", ref.Name,
				"kind", ref.Kind,
				"apiVersion", ref.APIVersion)
		}
	case []interface{}:
		// Handle array of references
		for i, item := range v {
			if refMap, ok := item.(map[string]interface{}); ok {
				arrayFieldName := fmt.Sprintf("%s[%d]", fieldName, i)
				f.processReferenceField(arrayFieldName, refMap, refs, correlationID)
			}
		}
	}
}

// initializeK8sClient initializes the Kubernetes client
func (f *Function) initializeK8sClient() error {
	config, err := rest.InClusterConfig()
	if err != nil {
		// For local development, try to use kubeconfig
		return fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	f.k8sClient, err = clientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Initialize schema cache
	f.schemaCache = NewSchemaCache(5 * time.Minute)

	return nil
}

// performSchemaDiscovery performs the main schema discovery logic with enhanced metrics
func (f *Function) performSchemaDiscovery(ctx context.Context, execCtx *ExecutionContext, enableTransitive bool, traversalDepth int, includeFullSchema bool, correlationID string) (map[string]*SchemaInfo, *DiscoveryStats, error) {
	f.slogger.Debug("Starting schema discovery",
		"correlationId", correlationID,
		"enableTransitive", enableTransitive,
		"traversalDepth", traversalDepth)

	schemas := make(map[string]*SchemaInfo)
	stats := &DiscoveryStats{}
	visited := make(map[string]bool)

	// Discover schemas for direct references
	for fieldName, ref := range execCtx.DirectReferences {
		if ref.Name == "" {
			continue
		}

		// Check if schema is in cache before discovery
		schemaKey := f.getSchemaKey(ref)
		if _, exists := f.schemaCache.Get(schemaKey); exists {
			stats.CacheHits++
		} else {
			stats.APICallsMade++
		}

		schema, err := f.discoverSchema(ctx, ref, includeFullSchema, correlationID)
		if err != nil {
			f.slogger.Warn("Failed to discover schema for reference",
				"correlationId", correlationID,
				"fieldName", fieldName,
				"refName", ref.Name,
				"error", err)
			continue
		}

		if schema != nil {
			// Set initial metadata
			schema.Depth = 0
			schema.ReferencePath = fieldName

			schemas[fieldName] = schema
			stats.SchemasRetrieved++

			// Count real vs fallback schemas
			if schema.Source == "kubernetes-api" || schema.Source == "cache" {
				stats.RealSchemasFound++
			}

			visited[f.getSchemaKey(ref)] = true

			// Perform transitive discovery if enabled
			if enableTransitive && traversalDepth > 0 {
				f.performTransitiveDiscovery(ctx, schema, visited, traversalDepth-1, includeFullSchema, correlationID)
				stats.MaxDepthReached = max(stats.MaxDepthReached, traversalDepth)
			}
		}
	}

	stats.TotalReferencesFound = len(execCtx.DirectReferences)

	// Count all transitive schemas for accurate metrics
	f.countTransitiveSchemas(schemas, stats)

	f.slogger.Debug("Schema discovery completed",
		"correlationId", correlationID,
		"schemasFound", len(schemas),
		"statsRetrieved", stats.SchemasRetrieved,
		"realSchemas", stats.RealSchemasFound,
		"cacheHits", stats.CacheHits,
		"apiCalls", stats.APICallsMade)

	return schemas, stats, nil
}

// countTransitiveSchemas recursively counts all transitive schemas for accurate metrics
func (f *Function) countTransitiveSchemas(schemas map[string]*SchemaInfo, stats *DiscoveryStats) {
	for _, schema := range schemas {
		f.countTransitiveInSchema(schema, stats)
	}
}

// countTransitiveInSchema counts transitive schemas within a single schema
func (f *Function) countTransitiveInSchema(schema *SchemaInfo, stats *DiscoveryStats) {
	if schema.TransitiveRefs == nil {
		return
	}

	for _, transitiveSchema := range schema.TransitiveRefs {
		stats.SchemasRetrieved++
		if transitiveSchema.Source == "kubernetes-api" || transitiveSchema.Source == "cache" {
			stats.RealSchemasFound++
		}
		// Recursively count nested transitive schemas
		f.countTransitiveInSchema(transitiveSchema, stats)
	}
}

// discoverSchema discovers schema for a given resource reference using real Kubernetes API
func (f *Function) discoverSchema(ctx context.Context, ref ResourceReference, includeFullSchema bool, correlationID string) (*SchemaInfo, error) {
	schemaKey := f.getSchemaKey(ref)
	
	// Check cache first
	if cached, exists := f.schemaCache.Get(schemaKey); exists {
		f.slogger.Debug("Schema found in cache",
			"correlationId", correlationID,
			"schemaKey", schemaKey)
		cached.Source = "cache"
		return cached, nil
	}

	f.slogger.Debug("Fetching schema from Kubernetes API",
		"correlationId", correlationID,
		"schemaKey", schemaKey,
		"kind", ref.Kind,
		"apiVersion", ref.APIVersion)

	// Try to get real CRD schema from Kubernetes API
	schema, err := f.fetchCRDSchema(ctx, ref, includeFullSchema, correlationID)
	if err != nil {
		f.slogger.Warn("Failed to fetch real CRD schema, using fallback",
			"correlationId", correlationID,
			"schemaKey", schemaKey,
			"error", err)
		
		// Fallback to basic schema structure
		schema = f.createFallbackSchema(ref, includeFullSchema)
		schema.Source = "fallback"
	} else {
		schema.Source = "kubernetes-api"
	}

	// Cache the schema
	f.schemaCache.Set(schemaKey, schema)

	return schema, nil
}

// fetchCRDSchema retrieves the actual CRD schema from Kubernetes API
func (f *Function) fetchCRDSchema(ctx context.Context, ref ResourceReference, includeFullSchema bool, correlationID string) (*SchemaInfo, error) {
	if f.k8sClient == nil {
		return nil, fmt.Errorf("kubernetes client not available")
	}

	// Parse the API version to get group
	group, version, err := f.parseAPIVersion(ref.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse API version %s: %w", ref.APIVersion, err)
	}

	// Construct CRD name (convention: <kind-plural>.<group>)
	crdName := f.constructCRDName(ref.Kind, group)

	f.slogger.Debug("Fetching CRD",
		"correlationId", correlationID,
		"crdName", crdName,
		"group", group,
		"version", version)

	// Get the CRD
	crd, err := f.k8sClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", crdName, err)
	}

	// Find the correct version in the CRD
	var versionSchema *apiextensionsv1.CustomResourceValidation
	for _, v := range crd.Spec.Versions {
		if v.Name == version {
			versionSchema = v.Schema
			break
		}
	}

	if versionSchema == nil || versionSchema.OpenAPIV3Schema == nil {
		return nil, fmt.Errorf("no schema found for version %s in CRD %s", version, crdName)
	}

	// Extract reference fields from the schema
	referenceFields := f.extractReferenceFieldsFromSchema(versionSchema.OpenAPIV3Schema, correlationID)

	// Build SchemaInfo
	schema := &SchemaInfo{
		Kind:            ref.Kind,
		APIVersion:      ref.APIVersion,
		ReferenceFields: referenceFields,
		RequiredFields:  f.extractRequiredFields(versionSchema.OpenAPIV3Schema),
	}

	if includeFullSchema {
		schema.OpenAPIV3Schema = versionSchema.OpenAPIV3Schema
	}

	f.slogger.Debug("Successfully fetched CRD schema",
		"correlationId", correlationID,
		"crdName", crdName,
		"referenceFields", len(referenceFields))

	return schema, nil
}

// createFallbackSchema creates a basic schema when real CRD is not available
func (f *Function) createFallbackSchema(ref ResourceReference, includeFullSchema bool) *SchemaInfo {
	schema := &SchemaInfo{
		Kind:            ref.Kind,
		APIVersion:      ref.APIVersion,
		ReferenceFields: []string{}, // Will be populated by enhanced reference detection
		RequiredFields:  []string{"metadata", "spec"},
	}

	if includeFullSchema {
		schema.OpenAPIV3Schema = &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"metadata": {Type: "object"},
				"spec":     {Type: "object"},
				"status":   {Type: "object"},
			},
			Required: []string{"metadata", "spec"},
		}
	}

	return schema
}

// parseAPIVersion splits apiVersion into group and version
func (f *Function) parseAPIVersion(apiVersion string) (group, version string, err error) {
	if apiVersion == "" {
		return "", "", fmt.Errorf("empty API version")
	}

	// Handle core API (e.g., "v1")
	if !strings.Contains(apiVersion, "/") {
		return "", apiVersion, nil
	}

	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid API version format: %s", apiVersion)
	}

	return parts[0], parts[1], nil
}

// constructCRDName constructs the CRD name from kind and group
func (f *Function) constructCRDName(kind, group string) string {
	// Convert kind to lowercase plural (simplified approach)
	kindLower := strings.ToLower(kind)
	plural := f.makeKindPlural(kindLower)

	if group == "" {
		return plural
	}
	return plural + "." + group
}

// makeKindPlural creates plural form of a kind (simplified)
func (f *Function) makeKindPlural(kind string) string {
	// Simple pluralization rules
	if strings.HasSuffix(kind, "s") {
		return kind + "es"
	}
	if strings.HasSuffix(kind, "y") {
		return strings.TrimSuffix(kind, "y") + "ies"
	}
	return kind + "s"
}

// extractRequiredFields extracts required fields from OpenAPI schema
func (f *Function) extractRequiredFields(schema *apiextensionsv1.JSONSchemaProps) []string {
	if schema == nil {
		return []string{"metadata", "spec"}
	}
	return schema.Required
}

// performTransitiveDiscovery recursively discovers transitive schema dependencies using real CRD analysis
func (f *Function) performTransitiveDiscovery(ctx context.Context, schema *SchemaInfo, visited map[string]bool, remainingDepth int, includeFullSchema bool, correlationID string) {
	if remainingDepth <= 0 {
		return
	}

	f.slogger.Debug("Performing transitive discovery",
		"correlationId", correlationID,
		"schema", schema.Kind,
		"remainingDepth", remainingDepth,
		"referenceFields", len(schema.ReferenceFields))

	if schema.TransitiveRefs == nil {
		schema.TransitiveRefs = make(map[string]*SchemaInfo)
	}

	// Process each reference field found in the schema
	for _, refField := range schema.ReferenceFields {
		f.slogger.Debug("Processing reference field for transitive discovery",
			"correlationId", correlationID,
			"refField", refField,
			"parentKind", schema.Kind)

		// Extract potential reference targets from the schema
		referenceTargets := f.extractReferenceTargets(schema, refField, correlationID)

		for _, refTarget := range referenceTargets {
			transitiveKey := f.getSchemaKey(refTarget)
			if visited[transitiveKey] {
				f.slogger.Debug("Reference already visited, skipping",
					"correlationId", correlationID,
					"refTarget", refTarget.Kind)
				continue
			}

			visited[transitiveKey] = true

			transitiveSchema, err := f.discoverSchema(ctx, refTarget, includeFullSchema, correlationID)
			if err != nil {
				f.slogger.Warn("Failed to discover transitive schema",
					"correlationId", correlationID,
					"refField", refField,
					"refTarget", refTarget.Kind,
					"error", err)
				continue
			}

			if transitiveSchema != nil {
				// Set transitive metadata
				transitiveSchema.Depth = schema.Depth + 1
				if schema.ReferencePath != "" {
					transitiveSchema.ReferencePath = schema.ReferencePath + " -> " + refField
				} else {
					transitiveSchema.ReferencePath = refField
				}

				schema.TransitiveRefs[refField] = transitiveSchema

				f.slogger.Debug("Successfully discovered transitive schema",
					"correlationId", correlationID,
					"refField", refField,
					"transitiveKind", transitiveSchema.Kind,
					"depth", transitiveSchema.Depth)

				// Recursive call for next level
				f.performTransitiveDiscovery(ctx, transitiveSchema, visited, remainingDepth-1, includeFullSchema, correlationID)
			}
		}
	}
}

// extractReferenceTargets extracts potential reference targets from a reference field
func (f *Function) extractReferenceTargets(schema *SchemaInfo, refField string, correlationID string) []ResourceReference {
	var targets []ResourceReference

	// Try to infer kind and apiVersion from the reference field name
	if refTarget := f.inferReferenceTarget(refField, schema, correlationID); refTarget != nil {
		targets = append(targets, *refTarget)
	}

	// If we have the full schema, try to extract from OpenAPI spec
	if schema.OpenAPIV3Schema != nil {
		schemaTargets := f.extractTargetsFromOpenAPISchema(schema.OpenAPIV3Schema, refField, correlationID)
		targets = append(targets, schemaTargets...)
	}

	return targets
}

// inferReferenceTarget tries to infer the target kind and apiVersion from field name patterns
func (f *Function) inferReferenceTarget(refField string, parentSchema *SchemaInfo, correlationID string) *ResourceReference {
	// Common reference field patterns
	patterns := map[string]ReferencePattern{
		"githubProjectRef":    {KindHint: "GitHubProject", APIVersion: "github.platform.kubecore.io/v1alpha1"},
		"githubProviderRef":   {KindHint: "GithubProvider", APIVersion: "github.platform.kubecore.io/v1alpha1"},
		"providerConfigRef":   {KindHint: "ProviderConfig", APIVersion: "pkg.crossplane.io/v1"},
		"secretRef":          {KindHint: "Secret", APIVersion: "v1"},
		"configMapRef":       {KindHint: "ConfigMap", APIVersion: "v1"},
		"serviceAccountRef":  {KindHint: "ServiceAccount", APIVersion: "v1"},
		"qualityGateRef":     {KindHint: "QualityGate", APIVersion: "ci.platform.kubecore.io/v1alpha1"},
		"qualityGateRefs":    {KindHint: "QualityGate", APIVersion: "ci.platform.kubecore.io/v1alpha1", IsArray: true},
	}

	// Direct pattern match
	if pattern, exists := patterns[refField]; exists {
		return &ResourceReference{
			Kind:       pattern.KindHint,
			APIVersion: pattern.APIVersion,
		}
	}

	// Try pattern matching with regex
	if kind := f.extractKindFromRefField(refField); kind != "" {
		// Try to infer apiVersion from parent schema or use common patterns
		apiVersion := f.inferAPIVersionFromKind(kind, parentSchema)
		return &ResourceReference{
			Kind:       kind,
			APIVersion: apiVersion,
		}
	}

	return nil
}

// extractKindFromRefField extracts kind from reference field name
func (f *Function) extractKindFromRefField(refField string) string {
	// Remove common suffixes and convert to PascalCase
	patterns := []string{"Ref", "Refs", "Reference", "References"}
	kind := refField

	for _, suffix := range patterns {
		if strings.HasSuffix(kind, suffix) {
			kind = strings.TrimSuffix(kind, suffix)
			break
		}
	}

	// Convert camelCase to PascalCase
	if len(kind) > 0 {
		return strings.ToUpper(kind[:1]) + kind[1:]
	}

	return ""
}

// inferAPIVersionFromKind tries to infer API version from kind
func (f *Function) inferAPIVersionFromKind(kind string, parentSchema *SchemaInfo) string {
	// Common kind to API version mappings
	mappings := map[string]string{
		"Secret":             "v1",
		"ConfigMap":          "v1",
		"ServiceAccount":     "v1",
		"ProviderConfig":     "pkg.crossplane.io/v1",
		"GitHubProject":      "github.platform.kubecore.io/v1alpha1",
		"GithubProvider":     "github.platform.kubecore.io/v1alpha1",
		"QualityGate":        "ci.platform.kubecore.io/v1alpha1",
	}

	if apiVersion, exists := mappings[kind]; exists {
		return apiVersion
	}

	// If no mapping found, try to use parent's group with v1alpha1
	if parentSchema != nil && parentSchema.APIVersion != "" {
		if group, _, err := f.parseAPIVersion(parentSchema.APIVersion); err == nil && group != "" {
			return group + "/v1alpha1"
		}
	}

	// Default fallback
	return "v1alpha1"
}

// extractTargetsFromOpenAPISchema extracts reference targets from OpenAPI schema
func (f *Function) extractTargetsFromOpenAPISchema(schema *apiextensionsv1.JSONSchemaProps, refField string, correlationID string) []ResourceReference {
	var targets []ResourceReference

	// This is a complex implementation that would analyze the OpenAPI schema
	// to find reference patterns. For now, return empty slice as the field name
	// inference should handle most cases.

	return targets
}

// getSchemaKey generates a unique key for schema caching
func (f *Function) getSchemaKey(ref ResourceReference) string {
	// For caching, we only need APIVersion and Kind (not specific instance name)
	// since the schema is the same for all instances of a CRD
	return fmt.Sprintf("%s/%s", ref.APIVersion, ref.Kind)
}

// extractReferenceFieldsFromSchema extracts reference field names from OpenAPI schema
func (f *Function) extractReferenceFieldsFromSchema(schema *apiextensionsv1.JSONSchemaProps, correlationID string) []string {
	var refFields []string

	if schema == nil || schema.Properties == nil {
		return refFields
	}

	// Look for reference fields in spec
	if specSchema, exists := schema.Properties["spec"]; exists {
		refFields = append(refFields, f.findReferenceFields(specSchema.Properties, "")...)
	}

	f.slogger.Debug("Extracted reference fields from schema",
		"correlationId", correlationID,
		"referenceFields", refFields)

	return refFields
}

// findReferenceFields recursively finds reference fields in schema properties
func (f *Function) findReferenceFields(properties map[string]apiextensionsv1.JSONSchemaProps, prefix string) []string {
	var refFields []string

	for fieldName, fieldSchema := range properties {
		fullFieldName := fieldName
		if prefix != "" {
			fullFieldName = prefix + "." + fieldName
		}

		// Check if this field is a reference field
		if f.isReferenceField(fieldName) {
			refFields = append(refFields, fullFieldName)
		}

		// Recursively check nested objects
		if fieldSchema.Type == "object" && fieldSchema.Properties != nil {
			nestedRefs := f.findReferenceFields(fieldSchema.Properties, fullFieldName)
			refFields = append(refFields, nestedRefs...)
		}

		// Check array items
		if fieldSchema.Type == "array" && fieldSchema.Items != nil && fieldSchema.Items.Schema != nil {
			if fieldSchema.Items.Schema.Properties != nil {
				arrayRefs := f.findReferenceFields(fieldSchema.Items.Schema.Properties, fullFieldName+"[]")
				refFields = append(refFields, arrayRefs...)
			}
		}
	}

	return refFields
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
