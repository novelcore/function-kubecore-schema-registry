package dynamic

import (
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// RefType represents the type of reference relationship
type RefType string

const (
	RefTypeOwnerRef  RefType = "ownerRef"  // metadata.ownerReferences
	RefTypeConfigMap RefType = "configMap" // Reference to ConfigMap
	RefTypeSecret    RefType = "secret"    // Reference to Secret
	RefTypeService   RefType = "service"   // Reference to Service
	RefTypePVC       RefType = "pvc"       // Reference to PersistentVolumeClaim
	RefTypeCustom    RefType = "custom"    // Custom reference (platform-specific)
)

// CRDInfo contains metadata and schema information extracted from a CRD
type CRDInfo struct {
	Name       string
	Group      string
	Version    string
	Kind       string
	Plural     string
	Singular   string
	Namespaced bool
	Schema     *ResourceSchema
	References []ReferenceField
	Metadata   *CRDMetadata
	ParsedAt   time.Time
}

// CRDMetadata contains additional metadata about the CRD
type CRDMetadata struct {
	OriginalCRD *apiextv1.CustomResourceDefinition
	Labels      map[string]string
	Annotations map[string]string
	Categories  []string
	ShortNames  []string
}

// ResourceSchema represents the parsed OpenAPI schema from a CRD
type ResourceSchema struct {
	Fields      map[string]*FieldDefinition
	Description string
	Required    []string
	Properties  map[string]*ResourceSchema
}

// FieldDefinition describes a field in the resource schema
type FieldDefinition struct {
	Type        string
	Format      string
	Description string
	Required    bool
	Properties  map[string]*FieldDefinition
	Items       *FieldDefinition
	Enum        []string
	Pattern     string
	Default     interface{}
}

// ReferenceField represents a field that references another resource
type ReferenceField struct {
	FieldPath       string
	FieldName       string
	TargetKind      string
	TargetGroup     string
	TargetVersion   string
	RefType         RefType
	Confidence      float64
	DetectionMethod string
}

// ReferencePattern defines patterns for detecting reference fields
type ReferencePattern struct {
	Pattern     string
	TargetKind  string
	TargetGroup string
	RefType     RefType
	Confidence  float64
}

// DiscoveryStatistics contains metrics about the discovery process
type DiscoveryStatistics struct {
	TotalCRDs       int
	MatchedCRDs     int
	ReferenceFields int
	APIGroups       []string
	DiscoveryTime   time.Duration
	Errors          []error
}

// BuildStatistics contains metrics about registry building
type BuildStatistics struct {
	ProcessedCRDs int
	BuiltTypes    int
	SkippedTypes  int
	BuildTime     time.Duration
	Errors        []error
}

// DetectionStats contains metrics about reference field detection
type DetectionStats struct {
	FieldsAnalyzed   int
	ReferencesFound  int
	PatternMatches   int
	HeuristicMatches int
	DetectionTime    time.Duration
}

// RegistryMode defines the mode of operation for the registry
type RegistryMode string

const (
	RegistryModeEmbedded RegistryMode = "embedded"
	RegistryModeDynamic  RegistryMode = "dynamic"
	RegistryModeHybrid   RegistryMode = "hybrid"
)

// SourceInfo contains information about the registry data source
type SourceInfo struct {
	Mode           RegistryMode
	DynamicTypes   int
	EmbeddedTypes  int
	LastDiscovery  time.Time
	DiscoveryError error
}

// RegistryConfig contains configuration for registry creation
type RegistryConfig struct {
	Mode             RegistryMode
	APIGroupPatterns []string
	Timeout          time.Duration
	FallbackEnabled  bool
	RefPatterns      []string
	CacheEnabled     bool
	CacheTTL         time.Duration
	LogLevel         string
}

// Default reference patterns for detecting reference fields
var DefaultReferencePatterns = []ReferencePattern{
	{
		Pattern:    "*Ref",
		RefType:    RefTypeCustom,
		Confidence: 0.9,
	},
	{
		Pattern:    "*Reference",
		RefType:    RefTypeCustom,
		Confidence: 0.9,
	},
	{
		Pattern:    "*RefName",
		RefType:    RefTypeCustom,
		Confidence: 0.8,
	},
	{
		Pattern:     "configMapRef*",
		TargetKind:  "ConfigMap",
		TargetGroup: "",
		RefType:     RefTypeConfigMap,
		Confidence:  0.95,
	},
	{
		Pattern:     "secretRef*",
		TargetKind:  "Secret",
		TargetGroup: "",
		RefType:     RefTypeSecret,
		Confidence:  0.95,
	},
	{
		Pattern:     "serviceRef*",
		TargetKind:  "Service",
		TargetGroup: "",
		RefType:     RefTypeService,
		Confidence:  0.95,
	},
	{
		Pattern:     "pvcRef*",
		TargetKind:  "PersistentVolumeClaim",
		TargetGroup: "",
		RefType:     RefTypePVC,
		Confidence:  0.95,
	},
	{
		Pattern:    "providerConfigRef*",
		RefType:    RefTypeCustom,
		Confidence: 0.85,
	},
	{
		Pattern:     "kubeClusterRef*",
		TargetKind:  "KubeCluster",
		TargetGroup: "platform.kubecore.io",
		RefType:     RefTypeCustom,
		Confidence:  0.9,
	},
	{
		Pattern:     "kubEnvRef*",
		TargetKind:  "KubEnv",
		TargetGroup: "platform.kubecore.io",
		RefType:     RefTypeCustom,
		Confidence:  0.9,
	},
	{
		Pattern:     "githubProviderRef*",
		TargetKind:  "GithubProvider",
		TargetGroup: "github.platform.kubecore.io",
		RefType:     RefTypeCustom,
		Confidence:  0.9,
	},
}

// Default configuration values
const (
	DefaultDiscoveryTimeout = 5 * time.Second
	DefaultCacheTTL         = 10 * time.Minute
	DefaultMaxConcurrency   = 5
)

// Default API group patterns for KubeCore
var DefaultAPIGroupPatterns = []string{
	"*.kubecore.io",
	"platform.kubecore.io",
	"github.platform.kubecore.io",
	"app.kubecore.io",
}
