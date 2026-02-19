// Package tools implements the Fix Fast analysis tools.
// Inspired by Facebook's Fix Fast system: https://engineering.fb.com/2021/02/17/developer-tools/fix-fast/
package tools

import (
	"encoding/json"
	"fmt"
	"math"
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
	// RunHistory is an optional list of recent run results for the same test or check,
	// oldest first. Each entry must be "pass" or "fail". When provided, statistical
	// confidence is derived from the failure rate across runs (BrowserLab-style) rather
	// than purely from keyword scoring.
	RunHistory []string `json:"run_history,omitempty" jsonschema_description:"Recent run results oldest-first, each 'pass' or 'fail'. Enables statistical confidence scoring."`
}

// RunHistoryStats holds the statistical analysis of the run history.
type RunHistoryStats struct {
	TotalRuns     int     `json:"total_runs"`
	FailureCount  int     `json:"failure_count"`
	FailureRate   float64 `json:"failure_rate"`
	// StatConfidence is the Wilson score lower bound — the minimum failure rate
	// we can assert with ~68% confidence given the observed sample.
	StatConfidence float64 `json:"stat_confidence"`
	IsLikelyFlake  bool    `json:"is_likely_flake"`
}

// DetectRegressionOutput is the structured result of regression detection.
type DetectRegressionOutput struct {
	IsRegression       bool             `json:"is_regression"`
	RegressionType     RegressionType   `json:"regression_type"`
	Severity           Severity         `json:"severity"`
	AffectedComponents []string         `json:"affected_components"`
	Indicators         []string         `json:"indicators"`
	Confidence         float64          `json:"confidence"`
	RunStats           *RunHistoryStats `json:"run_stats,omitempty"`
	Summary            string           `json:"summary"`
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

	// If run history is provided, compute statistical confidence from failure rate.
	// This mirrors BrowserLab's approach: confidence comes from observed runs, not
	// just from signal keywords. Uses the Wilson score lower bound (z=1.0, ~68% CI)
	// so that a single failure in a small sample is treated with appropriate scepticism.
	if len(input.RunHistory) > 0 {
		failures := 0
		for _, r := range input.RunHistory {
			if strings.ToLower(strings.TrimSpace(r)) == "fail" {
				failures++
			}
		}
		total := len(input.RunHistory)
		rate := float64(failures) / float64(total)
		statConf := wilsonLowerBound(failures, total, 1.0)

		output.RunStats = &RunHistoryStats{
			TotalRuns:      total,
			FailureCount:   failures,
			FailureRate:    math.Round(rate*1000) / 1000,
			StatConfidence: math.Round(statConf*1000) / 1000,
			IsLikelyFlake:  statConf < 0.4,
		}

		if failures > 0 {
			output.IsRegression = true
			// Statistical confidence takes precedence over keyword confidence.
			if statConf > output.Confidence {
				output.Confidence = math.Min(statConf, 0.95)
			}
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
		if output.RunStats != nil {
			flakeNote := ""
			if output.RunStats.IsLikelyFlake {
				flakeNote = " Low recurrence — likely a flake."
			}
			output.Summary += fmt.Sprintf(
				" Run history: %d/%d failures (%.0f%% failure rate, stat confidence %.2f).%s",
				output.RunStats.FailureCount, output.RunStats.TotalRuns,
				output.RunStats.FailureRate*100, output.RunStats.StatConfidence,
				flakeNote,
			)
		}
	} else {
		output.Summary = "No clear regression patterns detected. Manual review recommended."
		output.Confidence = 0.2
	}

	result, err := json.Marshal(output)
	return string(result), err
}

// wilsonLowerBound returns the lower bound of the Wilson score confidence interval.
// successes = number of failures observed, total = total runs, z = z-score.
// z=1.0 (~68% CI) gives intuitive results for small sample sizes: a single failure
// in one run scores 0.50, three failures in three runs scores 0.63, one failure
// in three runs scores 0.14 (likely flake).
func wilsonLowerBound(successes, total int, z float64) float64 {
	if total == 0 {
		return 0
	}
	p := float64(successes) / float64(total)
	n := float64(total)
	z2 := z * z
	numerator := p + z2/(2*n) - z*math.Sqrt(p*(1-p)/n+z2/(4*n*n))
	denominator := 1 + z2/n
	lb := numerator / denominator
	if lb < 0 {
		return 0
	}
	return lb
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
