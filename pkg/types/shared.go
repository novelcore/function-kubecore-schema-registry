package types

import "time"

// RegistryMode defines the mode of operation for the registry
type RegistryMode string

const (
	RegistryModeEmbedded RegistryMode = "embedded"
	RegistryModeDynamic  RegistryMode = "dynamic"
	RegistryModeHybrid   RegistryMode = "hybrid"
)

// RefType represents the type of reference relationship
type RefType string

const (
	RefTypeOwnerRef   RefType = "ownerRef"   // metadata.ownerReferences
	RefTypeConfigMap  RefType = "configMap"  // Reference to ConfigMap
	RefTypeSecret     RefType = "secret"     // Reference to Secret
	RefTypeService    RefType = "service"    // Reference to Service
	RefTypePVC        RefType = "pvc"        // Reference to PersistentVolumeClaim
	RefTypeCustom     RefType = "custom"     // Custom reference (platform-specific)
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
	Mode              RegistryMode
	APIGroupPatterns  []string
	Timeout           time.Duration
	FallbackEnabled   bool
	RefPatterns       []string
	CacheEnabled      bool
	CacheTTL          time.Duration
	LogLevel          string
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