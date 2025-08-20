package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/response"
	"k8s.io/client-go/rest"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/discovery"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/parser"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/registry"
	responsebuilder "github.com/crossplane/function-kubecore-schema-registry/pkg/response"
)

// Function implements the KubeCore Schema Registry Function (Phase 1 & 2)
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	log logging.Logger
	
	// Core components
	registry        registry.Registry
	parser          parser.XRParser
	responseBuilder responsebuilder.Builder
}

// NewFunction creates a new function instance
func NewFunction(log logging.Logger) *Function {
	return &Function{
		log:             log,
		registry:        registry.NewEmbeddedRegistry(),
		parser:          parser.NewDefaultXRParser(),
		responseBuilder: responsebuilder.NewDefaultBuilder(),
	}
}

// RunFunction implements the main function logic for Phase 1
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	startTime := time.Now()
	
	// Initialize response with default TTL
	rsp := response.To(req, response.DefaultTTL)

	// Determine phase based on input
	phase := "1"
	tempInput := &v1beta1.Input{}
	if request.GetInput(req, tempInput) == nil {
		if tempInput.Phase2Features != nil && *tempInput.Phase2Features {
			phase = "2"
		}
	}
	
	f.log.Info("KubeCore Schema Registry Function starting", "phase", phase)

	// Extract and validate XR
	xr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get observed composite"))
		return rsp, nil
	}

	f.log.Info("Processing XR", 
		"kind", xr.Resource.GetKind(), 
		"name", xr.Resource.GetName(),
		"namespace", xr.Resource.GetNamespace())

	// Extract function input
	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot get function input"))
		return rsp, nil
	}

	// Parse fetch requests from function input and XR spec
	var fetchRequests []v1beta1.ResourceRequest
	
	// First, use requests from function input if provided
	if len(in.FetchResources) > 0 {
		fetchRequests = in.FetchResources
		f.log.Info("Using fetch requests from function input", "count", len(fetchRequests))
	} else {
		// Fallback to parsing from XR spec
		xrRequests, err := f.parser.ParseFetchRequests(xr.Resource.Object)
		if err != nil {
			response.Fatal(rsp, errors.Wrap(err, "failed to parse fetch requests from XR"))
			return rsp, nil
		}
		fetchRequests = xrRequests
		f.log.Info("Using fetch requests from XR spec", "count", len(fetchRequests))
	}

	if len(fetchRequests) == 0 {
		f.log.Info("No fetch requests found")
		response.Normal(rsp, "No resources to fetch - completed successfully")
		return rsp, nil
	}

	// Parse timeout and max concurrent settings
	timeout := 5 * time.Second // default
	maxConcurrent := 10        // default

	if in.FetchTimeout != nil {
		if parsedTimeout, err := time.ParseDuration(*in.FetchTimeout); err == nil {
			timeout = parsedTimeout
		} else {
			f.log.Info("Invalid timeout format, using default", "provided", *in.FetchTimeout, "default", timeout)
		}
	}

	if in.MaxConcurrentFetches != nil {
		maxConcurrent = *in.MaxConcurrentFetches
	}

	// Determine if Phase 2 features are enabled
	phase2Enabled := in.Phase2Features != nil && *in.Phase2Features
	
	f.log.Info("Fetch configuration", 
		"timeout", timeout,
		"maxConcurrent", maxConcurrent,
		"requestCount", len(fetchRequests),
		"phase2Enabled", phase2Enabled)

	// Create discovery engine with Phase 2 capabilities if enabled
	discoveryEngine, err := f.createDiscoveryEngine(timeout, maxConcurrent, phase2Enabled)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "failed to create discovery engine"))
		return rsp, nil
	}

	// Fetch resources
	f.log.Info("Starting resource fetch operations")
	fetchResult, err := discoveryEngine.FetchResources(fetchRequests)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "resource fetch failed"))
		return rsp, nil
	}

	// Log summary
	f.log.Info("Resource fetch completed",
		"totalRequested", fetchResult.Summary.TotalRequested,
		"successful", fetchResult.Summary.Successful,
		"failed", fetchResult.Summary.Failed,
		"skipped", fetchResult.Summary.Skipped,
		"duration", fetchResult.Summary.TotalDuration)

	// Build and set response context
	if err := f.responseBuilder.SetContext(rsp, fetchResult); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "failed to build response context"))
		return rsp, nil
	}

	// Set appropriate response conditions
	if fetchResult.Summary.Failed > 0 {
		response.ConditionFalse(rsp, "ResourcesFetched", "SomeResourcesFailed").
			WithMessage(fmt.Sprintf("Failed to fetch %d out of %d resources", 
				fetchResult.Summary.Failed, fetchResult.Summary.TotalRequested)).
			TargetCompositeAndClaim()
		
		response.Warning(rsp, fmt.Errorf("Resource fetch partially failed: %d successful, %d failed, %d skipped",
			fetchResult.Summary.Successful, fetchResult.Summary.Failed, fetchResult.Summary.Skipped))
	} else {
		response.ConditionTrue(rsp, "ResourcesFetched", "AllResourcesFetched").
			WithMessage(fmt.Sprintf("Successfully fetched %d resources", fetchResult.Summary.Successful)).
			TargetCompositeAndClaim()

		response.Normal(rsp, fmt.Sprintf("Successfully fetched %d resources in %v", 
			fetchResult.Summary.Successful, fetchResult.Summary.TotalDuration))
	}

	// Log completion
	executionTime := time.Since(startTime)
	f.log.Info("Function execution completed", 
		"executionTime", executionTime,
		"phase", phase)

	return rsp, nil
}

// createDiscoveryEngine creates a Kubernetes discovery engine
func (f *Function) createDiscoveryEngine(timeout time.Duration, maxConcurrent int, phase2Enabled bool) (discovery.Engine, error) {
	// Get in-cluster configuration
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.KubernetesClientError(fmt.Sprintf("failed to get in-cluster config: %v", err))
	}

	// Use enhanced engine if Phase 2 is enabled, otherwise use legacy engine for compatibility
	if phase2Enabled {
		// Create enhanced discovery engine with Phase 2 capabilities
		discoveryContext := discovery.DiscoveryContext{
			FunctionNamespace:     "crossplane-system", // TODO: Get actual namespace
			TimeoutPerRequest:     timeout,
			MaxConcurrentRequests: maxConcurrent,
			Phase2Enabled:         true,
		}
		
		engine, err := discovery.NewEnhancedEngine(config, f.registry, discoveryContext)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create enhanced discovery engine")
		}
		
		return engine, nil
	} else {
		// Create legacy Kubernetes discovery engine for Phase 1 compatibility
		engine, err := discovery.NewKubernetesEngineWithTimeout(config, f.registry, timeout, maxConcurrent)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create Kubernetes discovery engine")
		}

		return engine, nil
	}
}