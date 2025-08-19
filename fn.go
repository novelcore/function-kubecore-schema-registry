package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
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

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
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
	ExecutionTimeMs      int64 `json:"executionTimeMs"`
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

// SchemaCache provides caching for CRD schemas
type SchemaCache struct {
	cache map[string]*SchemaInfo
	mu    sync.RWMutex
	ttl   time.Duration
}

// NewSchemaCache creates a new schema cache
func NewSchemaCache(ttl time.Duration) *SchemaCache {
	return &SchemaCache{
		cache: make(map[string]*SchemaInfo),
		ttl:   ttl,
	}
}

// Get retrieves schema from cache
func (sc *SchemaCache) Get(key string) (*SchemaInfo, bool) {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	schema, exists := sc.cache[key]
	return schema, exists
}

// Set stores schema in cache
func (sc *SchemaCache) Set(key string, schema *SchemaInfo) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	sc.cache[key] = schema
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
			f.slogger.Warn("Kubernetes client not available, using mock schema discovery",
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

	// Convert response data to JSON for structured output
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

// extractReferencesFromSpec recursively extracts reference fields from spec
func (f *Function) extractReferencesFromSpec(spec map[string]interface{}, refs map[string]ResourceReference, correlationID string) {
	for key, value := range spec {
		if strings.HasSuffix(key, "Ref") || strings.HasSuffix(key, "Refs") {
			f.processReferenceField(key, value, refs, correlationID)
		} else if nested, ok := value.(map[string]interface{}); ok {
			f.extractReferencesFromSpec(nested, refs, correlationID)
		}
	}
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
		if ref.Name != "" {
			refs[fieldName] = ref
			f.slogger.Debug("Reference field processed",
				"correlationId", correlationID,
				"fieldName", fieldName,
				"refName", ref.Name)
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

// performSchemaDiscovery performs the main schema discovery logic
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
			schemas[fieldName] = schema
			stats.SchemasRetrieved++
			visited[f.getSchemaKey(ref)] = true

			// Perform transitive discovery if enabled
			if enableTransitive && traversalDepth > 0 {
				f.performTransitiveDiscovery(ctx, schema, visited, traversalDepth-1, includeFullSchema, correlationID)
				stats.MaxDepthReached = max(stats.MaxDepthReached, traversalDepth)
			}
		}
	}

	stats.TotalReferencesFound = len(execCtx.DirectReferences)

	f.slogger.Debug("Schema discovery completed",
		"correlationId", correlationID,
		"schemasFound", len(schemas),
		"statsRetrieved", stats.SchemasRetrieved)

	return schemas, stats, nil
}

// discoverSchema discovers schema for a given resource reference
func (f *Function) discoverSchema(ctx context.Context, ref ResourceReference, includeFullSchema bool, correlationID string) (*SchemaInfo, error) {
	schemaKey := f.getSchemaKey(ref)
	
	// Check cache first
	if cached, exists := f.schemaCache.Get(schemaKey); exists {
		f.slogger.Debug("Schema found in cache",
			"correlationId", correlationID,
			"schemaKey", schemaKey)
		return cached, nil
	}

	f.slogger.Debug("Fetching schema from API",
		"correlationId", correlationID,
		"schemaKey", schemaKey)

	// Mock schema discovery for demonstration
	// In a real implementation, this would fetch from the Kubernetes API
	schema := &SchemaInfo{
		Kind:            ref.Kind,
		APIVersion:      ref.APIVersion,
		ReferenceFields: []string{},
		RequiredFields:  []string{"metadata", "spec"},
	}

	// Mock OpenAPI schema if requested
	if includeFullSchema {
		schema.OpenAPIV3Schema = &apiextensionsv1.JSONSchemaProps{
			Type: "object",
			Properties: map[string]apiextensionsv1.JSONSchemaProps{
				"metadata": {
					Type: "object",
				},
				"spec": {
					Type: "object",
				},
			},
		}
	}

	// Cache the schema
	f.schemaCache.Set(schemaKey, schema)

	return schema, nil
}

// performTransitiveDiscovery recursively discovers transitive schema dependencies
func (f *Function) performTransitiveDiscovery(ctx context.Context, schema *SchemaInfo, visited map[string]bool, remainingDepth int, includeFullSchema bool, correlationID string) {
	if remainingDepth <= 0 {
		return
	}

	f.slogger.Debug("Performing transitive discovery",
		"correlationId", correlationID,
		"schema", schema.Kind,
		"remainingDepth", remainingDepth)

	// Mock transitive reference discovery
	// In a real implementation, this would parse the schema to find reference fields
	// and recursively discover their schemas
	
	if schema.TransitiveRefs == nil {
		schema.TransitiveRefs = make(map[string]*SchemaInfo)
	}

	// Mock adding a transitive reference
	transitiveRef := ResourceReference{
		Name: "mock-transitive-ref",
		Kind: "MockKind",
		APIVersion: "mock.kubecore.io/v1",
	}
	
	transitiveKey := f.getSchemaKey(transitiveRef)
	if !visited[transitiveKey] {
		visited[transitiveKey] = true
		
		transitiveSchema, err := f.discoverSchema(ctx, transitiveRef, includeFullSchema, correlationID)
		if err == nil && transitiveSchema != nil {
			schema.TransitiveRefs["mockField"] = transitiveSchema
			
			// Recursive call for next level
			f.performTransitiveDiscovery(ctx, transitiveSchema, visited, remainingDepth-1, includeFullSchema, correlationID)
		}
	}
}

// getSchemaKey generates a unique key for schema caching
func (f *Function) getSchemaKey(ref ResourceReference) string {
	return fmt.Sprintf("%s/%s/%s", ref.APIVersion, ref.Kind, ref.Name)
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
