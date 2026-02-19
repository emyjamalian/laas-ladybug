package tools

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// AttributeIssueInput is the input for the attribute_to_owner tool.
type AttributeIssueInput struct {
	FilesChanged   []string `json:"files_changed" jsonschema_description:"List of files changed in the suspected commit or diff"`
	Description    string   `json:"description" jsonschema_description:"Description of the regression or bug"`
	RegressionType string   `json:"regression_type" jsonschema_description:"Type of regression from detect_regression"`
}

// SuspectedOwner represents a likely owner with attribution confidence.
type SuspectedOwner struct {
	Component  string   `json:"component"`
	FilePaths  []string `json:"file_paths"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

// AttributeIssueOutput contains ownership attribution results.
type AttributeIssueOutput struct {
	SuspectedOwners     []SuspectedOwner `json:"suspected_owners"`
	HighestConfidence   string           `json:"highest_confidence_component"`
	AttributionSignals  []string         `json:"attribution_signals"`
	RecommendedReviewer string           `json:"recommended_reviewer"`
	Summary             string           `json:"summary"`
}

// componentPatterns maps file path patterns to component names.
// In a real system this would be driven by CODEOWNERS or a service registry.
var componentPatterns = []struct {
	patterns  []string
	component string
	role      string
}{
	{[]string{"auth", "login", "session", "token", "oauth"}, "auth-service", "Security"},
	{[]string{"db", "database", "model", "migration", "schema", "sql", "query"}, "data-layer", "Backend"},
	{[]string{"api", "handler", "route", "endpoint", "controller", "server"}, "api-layer", "Backend"},
	{[]string{"ui", "frontend", "component", "view", "react", "vue", "css", "html"}, "frontend", "Frontend"},
	{[]string{"cache", "redis", "memcache", "ttl"}, "caching-layer", "Infrastructure"},
	{[]string{"queue", "worker", "job", "task", "async", "consumer"}, "async-workers", "Backend"},
	{[]string{"test", "spec", "_test", "mock", "fixture"}, "test-suite", "QA"},
	{[]string{"config", "env", "setting", "yaml", "toml", "json"}, "configuration", "DevOps"},
	{[]string{"deploy", "k8s", "docker", "helm", "terraform", "ci", "cd"}, "infra-pipeline", "DevOps"},
	{[]string{"metric", "log", "trace", "monitor", "alert", "dashboard"}, "observability", "SRE"},
}

// AttributeToOwner identifies suspected owners based on files changed and regression type.
func AttributeToOwner(inputJSON string) (string, error) {
	var input AttributeIssueInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		return "", err
	}

	// Build a map from component -> files
	componentFiles := make(map[string][]string)
	for _, f := range input.FilesChanged {
		lower := strings.ToLower(f)
		base := strings.ToLower(filepath.Base(f))
		matched := false
		for _, cp := range componentPatterns {
			for _, pattern := range cp.patterns {
				if strings.Contains(lower, pattern) || strings.Contains(base, pattern) {
					componentFiles[cp.component] = append(componentFiles[cp.component], f)
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			componentFiles["core-logic"] = append(componentFiles["core-logic"], f)
		}
	}

	// Also check description for component hints.
	descLower := strings.ToLower(input.Description)
	signals := []string{}
	for _, cp := range componentPatterns {
		for _, pattern := range cp.patterns {
			if strings.Contains(descLower, pattern) {
				signals = append(signals, "Description mentions '"+pattern+"' → "+cp.component)
				break
			}
		}
	}

	// Build owners list.
	var owners []SuspectedOwner
	totalFiles := len(input.FilesChanged)
	if totalFiles == 0 {
		totalFiles = 1
	}

	for _, cp := range componentPatterns {
		files, ok := componentFiles[cp.component]
		if !ok {
			continue
		}
		confidence := float64(len(files)) / float64(totalFiles)
		if confidence > 0.95 {
			confidence = 0.95
		}
		owners = append(owners, SuspectedOwner{
			Component:  cp.component,
			FilePaths:  files,
			Confidence: confidence,
			Reason:     "Files match " + cp.component + " pattern (" + strings.Join(cp.patterns[:min(3, len(cp.patterns))], ", ") + ")",
		})
	}

	// Add core-logic if any unmatched files.
	if files, ok := componentFiles["core-logic"]; ok {
		owners = append(owners, SuspectedOwner{
			Component:  "core-logic",
			FilePaths:  files,
			Confidence: 0.4,
			Reason:     "Files did not match any known component pattern",
		})
	}

	// Sort owners by confidence (simple bubble sort for small slices).
	for i := 0; i < len(owners); i++ {
		for j := i + 1; j < len(owners); j++ {
			if owners[j].Confidence > owners[i].Confidence {
				owners[i], owners[j] = owners[j], owners[i]
			}
		}
	}

	highestComponent := "unknown"
	reviewer := "team-lead"
	if len(owners) > 0 {
		highestComponent = owners[0].Component
		reviewer = highestComponent + "-owner"
	}

	// Add regression-type specific signal.
	switch input.RegressionType {
	case "null_pointer":
		signals = append(signals, "NPE regressions are 3x more likely in non-null-safe files")
	case "performance":
		signals = append(signals, "Performance regressions often originate in data-layer or caching changes")
	case "security_flaw":
		signals = append(signals, "Security regressions require immediate auth-service review")
	}

	output := AttributeIssueOutput{
		SuspectedOwners:     owners,
		HighestConfidence:   highestComponent,
		AttributionSignals:  signals,
		RecommendedReviewer: reviewer,
		Summary: "Attribution complete. Highest confidence component: " + highestComponent +
			". " + reviewerAdvice(input.RegressionType),
	}

	result, err := json.Marshal(output)
	return string(result), err
}

func reviewerAdvice(regrType string) string {
	switch regrType {
	case "security_flaw":
		return "Security review mandatory before any fix is merged."
	case "data_corruption":
		return "Data team and DBA must approve the fix."
	case "api_breaking_change":
		return "All downstream service owners must be notified."
	default:
		return "Route to the identified component owner for fastest resolution."
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
