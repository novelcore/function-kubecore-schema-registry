package labels

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/crossplane/function-sdk-go/logging"

	"github.com/crossplane/function-kubecore-schema-registry/input/v1beta1"
	"github.com/crossplane/function-kubecore-schema-registry/pkg/errors"
)

// Transformer handles value transformations for labels
type Transformer struct {
	log logging.Logger
}

// NewTransformer creates a new transformer
func NewTransformer(log logging.Logger) *Transformer {
	return &Transformer{
		log: log,
	}
}

// Transform applies a transformation to a value
func (t *Transformer) Transform(value string, config *v1beta1.LabelTransform) (string, error) {
	if config == nil {
		return value, nil
	}

	t.log.Debug("Applying transformation",
		"type", config.Type,
		"original_value", value)

	var result string
	var err error

	switch config.Type {
	case v1beta1.TransformTypeLowercase:
		result = strings.ToLower(value)

	case v1beta1.TransformTypeUppercase:
		result = strings.ToUpper(value)

	case v1beta1.TransformTypePrefix:
		if config.Options == nil || config.Options.Prefix == "" {
			return "", errors.ValidationError("prefix transformation requires prefix option")
		}
		result = config.Options.Prefix + value

	case v1beta1.TransformTypeSuffix:
		if config.Options == nil || config.Options.Suffix == "" {
			return "", errors.ValidationError("suffix transformation requires suffix option")
		}
		result = value + config.Options.Suffix

	case v1beta1.TransformTypeReplace:
		if config.Options == nil || config.Options.Old == "" {
			return "", errors.ValidationError("replace transformation requires old option")
		}
		new := ""
		if config.Options.New != "" {
			new = config.Options.New
		}
		result = strings.ReplaceAll(value, config.Options.Old, new)

	case v1beta1.TransformTypeTruncate:
		if config.Options == nil || config.Options.Length <= 0 {
			return "", errors.ValidationError("truncate transformation requires positive length option")
		}
		if len(value) > config.Options.Length {
			result = value[:config.Options.Length]
		} else {
			result = value
		}

	case v1beta1.TransformTypeHash:
		result, err = t.applyHashTransformation(value, config.Options)
		if err != nil {
			return "", errors.Wrapf(err, "hash transformation failed")
		}

	default:
		return "", errors.ValidationError(fmt.Sprintf("unsupported transformation type: %s", config.Type))
	}

	// Validate the result
	if err := t.validateTransformedValue(result); err != nil {
		return "", errors.Wrapf(err, "transformation produced invalid value")
	}

	t.log.Debug("Transformation completed",
		"type", config.Type,
		"original_value", value,
		"transformed_value", result)

	return result, nil
}

// applyHashTransformation applies hash transformation with specified algorithm
func (t *Transformer) applyHashTransformation(value string, options *v1beta1.TransformOptions) (string, error) {
	if options == nil {
		options = &v1beta1.TransformOptions{
			HashAlgorithm: "sha256",
			HashLength:    8,
		}
	}

	algorithm := options.HashAlgorithm
	if algorithm == "" {
		algorithm = "sha256"
	}

	length := options.HashLength
	if length <= 0 {
		length = 8
	}

	var hashBytes []byte
	switch algorithm {
	case "md5":
		hash := md5.Sum([]byte(value))
		hashBytes = hash[:]
	case "sha1":
		hash := sha1.Sum([]byte(value))
		hashBytes = hash[:]
	case "sha256":
		hash := sha256.Sum256([]byte(value))
		hashBytes = hash[:]
	default:
		return "", errors.ValidationError(fmt.Sprintf("unsupported hash algorithm: %s", algorithm))
	}

	// Convert to hex string
	hexString := fmt.Sprintf("%x", hashBytes)

	// Truncate to specified length
	if len(hexString) > length {
		hexString = hexString[:length]
	}

	return hexString, nil
}

// validateTransformedValue ensures the transformed value is valid for Kubernetes labels
func (t *Transformer) validateTransformedValue(value string) error {
	// Check length (Kubernetes label values must be <= 63 characters)
	if len(value) > 63 {
		return errors.ValidationError(fmt.Sprintf("transformed value too long: %d characters (max 63)", len(value)))
	}

	// Check for empty value
	if value == "" {
		return nil // Empty values are allowed
	}

	// Check valid characters (alphanumeric, '.', '-', '_')
	for i, r := range value {
		if !isValidLabelChar(r, i == 0 || i == len(value)-1) {
			return errors.ValidationError(fmt.Sprintf("invalid character '%c' in transformed value", r))
		}
	}

	return nil
}

// isValidLabelChar checks if a character is valid in a Kubernetes label value
func isValidLabelChar(r rune, isFirstOrLast bool) bool {
	// Alphanumeric characters are always valid
	if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
		return true
	}

	// Special characters are valid only in the middle
	if !isFirstOrLast {
		return r == '.' || r == '-' || r == '_'
	}

	return false
}

// TransformChain applies multiple transformations in sequence
func (t *Transformer) TransformChain(value string, transforms []*v1beta1.LabelTransform) (string, error) {
	result := value
	
	for i, transform := range transforms {
		var err error
		result, err = t.Transform(result, transform)
		if err != nil {
			return "", errors.Wrapf(err, "transformation %d failed", i+1)
		}
	}
	
	return result, nil
}

// ValidateTransformConfig validates a transformation configuration
func (t *Transformer) ValidateTransformConfig(config *v1beta1.LabelTransform) error {
	if config == nil {
		return nil
	}

	switch config.Type {
	case v1beta1.TransformTypePrefix:
		if config.Options == nil || config.Options.Prefix == "" {
			return errors.ValidationError("prefix transformation requires prefix option")
		}
		// Validate prefix characters
		for _, r := range config.Options.Prefix {
			if !isValidLabelChar(r, false) {
				return errors.ValidationError(fmt.Sprintf("invalid character '%c' in prefix", r))
			}
		}

	case v1beta1.TransformTypeSuffix:
		if config.Options == nil || config.Options.Suffix == "" {
			return errors.ValidationError("suffix transformation requires suffix option")
		}
		// Validate suffix characters
		for _, r := range config.Options.Suffix {
			if !isValidLabelChar(r, false) {
				return errors.ValidationError(fmt.Sprintf("invalid character '%c' in suffix", r))
			}
		}

	case v1beta1.TransformTypeReplace:
		if config.Options == nil || config.Options.Old == "" {
			return errors.ValidationError("replace transformation requires old option")
		}

	case v1beta1.TransformTypeTruncate:
		if config.Options == nil || config.Options.Length <= 0 {
			return errors.ValidationError("truncate transformation requires positive length option")
		}
		if config.Options.Length > 63 {
			return errors.ValidationError("truncate length cannot exceed 63 characters")
		}

	case v1beta1.TransformTypeHash:
		if config.Options != nil {
			if config.Options.HashAlgorithm != "" &&
				config.Options.HashAlgorithm != "md5" &&
				config.Options.HashAlgorithm != "sha1" &&
				config.Options.HashAlgorithm != "sha256" {
				return errors.ValidationError(fmt.Sprintf("unsupported hash algorithm: %s", config.Options.HashAlgorithm))
			}
			if config.Options.HashLength < 4 || config.Options.HashLength > 64 {
				return errors.ValidationError("hash length must be between 4 and 64")
			}
		}

	case v1beta1.TransformTypeLowercase, v1beta1.TransformTypeUppercase:
		// These transformations don't require options

	default:
		return errors.ValidationError(fmt.Sprintf("unsupported transformation type: %s", config.Type))
	}

	return nil
}