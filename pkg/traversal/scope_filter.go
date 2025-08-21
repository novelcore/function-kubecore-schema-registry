package traversal

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/crossplane/function-sdk-go/logging"

	dynamictypes "github.com/crossplane/function-kubecore-schema-registry/pkg/dynamic"
)

// ScopeFilter filters resources based on scope criteria
type ScopeFilter interface {
	// FilterResources filters resources based on scope configuration
	FilterResources(resources []*unstructured.Unstructured, config *ScopeFilterConfig) []*unstructured.Unstructured

	// FilterReferences filters reference fields based on scope configuration
	FilterReferences(references []dynamictypes.ReferenceField, config *ScopeFilterConfig) []dynamictypes.ReferenceField

	// ShouldIncludeResource determines if a resource should be included in traversal
	ShouldIncludeResource(resource *unstructured.Unstructured, config *ScopeFilterConfig) bool

	// ShouldFollowReference determines if a reference should be followed
	ShouldFollowReference(reference dynamictypes.ReferenceField, config *ScopeFilterConfig) bool

	// GetFilterStatistics returns statistics about filtering operations
	GetFilterStatistics() *FilterStatistics
}

// DefaultScopeFilter implements ScopeFilter interface
type DefaultScopeFilter struct {
	// platformChecker determines if resources belong to platform scope
	platformChecker PlatformChecker

	// logger provides structured logging
	logger logging.Logger

	// statistics tracks filtering operations
	statistics *FilterStatistics
}

// FilterStatistics contains statistics about filtering operations
type FilterStatistics struct {
	// ResourcesEvaluated is the total number of resources evaluated
	ResourcesEvaluated int

	// ResourcesIncluded is the number of resources included after filtering
	ResourcesIncluded int

	// ResourcesExcluded is the number of resources excluded by filtering
	ResourcesExcluded int

	// ReferencesEvaluated is the total number of references evaluated
	ReferencesEvaluated int

	// ReferencesIncluded is the number of references included after filtering
	ReferencesIncluded int

	// ReferencesExcluded is the number of references excluded by filtering
	ReferencesExcluded int

	// FilterReasons tracks reasons for exclusion
	FilterReasons map[string]int
}

// PlatformChecker determines if resources belong to platform scope
type PlatformChecker interface {
	// IsPlatformResource determines if a resource belongs to the platform
	IsPlatformResource(resource *unstructured.Unstructured) bool

	// IsPlatformAPIGroup determines if an API group belongs to the platform
	IsPlatformAPIGroup(apiGroup string) bool

	// IsPlatformKind determines if a resource kind belongs to the platform
	IsPlatformKind(kind string, apiGroup string) bool

	// GetPlatformAPIGroups returns all platform API groups
	GetPlatformAPIGroups() []string
}

// DefaultPlatformChecker implements PlatformChecker interface
type DefaultPlatformChecker struct {
	// platformAPIGroups contains patterns for platform API groups
	platformAPIGroups []string

	// platformKinds contains platform-specific kinds
	platformKinds map[string]bool
}

// NewDefaultScopeFilter creates a new default scope filter
func NewDefaultScopeFilter(platformChecker PlatformChecker, logger logging.Logger) *DefaultScopeFilter {
	return &DefaultScopeFilter{
		platformChecker: platformChecker,
		logger:          logger,
		statistics: &FilterStatistics{
			FilterReasons: make(map[string]int),
		},
	}
}

// NewDefaultPlatformChecker creates a new default platform checker
func NewDefaultPlatformChecker(platformAPIGroups []string) *DefaultPlatformChecker {
	platformKinds := map[string]bool{
		// Core Kubernetes resources that are NOT platform resources
		"Pod":                   false,
		"Service":               false,
		"ConfigMap":             false,
		"Secret":                false,
		"PersistentVolumeClaim": false,
		"PersistentVolume":      false,
		"StorageClass":          false,
		"Namespace":             false,
		"Node":                  false,
		"Deployment":            false,
		"ReplicaSet":            false,
		"DaemonSet":             false,
		"StatefulSet":           false,
		"Job":                   false,
		"CronJob":               false,

		// KubeCore platform resources
		"KubeCluster":    true,
		"KubEnv":         true,
		"KubeApp":        true,
		"KubeSystem":     true,
		"KubeNet":        true,
		"QualityGate":    true,
		"GitHubProject":  true,
		"GitHubInfra":    true,
		"GitHubSystem":   true,
		"GithubProvider": true,
	}

	return &DefaultPlatformChecker{
		platformAPIGroups: platformAPIGroups,
		platformKinds:     platformKinds,
	}
}

// FilterResources filters resources based on scope configuration
func (sf *DefaultScopeFilter) FilterResources(resources []*unstructured.Unstructured, config *ScopeFilterConfig) []*unstructured.Unstructured {
	var filtered []*unstructured.Unstructured

	for _, resource := range resources {
		sf.statistics.ResourcesEvaluated++

		if sf.ShouldIncludeResource(resource, config) {
			filtered = append(filtered, resource)
			sf.statistics.ResourcesIncluded++
		} else {
			sf.statistics.ResourcesExcluded++
		}
	}

	sf.logger.Debug("Filtered resources",
		"total", len(resources),
		"included", len(filtered),
		"excluded", len(resources)-len(filtered))

	return filtered
}

// FilterReferences filters reference fields based on scope configuration
func (sf *DefaultScopeFilter) FilterReferences(references []dynamictypes.ReferenceField, config *ScopeFilterConfig) []dynamictypes.ReferenceField {
	var filtered []dynamictypes.ReferenceField

	for _, reference := range references {
		sf.statistics.ReferencesEvaluated++

		if sf.ShouldFollowReference(reference, config) {
			filtered = append(filtered, reference)
			sf.statistics.ReferencesIncluded++
		} else {
			sf.statistics.ReferencesExcluded++
		}
	}

	sf.logger.Debug("Filtered references",
		"total", len(references),
		"included", len(filtered),
		"excluded", len(references)-len(filtered))

	return filtered
}

// ShouldIncludeResource determines if a resource should be included in traversal
func (sf *DefaultScopeFilter) ShouldIncludeResource(resource *unstructured.Unstructured, config *ScopeFilterConfig) bool {
	// Extract resource information
	apiVersion := resource.GetAPIVersion()
	kind := resource.GetKind()
	namespace := resource.GetNamespace()
	apiGroup := sf.extractAPIGroup(apiVersion)

	// Apply platform-only filter
	if config.PlatformOnly {
		if !sf.platformChecker.IsPlatformResource(resource) {
			sf.statistics.FilterReasons["not_platform"]++
			return false
		}
	}

	// Apply API group filters
	if len(config.IncludeAPIGroups) > 0 {
		if !sf.matchesAPIGroupPatterns(apiGroup, config.IncludeAPIGroups) {
			sf.statistics.FilterReasons["api_group_not_included"]++
			return false
		}
	}

	if len(config.ExcludeAPIGroups) > 0 {
		if sf.matchesAPIGroupPatterns(apiGroup, config.ExcludeAPIGroups) {
			sf.statistics.FilterReasons["api_group_excluded"]++
			return false
		}
	}

	// Apply kind filters
	if len(config.IncludeKinds) > 0 {
		if !sf.stringInSlice(kind, config.IncludeKinds) {
			sf.statistics.FilterReasons["kind_not_included"]++
			return false
		}
	}

	if len(config.ExcludeKinds) > 0 {
		if sf.stringInSlice(kind, config.ExcludeKinds) {
			sf.statistics.FilterReasons["kind_excluded"]++
			return false
		}
	}

	// Apply namespace filters
	if namespace != "" { // Only apply to namespaced resources
		if len(config.IncludeNamespaces) > 0 {
			if !sf.stringInSlice(namespace, config.IncludeNamespaces) {
				sf.statistics.FilterReasons["namespace_not_included"]++
				return false
			}
		}

		if len(config.ExcludeNamespaces) > 0 {
			if sf.stringInSlice(namespace, config.ExcludeNamespaces) {
				sf.statistics.FilterReasons["namespace_excluded"]++
				return false
			}
		}
	}

	return true
}

// ShouldFollowReference determines if a reference should be followed
func (sf *DefaultScopeFilter) ShouldFollowReference(reference dynamictypes.ReferenceField, config *ScopeFilterConfig) bool {
	// Apply platform-only filter
	if config.PlatformOnly {
		if !sf.platformChecker.IsPlatformKind(reference.TargetKind, reference.TargetGroup) {
			sf.statistics.FilterReasons["ref_target_not_platform"]++
			return false
		}
	}

	// Apply API group filters for references
	if len(config.IncludeAPIGroups) > 0 {
		if !sf.matchesAPIGroupPatterns(reference.TargetGroup, config.IncludeAPIGroups) {
			sf.statistics.FilterReasons["ref_api_group_not_included"]++
			return false
		}
	}

	if len(config.ExcludeAPIGroups) > 0 {
		if sf.matchesAPIGroupPatterns(reference.TargetGroup, config.ExcludeAPIGroups) {
			sf.statistics.FilterReasons["ref_api_group_excluded"]++
			return false
		}
	}

	// Apply kind filters for references
	if len(config.IncludeKinds) > 0 {
		if !sf.stringInSlice(reference.TargetKind, config.IncludeKinds) {
			sf.statistics.FilterReasons["ref_kind_not_included"]++
			return false
		}
	}

	if len(config.ExcludeKinds) > 0 {
		if sf.stringInSlice(reference.TargetKind, config.ExcludeKinds) {
			sf.statistics.FilterReasons["ref_kind_excluded"]++
			return false
		}
	}

	// Check cross-namespace references
	if !config.CrossNamespaceEnabled {
		// This is a simplified check - in practice we'd need to compare
		// the source resource's namespace with the target's namespace
		if reference.RefType != dynamictypes.RefTypeOwnerRef {
			// For now, allow owner references across namespaces
			// but restrict other reference types
			sf.statistics.FilterReasons["cross_namespace_disabled"]++
			return false
		}
	}

	return true
}

// GetFilterStatistics returns statistics about filtering operations
func (sf *DefaultScopeFilter) GetFilterStatistics() *FilterStatistics {
	return sf.statistics
}

// PlatformChecker implementation methods

// IsPlatformResource determines if a resource belongs to the platform
func (pc *DefaultPlatformChecker) IsPlatformResource(resource *unstructured.Unstructured) bool {
	apiGroup := pc.extractAPIGroup(resource.GetAPIVersion())
	kind := resource.GetKind()

	// First check by API group
	if pc.IsPlatformAPIGroup(apiGroup) {
		return true
	}

	// Then check by kind
	return pc.IsPlatformKind(kind, apiGroup)
}

// IsPlatformAPIGroup determines if an API group belongs to the platform
func (pc *DefaultPlatformChecker) IsPlatformAPIGroup(apiGroup string) bool {
	for _, pattern := range pc.platformAPIGroups {
		if pc.matchesPattern(apiGroup, pattern) {
			return true
		}
	}
	return false
}

// IsPlatformKind determines if a resource kind belongs to the platform
func (pc *DefaultPlatformChecker) IsPlatformKind(kind string, apiGroup string) bool {
	// First check if it's explicitly marked as platform kind
	if isPlatform, exists := pc.platformKinds[kind]; exists {
		return isPlatform
	}

	// If kind is unknown, check by API group
	return pc.IsPlatformAPIGroup(apiGroup)
}

// GetPlatformAPIGroups returns all platform API groups
func (pc *DefaultPlatformChecker) GetPlatformAPIGroups() []string {
	return pc.platformAPIGroups
}

// GetAPIGroupScope returns the scope of an API group (platform, external)
func (pc *DefaultPlatformChecker) GetAPIGroupScope(apiVersion string) string {
	apiGroup := pc.extractAPIGroup(apiVersion)
	if pc.IsPlatformAPIGroup(apiGroup) {
		return "platform"
	}
	return "external"
}

// Helper methods

// extractAPIGroup extracts the API group from an API version
func (sf *DefaultScopeFilter) extractAPIGroup(apiVersion string) string {
	if strings.Contains(apiVersion, "/") {
		parts := strings.Split(apiVersion, "/")
		return parts[0]
	}
	return "" // Core API group
}

// extractAPIGroup extracts the API group from an API version (PlatformChecker version)
func (pc *DefaultPlatformChecker) extractAPIGroup(apiVersion string) string {
	if strings.Contains(apiVersion, "/") {
		parts := strings.Split(apiVersion, "/")
		return parts[0]
	}
	return "" // Core API group
}

// matchesAPIGroupPatterns checks if an API group matches any of the patterns
func (sf *DefaultScopeFilter) matchesAPIGroupPatterns(apiGroup string, patterns []string) bool {
	for _, pattern := range patterns {
		if sf.matchesPattern(apiGroup, pattern) {
			return true
		}
	}
	return false
}

// matchesPattern checks if a string matches a pattern (supports wildcards)
func (sf *DefaultScopeFilter) matchesPattern(value, pattern string) bool {
	// Simple wildcard matching
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(value, suffix)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix)
	}

	return value == pattern
}

// matchesPattern checks if a string matches a pattern (PlatformChecker version)
func (pc *DefaultPlatformChecker) matchesPattern(value, pattern string) bool {
	// Simple wildcard matching
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(value, suffix)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(value, prefix)
	}

	return value == pattern
}

// stringInSlice checks if a string is in a slice
func (sf *DefaultScopeFilter) stringInSlice(str string, slice []string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// ResetStatistics resets the filtering statistics
func (sf *DefaultScopeFilter) ResetStatistics() {
	sf.statistics = &FilterStatistics{
		FilterReasons: make(map[string]int),
	}
}

// LogFilteringSummary logs a summary of filtering operations
func (sf *DefaultScopeFilter) LogFilteringSummary() {
	sf.logger.Info("Filtering summary",
		"resourcesEvaluated", sf.statistics.ResourcesEvaluated,
		"resourcesIncluded", sf.statistics.ResourcesIncluded,
		"resourcesExcluded", sf.statistics.ResourcesExcluded,
		"referencesEvaluated", sf.statistics.ReferencesEvaluated,
		"referencesIncluded", sf.statistics.ReferencesIncluded,
		"referencesExcluded", sf.statistics.ReferencesExcluded,
		"filterReasons", sf.statistics.FilterReasons)
}
