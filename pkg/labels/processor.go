package labels

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crossplane/function-sdk-go/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/google/uuid"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// Processor handles XR label injection operations
type Processor struct {
	log               logging.Logger
	functionNamespace string
	fieldExtractor    *FieldExtractor
	transformer       *Transformer
}

// NewProcessor creates a new label processor
func NewProcessor(log logging.Logger, functionNamespace string) *Processor {
	return &Processor{
		log:               log,
		functionNamespace: functionNamespace,
		fieldExtractor:    NewFieldExtractor(log),
		transformer:       NewTransformer(log),
	}
}

// ProcessLabels applies label configuration to an XR
func (p *Processor) ProcessLabels(ctx context.Context, xr *resource.Composite, config *v1beta1.XRLabelConfig) error {
	if config == nil || !config.Enabled {
		p.log.Debug("XR label processing disabled")
		return nil
	}

	p.log.Info("Starting XR label processing",
		"xr_name", xr.Resource.GetName(),
		"xr_kind", xr.Resource.GetKind(),
		"static_labels", len(config.Labels),
		"dynamic_labels", len(config.DynamicLabels))

	// Get existing labels
	existingLabels := xr.Resource.GetLabels()
	if existingLabels == nil {
		existingLabels = make(map[string]string)
	}

	// Process static labels
	newLabels := make(map[string]string)
	for key, value := range config.Labels {
		newLabels[key] = value
	}

	// Process dynamic labels
	for _, dynamicLabel := range config.DynamicLabels {
		value, err := p.processDynamicLabel(ctx, xr, &dynamicLabel)
		if err != nil {
			if dynamicLabel.Required {
				return errors.Wrapf(err, "failed to process required dynamic label '%s'", dynamicLabel.Key)
			}
			p.log.Info("Skipping optional dynamic label due to error",
				"label_key", dynamicLabel.Key,
				"error", err.Error())
			continue
		}
		newLabels[dynamicLabel.Key] = value
	}

	// Process namespace detection
	if config.NamespaceDetection != nil && config.NamespaceDetection.Enabled {
		err := p.processNamespaceDetection(ctx, xr, config.NamespaceDetection, newLabels)
		if err != nil {
			p.log.Info("Failed to detect namespace, continuing without namespace label",
				"error", err.Error())
		}
	}

	// Apply merge strategy
	finalLabels, err := p.applyMergeStrategy(existingLabels, newLabels, config.MergeStrategy, config.EnforceLabels)
	if err != nil {
		return errors.Wrapf(err, "failed to apply merge strategy")
	}

	// Update XR labels
	xr.Resource.SetLabels(finalLabels)

	p.log.Info("XR label processing completed",
		"total_labels_applied", len(finalLabels),
		"new_labels_added", len(newLabels))

	return nil
}

// processDynamicLabel processes a single dynamic label
func (p *Processor) processDynamicLabel(ctx context.Context, xr *resource.Composite, label *v1beta1.DynamicLabel) (string, error) {
	var value string
	var err error

	// Extract value based on source
	switch label.Source {
	case v1beta1.LabelSourceXRField:
		value, err = p.fieldExtractor.ExtractFromXR(xr.Resource.Object, label.SourcePath)
		if err != nil {
			return "", errors.Wrapf(err, "failed to extract field '%s'", label.SourcePath)
		}

	case v1beta1.LabelSourceEnvironment:
		if label.SourcePath == "" {
			return "", errors.ValidationError("sourcePath required for environment source")
		}
		value = os.Getenv(label.SourcePath)
		if value == "" {
			return "", errors.ValidationError(fmt.Sprintf("environment variable '%s' not found or empty", label.SourcePath))
		}

	case v1beta1.LabelSourceTimestamp:
		value = time.Now().Format(time.RFC3339)

	case v1beta1.LabelSourceUUID:
		value = uuid.New().String()

	case v1beta1.LabelSourceConstant:
		if label.Value == "" {
			return "", errors.ValidationError("value required for constant source")
		}
		value = label.Value

	default:
		return "", errors.ValidationError(fmt.Sprintf("unsupported label source: %s", label.Source))
	}

	// Apply transformation if specified
	if label.Transform != nil {
		value, err = p.transformer.Transform(value, label.Transform)
		if err != nil {
			return "", errors.Wrapf(err, "failed to transform label value")
		}
	}

	// Validate label value
	if err := p.validateLabelValue(label.Key, value); err != nil {
		return "", errors.Wrapf(err, "invalid label value")
	}

	return value, nil
}

// processNamespaceDetection handles automatic namespace detection
func (p *Processor) processNamespaceDetection(ctx context.Context, xr *resource.Composite, config *v1beta1.NamespaceDetection, labels map[string]string) error {
	labelKey := config.LabelKey
	if labelKey == "" {
		labelKey = "kubecore.io/namespace"
	}

	var namespace string
	var err error

	// Try primary strategy
	namespace, err = p.detectNamespace(xr, config.Strategy)
	if err != nil || namespace == "" {
		// Try fallback strategy
		if config.FallbackStrategy != v1beta1.NamespaceStrategySkip {
			namespace, err = p.detectNamespace(xr, config.FallbackStrategy)
			if err != nil || namespace == "" {
				// Use default namespace if provided
				if config.DefaultNamespace != "" {
					namespace = config.DefaultNamespace
				} else {
					return errors.ValidationError("unable to detect namespace and no default provided")
				}
			}
		}
	}

	if namespace != "" {
		labels[labelKey] = namespace
	}

	return nil
}

// detectNamespace detects namespace using specified strategy
func (p *Processor) detectNamespace(xr *resource.Composite, strategy v1beta1.NamespaceStrategy) (string, error) {
	switch strategy {
	case v1beta1.NamespaceStrategyXRNamespace:
		ns := xr.Resource.GetNamespace()
		if ns == "" {
			return "", errors.ValidationError("XR has no namespace")
		}
		return ns, nil

	case v1beta1.NamespaceStrategyFunctionNamespace:
		if p.functionNamespace == "" {
			return "", errors.ValidationError("function namespace not available")
		}
		return p.functionNamespace, nil

	case v1beta1.NamespaceStrategyAuto:
		// Try XR namespace first, then function namespace
		if ns := xr.Resource.GetNamespace(); ns != "" {
			return ns, nil
		}
		if p.functionNamespace != "" {
			return p.functionNamespace, nil
		}
		return "", errors.ValidationError("unable to auto-detect namespace")

	default:
		return "", errors.ValidationError(fmt.Sprintf("unsupported namespace strategy: %s", strategy))
	}
}

// applyMergeStrategy merges labels according to specified strategy
func (p *Processor) applyMergeStrategy(existing, new map[string]string, strategy v1beta1.MergeStrategy, enforceLabels []string) (map[string]string, error) {
	result := make(map[string]string)

	// Create enforce set for fast lookup
	enforceSet := make(map[string]bool)
	for _, label := range enforceLabels {
		enforceSet[label] = true
	}

	switch strategy {
	case v1beta1.MergeStrategyMerge:
		// Start with existing labels
		for k, v := range existing {
			result[k] = v
		}
		// Merge new labels, checking enforce list
		for k, v := range new {
			if existingVal, exists := existing[k]; exists && enforceSet[k] && existingVal != v {
				return nil, errors.ValidationError(fmt.Sprintf("cannot override enforced label '%s'", k))
			}
			result[k] = v
		}

	case v1beta1.MergeStrategyReplace:
		// Check enforce labels before replacing
		for k := range existing {
			if enforceSet[k] {
				if newVal, exists := new[k]; !exists || newVal != existing[k] {
					return nil, errors.ValidationError(fmt.Sprintf("cannot remove or change enforced label '%s'", k))
				}
			}
		}
		// Use only new labels
		for k, v := range new {
			result[k] = v
		}

	case v1beta1.MergeStrategyFailOnConflict:
		// Start with existing labels
		for k, v := range existing {
			result[k] = v
		}
		// Check for conflicts
		for k, v := range new {
			if existingVal, exists := existing[k]; exists && existingVal != v {
				return nil, errors.ValidationError(fmt.Sprintf("label conflict for key '%s': existing='%s', new='%s'", k, existingVal, v))
			}
			result[k] = v
		}

	default:
		return nil, errors.ValidationError(fmt.Sprintf("unsupported merge strategy: %s", strategy))
	}

	return result, nil
}

// validateLabelValue validates that a label key and value are valid
func (p *Processor) validateLabelValue(key, value string) error {
	// Kubernetes label value validation
	if len(value) > 63 {
		return errors.ValidationError(fmt.Sprintf("label value too long (max 63 chars): %d", len(value)))
	}

	// Check for invalid characters in value
	if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		return errors.ValidationError("label value contains invalid characters")
	}

	return nil
}