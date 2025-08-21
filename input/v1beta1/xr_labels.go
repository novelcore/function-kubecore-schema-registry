// Package v1beta1 contains XR Label configuration types
// +kubebuilder:object:generate=true
package v1beta1

// XRLabelConfig defines configuration for XR label injection
type XRLabelConfig struct {
	// Enabled controls whether XR labeling is active
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// Labels defines static labels to apply to the XR
	Labels map[string]string `json:"labels,omitempty"`

	// DynamicLabels defines labels with dynamic value computation
	DynamicLabels []DynamicLabel `json:"dynamicLabels,omitempty"`

	// NamespaceDetection configures automatic namespace label injection
	NamespaceDetection *NamespaceDetection `json:"namespaceDetection,omitempty"`

	// MergeStrategy defines how labels are merged with existing XR labels
	// +kubebuilder:validation:Enum=merge;replace;fail-on-conflict
	// +kubebuilder:default="merge"
	MergeStrategy MergeStrategy `json:"mergeStrategy,omitempty"`

	// EnforceLabels ensures specified labels cannot be overridden
	EnforceLabels []string `json:"enforceLabels,omitempty"`
}

// DynamicLabel defines a label with dynamic value computation
type DynamicLabel struct {
	// Key is the label key to set
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern="^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$"
	Key string `json:"key"`

	// Source defines where the label value comes from
	// +kubebuilder:validation:Enum=xr-field;environment;timestamp;uuid;constant
	// +kubebuilder:default="xr-field"
	Source LabelSource `json:"source,omitempty"`

	// SourcePath specifies the path to extract value from (for xr-field source)
	// Uses JSONPath syntax: "metadata.name", "spec.parameters.region"
	SourcePath string `json:"sourcePath,omitempty"`

	// Value provides a constant value (for constant source)
	Value string `json:"value,omitempty"`

	// Transform defines optional value transformation
	Transform *LabelTransform `json:"transform,omitempty"`

	// Required indicates if this label must be successfully applied
	// +kubebuilder:default=false
	Required bool `json:"required,omitempty"`
}

// LabelSource defines the source for label values
type LabelSource string

const (
	// LabelSourceXRField extracts value from XR field
	LabelSourceXRField LabelSource = "xr-field"
	// LabelSourceEnvironment extracts value from environment variable
	LabelSourceEnvironment LabelSource = "environment"
	// LabelSourceTimestamp generates timestamp value
	LabelSourceTimestamp LabelSource = "timestamp"
	// LabelSourceUUID generates UUID value
	LabelSourceUUID LabelSource = "uuid"
	// LabelSourceConstant uses provided constant value
	LabelSourceConstant LabelSource = "constant"
)

// LabelTransform defines value transformation configuration
type LabelTransform struct {
	// Type specifies the transformation to apply
	// +kubebuilder:validation:Enum=lowercase;uppercase;prefix;suffix;replace;truncate;hash
	// +kubebuilder:validation:Required
	Type TransformType `json:"type"`

	// Options contains transformation-specific configuration
	Options *TransformOptions `json:"options,omitempty"`
}

// TransformType defines available transformation types
type TransformType string

const (
	// TransformTypeLowercase converts to lowercase
	TransformTypeLowercase TransformType = "lowercase"
	// TransformTypeUppercase converts to uppercase
	TransformTypeUppercase TransformType = "uppercase"
	// TransformTypePrefix adds prefix to value
	TransformTypePrefix TransformType = "prefix"
	// TransformTypeSuffix adds suffix to value
	TransformTypeSuffix TransformType = "suffix"
	// TransformTypeReplace performs string replacement
	TransformTypeReplace TransformType = "replace"
	// TransformTypeTruncate truncates value to specified length
	TransformTypeTruncate TransformType = "truncate"
	// TransformTypeHash generates hash of value
	TransformTypeHash TransformType = "hash"
)

// TransformOptions contains transformation-specific configuration
type TransformOptions struct {
	// Prefix for prefix transformation
	Prefix string `json:"prefix,omitempty"`

	// Suffix for suffix transformation
	Suffix string `json:"suffix,omitempty"`

	// Old string for replace transformation
	Old string `json:"old,omitempty"`

	// New string for replace transformation
	New string `json:"new,omitempty"`

	// Length for truncate transformation
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=253
	Length int `json:"length,omitempty"`

	// HashAlgorithm for hash transformation
	// +kubebuilder:validation:Enum=md5;sha1;sha256
	// +kubebuilder:default="sha256"
	HashAlgorithm string `json:"hashAlgorithm,omitempty"`

	// HashLength specifies length of hash output (for hash transformation)
	// +kubebuilder:validation:Minimum=4
	// +kubebuilder:validation:Maximum=64
	// +kubebuilder:default=8
	HashLength int `json:"hashLength,omitempty"`
}

// NamespaceDetection configures automatic namespace scope labeling
type NamespaceDetection struct {
	// Enabled controls whether namespace detection is active
	// +kubebuilder:default=false
	Enabled bool `json:"enabled"`

	// LabelKey is the label key to use for namespace information
	// +kubebuilder:default="kubecore.io/namespace"
	// +kubebuilder:validation:Pattern="^[a-z0-9A-Z]([a-z0-9A-Z._-]*[a-z0-9A-Z])?$"
	LabelKey string `json:"labelKey,omitempty"`

	// Strategy defines how namespace is determined
	// +kubebuilder:validation:Enum=xr-namespace;function-namespace;auto
	// +kubebuilder:default="auto"
	Strategy NamespaceStrategy `json:"strategy,omitempty"`

	// FallbackStrategy defines fallback when primary strategy fails
	// +kubebuilder:validation:Enum=xr-namespace;function-namespace;skip
	// +kubebuilder:default="function-namespace"
	FallbackStrategy NamespaceStrategy `json:"fallbackStrategy,omitempty"`

	// DefaultNamespace provides default when all strategies fail
	DefaultNamespace string `json:"defaultNamespace,omitempty"`
}

// NamespaceStrategy defines namespace detection strategies
type NamespaceStrategy string

const (
	// NamespaceStrategyXRNamespace uses XR's namespace
	NamespaceStrategyXRNamespace NamespaceStrategy = "xr-namespace"
	// NamespaceStrategyFunctionNamespace uses function's namespace
	NamespaceStrategyFunctionNamespace NamespaceStrategy = "function-namespace"
	// NamespaceStrategyAuto automatically determines best strategy
	NamespaceStrategyAuto NamespaceStrategy = "auto"
	// NamespaceStrategySkip skips namespace detection
	NamespaceStrategySkip NamespaceStrategy = "skip"
)

// MergeStrategy defines how labels are merged with existing XR labels
type MergeStrategy string

const (
	// MergeStrategyMerge merges new labels with existing ones, new labels take precedence
	MergeStrategyMerge MergeStrategy = "merge"
	// MergeStrategyReplace replaces all existing labels with new ones
	MergeStrategyReplace MergeStrategy = "replace"
	// MergeStrategyFailOnConflict fails if there are conflicting labels
	MergeStrategyFailOnConflict MergeStrategy = "fail-on-conflict"
)