package main

import (
	"context"

	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
)

// Function implements the main function interface as a wrapper around RefactoredFunction
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer
	
	refactoredFunc *RefactoredFunction
}

// NewFunction creates a new function instance
func NewFunction(log logging.Logger) *Function {
	return &Function{
		refactoredFunc: NewRefactoredFunction(log),
	}
}

// SetKubernetesClient sets the Kubernetes client for testing
func (f *Function) SetKubernetesClient(client clientset.Interface) {
	f.refactoredFunc.SetKubernetesClient(client)
}

// RunFunction delegates to the refactored implementation
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	return f.refactoredFunc.RunFunction(ctx, req)
}