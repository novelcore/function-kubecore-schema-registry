package dynamic

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/crossplane/function-sdk-go/logging"
)

// ReferenceDetector interface for detecting reference fields in schemas
type ReferenceDetector interface {
	DetectReferences(schema *ResourceSchema) ([]ReferenceField, error)
	MatchesReferencePattern(fieldName string, fieldDef *FieldDefinition) bool
	ExtractReferenceMetadata(fieldName string, fieldDef *FieldDefinition) *ReferenceMetadata
	AddPattern(pattern ReferencePattern)
	GetPatterns() []ReferencePattern
}

// ReferenceMetadata contains metadata about a detected reference
type ReferenceMetadata struct {
	TargetKind      string
	TargetGroup     string
	TargetVersion   string
	RefType         RefType
	Confidence      float64
	DetectionMethod string
	MatchedPattern  string
}

// PatternBasedDetector implements reference detection using configurable patterns
type PatternBasedDetector struct {
	patterns   []ReferencePattern
	regexCache map[string]*regexp.Regexp
	logger     logging.Logger
	stats      *DetectionStats
	mu         sync.RWMutex
}

// NewReferenceDetector creates a new pattern-based reference detector
func NewReferenceDetector(logger logging.Logger) *PatternBasedDetector {
	detector := &PatternBasedDetector{
		patterns:   make([]ReferencePattern, len(DefaultReferencePatterns)),
		regexCache: make(map[string]*regexp.Regexp),
		logger:     logger,
		stats:      &DetectionStats{},
	}

	// Copy default patterns
	copy(detector.patterns, DefaultReferencePatterns)

	return detector
}

// DetectReferences analyzes a schema and detects all reference fields
func (d *PatternBasedDetector) DetectReferences(schema *ResourceSchema) ([]ReferenceField, error) {
	d.resetStats()

	var references []ReferenceField

	// Analyze all fields recursively
	for fieldName, fieldDef := range schema.Fields {
		refs := d.analyzeFieldRecursively(fieldName, fieldDef, "")
		references = append(references, refs...)
	}

	d.stats.ReferencesFound = len(references)

	d.logger.Debug("Reference detection completed",
		"fields_analyzed", d.stats.FieldsAnalyzed,
		"references_found", d.stats.ReferencesFound,
		"pattern_matches", d.stats.PatternMatches,
		"heuristic_matches", d.stats.HeuristicMatches)

	return references, nil
}

// analyzeFieldRecursively analyzes a field and its nested properties for references
func (d *PatternBasedDetector) analyzeFieldRecursively(fieldName string, fieldDef *FieldDefinition, basePath string) []ReferenceField {
	var references []ReferenceField

	d.stats.FieldsAnalyzed++

	// Build current field path
	fieldPath := d.buildFieldPath(basePath, fieldName)

	// Check if this field is a reference
	if ref := d.analyzeFieldForReference(fieldName, fieldDef, fieldPath); ref != nil {
		references = append(references, *ref)
	}

	// Recursively analyze nested properties
	if fieldDef.Properties != nil {
		for propName, propDef := range fieldDef.Properties {
			nestedRefs := d.analyzeFieldRecursively(propName, propDef, fieldPath)
			references = append(references, nestedRefs...)
		}
	}

	// Analyze array items
	if fieldDef.Items != nil {
		arrayPath := fieldPath + "[*]"
		itemRefs := d.analyzeFieldRecursively("", fieldDef.Items, arrayPath)
		references = append(references, itemRefs...)
	}

	return references
}

// analyzeFieldForReference analyzes a single field to determine if it's a reference
func (d *PatternBasedDetector) analyzeFieldForReference(fieldName string, fieldDef *FieldDefinition, fieldPath string) *ReferenceField {
	// Pattern-based detection
	if ref := d.detectByPattern(fieldName, fieldDef, fieldPath); ref != nil {
		d.stats.PatternMatches++
		return ref
	}

	// Heuristic-based detection
	if ref := d.detectByHeuristics(fieldName, fieldDef, fieldPath); ref != nil {
		d.stats.HeuristicMatches++
		return ref
	}

	return nil
}

// detectByPattern detects references using configured patterns
func (d *PatternBasedDetector) detectByPattern(fieldName string, fieldDef *FieldDefinition, fieldPath string) *ReferenceField {
	for _, pattern := range d.patterns {
		matchesName := d.matchesPattern(fieldName, pattern.Pattern)
		compatibleType := d.isCompatibleType(fieldDef, pattern)
		
		d.logger.Debug("Pattern matching attempt", 
			"fieldName", fieldName, 
			"fieldPath", fieldPath,
			"pattern", pattern.Pattern,
			"matchesName", matchesName,
			"compatibleType", compatibleType,
			"fieldType", fieldDef.Type,
			"hasProperties", fieldDef.Properties != nil)
		
		if matchesName && compatibleType {
			// Construct proper field path: if we have a simple field name, 
			// assume it's within spec unless already fully qualified
			finalFieldPath := fieldPath
			if fieldPath == fieldName && fieldName != "" {
				// If fieldPath equals fieldName, it means we're at root level
				// For most Kubernetes resources, references are in spec
				finalFieldPath = "spec." + fieldName
			}
			
			targetKind := d.inferTargetKind(fieldName, pattern)
			
			d.logger.Debug("Pattern match found!", 
				"fieldName", fieldName, 
				"pattern", pattern.Pattern,
				"targetKind", targetKind,
				"targetGroup", pattern.TargetGroup,
				"finalFieldPath", finalFieldPath)
			
			return &ReferenceField{
				FieldPath:       finalFieldPath,
				FieldName:       fieldName,
				TargetKind:      targetKind,
				TargetGroup:     pattern.TargetGroup,
				RefType:         pattern.RefType,
				Confidence:      pattern.Confidence,
				DetectionMethod: "pattern_match",
			}
		}
	}

	return nil
}

// detectByHeuristics detects references using heuristic analysis
func (d *PatternBasedDetector) detectByHeuristics(fieldName string, fieldDef *FieldDefinition, fieldPath string) *ReferenceField {
	// Construct proper field path for heuristic matches too
	finalFieldPath := fieldPath
	if fieldPath == fieldName && fieldName != "" {
		finalFieldPath = "spec." + fieldName
	}

	// Check description for reference keywords
	if d.containsReferenceKeywords(fieldDef.Description) {
		return &ReferenceField{
			FieldPath:       finalFieldPath,
			FieldName:       fieldName,
			RefType:         RefTypeCustom,
			Confidence:      0.7,
			DetectionMethod: "description_analysis",
		}
	}

	// Check for common reference field naming patterns
	if d.looksLikeReference(fieldName) {
		return &ReferenceField{
			FieldPath:       finalFieldPath,
			FieldName:       fieldName,
			RefType:         RefTypeCustom,
			Confidence:      0.6,
			DetectionMethod: "naming_heuristic",
		}
	}

	// Check for nested reference structure (e.g., {name: string, namespace: string})
	if d.hasReferenceStructure(fieldDef) {
		return &ReferenceField{
			FieldPath:       finalFieldPath,
			FieldName:       fieldName,
			RefType:         RefTypeCustom,
			Confidence:      0.8,
			DetectionMethod: "structure_analysis",
		}
	}

	return nil
}

// matchesPattern checks if a field name matches a pattern
func (d *PatternBasedDetector) matchesPattern(fieldName, pattern string) bool {
	// Enhanced debug logging for reference pattern matching
	if strings.Contains(fieldName, "Ref") || strings.Contains(pattern, "Ref") {
		d.logger.Debug("Reference pattern matching check",
			"fieldName", fieldName,
			"pattern", pattern)
	}
	
	// Use glob pattern matching first
	if matched, err := filepath.Match(pattern, fieldName); err == nil && matched {
		// Enhanced debug logging for successful matches
		if strings.Contains(fieldName, "Ref") || strings.Contains(pattern, "Ref") {
			d.logger.Debug("Pattern match successful (glob)",
				"fieldName", fieldName,
				"pattern", pattern,
				"matched", matched)
		}
		return true
	}

	// Try case-insensitive matching
	if matched, err := filepath.Match(strings.ToLower(pattern), strings.ToLower(fieldName)); err == nil && matched {
		// Enhanced debug logging for case-insensitive matches
		if strings.Contains(fieldName, "Ref") || strings.Contains(pattern, "Ref") {
			d.logger.Debug("Pattern match successful (case-insensitive)",
				"fieldName", fieldName,
				"pattern", pattern,
				"matched", matched)
		}
		return true
	}

	// Try regex pattern if it looks like regex
	if strings.Contains(pattern, "\\") || strings.Contains(pattern, "^") || strings.Contains(pattern, "$") {
		matched := d.matchesRegex(fieldName, pattern)
		if matched && (strings.Contains(fieldName, "Ref") || strings.Contains(pattern, "Ref")) {
			d.logger.Debug("Pattern match successful (regex)",
				"fieldName", fieldName,
				"pattern", pattern,
				"matched", matched)
		}
		return matched
	}

	// Enhanced debug logging for failed matches
	if strings.Contains(fieldName, "Ref") || strings.Contains(pattern, "Ref") {
		d.logger.Debug("Pattern match failed",
			"fieldName", fieldName,
			"pattern", pattern,
			"matched", false)
	}

	return false
}

// matchesRegex checks if field name matches a regex pattern
func (d *PatternBasedDetector) matchesRegex(fieldName, pattern string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	regex, exists := d.regexCache[pattern]
	if !exists {
		var err error
		regex, err = regexp.Compile(pattern)
		if err != nil {
			d.logger.Debug("Invalid regex pattern", "pattern", pattern, "error", err)
			return false
		}
		d.regexCache[pattern] = regex
	}

	return regex.MatchString(fieldName)
}

// isCompatibleType checks if field type is compatible with the pattern
func (d *PatternBasedDetector) isCompatibleType(fieldDef *FieldDefinition, pattern ReferencePattern) bool {
	// Reference fields are typically strings or objects
	switch fieldDef.Type {
	case "string":
		return true
	case "object":
		// Objects can be references if they have name/namespace structure
		// Be more lenient for KubeCore platform references
		hasRefStructure := d.hasReferenceStructure(fieldDef)
		
		// Enhanced logging for object type compatibility
		d.logger.Debug("Object type compatibility check",
			"pattern", pattern.Pattern,
			"targetKind", pattern.TargetKind,
			"targetGroup", pattern.TargetGroup,
			"hasReferenceStructure", hasRefStructure)
			
		return hasRefStructure
	default:
		return false
	}
}

// inferTargetKind infers the target kind from field name and pattern
func (d *PatternBasedDetector) inferTargetKind(fieldName string, pattern ReferencePattern) string {
	// If pattern explicitly defines target kind, use it
	if pattern.TargetKind != "" {
		return pattern.TargetKind
	}

	// Try to infer from field name
	fieldLower := strings.ToLower(fieldName)

	// Common Kubernetes resources
	kindMappings := map[string]string{
		"configmap":    "ConfigMap",
		"secret":       "Secret",
		"service":      "Service",
		"pod":          "Pod",
		"deployment":   "Deployment",
		"pvc":          "PersistentVolumeClaim",
		"pv":           "PersistentVolume",
		"storageclass": "StorageClass",
		"namespace":    "Namespace",
		"node":         "Node",

		// KubeCore specific
		"kubecluster": "KubeCluster",
		"kubenv":      "KubEnv",
		"kubeapp":     "KubeApp",
		"kubesystem":  "KubeSystem",
		"kubenet":     "KubeNet",
		"qualitygate": "QualityGate",

		// GitHub platform specific
		"githubproject":  "GitHubProject",
		"githubinfra":    "GitHubInfra",
		"githubsystem":   "GitHubSystem",
		"githubprovider": "GithubProvider",
	}

	for keyword, kind := range kindMappings {
		if strings.Contains(fieldLower, keyword) {
			return kind
		}
	}

	// Extract potential kind from field name
	// e.g., "myResourceRef" -> "MyResource"
	if strings.HasSuffix(fieldLower, "ref") {
		base := strings.TrimSuffix(fieldName, "Ref")
		base = strings.TrimSuffix(base, "ref")
		if base != "" {
			return strings.Title(base)
		}
	}

	return ""
}

// containsReferenceKeywords checks if description contains reference-related keywords
func (d *PatternBasedDetector) containsReferenceKeywords(description string) bool {
	if description == "" {
		return false
	}

	descLower := strings.ToLower(description)
	keywords := []string{
		"reference to",
		"references",
		"refers to",
		"points to",
		"name of the",
		"identifier of",
		"id of",
	}

	for _, keyword := range keywords {
		if strings.Contains(descLower, keyword) {
			return true
		}
	}

	return false
}

// looksLikeReference checks if field name follows common reference naming patterns
func (d *PatternBasedDetector) looksLikeReference(fieldName string) bool {
	fieldLower := strings.ToLower(fieldName)

	// Common reference suffixes
	suffixes := []string{"ref", "reference", "id", "name"}
	for _, suffix := range suffixes {
		if strings.HasSuffix(fieldLower, suffix) {
			return true
		}
	}

	// Common reference prefixes
	prefixes := []string{"target", "source", "parent", "owner"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(fieldLower, prefix) {
			return true
		}
	}

	return false
}

// hasReferenceStructure checks if field has a structure typical of references
func (d *PatternBasedDetector) hasReferenceStructure(fieldDef *FieldDefinition) bool {
	if fieldDef.Type != "object" || fieldDef.Properties == nil {
		d.logger.Debug("Reference structure check failed", 
			"type", fieldDef.Type, 
			"hasProperties", fieldDef.Properties != nil)
		return false
	}

	// Look for common reference field combinations
	hasName := false
	hasKind := false
	
	propertyNames := make([]string, 0, len(fieldDef.Properties))
	for propName := range fieldDef.Properties {
		propertyNames = append(propertyNames, propName)
		propLower := strings.ToLower(propName)
		switch propLower {
		case "name":
			hasName = true
		case "kind":
			hasKind = true
		}
	}

	result := hasName || (hasKind && hasName)
	
	d.logger.Debug("Reference structure analysis", 
		"propertyNames", propertyNames,
		"hasName", hasName,
		"hasKind", hasKind,
		"isReferenceStructure", result)

	// Reference structures typically have at least a name
	// Optionally kind for typed references
	return result
}

// MatchesReferencePattern checks if a field matches any reference pattern
func (d *PatternBasedDetector) MatchesReferencePattern(fieldName string, fieldDef *FieldDefinition) bool {
	for _, pattern := range d.patterns {
		if d.matchesPattern(fieldName, pattern.Pattern) && d.isCompatibleType(fieldDef, pattern) {
			return true
		}
	}
	return false
}

// ExtractReferenceMetadata extracts reference metadata for a field
func (d *PatternBasedDetector) ExtractReferenceMetadata(fieldName string, fieldDef *FieldDefinition) *ReferenceMetadata {
	// Try pattern-based detection first
	for _, pattern := range d.patterns {
		if d.matchesPattern(fieldName, pattern.Pattern) && d.isCompatibleType(fieldDef, pattern) {
			return &ReferenceMetadata{
				TargetKind:      d.inferTargetKind(fieldName, pattern),
				TargetGroup:     pattern.TargetGroup,
				RefType:         pattern.RefType,
				Confidence:      pattern.Confidence,
				DetectionMethod: "pattern_match",
				MatchedPattern:  pattern.Pattern,
			}
		}
	}

	// Fall back to heuristics
	if d.containsReferenceKeywords(fieldDef.Description) {
		return &ReferenceMetadata{
			RefType:         RefTypeCustom,
			Confidence:      0.7,
			DetectionMethod: "description_analysis",
		}
	}

	return nil
}

// AddPattern adds a new reference pattern
func (d *PatternBasedDetector) AddPattern(pattern ReferencePattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.patterns = append(d.patterns, pattern)
}

// GetPatterns returns all configured patterns
func (d *PatternBasedDetector) GetPatterns() []ReferencePattern {
	d.mu.RLock()
	defer d.mu.RUnlock()

	patterns := make([]ReferencePattern, len(d.patterns))
	copy(patterns, d.patterns)
	return patterns
}

// Helper methods

func (d *PatternBasedDetector) buildFieldPath(basePath, fieldName string) string {
	if basePath == "" {
		return fieldName
	}
	if fieldName == "" {
		return basePath
	}
	return fmt.Sprintf("%s.%s", basePath, fieldName)
}

func (d *PatternBasedDetector) resetStats() {
	d.stats = &DetectionStats{}
}

// GetDetectionStats returns current detection statistics
func (d *PatternBasedDetector) GetDetectionStats() *DetectionStats {
	return d.stats
}

// ClearRegexCache clears the compiled regex cache
func (d *PatternBasedDetector) ClearRegexCache() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.regexCache = make(map[string]*regexp.Regexp)
}

// LoadCustomPatterns loads custom patterns from configuration
func (d *PatternBasedDetector) LoadCustomPatterns(patterns []ReferencePattern) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Replace patterns with custom ones
	d.patterns = make([]ReferencePattern, len(patterns))
	copy(d.patterns, patterns)

	// Clear regex cache since patterns changed
	d.regexCache = make(map[string]*regexp.Regexp)
}
