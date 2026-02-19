// Package tools implements the Fix Fast analysis tools.
// Inspired by Facebook's Fix Fast system: https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/
package tools

import (
	"encoding/json"
	"strings"
)

// RegressionType classifies what kind of regression was detected.
type RegressionType string

const (
	RegressionTypeNullPointer   RegressionType = "null_pointer"
	RegressionTypePerformance   RegressionType = "performance"
	RegressionTypeCrash         RegressionType = "crash"
	RegressionTypeMemoryLeak    RegressionType = "memory_leak"
	RegressionTypeLogicError    RegressionType = "logic_error"
	RegressionTypeDataCorrupt   RegressionType = "data_corruption"
	RegressionTypeAPIBreaking   RegressionType = "api_breaking_change"
	RegressionTypeSecurityFlaw  RegressionType = "security_flaw"
	RegressionTypeUnknown       RegressionType = "unknown"
)

// Severity represents how critical the regression is.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// DetectRegressionInput is the input for the detect_regression tool.
type DetectRegressionInput struct {
	Description  string   `json:"description" jsonschema_description:"Description of the bug, crash, or code change to analyze for regressions"`
	FilesChanged []string `json:"files_changed,omitempty" jsonschema_description:"List of files modified in the change (optional)"`
	Environment  string   `json:"environment" jsonschema_description:"Where the issue was found: ide, local_test, ci, code_review, staging, or production"`
	ErrorMessage string   `json:"error_message,omitempty" jsonschema_description:"The actual error or stack trace if available"`
}

// DetectRegressionOutput is the structured result of regression detection.
type DetectRegressionOutput struct {
	IsRegression       bool           `json:"is_regression"`
	RegressionType     RegressionType `json:"regression_type"`
	Severity           Severity       `json:"severity"`
	AffectedComponents []string       `json:"affected_components"`
	Indicators         []string       `json:"indicators"`
	Confidence         float64        `json:"confidence"`
	Summary            string         `json:"summary"`
}

// regressionSignals maps keywords to regression types and severities.
var regressionSignals = []struct {
	keywords      []string
	regrType      RegressionType
	severity      Severity
	indicator     string
}{
	{[]string{"null", "nil", "npe", "nullpointerexception", "nil pointer", "null reference"}, RegressionTypeNullPointer, SeverityHigh, "Null/nil dereference pattern"},
	{[]string{"panic", "crash", "segfault", "sigsegv", "abort", "fatal error"}, RegressionTypeCrash, SeverityCritical, "Application crash signal"},
	{[]string{"slow", "latency", "timeout", "performance", "memory usage", "cpu spike", "throughput"}, RegressionTypePerformance, SeverityMedium, "Performance degradation signal"},
	{[]string{"memory leak", "oom", "out of memory", "heap", "alloc", "gc pressure"}, RegressionTypeMemoryLeak, SeverityHigh, "Memory leak indicator"},
	{[]string{"wrong result", "incorrect", "unexpected value", "off by one", "logic", "calculation"}, RegressionTypeLogicError, SeverityMedium, "Logic error pattern"},
	{[]string{"corrupt", "data loss", "inconsistent state", "transaction", "atomic"}, RegressionTypeDataCorrupt, SeverityCritical, "Data integrity concern"},
	{[]string{"breaking change", "api change", "interface", "signature", "deprecated", "removed method"}, RegressionTypeAPIBreaking, SeverityHigh, "API contract violation"},
	{[]string{"sql injection", "xss", "csrf", "auth bypass", "privilege", "cve", "vulnerability"}, RegressionTypeSecurityFlaw, SeverityCritical, "Security vulnerability signal"},
}

// DetectRegression analyzes a description and returns a structured regression report.
func DetectRegression(inputJSON string) (string, error) {
	var input DetectRegressionInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", err
	}

	desc := strings.ToLower(input.Description + " " + input.ErrorMessage)
	output := DetectRegressionOutput{
		IsRegression:       false,
		RegressionType:     RegressionTypeUnknown,
		Severity:           SeverityLow,
		AffectedComponents: []string{},
		Indicators:         []string{},
		Confidence:         0.0,
	}

	// Score each regression type by keyword matches.
	type scored struct {
		regrType  RegressionType
		severity  Severity
		score     int
		indicator string
	}
	var scores []scored

	for _, sig := range regressionSignals {
		count := 0
		for _, kw := range sig.keywords {
			if strings.Contains(desc, kw) {
				count++
			}
		}
		if count > 0 {
			scores = append(scores, scored{sig.regrType, sig.severity, count, sig.indicator})
			output.Indicators = append(output.Indicators, sig.indicator)
		}
	}

	// Pick the highest-scoring type.
	if len(scores) > 0 {
		output.IsRegression = true
		best := scores[0]
		for _, s := range scores[1:] {
			if s.score > best.score {
				best = s
			}
		}
		output.RegressionType = best.regrType
		output.Severity = best.severity
		output.Confidence = float64(best.score) / float64(len(regressionSignals[0].keywords)+3)
		if output.Confidence > 0.9 {
			output.Confidence = 0.9
		}
		if output.Confidence < 0.3 {
			output.Confidence = 0.3
		}
	}

	// Extract affected components from file list.
	for _, f := range input.FilesChanged {
		parts := strings.Split(f, "/")
		if len(parts) > 1 {
			output.AffectedComponents = append(output.AffectedComponents, parts[0])
		} else {
			output.AffectedComponents = append(output.AffectedComponents, f)
		}
	}
	output.AffectedComponents = deduplicate(output.AffectedComponents)

	// Build summary.
	if output.IsRegression {
		output.Summary = "Detected a " + string(output.Severity) + " " + string(output.RegressionType) +
			" regression found in " + input.Environment + " environment."
	} else {
		output.Summary = "No clear regression patterns detected. Manual review recommended."
		output.Confidence = 0.2
	}

	result, err := json.Marshal(output)
	return string(result), err
}

func deduplicate(s []string) []string {
	seen := make(map[string]bool)
	out := []string{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
