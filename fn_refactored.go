package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"google.golang.org/protobuf/types/known/structpb"

	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"github.com/crossplane/function-kubecore-schema-registry/internal/cache"
	"github.com/crossplane/function-kubecore-schema-registry/internal/config"
	"github.com/crossplane/function-kubecore-schema-registry/internal/domain"
	"github.com/crossplane/function-kubecore-schema-registry/internal/repository"
	"github.com/crossplane/function-kubecore-schema-registry/internal/service"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/factory"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/interfaces"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/utils"
)

// RefactoredFunction implements the schema registry function with proper architecture
type RefactoredFunction struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	// SDK logger for compatibility
	log logging.Logger

	// Dependencies
	config           *config.Config
	logger           interfaces.Logger
	cache            interfaces.CacheProvider
	repository       interfaces.SchemaRepository
	factory          interfaces.SchemaFactory
	refExtractor     interfaces.ReferenceExtractor
	contextExtractor interfaces.ContextExtractor
	discoveryService interfaces.SchemaDiscoveryService
	converterService *service.ConverterService

	// Kubernetes client for repository
	k8sClient clientset.Interface
}

// NewRefactoredFunction creates a new refactored function with dependency injection
func NewRefactoredFunction(log logging.Logger) *RefactoredFunction {
	// Initialize configuration
	cfg := config.New()

	// Initialize logger
	logger := utils.NewSlogLogger()

	// Initialize cache
	cacheProvider := cache.NewMemoryCache(cfg.CacheTTL)

	// Initialize factory
	schemaFactory := factory.NewSchemaFactory(logger)

	// Initialize reference extractor
	refExtractor := utils.NewReferenceExtractor(cfg, logger)

	// Initialize context extractor
	contextExtractor := utils.NewContextExtractor(refExtractor, logger)

	// Initialize repository (will be nil if Kubernetes is not available)
	var repo interfaces.SchemaRepository
	var k8sClient clientset.Interface
	
	// Try to initialize Kubernetes repository
	if kubeRepo, err := repository.NewKubernetesRepository(logger); err != nil {
		logger.Warn("Kubernetes repository not available, will use fallback schemas", "error", err)
		repo = nil
	} else {
		repo = kubeRepo
	}

	// Initialize discovery service
	discoveryService := service.NewDiscoveryService(
		repo,
		cacheProvider,
		schemaFactory,
		refExtractor,
		logger,
	)

	// Initialize converter service
	converterService := service.NewConverterService(logger)

	return &RefactoredFunction{
		log:              log,
		config:           cfg,
		logger:           logger,
		cache:            cacheProvider,
		repository:       repo,
		factory:          schemaFactory,
		refExtractor:     refExtractor,
		contextExtractor: contextExtractor,
		discoveryService: discoveryService,
		converterService: converterService,
		k8sClient:        k8sClient,
	}
}

// SetKubernetesClient sets the Kubernetes client for testing
func (f *RefactoredFunction) SetKubernetesClient(client clientset.Interface) {
	f.k8sClient = client
	if client != nil {
		f.repository = repository.NewKubernetesRepositoryWithClient(client, f.logger)
		// Recreate discovery service with new repository
		f.discoveryService = service.NewDiscoveryService(
			f.repository,
			f.cache,
			f.factory,
			f.refExtractor,
			f.logger,
		)
		// Recreate converter service
		f.converterService = service.NewConverterService(f.logger)
	}
}

// RunFunction implements the main function logic with refactored architecture
func (f *RefactoredFunction) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	startTime := time.Now()
	correlationID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	f.logger.Info("RunFunction started",
		"correlationId", correlationID,
		"tag", req.GetMeta().GetTag())

	// Initialize response with TTL
	rsp := response.To(req, response.DefaultTTL)

	// Extract and validate function input
	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		f.logger.Error("Failed to get function input",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrapf(err, "cannot get function input"))
		return rsp, nil
	}

	// Set default values for input parameters
	enableTransitive := f.config.DefaultEnableTransitive
	if in.EnableTransitiveDiscovery != nil {
		enableTransitive = *in.EnableTransitiveDiscovery
	}

	traversalDepth := f.config.DefaultTraversalDepth
	if in.TraversalDepth != nil {
		traversalDepth = *in.TraversalDepth
	}

	includeFullSchema := f.config.DefaultIncludeFullSchema
	if in.IncludeFullSchema != nil {
		includeFullSchema = *in.IncludeFullSchema
	}

	f.logger.Debug("Function input processed",
		"correlationId", correlationID,
		"enableTransitive", enableTransitive,
		"traversalDepth", traversalDepth,
		"includeFullSchema", includeFullSchema)

	// Extract XR (composite resource)
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		f.logger.Error("Failed to get observed composite resource",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite resource"))
		return rsp, nil
	}

	// Extract execution context from XR using context extractor
	execCtx, err := f.contextExtractor.ExtractExecutionContext(ctx, xr, correlationID)
	if err != nil {
		f.logger.Error("Failed to extract execution context",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "cannot extract execution context"))
		return rsp, nil
	}

	f.logger.Info("Execution context extracted",
		"correlationId", correlationID,
		"sourceXResource", execCtx.SourceXResource,
		"claimName", execCtx.ClaimName,
		"directReferences", len(execCtx.DirectReferences))

	// Create discovery options
	opts := &domain.DiscoveryOptions{
		EnableTransitive:  enableTransitive,
		TraversalDepth:    traversalDepth,
		IncludeFullSchema: includeFullSchema,
		CorrelationID:     correlationID,
	}

	// Perform schema discovery using discovery service
	discoveryResult, err := f.discoveryService.DiscoverSchemas(ctx, execCtx, opts)
	if err != nil {
		f.logger.Error("Schema discovery failed",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "schema discovery failed"))
		return rsp, nil
	}

	// Calculate execution time
	discoveryResult.Stats.ExecutionTimeMs = time.Since(startTime).Milliseconds()

	f.logger.Info("Schema discovery completed",
		"correlationId", correlationID,
		"schemasRetrieved", discoveryResult.Stats.SchemasRetrieved,
		"executionTimeMs", discoveryResult.Stats.ExecutionTimeMs)

	// Convert to Go template-friendly format
	schemaRegistryOutput, err := f.converterService.ConvertToSchemaRegistryOutput(discoveryResult, execCtx, correlationID)
	if err != nil {
		f.logger.Error("Failed to convert to schema registry output",
			"correlationId", correlationID,
			"error", err)
		response.Fatal(rsp, errors.Wrap(err, "schema registry conversion failed"))
		return rsp, nil
	}

	// Store the converted result in the discovery result
	discoveryResult.SchemaRegistryOutput = schemaRegistryOutput

	// Build response structure with both legacy and new formats
	responseData := map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"type":   "Ready",
					"status": "True",
				},
			},
			"executionContext":           execCtx,
			// Legacy format for backward compatibility
			"referencedResourceSchemas": discoveryResult.Schemas,
			"discoveryStats":            discoveryResult.Stats,
			// New Go template-friendly format
			"schemaRegistryResults":     schemaRegistryOutput,
		},
	}

	// Store both formats in context for potential access by other functions
	statusJSON, _ := json.Marshal(map[string]interface{}{
		"executionContext":           execCtx,
		"referencedResourceSchemas": discoveryResult.Schemas,
		"discoveryStats":            discoveryResult.Stats,
		"schemaRegistryResults":     schemaRegistryOutput,
	})

	if statusValue, err := structpb.NewValue(string(statusJSON)); err == nil {
		response.SetContextKey(rsp, "kubecore.schemaRegistry.detailedStatus", statusValue)
	}

	// Convert to basic types that structpb can handle
	schemaRegistryJSON, err := json.Marshal(schemaRegistryOutput)
	if err != nil {
		f.logger.Error("Failed to marshal schema registry output", 
			"correlationId", correlationID,
			"error", err)
	} else {
		var schemaRegistryMap map[string]interface{}
		if err := json.Unmarshal(schemaRegistryJSON, &schemaRegistryMap); err != nil {
			f.logger.Error("Failed to unmarshal to interface map",
				"correlationId", correlationID,
				"error", err)
		} else {
			// Store the structured data for Go template access
			if schemaRegistryStruct, err := structpb.NewStruct(schemaRegistryMap); err == nil {
				response.SetContextKey(rsp, "schemaRegistryResults", structpb.NewStructValue(schemaRegistryStruct))
				f.logger.Info("Successfully set structured schemaRegistryResults context key",
					"correlationId", correlationID)
			} else {
				f.logger.Error("Failed to create structured schemaRegistryResults",
					"correlationId", correlationID,
					"error", err)
			}
		}
	}

	f.logger.Info("Schema registry discovery results available",
		"correlationId", correlationID,
		"discoveredResourcesCount", len(schemaRegistryOutput.DiscoveredResources),
		"resourceSchemasCount", len(schemaRegistryOutput.ResourceSchemas),
		"referenceChainsCount", len(schemaRegistryOutput.ReferenceChains),
		"resourceKindsCount", len(schemaRegistryOutput.ResourcesByKind),
		"statusSize", len(statusJSON))

	// Convert response data to JSON for logging
	if responseJSON, err := json.Marshal(responseData); err == nil {
		f.logger.Debug("Schema registry response prepared",
			"correlationId", correlationID,
			"responseSize", len(responseJSON))

		response.Normalf(rsp, "Schema registry discovery completed successfully. Found %d resources across %d schemas in %dms",
			schemaRegistryOutput.DiscoveryStats.TotalResourcesFound, 
			schemaRegistryOutput.DiscoveryStats.TotalSchemasRetrieved,
			schemaRegistryOutput.DiscoveryStats.ExecutionTimeMs)
	}

	// Set success condition
	response.ConditionTrue(rsp, "FunctionSuccess", "SchemaDiscoveryComplete").
		WithMessage(fmt.Sprintf("Successfully discovered %d resources across %d schemas with %d reference chains",
			schemaRegistryOutput.DiscoveryStats.TotalResourcesFound,
			schemaRegistryOutput.DiscoveryStats.TotalSchemasRetrieved,
			len(schemaRegistryOutput.ReferenceChains))).
		TargetCompositeAndClaim()

	f.logger.Info("RunFunction completed successfully",
		"correlationId", correlationID,
		"executionTimeMs", schemaRegistryOutput.DiscoveryStats.ExecutionTimeMs,
		"schemaRegistryOutputGenerated", true)

	return rsp, nil
}